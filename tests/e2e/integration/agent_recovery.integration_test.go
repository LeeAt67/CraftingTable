package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeat/craftingtable/server/testkit"
)

type slowAgentProvider struct{}

func (slowAgentProvider) Stream(
	ctx context.Context,
	_ []testkit.ProviderMessage,
) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		if _, err := io.WriteString(
			writer,
			"data: {\"choices\":[{\"delta\":{\"content\":\"第一段\"}}]}\n\n",
		); err != nil {
			return
		}
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return
		}
		if _, err := io.WriteString(
			writer,
			"data: {\"choices\":[{\"delta\":{\"content\":\"第二段\"}}]}\n\n",
		); err != nil {
			return
		}
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}()
	return reader, nil
}

func TestAgentRunContinuesAfterSubscriberDisconnects(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "recovery.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentService(db, slowAgentProvider{}, logger)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "recovery-session-request",
		RequestHash:     "recovery-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "recovery-send-request",
		RequestHash:     "recovery-send-hash",
		UserMessageID:   "recovery-user-message",
		UserParts:       []string{"恢复测试"},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, cancel, err := service.SubscribeRun(run.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event, open := <-events:
			if !open {
				t.Fatal("subscription closed before the first delta")
			}
			if event.Kind == testkit.StreamTextDelta && event.Text == "第一段" {
				cancel()
				goto waitForCompletion
			}
		case <-deadline:
			t.Fatal("did not receive the first text delta")
		}
	}

waitForCompletion:
	completionDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(completionDeadline) {
		detail, err := service.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if detail.ActiveRun == nil {
			for _, part := range detail.MessageParts {
				if part.RunID != nil &&
					*part.RunID == run.RunID &&
					part.Text == "第一段第二段" {
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal(fmt.Sprintf("run %s never reached completed text", run.RunID))
}

func TestAgentRunReplayRespectsCursor(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "replay.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentService(db, slowAgentProvider{}, logger)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "replay-session",
		RequestHash:     "replay-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "replay-send",
		RequestHash:     "replay-send-hash",
		UserMessageID:   "replay-user-message",
		UserParts:       []string{"游标测试"},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, cancel, err := service.SubscribeRun(run.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	var firstDelta testkit.StreamChunk
	for event := range events {
		if event.Kind == testkit.StreamTextDelta {
			firstDelta = event
			cancel()
			break
		}
	}
	if firstDelta.Sequence == 0 {
		t.Fatal("first delta was not observed")
	}

	replayed, stopReplay, err := service.SubscribeRun(
		run.RunID,
		firstDelta.Sequence,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer stopReplay()
	for event := range replayed {
		if event.Sequence <= firstDelta.Sequence {
			t.Fatalf("replayed cursor %d after %d", event.Sequence, firstDelta.Sequence)
		}
		if strings.Contains(event.Text, "第一段") && event.Text != "第二段" {
			t.Fatalf("replayed old text: %#v", event)
		}
		if event.Kind == testkit.StreamFinish {
			return
		}
	}
	t.Fatal("replayed stream ended without a finish event")
}
