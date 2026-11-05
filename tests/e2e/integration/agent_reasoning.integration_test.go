package integration

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/leeat/craftingtable/server/testkit"
)

func TestReasoningPartPersistsSeparatelyFromAssistantText(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "reasoning.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "reasoning-session",
		RequestHash:     "reasoning-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "reasoning-send",
		RequestHash:     "reasoning-send-hash",
		UserMessageID:   "reasoning-user",
		UserParts:       []string{"解释一下推理"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if execution.ReasoningPartID == "" {
		t.Fatal("reasoning part id is empty")
	}
	if execution.ReasoningPartID == execution.AssistantPartID {
		t.Fatal("reasoning and text parts share an id")
	}

	err = db.FinalizeAgentRun(ctx, testkit.FinalizeAgentRunInput{
		RunID:              execution.Run.ID,
		Status:             testkit.RunCompleted,
		AssistantPartID:    execution.AssistantPartID,
		AssistantText:      "最终回答",
		ReasoningPartID:    execution.ReasoningPartID,
		ReasoningText:      "先分析问题，再组织回答",
		MessageStatus:      testkit.MessageComplete,
		PartStatus:         testkit.PartComplete,
		CompletedEventType: "run.completed",
	})
	if err != nil {
		t.Fatal(err)
	}

	detail, err := db.GetAgentSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var (
		reasoningText   string
		reasoningIndex  = -1
		assistantText   string
		assistantIndex  = -1
		reasoningStatus string
	)
	for _, part := range detail.MessageParts {
		if part.RunID == nil || *part.RunID != execution.Run.ID {
			continue
		}
		switch part.Type {
		case testkit.PartReasoning:
			reasoningText = part.Text
			reasoningIndex = part.Index
			reasoningStatus = string(part.Status)
		case testkit.PartText:
			assistantText = part.Text
			assistantIndex = part.Index
		}
	}
	if reasoningText != "先分析问题，再组织回答" {
		t.Fatalf("reasoning text = %q", reasoningText)
	}
	if assistantText != "最终回答" {
		t.Fatalf("assistant text = %q", assistantText)
	}
	if reasoningIndex != 0 || assistantIndex != 1 {
		t.Fatalf("indexes = reasoning %d, text %d", reasoningIndex, assistantIndex)
	}
	if reasoningStatus != "complete" {
		t.Fatalf("reasoning status = %q", reasoningStatus)
	}
}

func TestFinalizeDropsEmptyReasoningPart(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "reasoning-empty.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "empty-session",
		RequestHash:     "empty-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "empty-send",
		RequestHash:     "empty-send-hash",
		UserMessageID:   "empty-user",
		UserParts:       []string{"没有推理"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinalizeAgentRun(ctx, testkit.FinalizeAgentRunInput{
		RunID:              execution.Run.ID,
		Status:             testkit.RunCompleted,
		AssistantPartID:    execution.AssistantPartID,
		AssistantText:      "只有正文",
		ReasoningPartID:    execution.ReasoningPartID,
		MessageStatus:      testkit.MessageComplete,
		PartStatus:         testkit.PartComplete,
		CompletedEventType: "run.completed",
	}); err != nil {
		t.Fatal(err)
	}

	detail, err := db.GetAgentSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range detail.MessageParts {
		if part.RunID != nil &&
			*part.RunID == execution.Run.ID &&
			part.Type == testkit.PartReasoning {
			t.Fatalf("empty reasoning part survived: %#v", part)
		}
	}
}

func TestInterruptKeepsReceivedReasoning(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "reasoning-interrupt.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "interrupt-session",
		RequestHash:     "interrupt-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "interrupt-send",
		RequestHash:     "interrupt-send-hash",
		UserMessageID:   "interrupt-user",
		UserParts:       []string{"中断推理"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.InterruptAgentRun(ctx, testkit.InterruptAgentRunInput{
		RunID:           execution.Run.ID,
		AssistantPartID: execution.AssistantPartID,
		AssistantText:   "部分正文",
		ReasoningPartID: execution.ReasoningPartID,
		ReasoningText:   "部分推理",
		AgentError: testkit.NewAgentError(
			"AGENT_RUN_INTERRUPTED",
			"server",
			"Agent 运行在服务重启时中断",
			true,
		),
	})
	if err != nil {
		t.Fatal(err)
	}

	detail, err := db.GetAgentSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var reasoning string
	for _, part := range detail.MessageParts {
		if part.RunID != nil &&
			*part.RunID == execution.Run.ID &&
			part.Type == testkit.PartReasoning {
			reasoning = part.Text
			if part.Status != testkit.PartInterrupted {
				t.Fatalf("reasoning status = %q", part.Status)
			}
		}
	}
	if reasoning != "部分推理" {
		t.Fatalf("reasoning = %q", reasoning)
	}
}

func TestReasoningMigrationUpgradesLegacyParts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "reasoning-legacy.sqlite")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
		  name TEXT PRIMARY KEY,
		  checksum TEXT NOT NULL,
		  applied_at TEXT NOT NULL
		)
	`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_initial.sql", "002_agent.sql"} {
		body, err := testkit.MigrationContent(name)
		if err != nil {
			t.Fatal(err)
		}
		checksum, err := testkit.MigrationChecksum(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := legacy.ExecContext(ctx, body); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := legacy.ExecContext(
			ctx,
			`INSERT INTO schema_migrations (name, checksum, applied_at)
			 VALUES (?, ?, '2026-11-05T00:00:00Z')`,
			name,
			checksum,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := legacy.ExecContext(ctx, `
		INSERT INTO agent_sessions (
		  id, title, client_request_id, activity_seq, created_at, updated_at
		) VALUES (
		  'legacy-session', '旧会话', 'legacy-create', 0,
		  '2026-11-05T00:00:00Z', '2026-11-05T00:00:00Z'
		);
		INSERT INTO agent_messages (
		  id, session_id, seq, role, status, created_at, updated_at
		) VALUES (
		  'legacy-user', 'legacy-session', 1, 'user', 'complete',
		  '2026-11-05T00:00:00Z', '2026-11-05T00:00:00Z'
		);
		INSERT INTO agent_messages (
		  id, session_id, seq, role, status, user_message_id,
		  created_at, updated_at
		) VALUES (
		  'legacy-assistant', 'legacy-session', 2, 'assistant', 'failed',
		  'legacy-user', '2026-11-05T00:00:00Z', '2026-11-05T00:00:01Z'
		);
		INSERT INTO agent_runs (
		  id, session_id, user_message_id, assistant_message_id, status,
		  client_request_id, next_event_sequence, started_at, finished_at,
		  created_at, updated_at
		) VALUES (
		  'legacy-run', 'legacy-session', 'legacy-user', 'legacy-assistant',
		  'failed', 'legacy-send', 4, '2026-11-05T00:00:00Z',
		  '2026-11-05T00:00:01Z', '2026-11-05T00:00:00Z',
		  '2026-11-05T00:00:01Z'
		);
		UPDATE agent_messages SET active_run_id = 'legacy-run'
		 WHERE id = 'legacy-assistant';
		INSERT INTO agent_message_parts (
		  id, message_id, run_id, part_index, type, status, text_content,
		  created_at, updated_at
		) VALUES (
		  'legacy-text', 'legacy-assistant', 'legacy-run', 0, 'text',
		  'failed', '旧正文', '2026-11-05T00:00:00Z',
		  '2026-11-05T00:00:01Z'
		);
		INSERT INTO agent_message_parts (
		  id, message_id, run_id, part_index, type, status, text_content,
		  error_json, created_at, updated_at
		) VALUES (
		  'legacy-error', 'legacy-assistant', 'legacy-run', 1, 'error',
		  'failed', '', '{"code":"AGENT_PROVIDER_FAILED","kind":"server",
		  "message":"旧错误","retryable":true}',
		  '2026-11-05T00:00:01Z', '2026-11-05T00:00:01Z'
		);
	`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	defer db.Close()

	detail, err := db.GetAgentSession(ctx, "legacy-session")
	if err != nil {
		t.Fatal(err)
	}
	var text, errorMessage string
	for _, part := range detail.MessageParts {
		if part.Type == testkit.PartText && part.ID == "legacy-text" {
			text = part.Text
		}
		if part.Type == testkit.PartError && part.Error != nil {
			errorMessage = part.Error.Message
		}
	}
	if text != "旧正文" {
		t.Fatalf("legacy text = %q", text)
	}
	if errorMessage != "旧错误" {
		t.Fatalf("legacy error = %q", errorMessage)
	}

	session, err := db.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "post-upgrade-session",
		RequestHash:     "post-upgrade-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := db.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "post-upgrade-send",
		RequestHash:     "post-upgrade-send-hash",
		UserMessageID:   "post-upgrade-user",
		UserParts:       []string{"升级后写入推理"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinalizeAgentRun(ctx, testkit.FinalizeAgentRunInput{
		RunID:              execution.Run.ID,
		Status:             testkit.RunCompleted,
		AssistantPartID:    execution.AssistantPartID,
		AssistantText:      "升级后的正文",
		ReasoningPartID:    execution.ReasoningPartID,
		ReasoningText:      "升级后的推理",
		MessageStatus:      testkit.MessageComplete,
		PartStatus:         testkit.PartComplete,
		CompletedEventType: "run.completed",
	}); err != nil {
		t.Fatalf("reasoning write after upgrade: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO agent_message_parts (
		  id, message_id, run_id, part_index, type, status, text_content,
		  created_at, updated_at
		) VALUES (
		  'duplicate-part', ?, ?, 1, 'text', 'complete', '',
		  '2026-11-05T00:00:00Z', '2026-11-05T00:00:00Z'
		)
	`, execution.Run.AssistantMessageID, execution.Run.ID); err == nil {
		t.Fatal("assistant run part unique index was lost")
	}
}
