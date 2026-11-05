package integration

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/leeat/craftingtable/server/testkit"
)

func TestAgentSessionAndCompletedMessageSurviveReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agent-persistence.sqlite")

	first, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	session, err := first.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "session-request-1",
		RequestHash:     "hash-1",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	execution, err := first.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "send-request-1",
		RequestHash:     "send-hash-1",
		UserMessageID:   "user-message-1",
		UserParts:       []string{"你好"},
	})
	if err != nil {
		t.Fatalf("create send run: %v", err)
	}
	err = first.FinalizeAgentRun(ctx, testkit.FinalizeAgentRunInput{
		RunID:              execution.Run.ID,
		Status:             testkit.RunCompleted,
		AssistantPartID:    execution.AssistantPartID,
		AssistantText:      "你好，Leeat",
		MessageStatus:      testkit.MessageComplete,
		PartStatus:         testkit.PartComplete,
		CompletedEventType: "run.completed",
	})
	if err != nil {
		t.Fatalf("finalize run: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	second, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer second.Close()

	detail, err := second.GetAgentSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("get persisted session: %v", err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(detail.Messages))
	}
	if len(detail.MessageParts) != 2 {
		t.Fatalf("parts = %d, want 2", len(detail.MessageParts))
	}
	var assistantText string
	for _, part := range detail.MessageParts {
		if part.Type == testkit.PartText &&
			part.RunID != nil &&
			*part.RunID == execution.Run.ID {
			assistantText = part.Text
		}
	}
	if assistantText != "你好，Leeat" {
		t.Fatalf("assistant text = %q", assistantText)
	}
}

func TestRecoverInterruptedAgentRunsMarksOnlyRunningRuns(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "recovery.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "session-1",
		RequestHash:     "hash-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "send-1",
		RequestHash:     "send-hash-1",
		UserMessageID:   "user-1",
		UserParts:       []string{"hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	count, err := db.RecoverInterruptedAgentRuns(ctx)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	run, err := db.GetAgentRun(ctx, execution.Run.ID)
	if err != nil || run.Status != testkit.RunInterrupted {
		t.Fatalf("run=%#v err=%v", run, err)
	}
	count, err = db.RecoverInterruptedAgentRuns(ctx)
	if err != nil || count != 0 {
		t.Fatalf("second recovery count=%d err=%v", count, err)
	}
}

func TestDuplicateSendRequestReturnsSameRun(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "idempotency.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "session-2",
		RequestHash:     "hash-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	input := testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "send-2",
		RequestHash:     "send-hash-2",
		UserMessageID:   "user-2",
		UserParts:       []string{"duplicate"},
	}
	first, err := db.CreateAgentSendRun(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.CreateAgentSendRun(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if second.Run.ID != first.Run.ID {
		t.Fatalf("run IDs differ: %s != %s", second.Run.ID, first.Run.ID)
	}
	detail, err := db.GetAgentSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(detail.Messages))
	}
}
