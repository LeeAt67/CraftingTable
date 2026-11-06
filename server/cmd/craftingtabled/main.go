// Command craftingtabled 启动 CraftingTable 本地 Go 服务。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/leeat/craftingtable/server/internal/agent/access"
	"github.com/leeat/craftingtable/server/internal/agent/core"
	"github.com/leeat/craftingtable/server/internal/agent/provider"
	agenttools "github.com/leeat/craftingtable/server/internal/agent/tools"
	"github.com/leeat/craftingtable/server/internal/platform/config"
	"github.com/leeat/craftingtable/server/internal/platform/httpapi"
	"github.com/leeat/craftingtable/server/internal/platform/store"
	"github.com/leeat/craftingtable/server/internal/today"
)

func main() {
	if err := run(); err != nil {
		slog.Error("craftingtabled stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	envFile := flag.String(
		"env-file",
		".env.local",
		"optional local environment file",
	)
	resetDatabase := flag.Bool(
		"reset-database",
		false,
		"remove the selected SQLite database before startup",
	)
	flag.Parse()

	if err := loadEnvironment(*envFile); err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	databasePath, err := resolveDatabasePath()
	if err != nil {
		return err
	}
	if *resetDatabase {
		if err := removeDatabase(databasePath); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}

	openContext, cancelOpen := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelOpen()
	sqliteStore, err := store.OpenSQLite(openContext, databasePath)
	if err != nil {
		return err
	}
	defer sqliteStore.Close()

	agentConfig := provider.OpenAICompatibleConfig{
		BaseURL: firstEnvironment(
			"CRAFTINGTABLE_AGENT_BASE_URL",
			"AGENT_API_BASE_URL",
		),
		APIKey: firstEnvironment(
			"CRAFTINGTABLE_AGENT_API_KEY",
			"AGENT_API_KEY",
		),
		Model: firstEnvironment(
			"CRAFTINGTABLE_AGENT_MODEL",
			"AGENT_MODEL",
		),
		SystemPrompt: os.Getenv("CRAFTINGTABLE_AGENT_SYSTEM_PROMPT"),
	}
	var streamProvider core.StreamProvider
	if agentConfig.Configured() {
		streamProvider = provider.NewOpenAICompatible(agentConfig)
	}

	runtimeConfig, err := resolveAgentRuntimeConfig()
	if err != nil {
		return err
	}
	todayTools, err := agenttools.NewTodayTools(sqliteStore, time.Now)
	if err != nil {
		return fmt.Errorf("create agent tools: %w", err)
	}
	toolRegistry, err := core.NewToolRegistry(todayTools)
	if err != nil {
		return fmt.Errorf("create agent tool registry: %w", err)
	}
	agentService := core.NewServiceWithTools(
		sqliteStore,
		streamProvider,
		toolRegistry,
		logger,
		runtimeConfig,
	)
	if err := agentService.Recover(context.Background()); err != nil {
		return fmt.Errorf("recover agent runs: %w", err)
	}

	router := httpapi.NewRouter(logger)
	today.NewController(sqliteStore, logger).Register(router)
	access.NewSessionController(agentService, logger).Register(router)
	streamHeartbeat, err := resolveAgentStreamHeartbeat()
	if err != nil {
		return err
	}
	access.NewRunControllerWithOptions(
		agentService,
		logger,
		access.RunControllerOptions{
			HeartbeatInterval: streamHeartbeat,
		},
	).Register(router)
	if os.Getenv("CRAFTINGTABLE_TEST_HOOKS") == "1" {
		access.NewTestController(agentService, logger).Register(router)
	}
	access.NewController(streamProvider, logger).Register(router)

	port, err := resolvePort()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("listen on loopback: %w", err)
	}

	server := &http.Server{
		Handler:           router.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info(
			"craftingtabled listening",
			"address", listener.Addr().String(),
			"database", databasePath,
			"agent_configured", streamProvider != nil,
			"agent_tools_enabled", runtimeConfig.ToolsEnabled,
			"agent_max_model_steps", runtimeConfig.MaxModelSteps,
			"agent_max_tool_calls", runtimeConfig.MaxToolCalls,
		)
		serverErrors <- server.Serve(listener)
	}()

	shutdownContext, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	select {
	case <-shutdownContext.Done():
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	}

	gracefulContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(gracefulContext); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	return nil
}

func loadEnvironment(path string) error {
	values, err := config.LoadEnvFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for key, value := range values {
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s from %s: %w", key, path, err)
			}
		}
	}
	return nil
}

func resolveDatabasePath() (string, error) {
	configured := firstEnvironment(
		"CRAFTINGTABLE_DATABASE_PATH",
		"LEEAT_DATABASE_PATH",
	)
	if configured != "" {
		return filepath.Abs(configured)
	}

	current := filepath.Join("data", "craftingtable.sqlite")
	if _, err := os.Stat(current); err == nil {
		return filepath.Abs(current)
	}
	legacy := filepath.Join("data", "leeat.sqlite")
	if _, err := os.Stat(legacy); err == nil {
		return filepath.Abs(legacy)
	}
	return filepath.Abs(current)
}

func removeDatabase(path string) error {
	if path == ":memory:" {
		return nil
	}
	for _, suffix := range []string{"", "-shm", "-wal"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("reset database %s: %w", path+suffix, err)
		}
	}
	return nil
}

func resolvePort() (int, error) {
	value := strings.TrimSpace(os.Getenv("API_PORT"))
	if value == "" {
		return 3001, nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 0 || port > 65535 {
		return 0, fmt.Errorf("API_PORT must be an integer from 0 to 65535")
	}
	return port, nil
}

func resolveAgentStreamHeartbeat() (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv("CRAFTINGTABLE_AGENT_STREAM_HEARTBEAT"))
	if value == "" {
		return 15 * time.Second, nil
	}
	interval, err := time.ParseDuration(value)
	if err != nil || interval <= 0 {
		return 0, fmt.Errorf(
			"CRAFTINGTABLE_AGENT_STREAM_HEARTBEAT must be a positive duration",
		)
	}
	return interval, nil
}

func resolveAgentRuntimeConfig() (core.RuntimeConfig, error) {
	config := core.DefaultRuntimeConfig()

	enabledValue := strings.TrimSpace(
		os.Getenv("CRAFTINGTABLE_AGENT_TOOLS_ENABLED"),
	)
	if enabledValue != "" {
		enabled, err := strconv.ParseBool(enabledValue)
		if err != nil {
			return core.RuntimeConfig{}, fmt.Errorf(
				"CRAFTINGTABLE_AGENT_TOOLS_ENABLED must be true or false",
			)
		}
		config.ToolsEnabled = enabled
	}

	maxModelSteps, err := positiveEnvironmentInteger(
		"CRAFTINGTABLE_AGENT_MAX_MODEL_STEPS",
		config.MaxModelSteps,
	)
	if err != nil {
		return core.RuntimeConfig{}, err
	}
	config.MaxModelSteps = maxModelSteps

	maxToolCalls, err := positiveEnvironmentInteger(
		"CRAFTINGTABLE_AGENT_MAX_TOOL_CALLS",
		config.MaxToolCalls,
	)
	if err != nil {
		return core.RuntimeConfig{}, err
	}
	config.MaxToolCalls = maxToolCalls
	return config, nil
}

func positiveEnvironmentInteger(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func firstEnvironment(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}
