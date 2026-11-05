package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
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

func TestFirstMessageTitleUsesFirst32Runes(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "session-title.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "title-session",
		RequestHash:     "title-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("界", 40)
	_, err = db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "title-send",
		RequestHash:     "title-send-hash",
		UserMessageID:   "title-user",
		UserParts:       []string{text},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := db.GetAgentSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("界", 32)
	if detail.Session.Title != want {
		t.Fatalf("title = %q, want %q", detail.Session.Title, want)
	}
}

func TestActiveRunDoesNotExposeDurableEventSequenceAsStreamCursor(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "active-run-cursor.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "active-run-cursor-session",
		RequestHash:     "active-run-cursor-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := db.CreateAgentSendRun(
		ctx,
		testkit.CreateAgentSendRunInput{
			SessionID:       session.ID,
			ClientRequestID: "active-run-cursor-send",
			RequestHash:     "active-run-cursor-send-hash",
			UserMessageID:   "active-run-cursor-user",
			UserParts:       []string{"游标"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := db.GetAgentSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ActiveRun == nil {
		t.Fatal("active run is nil")
	}
	if detail.ActiveRun.LastSequence != 0 {
		t.Fatalf(
			"active run stream cursor = %d before live journal registration, want 0",
			detail.ActiveRun.LastSequence,
		)
	}
	events, err := db.ListAgentRunEvents(ctx, execution.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 3 {
		t.Fatalf("durable events = %d, want initial lifecycle events", len(events))
	}
}

func TestAgentSessionDetailSnapshotNeverMixesRunningAndTerminalState(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "session-snapshot.sqlite")
	writer, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	reader, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	session, err := writer.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "snapshot-session",
		RequestHash:     "snapshot-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := writer.CreateAgentSendRun(
		ctx,
		testkit.CreateAgentSendRunInput{
			SessionID:       session.ID,
			ClientRequestID: "snapshot-send",
			RequestHash:     "snapshot-send-hash",
			UserMessageID:   "snapshot-user",
			UserParts:       []string{"一致性"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	finalized := make(chan error, 1)
	go func() {
		<-start
		finalized <- writer.FinalizeAgentRun(
			context.Background(),
			testkit.FinalizeAgentRunInput{
				RunID:              execution.Run.ID,
				Status:             testkit.RunCompleted,
				AssistantPartID:    execution.AssistantPartID,
				AssistantText:      "完成",
				MessageStatus:      testkit.MessageComplete,
				PartStatus:         testkit.PartComplete,
				CompletedEventType: "run.completed",
			},
		)
	}()

	var wait sync.WaitGroup
	failures := make(chan string, 1)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 100 {
				detail, detailErr := reader.GetAgentSession(ctx, session.ID)
				if detailErr != nil {
					select {
					case failures <- detailErr.Error():
					default:
					}
					return
				}
				assistantIndex := -1
				for index, message := range detail.Messages {
					if message.ID == execution.Run.AssistantMessageID {
						assistantIndex = index
						break
					}
				}
				if assistantIndex < 0 {
					select {
					case failures <- "assistant message missing from snapshot":
					default:
					}
					return
				}
				message := detail.Messages[assistantIndex]
				if detail.ActiveRun == nil {
					if message.Status == "streaming" {
						select {
						case failures <- "terminal snapshot kept a streaming assistant":
						default:
						}
						return
					}
					continue
				}
				if message.Status != "streaming" ||
					detail.ActiveRun.RunID != execution.Run.ID {
					select {
					case failures <- "running snapshot mixed terminal message state":
					default:
					}
					return
				}
			}
		}()
	}
	close(start)
	wait.Wait()
	if finalizeErr := <-finalized; finalizeErr != nil {
		t.Fatal(finalizeErr)
	}
	select {
	case failure := <-failures:
		t.Fatal(failure)
	default:
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
	events, err := db.ListAgentRunEvents(ctx, execution.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{
		"run.started",
		"step.started",
		"message.part.started",
		"message.part.finished",
		"step.finished",
		"run.interrupted",
	}
	if len(events) != len(wantEvents) {
		t.Fatalf("events = %#v, want %v", events, wantEvents)
	}
	for index, eventType := range wantEvents {
		if events[index].Type != eventType {
			t.Fatalf("event[%d] = %q, want %q", index, events[index].Type, eventType)
		}
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

func TestAgentContextExcludesAbandonedTurns(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "context-status.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "context-status-session",
		RequestHash:     "context-status-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "context-status-completed",
		RequestHash:     "context-status-completed-hash",
		UserMessageID:   "context-completed-user",
		UserParts:       []string{"完整问题"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinalizeAgentRun(ctx, testkit.FinalizeAgentRunInput{
		RunID:             completed.Run.ID,
		Status:            testkit.RunCompleted,
		AssistantPartID:   completed.AssistantPartID,
		AssistantText:     "完整回答",
		MessageStatus:     testkit.MessageComplete,
		PartStatus:        testkit.PartComplete,
		TerminalEventType: "run.completed",
	}); err != nil {
		t.Fatal(err)
	}
	abandoned, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "context-status-failed",
		RequestHash:     "context-status-failed-hash",
		UserMessageID:   "context-failed-user",
		UserParts:       []string{"失败问题"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinalizeAgentRun(ctx, testkit.FinalizeAgentRunInput{
		RunID:             abandoned.Run.ID,
		Status:            "failed",
		AssistantPartID:   abandoned.AssistantPartID,
		AssistantText:     "失败残缺回答",
		MessageStatus:     "failed",
		PartStatus:        "failed",
		TerminalEventType: "run.failed",
	}); err != nil {
		t.Fatal(err)
	}

	current, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "context-status-current",
		RequestHash:     "context-status-current-hash",
		UserMessageID:   "context-current-user",
		UserParts:       []string{"当前问题"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []testkit.ContextMessage{
		{Role: "user", Content: "完整问题"},
		{Role: "assistant", Content: "完整回答"},
		{Role: "user", Content: "当前问题"},
	}
	if fmt.Sprint(current.Messages) != fmt.Sprint(want) {
		t.Fatalf("context = %#v, want %#v", current.Messages, want)
	}
}

func TestAgentContextKeepsNewestCompleteTurnsWithinLimits(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "context-limits.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "context-limits-session",
		RequestHash:     "context-limits-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 21; index++ {
		run, err := db.CreateAgentSendRun(
			ctx,
			testkit.CreateAgentSendRunInput{
				SessionID:       session.ID,
				ClientRequestID: fmt.Sprintf("context-limit-send-%d", index),
				RequestHash:     fmt.Sprintf("context-limit-hash-%d", index),
				UserMessageID:   fmt.Sprintf("context-limit-user-%d", index),
				UserParts:       []string{fmt.Sprintf("问题-%d", index)},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.FinalizeAgentRun(ctx, testkit.FinalizeAgentRunInput{
			RunID:             run.Run.ID,
			Status:            testkit.RunCompleted,
			AssistantPartID:   run.AssistantPartID,
			AssistantText:     fmt.Sprintf("回答-%d", index),
			MessageStatus:     testkit.MessageComplete,
			PartStatus:        testkit.PartComplete,
			TerminalEventType: "run.completed",
		}); err != nil {
			t.Fatal(err)
		}
	}

	current, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "context-limit-current",
		RequestHash:     "context-limit-current-hash",
		UserMessageID:   "context-limit-current-user",
		UserParts:       []string{"当前问题"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Messages) > 40 {
		t.Fatalf("context messages = %d, want <= 40", len(current.Messages))
	}
	if len(current.Messages) != 39 {
		t.Fatalf("context messages = %d, want 39", len(current.Messages))
	}
	if current.Messages[0].Content != "问题-3" {
		t.Fatalf("oldest retained message = %q, want 问题-3", current.Messages[0].Content)
	}
	if current.Messages[len(current.Messages)-1].Content != "当前问题" {
		t.Fatalf("last context message = %q", current.Messages[len(current.Messages)-1].Content)
	}
}

func TestAgentContextTruncatesOldTurnsByRuneCount(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "context-runes.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "context-runes-session",
		RequestHash:     "context-runes-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 3; index++ {
		run, err := db.CreateAgentSendRun(
			ctx,
			testkit.CreateAgentSendRunInput{
				SessionID:       session.ID,
				ClientRequestID: fmt.Sprintf("context-runes-send-%d", index),
				RequestHash:     fmt.Sprintf("context-runes-hash-%d", index),
				UserMessageID:   fmt.Sprintf("context-runes-user-%d", index),
				UserParts:       []string{strings.Repeat("问", 9_000)},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.FinalizeAgentRun(ctx, testkit.FinalizeAgentRunInput{
			RunID:             run.Run.ID,
			Status:            testkit.RunCompleted,
			AssistantPartID:   run.AssistantPartID,
			AssistantText:     strings.Repeat("答", 9_000),
			MessageStatus:     testkit.MessageComplete,
			PartStatus:        testkit.PartComplete,
			TerminalEventType: "run.completed",
		}); err != nil {
			t.Fatal(err)
		}
	}
	current, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "context-runes-current",
		RequestHash:     "context-runes-current-hash",
		UserMessageID:   "context-runes-current-user",
		UserParts:       []string{"当前问题"},
	})
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, message := range current.Messages {
		total += len([]rune(message.Content))
	}
	if total > 20_000 {
		t.Fatalf("context runes = %d, want <= 20000", total)
	}
	if current.Messages[len(current.Messages)-1].Content != "当前问题" {
		t.Fatal("current user message was truncated away")
	}
	if len(current.Messages) != 3 {
		t.Fatalf("context messages = %d, want current user plus newest pair", len(current.Messages))
	}
}

func TestFinalizeAgentRunWritesCompleteLifecycleEventOrder(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "lifecycle-events.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "lifecycle-session",
		RequestHash:     "lifecycle-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "lifecycle-send",
		RequestHash:     "lifecycle-send-hash",
		UserMessageID:   "lifecycle-user",
		UserParts:       []string{"生命周期"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinalizeAgentRun(ctx, testkit.FinalizeAgentRunInput{
		RunID:             run.Run.ID,
		Status:            testkit.RunCompleted,
		AssistantPartID:   run.AssistantPartID,
		AssistantText:     "完成",
		MessageStatus:     testkit.MessageComplete,
		PartStatus:        testkit.PartComplete,
		TerminalEventType: "run.completed",
	}); err != nil {
		t.Fatal(err)
	}

	events, err := db.ListAgentRunEvents(ctx, run.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"run.started",
		"step.started",
		"message.part.started",
		"message.part.finished",
		"step.finished",
		"run.completed",
	}
	if len(events) != len(want) {
		t.Fatalf("events = %#v, want %v", events, want)
	}
	for index, eventType := range want {
		if events[index].Type != eventType {
			t.Fatalf(
				"event[%d] = %q, want %q",
				index,
				events[index].Type,
				eventType,
			)
		}
		if events[index].Sequence != index+1 {
			t.Fatalf(
				"event[%d] sequence = %d",
				index,
				events[index].Sequence,
			)
		}
	}
}
