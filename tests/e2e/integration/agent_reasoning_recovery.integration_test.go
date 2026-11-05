package integration

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeat/craftingtable/server/testkit"
)

type gatedReasoningProvider struct {
	release chan struct{}
}

func (p *gatedReasoningProvider) Stream(
	ctx context.Context,
	_ []testkit.ProviderMessage,
) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		if _, err := io.WriteString(
			writer,
			"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"进行中的推理\"}}]}\n\n",
		); err != nil {
			return
		}
		select {
		case <-p.release:
		case <-ctx.Done():
			return
		}
		if _, err := io.WriteString(
			writer,
			"data: {\"choices\":[{\"delta\":{\"content\":\"最终回答\"}}]}\n\n"+
				"data: [DONE]\n\n",
		); err != nil {
			return
		}
	}()
	return reader, nil
}

func TestSessionDetailBackfillsStreamingReasoning(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "reasoning-recovery.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	provider := &gatedReasoningProvider{release: make(chan struct{})}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentService(db, provider, logger)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "recovery-session",
		RequestHash:     "recovery-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "recovery-send",
		RequestHash:     "recovery-send-hash",
		UserMessageID:   "recovery-user",
		UserParts:       []string{"恢复推理"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var (
		liveReasoning string
		liveText      string
		lastSequence  int
	)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		detail, err := service.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if detail.ActiveRun != nil {
			lastSequence = detail.ActiveRun.LastSequence
		}
		for _, part := range detail.MessageParts {
			if part.RunID == nil || *part.RunID != run.RunID {
				continue
			}
			switch part.Type {
			case testkit.PartReasoning:
				liveReasoning = part.Text
			case testkit.PartText:
				liveText = part.Text
			}
		}
		if strings.Contains(liveReasoning, "进行中的推理") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(liveReasoning, "进行中的推理") {
		t.Fatalf("live reasoning = %q", liveReasoning)
	}
	if liveText != "" {
		t.Fatalf("live text = %q, want empty while reasoning streams", liveText)
	}
	if lastSequence < 4 {
		t.Fatalf("journal cursor = %d, want reasoning delta cursor", lastSequence)
	}

	close(provider.release)
	waitForFinish(t, service, session.ID, run.RunID)

	detail, err := service.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var (
		reasoning string
		text      string
	)
	for _, part := range detail.MessageParts {
		if part.RunID == nil || *part.RunID != run.RunID {
			continue
		}
		switch part.Type {
		case testkit.PartReasoning:
			reasoning = part.Text
		case testkit.PartText:
			text = part.Text
		}
	}
	if reasoning != "进行中的推理" || text != "最终回答" {
		t.Fatalf("persisted reasoning = %q text = %q", reasoning, text)
	}
}
