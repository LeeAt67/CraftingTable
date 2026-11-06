package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var serverBinary string

func TestMain(m *testing.M) {
	root := repositoryRoot()
	tempDirectory, err := os.MkdirTemp("", "craftingtable-go-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp directory: %v\n", err)
		os.Exit(1)
	}
	serverBinary = filepath.Join(tempDirectory, executableName("craftingtabled"))
	build := exec.Command("go", "build", "-o", serverBinary, "./server/cmd/craftingtabled")
	build.Dir = root
	output, err := build.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build server: %v\n%s\n", err, output)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(tempDirectory)
	os.Exit(code)
}

type testServer struct {
	baseURL string
	command *exec.Cmd
	close   sync.Once
}

func startServer(t *testing.T, agentBaseURL string) *testServer {
	t.Helper()
	return startServerWithDatabase(
		t,
		agentBaseURL,
		filepath.Join(t.TempDir(), "craftingtable.sqlite"),
	)
}

func startServerWithDatabase(
	t *testing.T,
	agentBaseURL string,
	databasePath string,
) *testServer {
	return startServerWithEnvironment(t, agentBaseURL, databasePath, nil)
}

func startServerWithEnvironment(
	t *testing.T,
	agentBaseURL string,
	databasePath string,
	extraEnvironment map[string]string,
) *testServer {
	t.Helper()
	port := freePort(t)
	command := exec.Command(
		serverBinary,
		"-env-file="+filepath.Join(t.TempDir(), "missing.env"),
	)
	command.Dir = repositoryRoot()
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	command.Env = isolatedEnvironment(map[string]string{
		"API_PORT":                     fmt.Sprintf("%d", port),
		"CRAFTINGTABLE_DATABASE_PATH":  databasePath,
		"CRAFTINGTABLE_TEST_HOOKS":     "1",
		"CRAFTINGTABLE_AGENT_BASE_URL": agentBaseURL,
		"CRAFTINGTABLE_AGENT_API_KEY":  "integration-secret",
		"CRAFTINGTABLE_AGENT_MODEL":    "integration-model",
	})
	if agentBaseURL == "" {
		command.Env = isolatedEnvironment(map[string]string{
			"API_PORT":                    fmt.Sprintf("%d", port),
			"CRAFTINGTABLE_DATABASE_PATH": databasePath,
			"CRAFTINGTABLE_TEST_HOOKS":    "1",
		})
	}
	for key, value := range extraEnvironment {
		command.Env = append(command.Env, key+"="+value)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	server := &testServer{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		command: command,
	}
	t.Cleanup(func() {
		server.Close()
		if t.Failed() {
			t.Logf("server output:\n%s", output.String())
		}
	})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := (&http.Client{Timeout: time.Second}).Get(
			server.URL("/healthz"),
		)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return server
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server did not become healthy:\n%s", output.String())
	return nil
}

func (s *testServer) URL(path string) string {
	return s.baseURL + path
}

func (s *testServer) Close() {
	s.close.Do(func() {
		if s.command != nil && s.command.Process != nil {
			_ = s.command.Process.Kill()
			_ = s.command.Wait()
		}
	})
}

func requestJSON[T any](
	t *testing.T,
	method string,
	url string,
	body string,
) (int, T) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequestWithContext(
		context.Background(),
		method,
		url,
		reader,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()

	var result T
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response.StatusCode, result
}

func startFakeProvider(
	t *testing.T,
	handler http.HandlerFunc,
) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func isolatedEnvironment(overrides map[string]string) []string {
	blocked := map[string]struct{}{
		"API_PORT":                            {},
		"CRAFTINGTABLE_DATABASE_PATH":         {},
		"LEEAT_DATABASE_PATH":                 {},
		"AGENT_API_BASE_URL":                  {},
		"AGENT_API_KEY":                       {},
		"AGENT_MODEL":                         {},
		"CRAFTINGTABLE_AGENT_BASE_URL":        {},
		"CRAFTINGTABLE_AGENT_API_KEY":         {},
		"CRAFTINGTABLE_AGENT_MODEL":           {},
		"CRAFTINGTABLE_AGENT_SYSTEM_PROMPT":   {},
		"CRAFTINGTABLE_AGENT_TOOLS_ENABLED":   {},
		"CRAFTINGTABLE_AGENT_MAX_MODEL_STEPS": {},
		"CRAFTINGTABLE_AGENT_MAX_TOOL_CALLS":  {},
		"CRAFTINGTABLE_TEST_HOOKS":            {},
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		key, _, found := strings.Cut(value, "=")
		if found {
			if _, skip := blocked[strings.ToUpper(key)]; skip {
				continue
			}
		}
		environment = append(environment, value)
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func repositoryRoot() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve integration test location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
