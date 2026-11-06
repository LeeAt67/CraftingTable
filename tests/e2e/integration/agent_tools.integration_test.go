package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeat/craftingtable/server/testkit"
)

type toolHTTPProvider struct {
	mu       sync.Mutex
	requests []testkit.ModelRequest
	handler  func(int, testkit.ModelRequest, http.ResponseWriter)
}

func (p *toolHTTPProvider) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request testkit.ModelRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	index := len(p.requests)
	p.requests = append(p.requests, request)
	p.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	p.handler(index, request, w)
}

func (p *toolHTTPProvider) request(index int) testkit.ModelRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests[index]
}

func (p *toolHTTPProvider) requestCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

func toolCallScript(
	calls ...struct {
		ID        string
		Name      string
		Arguments string
	},
) string {
	parts := make([]string, 0, len(calls))
	for index, call := range calls {
		parts = append(parts, fmt.Sprintf(
			`{"index":%d,"id":%q,"type":"function","function":{"name":%q,"arguments":%q}}`,
			index,
			call.ID,
			call.Name,
			call.Arguments,
		))
	}
	return "data: " + fmt.Sprintf(
		`{"choices":[{"delta":{"tool_calls":[%s]},"finish_reason":"tool_calls"}]}`,
		strings.Join(parts, ","),
	) + "\n\n"
}

func textScript(text string) string {
	return "data: " + fmt.Sprintf(
		`{"choices":[{"delta":{"content":%q},"finish_reason":"stop"}]}`,
		text,
	) + "\n\n"
}

func createTodayToolService(
	t *testing.T,
	store *testkit.SQLiteStore,
	baseURL string,
	config testkit.RuntimeConfig,
) *testkit.Service {
	t.Helper()
	todayTools, err := testkit.NewTodayTools(store, func() time.Time {
		return time.Date(
			2026,
			time.November,
			6,
			8,
			0,
			0,
			0,
			time.UTC,
		)
	})
	if err != nil {
		t.Fatalf("NewTodayTools() error = %v", err)
	}
	registry, err := testkit.NewToolRegistry(todayTools)
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	provider := testkit.NewOpenAICompatible(testkit.OpenAICompatibleConfig{
		BaseURL: baseURL,
		APIKey:  "test-key",
		Model:   "tool-model",
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return testkit.NewServiceWithTools(store, provider, registry, logger, config)
}

func sendToolTestMessage(
	t *testing.T,
	service *testkit.Service,
	prefix string,
) testkit.RunHandle {
	t.Helper()
	ctx := context.Background()
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: prefix + "-session",
		RequestHash:     prefix + "-session-hash",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: prefix + "-send",
		RequestHash:     prefix + "-send-hash",
		UserMessageID:   prefix + "-user",
		UserParts:       []string{"查找今天的任务"},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	return run
}

func TestCoordinatorExecutesTodaySearchAndContinuesModel(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "tool-search.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	startAt := "2026-11-06T01:00:00Z"
	endAt := "2026-11-06T02:00:00Z"
	item, err := db.CreateItem(ctx, todayCreateInput(
		"Write report",
		&startAt,
		&endAt,
	))
	if err != nil {
		t.Fatal(err)
	}

	provider := &toolHTTPProvider{}
	provider.handler = func(
		index int,
		_ testkit.ModelRequest,
		w http.ResponseWriter,
	) {
		if index == 0 {
			_, _ = io.WriteString(w, toolCallScript(struct {
				ID        string
				Name      string
				Arguments string
			}{
				ID:   "call-search",
				Name: "today_search",
				Arguments: `{"date":"2026-11-06","timezone":"Asia/Shanghai",` +
					`"query":"report"}`,
			}))
			return
		}
		_, _ = io.WriteString(w, textScript("今天的报告任务在 09:00 开始。"))
	}
	server := httptest.NewServer(provider)
	defer server.Close()

	config := testkit.DefaultRuntimeConfig()
	config.MaxModelSteps = 4
	config.MaxToolCalls = 8
	service := createTodayToolService(t, db, server.URL+"/v1", config)
	run := sendToolTestMessage(t, service, "search")
	waitForFinish(t, service, run.SessionID, run.RunID)

	if provider.requestCount() != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.requestCount())
	}
	firstRequest := provider.request(0)
	if len(firstRequest.Tools) != 3 || firstRequest.ToolChoice != "auto" {
		t.Fatalf("first model request = %#v", firstRequest)
	}
	secondRequest := provider.request(1)
	if !requestContainsToolResult(secondRequest, "call-search", item.Title) {
		t.Fatalf("second model request missing tool result: %#v", secondRequest)
	}

	detail, err := service.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if assistantText(detail.MessageParts, run.RunID) != "今天的报告任务在 09:00 开始。" {
		t.Fatalf("assistant text = %q", assistantText(detail.MessageParts, run.RunID))
	}
}

func TestCoordinatorExecutesEveryTodayTool(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "all-tools.sqlite")
	db, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	item, err := db.CreateItem(ctx, todayCreateInput("Deep work", nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	provider := &toolHTTPProvider{}
	provider.handler = func(
		index int,
		_ testkit.ModelRequest,
		w http.ResponseWriter,
	) {
		if index == 0 {
			_, _ = io.WriteString(w, toolCallScript(
				struct {
					ID        string
					Name      string
					Arguments string
				}{
					ID:        "call-search",
					Name:      "today_search",
					Arguments: `{"date":"2026-11-06","timezone":"Asia/Shanghai"}`,
				},
				struct {
					ID        string
					Name      string
					Arguments string
				}{
					ID:        "call-get",
					Name:      "today_get_item",
					Arguments: `{"id":"` + item.ID + `"}`,
				},
				struct {
					ID        string
					Name      string
					Arguments string
				}{
					ID:        "call-summary",
					Name:      "today_summary",
					Arguments: `{"date":"2026-11-06","timezone":"Asia/Shanghai"}`,
				},
			))
			return
		}
		_, _ = io.WriteString(w, textScript("三种只读工具都已执行。"))
	}
	server := httptest.NewServer(provider)
	defer server.Close()

	config := testkit.DefaultRuntimeConfig()
	config.MaxModelSteps = 4
	config.MaxToolCalls = 8
	service := createTodayToolService(t, db, server.URL+"/v1", config)
	run := sendToolTestMessage(t, service, "all-tools")
	waitForFinish(t, service, run.SessionID, run.RunID)

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var invocationCount int
	if err := raw.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM agent_tool_invocations
		 WHERE run_id = ? AND result_json <> ''`,
		run.RunID,
	).Scan(&invocationCount); err != nil {
		t.Fatal(err)
	}
	if invocationCount != 3 {
		t.Fatalf("completed tool invocations = %d, want 3", invocationCount)
	}
	var toolSteps int
	if err := raw.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM agent_steps
		 WHERE run_id = ? AND kind = 'tool' AND status = 'completed'`,
		run.RunID,
	).Scan(&toolSteps); err != nil {
		t.Fatal(err)
	}
	if toolSteps != 3 {
		t.Fatalf("completed tool steps = %d, want 3", toolSteps)
	}
}

func TestCoordinatorFeedsToolErrorBackToModel(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "tool-error.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	provider := &toolHTTPProvider{}
	provider.handler = func(
		index int,
		_ testkit.ModelRequest,
		w http.ResponseWriter,
	) {
		if index == 0 {
			_, _ = io.WriteString(w, toolCallScript(struct {
				ID        string
				Name      string
				Arguments string
			}{
				ID:        "call-missing",
				Name:      "today_get_item",
				Arguments: `{"id":"missing-item"}`,
			}))
			return
		}
		_, _ = io.WriteString(w, textScript("没有找到这个任务。"))
	}
	server := httptest.NewServer(provider)
	defer server.Close()

	config := testkit.DefaultRuntimeConfig()
	config.MaxModelSteps = 4
	config.MaxToolCalls = 8
	service := createTodayToolService(t, db, server.URL+"/v1", config)
	run := sendToolTestMessage(t, service, "tool-error")
	waitForFinish(t, service, run.SessionID, run.RunID)

	second := provider.request(1)
	if !requestContainsToolResult(second, "call-missing", "not_found") {
		t.Fatalf("tool error was not fed back: %#v", second)
	}
	if requestContainsToolResult(second, "call-missing", "SELECT") {
		t.Fatal("tool error leaked SQL")
	}
	detail, err := service.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if assistantText(detail.MessageParts, run.RunID) != "没有找到这个任务。" {
		t.Fatal("model did not produce a final answer after tool error")
	}
}

func TestCoordinatorEnforcesModelAndToolBudgets(t *testing.T) {
	t.Run("tool calls", func(t *testing.T) {
		ctx := context.Background()
		db, err := testkit.OpenSQLite(
			ctx,
			filepath.Join(t.TempDir(), "tool-budget.sqlite"),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		provider := &toolHTTPProvider{}
		provider.handler = func(
			_ int,
			_ testkit.ModelRequest,
			w http.ResponseWriter,
		) {
			_, _ = io.WriteString(w, toolCallScript(
				struct {
					ID        string
					Name      string
					Arguments string
				}{
					ID:        "call-search-1",
					Name:      "today_search",
					Arguments: `{"date":"2026-11-06","timezone":"Asia/Shanghai"}`,
				},
				struct {
					ID        string
					Name      string
					Arguments string
				}{
					ID:        "call-summary-1",
					Name:      "today_summary",
					Arguments: `{"date":"2026-11-06","timezone":"Asia/Shanghai"}`,
				},
			))
		}
		server := httptest.NewServer(provider)
		defer server.Close()

		config := testkit.DefaultRuntimeConfig()
		config.MaxModelSteps = 4
		config.MaxToolCalls = 1
		service := createTodayToolService(t, db, server.URL+"/v1", config)
		run := sendToolTestMessage(t, service, "tool-budget")
		waitForFinish(t, service, run.SessionID, run.RunID)

		assertRunErrorCode(t, service, run, "AGENT_TOOL_LIMIT_EXCEEDED")
		if provider.requestCount() != 1 {
			t.Fatalf("provider calls = %d, want 1", provider.requestCount())
		}
	})

	t.Run("model steps", func(t *testing.T) {
		ctx := context.Background()
		db, err := testkit.OpenSQLite(
			ctx,
			filepath.Join(t.TempDir(), "model-budget.sqlite"),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		provider := &toolHTTPProvider{}
		provider.handler = func(
			_ int,
			_ testkit.ModelRequest,
			w http.ResponseWriter,
		) {
			_, _ = io.WriteString(w, toolCallScript(struct {
				ID        string
				Name      string
				Arguments string
			}{
				ID:        "call-search-model-budget",
				Name:      "today_search",
				Arguments: `{"date":"2026-11-06","timezone":"Asia/Shanghai"}`,
			}))
		}
		server := httptest.NewServer(provider)
		defer server.Close()

		config := testkit.DefaultRuntimeConfig()
		config.MaxModelSteps = 1
		config.MaxToolCalls = 8
		service := createTodayToolService(t, db, server.URL+"/v1", config)
		run := sendToolTestMessage(t, service, "model-budget")
		waitForFinish(t, service, run.SessionID, run.RunID)

		assertRunErrorCode(t, service, run, "AGENT_MODEL_STEP_LIMIT_EXCEEDED")
		if provider.requestCount() != 1 {
			t.Fatalf("provider calls = %d, want 1", provider.requestCount())
		}
	})
}

func TestCoordinatorCancellationDuringToolDoesNotBecomeToolResult(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "tool-cancel.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	started := make(chan struct{})
	blocking := testkit.Tool{
		Definition: testkit.ToolDefinition{
			Name:           "block_until_cancel",
			Namespace:      "test",
			Version:        "1.0.0",
			Title:          "Blocking tool",
			Description:    "Blocks until the Run is cancelled.",
			InputSchema:    json.RawMessage(`{"type":"object","additionalProperties":false}`),
			ReadOnly:       true,
			Idempotent:     true,
			Timeout:        30 * time.Second,
			MaxInputBytes:  1024,
			MaxResultBytes: 1024,
		},
		Handler: func(
			handlerCtx context.Context,
			_ json.RawMessage,
		) (testkit.ToolResult, error) {
			close(started)
			<-handlerCtx.Done()
			return testkit.ToolResult{}, handlerCtx.Err()
		},
	}
	registry, err := testkit.NewToolRegistry([]testkit.Tool{blocking})
	if err != nil {
		t.Fatal(err)
	}
	provider := &toolHTTPProvider{}
	provider.handler = func(
		_ int,
		_ testkit.ModelRequest,
		w http.ResponseWriter,
	) {
		_, _ = io.WriteString(w, toolCallScript(struct {
			ID        string
			Name      string
			Arguments string
		}{
			ID:        "call-block",
			Name:      "block_until_cancel",
			Arguments: `{}`,
		}))
	}
	server := httptest.NewServer(provider)
	defer server.Close()
	providerClient := testkit.NewOpenAICompatible(
		testkit.OpenAICompatibleConfig{
			BaseURL: server.URL + "/v1",
			APIKey:  "test-key",
			Model:   "tool-model",
		},
	)
	config := testkit.DefaultRuntimeConfig()
	config.MaxModelSteps = 4
	config.MaxToolCalls = 8
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewServiceWithTools(
		db,
		providerClient,
		registry,
		logger,
		config,
	)
	run := sendToolTestMessage(t, service, "tool-cancel")

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("tool did not start")
	}
	cancelled, err := service.CancelRun(ctx, run.SessionID, run.RunID)
	if err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	if cancelled.Status != testkit.RunRunning {
		t.Fatalf("cancel status = %q, want running transition", cancelled.Status)
	}
	waitForFinish(t, service, run.SessionID, run.RunID)
	stored, err := service.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "cancelled" {
		t.Fatalf("run status = %q, want cancelled", stored.Status)
	}
	if provider.requestCount() != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.requestCount())
	}
}

func requestContainsToolResult(
	request testkit.ModelRequest,
	callID string,
	text string,
) bool {
	for _, message := range request.Messages {
		if message.Role == "tool" &&
			message.ToolCallID == callID &&
			strings.Contains(message.Content, text) {
			return true
		}
	}
	return false
}

func assistantText(parts []testkit.MessagePart, runID string) string {
	for _, part := range parts {
		if part.Type == testkit.PartText &&
			part.RunID != nil &&
			*part.RunID == runID {
			return part.Text
		}
	}
	return ""
}

func assertRunErrorCode(
	t *testing.T,
	service *testkit.Service,
	run testkit.RunHandle,
	code string,
) {
	t.Helper()
	detail, err := service.GetSession(context.Background(), run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range detail.MessageParts {
		if part.Type == testkit.PartError &&
			part.Error != nil &&
			part.Error.Code == code {
			return
		}
	}
	t.Fatalf("error code %q not found in %#v", code, detail.MessageParts)
}

func todayCreateInput(title string, startAt, endAt *string) testkit.CreateItemInput {
	return testkit.CreateItemInput{
		Title:   title,
		StartAt: startAt,
		EndAt:   endAt,
	}
}
