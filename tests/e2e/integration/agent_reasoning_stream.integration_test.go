package integration

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeat/craftingtable/server/testkit"
)

type scriptedStreamProvider struct {
	mu      sync.Mutex
	scripts []string
	calls   []testkit.ModelRequest
}

func (p *scriptedStreamProvider) Stream(
	_ context.Context,
	request testkit.ModelRequest,
) (io.ReadCloser, error) {
	p.mu.Lock()
	p.calls = append(p.calls, request)
	index := len(p.calls) - 1
	p.mu.Unlock()
	body := ""
	if index < len(p.scripts) {
		body = p.scripts[index]
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

func (p *scriptedStreamProvider) callMessages(index int) []testkit.ProviderMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[index].Messages
}

func reasoningScript(reasoning, text string) string {
	return "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"" + reasoning +
		"\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"" + text +
		"\"}}]}\n\n" +
		"data: [DONE]\n\n"
}

func TestCoordinatorEmitsReasoningBeforeText(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "reasoning-stream.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	provider := &scriptedStreamProvider{
		scripts: []string{reasoningScript("先分析", "最终回答")},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentService(db, provider, logger)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "reasoning-stream-session",
		RequestHash:     "reasoning-stream-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "reasoning-stream-send",
		RequestHash:     "reasoning-stream-send-hash",
		UserMessageID:   "reasoning-stream-user",
		UserParts:       []string{"解释一下"},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, cancel, err := service.SubscribeRun(run.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	got := make([]string, 0, 10)
	for event := range events {
		got = append(got, string(event.Kind))
		if event.Kind == testkit.StreamFinish {
			break
		}
		if len(got) > 12 {
			t.Fatalf("unexpected event stream: %#v", got)
		}
	}
	want := []string{
		"start",
		"start-step",
		"reasoning-start",
		"reasoning-delta",
		"reasoning-end",
		"text-start",
		"text-delta",
		"text-end",
		"finish-step",
		"finish",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("chunks = %#v, want %#v", got, want)
	}

	detail, err := service.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range detail.MessageParts {
		if part.RunID == nil || *part.RunID != run.RunID {
			continue
		}
		switch part.Type {
		case testkit.PartReasoning:
			if part.Text != "先分析" {
				t.Fatalf("reasoning = %q", part.Text)
			}
		case testkit.PartText:
			if part.Text != "最终回答" {
				t.Fatalf("text = %q", part.Text)
			}
		}
	}
}

func TestCoordinatorSkipsReasoningEventsWithoutReasoning(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "reasoning-stream-plain.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	provider := &scriptedStreamProvider{
		scripts: []string{
			"data: {\"choices\":[{\"delta\":{\"content\":\"只有正文\"}}]}\n\n" +
				"data: [DONE]\n\n",
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentService(db, provider, logger)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "plain-session",
		RequestHash:     "plain-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "plain-send",
		RequestHash:     "plain-send-hash",
		UserMessageID:   "plain-user",
		UserParts:       []string{"没有推理"},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, cancel, err := service.SubscribeRun(run.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	for event := range events {
		if strings.HasPrefix(string(event.Kind), "reasoning") {
			t.Fatalf("unexpected reasoning chunk: %#v", event)
		}
		if event.Kind == testkit.StreamFinish {
			break
		}
	}
}

func TestReasoningDoesNotEnterProviderContext(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "reasoning-context.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	provider := &scriptedStreamProvider{
		scripts: []string{
			reasoningScript("隐藏推理", "第一轮回答"),
			"data: {\"choices\":[{\"delta\":{\"content\":\"第二轮回答\"}}]}\n\n" +
				"data: [DONE]\n\n",
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentService(db, provider, logger)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "context-session",
		RequestHash:     "context-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "context-send-1",
		RequestHash:     "context-hash-1",
		UserMessageID:   "context-user-1",
		UserParts:       []string{"第一个问题"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForFinish(t, service, session.ID, first.RunID)

	second, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "context-send-2",
		RequestHash:     "context-hash-2",
		UserMessageID:   "context-user-2",
		UserParts:       []string{"第二个问题"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForFinish(t, service, session.ID, second.RunID)

	messages := provider.callMessages(1)
	for _, message := range messages {
		if strings.Contains(message.Content, "隐藏推理") {
			t.Fatalf("provider context leaked reasoning: %#v", messages)
		}
	}
	if len(messages) < 3 {
		t.Fatalf("provider context = %#v, want user and assistant turns", messages)
	}
	foundAssistant := false
	for _, message := range messages {
		if message.Role == "assistant" && message.Content == "第一轮回答" {
			foundAssistant = true
			break
		}
	}
	if !foundAssistant {
		t.Fatalf("completed assistant context missing: %#v", messages)
	}
}

func waitForFinish(
	t *testing.T,
	service *testkit.Service,
	sessionID string,
	runID string,
) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		detail, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if detail.ActiveRun == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %s did not finish", runID)
}
