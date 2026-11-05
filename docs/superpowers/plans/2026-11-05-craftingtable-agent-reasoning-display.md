# CraftingTable Agent Reasoning Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stream MiMo `reasoning_content` into its own persisted MessagePart, recover it after reload, and show it above the answer in a collapsible "思考中..." panel.

**Architecture:** The Provider Parser turns `delta.reasoning_content` into a `reasoning` event; the Run Coordinator emits `reasoning-start`/`reasoning-delta`/`reasoning-end` chunks before `text-start`; the Event Journal keeps reasoning and text in separate buffers; Finalize writes both Parts in one transaction and deletes an empty reasoning Part. React maps the new stream events onto a `reasoning` Part through the existing Materializer and renders it above the text Part.

**Tech Stack:** Go 1.26, `net/http`, `database/sql`, `modernc.org/sqlite`, React 19, TypeScript, Zustand, Zod, `eventsource-parser`, Vitest, Playwright.

## Global Constraints

- The approved design is `docs/superpowers/specs/2026-11-05-craftingtable-agent-reasoning-display-design.md`.
- `reasoning` and `text` are separate MessageParts; Part indexes are `reasoning: 0`, `text: 1`, then `error`.
- `RunHandle.AssistantPartID` keeps meaning the text Part; `ReasoningPartID` is a new optional field.
- Reasoning never enters `ProviderMessage`; only `PartText` is used to build model context.
- `text-delta` and `reasoning-delta` are never written to SQLite per token; only Finalize/Interrupt persist content.
- A Run that produces no reasoning must not create a visible empty panel; its reasoning Part is deleted at Finalize.
- Reasoning status always mirrors the assistant Message terminal status.
- The UI title is exactly `思考中...`; no Brain icon and no Markdown rendering.
- Reasoning is collapsed after reload; expanded while reasoning streams; auto-collapsed on the first text token; a manual toggle wins over the automatic rule.
- Do not log reasoning bodies. Logs may contain Part IDs, chars and error kinds only.
- The legacy `/api/v1/agent/stream` endpoint keeps ignoring reasoning events; only the persistent pipeline changes.
- All new front-end and integration tests live under `tests/e2e`; Go in-package parser tests extend the existing `server/internal/agent/core/stream_parser_test.go`.
- Service keys must not appear in source, SQLite, fixtures, browser state or logs.
- Every task starts with a failing test and ends with focused tests plus a commit.

---

## File Responsibility Map

```text
server/internal/agent/domain/
  session.go        PartReasoning constant
  run.go            RunExecution.ReasoningPartID, RunHandle.ReasoningPartID

server/internal/agent/core/
  repository.go     Finalize/Interrupt records carry reasoning
  stream_parser.go  delta.reasoning_content -> "reasoning" event
  stream_chunk.go   StreamReasoningStart/Delta/End kinds
  event_journal.go  Separate Reasoning buffer in JournalSnapshot
  coordinator.go    Reasoning/text stream ordering and finalize payload
  service.go        Session detail backfill from the journal

server/internal/agent/access/
  ui_reasoning_part.go  reasoning-* UI Message Stream parts
  ui_stream.go          Chunk to UI part mapping

server/internal/platform/store/
  sqlite_store.go                     Migration content/checksum accessors
  agent_store.go                      Reasoning Part persistence
  migrations/003_agent_reasoning.sql  Part type constraint rebuild

src/models/
  agentContracts.ts        AgentReasoningPart contract
  agentStreamSchema.ts     reasoning-* chunk schema and events
  agentStreamReducer.ts    reasoning-delta event mapping
  agentRunMaterializer.ts  Separate reasoningPart and textPart
  agentRunClient.ts        Handles reasoning-delta events
  agentTransport.ts        Optional reasoningPartId in RunHandle
  apiAgentModel.ts         Passes reasoningPartId into the Materializer
  fakeAgentModel.ts        Optional reasoning stream for unit tests

src/components/agent/
  agentReasoningPanel.tsx  Collapsible reasoning panel
  agentMessageList.tsx     Panel above text, thinking announcement

tests/e2e/
  integration/agent_reasoning*.go     Persistence, stream order, recovery
  unit/agent/*.test.ts(x)             Protocol, materializer, panel
  modules/agent/reasoning.e2e.ts      Browser regression
```

---

### Task 1: Persist Reasoning As Its Own MessagePart

**Files:**
- Modify: `server/internal/agent/domain/session.go`
- Modify: `server/internal/agent/domain/run.go`
- Modify: `server/internal/agent/core/repository.go`
- Modify: `server/internal/platform/store/sqlite_store.go`
- Modify: `server/internal/platform/store/agent_store.go`
- Create: `server/internal/platform/store/migrations/003_agent_reasoning.sql`
- Modify: `server/testkit/agent.go`
- Test: `tests/e2e/integration/agent_reasoning.integration_test.go`
- Test: `tests/e2e/integration/migrations.integration_test.go`

**Interfaces:**
- Consumes: existing `testkit.OpenSQLite`, `SQLiteStore.CreateAgentSession`, `CreateAgentSendRun`, `FinalizeAgentRun`, `InterruptAgentRun`, `GetAgentSession`.
- Produces: `domain.PartReasoning`; `domain.RunExecution.ReasoningPartID`; `domain.RunHandle.ReasoningPartID`; `core.FinalizeRunRecord.ReasoningPartID` / `ReasoningText`; `core.InterruptRunRecord.ReasoningPartID` / `ReasoningText`; `store.MigrationContent(name)` and `store.MigrationChecksum(name)`; `testkit.PartReasoning`, `testkit.MigrationContent`, `testkit.MigrationChecksum`.

- [ ] **Step 1: Write the failing persistence tests**

Create `tests/e2e/integration/agent_reasoning.integration_test.go`:

```go
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

	store, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	defer store.Close()

	detail, err := store.GetAgentSession(ctx, "legacy-session")
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

	session, err := store.CreateAgentSession(ctx, testkit.CreateAgentSessionInput{
		ClientRequestID: "post-upgrade-session",
		RequestHash:     "post-upgrade-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.CreateAgentSendRun(ctx, testkit.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "post-upgrade-send",
		RequestHash:     "post-upgrade-send-hash",
		UserMessageID:   "post-upgrade-user",
		UserParts:       []string{"升级后写入推理"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinalizeAgentRun(ctx, testkit.FinalizeAgentRunInput{
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

	if err := store.Close(); err != nil {
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
```

Extend `tests/e2e/integration/migrations.integration_test.go` step list from
`[]string{"001_initial.sql", "002_agent.sql"}` to
`[]string{"001_initial.sql", "002_agent.sql", "003_agent_reasoning.sql"}`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tests/e2e/integration/ -run 'Reasoning|Migration' -v`
Expected: build failure referencing `testkit.PartReasoning`, `testkit.MigrationContent`, `ReasoningPartID`, `NewAgentError`, or `PartInterrupted` not defined.

- [ ] **Step 3: Add the domain and Port fields**

`server/internal/agent/domain/session.go`:

```go
const (
	PartText      PartType = "text"
	PartReasoning PartType = "reasoning"
	PartError     PartType = "error"
)
```

`server/internal/agent/domain/run.go`:

```go
type RunExecution struct {
	Run             Run
	AssistantPartID string
	ReasoningPartID string
	Messages        []ContextMessage
	Replayed        bool
}

type RunHandle struct {
	RunID              string    `json:"runId"`
	SessionID          string    `json:"sessionId"`
	UserMessageID      string    `json:"userMessageId"`
	AssistantMessageID string    `json:"assistantMessageId"`
	AssistantPartID    string    `json:"assistantPartId"`
	ReasoningPartID    string    `json:"reasoningPartId,omitempty"`
	Status             RunStatus `json:"status"`
}
```

`server/internal/agent/core/repository.go`:

```go
type FinalizeRunRecord struct {
	RunID              string
	Status             domain.RunStatus
	AssistantPartID    string
	AssistantText      string
	ReasoningPartID    string
	ReasoningText      string
	MessageStatus      domain.MessageStatus
	PartStatus         domain.PartStatus
	AgentError         *domain.Error
	TerminalEventType  string
	CompletedEventType string
}

type InterruptRunRecord struct {
	RunID           string
	AssistantPartID string
	AssistantText   string
	ReasoningPartID string
	ReasoningText   string
	AgentError      domain.Error
}
```

- [ ] **Step 4: Add migration 003**

Create `server/internal/platform/store/migrations/003_agent_reasoning.sql`:

```sql
CREATE TABLE agent_message_parts_with_reasoning (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL REFERENCES agent_messages(id) ON DELETE CASCADE,
  run_id TEXT REFERENCES agent_runs(id),
  part_index INTEGER NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('text', 'reasoning', 'error')),
  status TEXT NOT NULL CHECK (
    status IN ('streaming', 'complete', 'failed', 'cancelled', 'interrupted')
  ),
  text_content TEXT NOT NULL DEFAULT '',
  error_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

INSERT INTO agent_message_parts_with_reasoning (
  id, message_id, run_id, part_index, type, status, text_content,
  error_json, created_at, updated_at
)
SELECT id, message_id, run_id, part_index, type, status, text_content,
       error_json, created_at, updated_at
FROM agent_message_parts;

DROP TABLE agent_message_parts;

ALTER TABLE agent_message_parts_with_reasoning RENAME TO agent_message_parts;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_message_parts
  ON agent_message_parts(message_id, part_index) WHERE run_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_assistant_run_parts
  ON agent_message_parts(message_id, run_id, part_index) WHERE run_id IS NOT NULL;
```

Add migration accessors in `server/internal/platform/store/sqlite_store.go`, directly after `applyMigration`:

```go
// MigrationContent returns one embedded migration body for upgrade tests.
func MigrationContent(name string) (string, error) {
	body, err := migrationFiles.ReadFile("migrations/" + name)
	if err != nil {
		return "", fmt.Errorf("read migration %s: %w", name, err)
	}
	return string(body), nil
}

// MigrationChecksum returns the recorded SHA-256 of one embedded migration.
func MigrationChecksum(name string) (string, error) {
	body, err := MigrationContent(name)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:]), nil
}
```

Export them from `server/testkit/agent.go`:

```go
var MigrationContent = store.MigrationContent
var MigrationChecksum = store.MigrationChecksum
```

and extend the constants block:

```go
const (
	RunCompleted     = domain.RunCompleted
	RunRunning       = domain.RunRunning
	RunInterrupted   = domain.RunInterrupted
	MessageComplete  = domain.MessageComplete
	PartComplete     = domain.PartComplete
	PartInterrupted  = domain.PartInterrupted
	PartText         = domain.PartText
	PartReasoning    = domain.PartReasoning
	PartError        = domain.PartError
	StreamTextDelta  = core.StreamTextDelta
	StreamFinish     = core.StreamFinish
)

var NewAgentError = domain.NewError
```

- [ ] **Step 5: Create reasoning parts when a Run is created**

In `server/internal/platform/store/agent_store.go`, inside `CreateAgentSendRun`, replace the
single part-ID creation and single part insert with:

```go
		reasoningPartID, err := newID()
		if err != nil {
			return fmt.Errorf("create reasoning part id: %w", err)
		}
		assistantPartID, err := newID()
		if err != nil {
			return fmt.Errorf("create assistant part id: %w", err)
		}
```

and replace the `INSERT INTO agent_message_parts ... VALUES (?, ?, ?, 0, 'text', ...)`
block with:

```go
		for _, part := range []struct {
			id    string
			index int
			kind  domain.PartType
		}{
			{reasoningPartID, 0, domain.PartReasoning},
			{assistantPartID, 1, domain.PartText},
		} {
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO agent_message_parts (
				  id, message_id, run_id, part_index, type, status, text_content,
				  created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, 'streaming', '', ?, ?)`,
				part.id,
				assistantMessageID,
				runID,
				part.index,
				part.kind,
				now,
				now,
			); err != nil {
				return fmt.Errorf("insert assistant message part: %w", err)
			}
		}
```

Keep the `agent_steps` insert using `assistantPartID+"-step-0"`.

Apply the same two-part creation to `CreateAgentRetryRun`: add a `reasoningPartID` next to the
existing `partID`, and replace its part insert with the same two-entry loop using
`input.AssistantMessageID`, `runID`, `reasoningPartID` and `partID` (text index `1`).

- [ ] **Step 6: Load and settle reasoning parts**

In `getRunExecution`, replace the assistant-part lookup with:

```go
	var (
		assistantPartID string
		reasoningPartID string
	)
	err = s.db.QueryRowContext(
		ctx,
		`SELECT id FROM agent_message_parts
		 WHERE message_id = ? AND run_id = ? AND type = 'text'`,
		run.AssistantMessageID,
		run.ID,
	).Scan(&assistantPartID)
	if err != nil {
		return domain.RunExecution{}, fmt.Errorf("load run assistant part: %w", err)
	}
	err = s.db.QueryRowContext(
		ctx,
		`SELECT id FROM agent_message_parts
		 WHERE message_id = ? AND run_id = ? AND type = 'reasoning'`,
		run.AssistantMessageID,
		run.ID,
	).Scan(&reasoningPartID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.RunExecution{}, fmt.Errorf("load run reasoning part: %w", err)
	}
```

and return:

```go
	return domain.RunExecution{
		Run:             run,
		AssistantPartID: assistantPartID,
		ReasoningPartID: reasoningPartID,
		Messages: contextMessages(
			run,
			detail.Messages,
			detail.MessageParts,
		),
	}, nil
```

In `FinalizeAgentRun`, after the assistant Part `UPDATE` and before the `error` Part block, add:

```go
		if input.ReasoningPartID != "" {
			var reasoningMessageID string
			err = tx.QueryRowContext(
				ctx,
				`SELECT message_id FROM agent_message_parts
				 WHERE id = ? AND run_id = ? AND type = 'reasoning'`,
				input.ReasoningPartID,
				input.RunID,
			).Scan(&reasoningMessageID)
			if errors.Is(err, sql.ErrNoRows) {
				return core.ErrRunNotFound
			}
			if err != nil {
				return fmt.Errorf("load reasoning part: %w", err)
			}
			if reasoningMessageID != assistantMessageID {
				return core.ErrRunNotFound
			}
		}
		if strings.TrimSpace(input.ReasoningText) == "" {
			if _, err := tx.ExecContext(
				ctx,
				`DELETE FROM agent_message_parts
				 WHERE run_id = ? AND type = 'reasoning'`,
				input.RunID,
			); err != nil {
				return fmt.Errorf("delete empty reasoning part: %w", err)
			}
		} else if _, err := tx.ExecContext(
			ctx,
			`UPDATE agent_message_parts
			 SET text_content = ?, status = ?, updated_at = ?
			 WHERE run_id = ? AND type = 'reasoning'`,
			input.ReasoningText,
			input.PartStatus,
			now,
			input.RunID,
		); err != nil {
			return fmt.Errorf("finalize reasoning part: %w", err)
		}
```

In `InterruptAgentRun`, after the assistant Part `UPDATE`, add:

```go
		if strings.TrimSpace(input.ReasoningText) == "" {
			if _, err := tx.ExecContext(
				ctx,
				`DELETE FROM agent_message_parts
				 WHERE run_id = ? AND type = 'reasoning'`,
				input.RunID,
			); err != nil {
				return fmt.Errorf("delete empty reasoning part: %w", err)
			}
		} else if _, err := tx.ExecContext(
			ctx,
			`UPDATE agent_message_parts
			 SET text_content = ?, status = 'interrupted', updated_at = ?
			 WHERE run_id = ? AND type = 'reasoning'`,
			input.ReasoningText,
			now,
			input.RunID,
		); err != nil {
			return fmt.Errorf("interrupt reasoning part: %w", err)
		}
```

In `RecoverInterruptedAgentRuns`, change the assistant text query from
`WHERE run_id = ? AND type = 'text' AND part_index = 0` to
`WHERE run_id = ? AND type = 'text'`, and remove reasoning parts that were never
filled, immediately after the streaming-part update:

```go
			if _, err := tx.ExecContext(
				ctx,
				`DELETE FROM agent_message_parts
				 WHERE run_id = ? AND type = 'reasoning' AND text_content = ''`,
				run.id,
			); err != nil {
				return fmt.Errorf("delete empty reasoning part: %w", err)
			}
```

- [ ] **Step 7: Run the focused Go tests**

Run: `go test ./tests/e2e/integration/ -run 'Reasoning|Migration' -v`
Expected: PASS.

Run: `go test ./tests/e2e/integration/ ./server/... -count=1`
Expected: PASS (existing persistence and recovery suites still green, because an empty reasoning Part is deleted during Finalize).

- [ ] **Step 8: Commit**

```bash
git add server/internal/agent/domain server/internal/agent/core/repository.go server/internal/platform/store server/testkit tests/e2e/integration/agent_reasoning.integration_test.go tests/e2e/integration/migrations.integration_test.go
git commit -m "feat(agent): persist reasoning as its own message part"
```

---

### Task 2: Stream Reasoning Chunks Before Text

**Files:**
- Modify: `server/internal/agent/core/stream_parser.go`
- Modify: `server/internal/agent/core/stream_chunk.go`
- Modify: `server/internal/agent/core/event_journal.go`
- Modify: `server/internal/agent/core/coordinator.go`
- Create: `server/internal/agent/access/ui_reasoning_part.go`
- Modify: `server/internal/agent/access/ui_stream.go`
- Modify: `server/internal/agent/core/service.go`
- Modify: `server/testkit/agent.go`
- Test: `server/internal/agent/core/stream_parser_test.go`
- Test: `tests/e2e/integration/agent_reasoning_stream.integration_test.go`

**Interfaces:**
- Consumes: Task 1 `ReasoningPartID` and `PartReasoning`.
- Produces: `core.StreamReasoningStart`, `core.StreamReasoningDelta`, `core.StreamReasoningEnd`; `JournalSnapshot.Reasoning`; provider event name `"reasoning"`; UI parts `reasoning-start`, `reasoning-delta`, `reasoning-end`; `testkit.StreamReasoningStart/Delta/End`.

- [ ] **Step 1: Write the failing tests**

Append to `server/internal/agent/core/stream_parser_test.go`:

```go
func TestRelayProviderStreamSeparatesReasoningFromContent(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"先分析\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"最终回答\"}}]}\n\n" +
			"data: [DONE]\n\n",
	)
	events := make([]capturedEvent, 0)
	err := RelayProviderStream(
		context.Background(),
		source,
		func(name string, payload any) error {
			events = append(events, capturedEvent{name: name, payload: payload})
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RelayProviderStream() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	if events[0].name != "reasoning" {
		t.Fatalf("first event = %q, want reasoning", events[0].name)
	}
	if values, ok := events[0].payload.(map[string]string); !ok ||
		values["text"] != "先分析" {
		t.Fatalf("reasoning payload = %#v", events[0].payload)
	}
	if events[1].name != "delta" {
		t.Fatalf("second event = %q, want delta", events[1].name)
	}
}
```

Create `tests/e2e/integration/agent_reasoning_stream.integration_test.go`:

```go
package integration

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/leeat/craftingtable/server/testkit"
)

type scriptedStreamProvider struct {
	mu      sync.Mutex
	scripts []string
	calls   [][]testkit.ProviderMessage
}

func (p *scriptedStreamProvider) Stream(
	_ context.Context,
	messages []testkit.ProviderMessage,
) (io.ReadCloser, error) {
	p.mu.Lock()
	p.calls = append(p.calls, messages)
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
	return p.calls[index]
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
	last := messages[len(messages)-1]
	if last.Role != "assistant" || last.Content != "第一轮回答" {
		t.Fatalf("last assistant context = %#v", last)
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
```

Add `"time"` to that file's imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./server/internal/agent/core/ -run RelayProviderStream -v`
Expected: FAIL, the reasoning event is missing.

Run: `go test ./tests/e2e/integration/ -run 'Reasoning' -v`
Expected: build failure referencing `testkit.StreamReasoningStart` or `StreamFinish` ordering assertions later.

- [ ] **Step 3: Parse and relay `reasoning_content`**

In `server/internal/agent/core/stream_parser.go`, extend the upstream shape:

```go
type upstreamEvent struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}
```

and in `processData`, before the content branch:

```go
	if choice.Delta.ReasoningContent != "" {
		if err := sink("reasoning", map[string]string{
			"text": choice.Delta.ReasoningContent,
		}); err != nil {
			return false, err
		}
	}
```

- [ ] **Step 4: Add reasoning chunk kinds and journal buffer**

`server/internal/agent/core/stream_chunk.go`:

```go
const (
	StreamStart          StreamChunkKind = "start"
	StreamStartStep      StreamChunkKind = "start-step"
	StreamReasoningStart StreamChunkKind = "reasoning-start"
	StreamReasoningDelta StreamChunkKind = "reasoning-delta"
	StreamReasoningEnd   StreamChunkKind = "reasoning-end"
	StreamTextStart      StreamChunkKind = "text-start"
	StreamTextDelta      StreamChunkKind = "text-delta"
	StreamTextEnd        StreamChunkKind = "text-end"
	StreamFinishStep     StreamChunkKind = "finish-step"
	StreamFinish         StreamChunkKind = "finish"
	StreamError          StreamChunkKind = "error"
)
```

`server/internal/agent/core/event_journal.go`:

```go
type JournalSnapshot struct {
	Chunks       []StreamChunk
	Text         string
	Reasoning    string
	LastSequence int
}

type EventJournal struct {
	mu           sync.Mutex
	maxBytes     int64
	bytes        int64
	nextSequence int
	text         []byte
	reasoning    []byte
	chunks       []StreamChunk
	subscribers  map[uint64]*journalSubscriber
	nextID       uint64
	closed       bool
}
```

In `Append`:

```go
	if chunk.Kind == StreamTextDelta {
		j.text = append(j.text, chunk.Text...)
	}
	if chunk.Kind == StreamReasoningDelta {
		j.reasoning = append(j.reasoning, chunk.Text...)
	}
```

In `Snapshot`:

```go
	return JournalSnapshot{
		Chunks:       chunks,
		Text:         string(j.text),
		Reasoning:    string(j.reasoning),
		LastSequence: j.nextSequence - 1,
	}
```

- [ ] **Step 5: Order reasoning before text in the Coordinator**

Create the ordering helper inside `server/internal/agent/core/coordinator.go`, above `func (c *Coordinator) execute`:

```go
type runStream struct {
	active           *activeRun
	execution        domain.RunExecution
	reasoningStarted bool
	reasoningEnded   bool
	textStarted      bool
	textEnded        bool
}

func newRunStream(
	active *activeRun,
	execution domain.RunExecution,
) *runStream {
	return &runStream{active: active, execution: execution}
}

func (s *runStream) chunk(kind StreamChunkKind, partID string) StreamChunk {
	return StreamChunk{
		Kind:               kind,
		RunID:              s.execution.Run.ID,
		SessionID:          s.execution.Run.SessionID,
		UserMessageID:      s.execution.Run.UserMessageID,
		AssistantMessageID: s.execution.Run.AssistantMessageID,
		PartID:             partID,
	}
}

func (s *runStream) reasoning(text string) error {
	if s.execution.ReasoningPartID == "" || text == "" {
		return nil
	}
	if !s.reasoningStarted {
		if err := s.active.journal.Append(s.chunk(
			StreamReasoningStart,
			s.execution.ReasoningPartID,
		)); err != nil {
			return err
		}
		s.reasoningStarted = true
	}
	delta := s.chunk(StreamReasoningDelta, s.execution.ReasoningPartID)
	delta.Text = text
	return s.active.journal.Append(delta)
}

func (s *runStream) endReasoning() error {
	if !s.reasoningStarted || s.reasoningEnded {
		return nil
	}
	s.reasoningEnded = true
	return s.active.journal.Append(s.chunk(
		StreamReasoningEnd,
		s.execution.ReasoningPartID,
	))
}

func (s *runStream) text(text string) error {
	if text == "" {
		return nil
	}
	if err := s.endReasoning(); err != nil {
		return err
	}
	if !s.textStarted {
		if err := s.active.journal.Append(s.chunk(
			StreamTextStart,
			s.execution.AssistantPartID,
		)); err != nil {
			return err
		}
		s.textStarted = true
	}
	delta := s.chunk(StreamTextDelta, s.execution.AssistantPartID)
	delta.Text = text
	return s.active.journal.Append(delta)
}

func (s *runStream) close() error {
	if err := s.endReasoning(); err != nil {
		return err
	}
	if !s.textStarted || s.textEnded {
		return nil
	}
	s.textEnded = true
	return s.active.journal.Append(s.chunk(
		StreamTextEnd,
		s.execution.AssistantPartID,
	))
}
```

Add `stream *runStream` to `type activeRun struct`.

Rewrite `execute` so it no longer appends `StreamTextStart` up front:

```go
func (c *Coordinator) execute(active *activeRun) {
	defer close(active.done)
	defer c.stopRun(active)
	execution := active.execution
	active.stream = newRunStream(active, execution)
	for _, chunk := range []StreamChunk{
		{
			Kind:               StreamStart,
			RunID:              execution.Run.ID,
			SessionID:          execution.Run.SessionID,
			UserMessageID:      execution.Run.UserMessageID,
			AssistantMessageID: execution.Run.AssistantMessageID,
			PartID:             execution.AssistantPartID,
		},
		{
			Kind:               StreamStartStep,
			RunID:              execution.Run.ID,
			AssistantMessageID: execution.Run.AssistantMessageID,
			PartID:             execution.AssistantPartID,
		},
	} {
		if err := active.journal.Append(chunk); err != nil {
			c.finishFailure(active, streamFailure(err))
			return
		}
	}

	active.startIdleTimer(c.config.IdleTimeout)
	body, err := c.provider.Stream(
		active.ctx,
		ContextMessages(execution.Messages),
	)
	if err != nil {
		c.finishFromExecutionError(active, err)
		return
	}
	defer body.Close()

	err = relayProviderStream(
		active.ctx,
		body,
		func(event string, payload any) error {
			values, ok := payload.(map[string]string)
			if !ok {
				return ErrInvalidEvent
			}
			switch event {
			case "reasoning":
				return active.stream.reasoning(values["text"])
			case "delta":
				return active.stream.text(values["text"])
			default:
				return nil
			}
		},
		active.touch,
	)
	if err != nil {
		c.finishFromExecutionError(active, err)
		return
	}
	c.finishSuccess(active)
}
```

In `appendTerminal`, replace the unconditional `StreamTextEnd` append with:

```go
	execution := active.execution
	if active.stream != nil {
		_ = active.stream.close()
	}
	if status == domain.RunFailed {
```

so the failed branch is:

```go
	if status == domain.RunFailed {
		_ = active.journal.Append(StreamChunk{
			Kind:               StreamError,
			RunID:              execution.Run.ID,
			SessionID:          execution.Run.SessionID,
			UserMessageID:      execution.Run.UserMessageID,
			AssistantMessageID: execution.Run.AssistantMessageID,
			PartID:             execution.AssistantPartID,
			Status:             status,
			Error:              agentError,
		})
		return
	}
```

- [ ] **Step 6: Persist reasoning during Finalize and Interrupt**

In each of `finishCompleted`, `finishCancelled`, `finishFailure`, and `finishInterrupted`, take:

```go
	snapshot := active.journal.Snapshot()
```

and pass `ReasoningPartID: execution.ReasoningPartID` plus `ReasoningText: snapshot.Reasoning`
into the corresponding record. Example for `finishCompleted`:

```go
func (c *Coordinator) finishCompleted(active *activeRun) {
	execution := active.execution
	snapshot := active.journal.Snapshot()
	err := c.finalize(
		active,
		FinalizeRunRecord{
			RunID:             execution.Run.ID,
			Status:            domain.RunCompleted,
			AssistantPartID:   execution.AssistantPartID,
			AssistantText:     snapshot.Text,
			ReasoningPartID:   execution.ReasoningPartID,
			ReasoningText:     snapshot.Reasoning,
			MessageStatus:     domain.MessageComplete,
			PartStatus:        domain.PartComplete,
			TerminalEventType: "run.completed",
		},
	)
	if err != nil {
		c.logFinalizeError(active, err)
		c.finishInterrupted(active)
		return
	}
	c.appendTerminal(active, domain.RunCompleted, nil)
	c.scheduleJournalRemoval(active)
}
```

`finishInterrupted`:

```go
	snapshot := active.journal.Snapshot()
	err := interruptRunWithRetry(
		c.repository,
		c.config,
		InterruptRunRecord{
			RunID:           execution.Run.ID,
			AssistantPartID: execution.AssistantPartID,
			AssistantText:   snapshot.Text,
			ReasoningPartID: execution.ReasoningPartID,
			ReasoningText:   snapshot.Reasoning,
			AgentError:      agentError,
		},
	)
```

Apply the equivalent change in `handleFromExecution` (`server/internal/agent/core/service.go`):

```go
func handleFromExecution(execution domain.RunExecution) domain.RunHandle {
	return domain.RunHandle{
		RunID:              execution.Run.ID,
		SessionID:          execution.Run.SessionID,
		UserMessageID:      execution.Run.UserMessageID,
		AssistantMessageID: execution.Run.AssistantMessageID,
		AssistantPartID:    execution.AssistantPartID,
		ReasoningPartID:    execution.ReasoningPartID,
		Status:             execution.Run.Status,
	}
}
```

- [ ] **Step 7: Encode reasoning UI Message Stream parts**

Create `server/internal/agent/access/ui_reasoning_part.go`:

```go
package access

import "encoding/json"

type uiReasoningStartPart struct {
	ID string
}

func (uiReasoningStartPart) UIStreamType() string {
	return "reasoning-start"
}

func (p uiReasoningStartPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"reasoning-start", p.ID})
}

type uiReasoningDeltaPart struct {
	ID    string
	Delta string
}

func (uiReasoningDeltaPart) UIStreamType() string {
	return "reasoning-delta"
}

func (p uiReasoningDeltaPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Delta string `json:"delta"`
	}{"reasoning-delta", p.ID, p.Delta})
}

type uiReasoningEndPart struct {
	ID string
}

func (uiReasoningEndPart) UIStreamType() string {
	return "reasoning-end"
}

func (p uiReasoningEndPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"reasoning-end", p.ID})
}
```

In `server/internal/agent/access/ui_stream.go`, extend `uiPartFromStreamChunk` before the text cases:

```go
	case core.StreamReasoningStart:
		return uiReasoningStartPart{ID: chunk.PartID}
	case core.StreamReasoningDelta:
		return uiReasoningDeltaPart{ID: chunk.PartID, Delta: chunk.Text}
	case core.StreamReasoningEnd:
		return uiReasoningEndPart{ID: chunk.PartID}
```

Add the stream constants to `server/testkit/agent.go`:

```go
	StreamReasoningStart = core.StreamReasoningStart
	StreamReasoningDelta = core.StreamReasoningDelta
	StreamReasoningEnd   = core.StreamReasoningEnd
```

- [ ] **Step 8: Backfill reasoning in Session details**

In `server/internal/agent/core/service.go` `GetSession`, replace the part loop with:

```go
	for index := range detail.MessageParts {
		part := &detail.MessageParts[index]
		if part.MessageID != detail.ActiveRun.AssistantMessageID ||
			part.RunID == nil ||
			*part.RunID != detail.ActiveRun.RunID {
			continue
		}
		switch part.Type {
		case domain.PartText:
			part.Text = snapshot.Text
		case domain.PartReasoning:
			part.Text = snapshot.Reasoning
		}
	}
```

- [ ] **Step 9: Run the tests**

Run: `go test ./server/internal/agent/core/ -run RelayProviderStream -v`
Expected: PASS.

Run: `go test ./tests/e2e/integration/ -run 'Reasoning' -v`
Expected: PASS.

Run: `go test ./server/... ./tests/e2e/integration/ -count=1`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add server/internal/agent server/testkit tests/e2e/integration/agent_reasoning_stream.integration_test.go
git commit -m "feat(agent): stream reasoning before assistant text"
```

---

### Task 3: Verify Journal-Backed Reasoning Recovery

**Files:**
- Test: `tests/e2e/integration/agent_reasoning_recovery.integration_test.go`

**Interfaces:**
- Consumes: `Service.GetSession` backfill from Task 2.
- Produces: regression coverage only; no production code changes.

- [ ] **Step 1: Write the failing recovery test**

Create `tests/e2e/integration/agent_reasoning_recovery.integration_test.go`:

```go
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
		t.Fatalf("journal cursor = %d, want text start after reasoning", lastSequence)
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
```

- [ ] **Step 2: Run the test**

Run: `go test ./tests/e2e/integration/ -run TestSessionDetailBackfillsStreamingReasoning -v`
Expected: PASS if Task 2 is complete. If it fails with an empty reasoning Part, re-check the `domain.PartReasoning` branch in `Service.GetSession`.

- [ ] **Step 3: Run the whole Go suite**

Run: `go test ./server/... ./tests/e2e/integration/ -count=1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add tests/e2e/integration/agent_reasoning_recovery.integration_test.go
git commit -m "test(agent): cover reasoning recovery from the run journal"
```

---

### Task 4: Front-End Reasoning Protocol

**Files:**
- Modify: `src/models/agentContracts.ts`
- Modify: `src/models/agentStreamSchema.ts`
- Modify: `src/models/agentStreamReducer.ts`
- Test: `tests/e2e/unit/agent/agentStreamSchema.test.ts`
- Test: `tests/e2e/unit/agent/agentStreamReducer.test.ts`

**Interfaces:**
- Consumes: server UI stream parts `reasoning-start`, `reasoning-delta`, `reasoning-end`.
- Produces: `AgentReasoningPart`, `AgentReasoningPartContent`, `AgentRunHandle.reasoningPartId`, `AgentStreamEvent` variant `{ type: "reasoning-delta"; id; delta; cursor }`.

- [ ] **Step 1: Write the failing tests**

Append to `tests/e2e/unit/agent/agentStreamSchema.test.ts`:

```ts
  it("accepts reasoning chunks from the server protocol", () => {
    const [started, startError] = parseAgentStreamPart({
      type: "reasoning-start",
      id: "reasoning-1",
    });
    const [delta, deltaError] = parseAgentStreamPart({
      type: "reasoning-delta",
      id: "reasoning-1",
      delta: "先分析",
    });
    const [ended, endError] = parseAgentStreamPart({
      type: "reasoning-end",
      id: "reasoning-1",
    });

    expect(startError).toBeNull();
    expect(deltaError).toBeNull();
    expect(endError).toBeNull();
    expect(started?.kind).toBe("known");
    expect(delta?.kind).toBe("known");
    expect(ended?.kind).toBe("known");
  });
```

Append to `tests/e2e/unit/agent/agentStreamReducer.test.ts`:

```ts
  it("emits reasoning deltas without leaving the streaming phase", () => {
    const [transition, error] = reduceAgentStream(
      { phase: "streaming", lastCursor: 2 },
      frame(
        { type: "reasoning-delta", id: "reasoning-1", delta: "先分析" },
        3,
      ),
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "streaming", lastCursor: 3 },
      events: [
        {
          type: "reasoning-delta",
          id: "reasoning-1",
          delta: "先分析",
          cursor: 3,
        },
      ],
    });
  });

  it("ignores reasoning start and end markers as state-only chunks", () => {
    const [started, startError] = reduceAgentStream(
      { phase: "streaming", lastCursor: 3 },
      frame({ type: "reasoning-start", id: "reasoning-1" }, 4),
    );
    const [ended, endError] = reduceAgentStream(
      { phase: "streaming", lastCursor: 4 },
      frame({ type: "reasoning-end", id: "reasoning-1" }, 5),
    );

    expect(startError).toBeNull();
    expect(endError).toBeNull();
    expect(started).toEqual({
      state: { phase: "streaming", lastCursor: 4 },
      events: [],
    });
    expect(ended).toEqual({
      state: { phase: "streaming", lastCursor: 5 },
      events: [],
    });
  });
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm vitest run tests/e2e/unit/agent/agentStreamSchema.test.ts tests/e2e/unit/agent/agentStreamReducer.test.ts`
Expected: FAIL, reasoning chunk types are rejected or produce no event.

- [ ] **Step 3: Extend the contracts**

In `src/models/agentContracts.ts`:

```ts
export type AgentReasoningPartContent = {
  type: "reasoning";
  text: string;
};

export type AgentTextPartContent = {
  type: "text";
  text: string;
};

export type AgentErrorPartContent = {
  type: "error";
  error: AgentError;
};

type AgentMessagePartBase = {
  id: string;
  messageId: string;
  runId?: string;
  index: number;
  status: AgentMessagePartStatus;
  createdAt: string;
  updatedAt: string;
};

export type AgentTextPart = AgentMessagePartBase & {
  type: "text";
  content: AgentTextPartContent;
};

export type AgentReasoningPart = AgentMessagePartBase & {
  type: "reasoning";
  content: AgentReasoningPartContent;
};

export type AgentErrorPart = AgentMessagePartBase & {
  type: "error";
  content: AgentErrorPartContent;
};

export type AgentMessagePart = AgentTextPart | AgentReasoningPart | AgentErrorPart;
```

and in `AgentRunHandle`:

```ts
export type AgentRunHandle = {
  runId: string;
  sessionId: string;
  userMessageId: string;
  assistantMessageId: string;
  assistantPartId: string;
  reasoningPartId?: string;
  status: AgentRunStatus;
};
```

- [ ] **Step 4: Extend the stream schema**

In `src/models/agentStreamSchema.ts`, add after `startStepPartSchema`:

```ts
const reasoningStartPartSchema = z.object({
  type: z.literal("reasoning-start"),
  id: z.string().min(1),
});
const reasoningDeltaPartSchema = z.object({
  type: z.literal("reasoning-delta"),
  id: z.string().min(1),
  delta: z.string(),
});
const reasoningEndPartSchema = z.object({
  type: z.literal("reasoning-end"),
  id: z.string().min(1),
});
```

add all three to `streamPartSchema`, extend `knownPartTypes` with
`"reasoning-start"`, `"reasoning-delta"`, `"reasoning-end"`, and extend `AgentStreamEvent`:

```ts
export type AgentStreamEvent =
  | { type: "reasoning-delta"; id: string; delta: string; cursor: number }
  | { type: "text-delta"; id: string; delta: string; cursor: number }
  | {
      type: "finish";
      finishReason?: string;
      runId?: string;
      status?: string;
      cursor: number;
    }
  | { type: "error"; error: AgentError; cursor: number };
```

- [ ] **Step 5: Extend the reducer**

In `src/models/agentStreamReducer.ts`:

```ts
    case "start":
      return invalid();
    case "start-step":
    case "reasoning-start":
    case "reasoning-end":
    case "text-start":
    case "text-end":
    case "finish-step":
      return [{ state: nextState(state.phase), events: [] }, null];
    case "reasoning-delta":
      return [
        {
          state: nextState("streaming"),
          events: [
            {
              type: "reasoning-delta",
              id: part.id,
              delta: part.delta,
              cursor: frame.cursor,
            },
          ],
        },
        null,
      ];
    case "text-delta":
```

- [ ] **Step 6: Run the tests**

Run: `pnpm vitest run tests/e2e/unit/agent/agentStreamSchema.test.ts tests/e2e/unit/agent/agentStreamReducer.test.ts tests/e2e/unit/agent/agentStreamProtocol.test.ts`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/models/agentContracts.ts src/models/agentStreamSchema.ts src/models/agentStreamReducer.ts tests/e2e/unit/agent/agentStreamSchema.test.ts tests/e2e/unit/agent/agentStreamReducer.test.ts
git commit -m "feat(agent): parse reasoning stream chunks on the client"
```

---

### Task 5: Materialize And Recover The Reasoning Part

**Files:**
- Modify: `src/models/agentRunMaterializer.ts`
- Modify: `src/models/agentRunClient.ts`
- Modify: `src/models/agentTransport.ts`
- Modify: `src/models/apiAgentModel.ts`
- Modify: `src/models/fakeAgentModel.ts`
- Test: `tests/e2e/unit/agent/agentRunMaterializer.test.ts`
- Test: `tests/e2e/unit/agent/agentRunClient.test.ts`
- Test: `tests/e2e/unit/agent/fakeAgentModel.test.ts`

**Interfaces:**
- Consumes: Task 4 `AgentReasoningPart` and `reasoning-delta` events.
- Produces: `RunMaterializer.reasoningDelta(text, cursor)`; `createAgentRunMaterializer({ ..., reasoningPartId })`; `runHandleSchema` accepts `reasoningPartId`; `createFakeAgentModel({ ..., reasoningChunks })`.

- [ ] **Step 1: Write the failing materializer test**

Create `tests/e2e/unit/agent/agentRunMaterializer.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { createAgentRunMaterializer } from "@/models/agentRunMaterializer";

const handle = {
  runId: "run-1",
  sessionId: "session-1",
  userMessageId: "user-1",
  assistantMessageId: "assistant-1",
  assistantPartId: "text-1",
  reasoningPartId: "reasoning-1",
  status: "running" as const,
};

function materializer() {
  return createAgentRunMaterializer({
    sessionId: "session-1",
    runId: "run-1",
    userMessageId: "user-1",
    assistantMessageId: "assistant-1",
    assistantPartId: "text-1",
    reasoningPartId: "reasoning-1",
  });
}

describe("AgentRunMaterializer reasoning", () => {
  it("keeps reasoning and text in separate ordered parts", () => {
    const run = materializer();
    run.start(handle, "2026-11-05T00:00:00.000Z");
    run.reasoningDelta("先分析", 3);
    const snapshot = run.delta("最终回答", 4);

    expect(snapshot.parts).toHaveLength(2);
    expect(snapshot.parts[0]).toMatchObject({
      id: "reasoning-1",
      index: 0,
      type: "reasoning",
      content: { type: "reasoning", text: "先分析" },
    });
    expect(snapshot.parts[1]).toMatchObject({
      id: "text-1",
      index: 1,
      type: "text",
      content: { type: "text", text: "最终回答" },
    });
  });

  it("marks both parts terminal when the run finishes", () => {
    const run = materializer();
    run.start(handle, "2026-11-05T00:00:00.000Z");
    run.reasoningDelta("推理", 3);
    const snapshot = run.finish("completed", "2026-11-05T00:00:02.000Z");

    expect(snapshot.message.status).toBe("complete");
    for (const part of snapshot.parts) {
      expect(part.status).toBe("complete");
    }
  });

  it("restores both parts from an existing snapshot", () => {
    const run = createAgentRunMaterializer({
      sessionId: "session-1",
      runId: "run-1",
      userMessageId: "user-1",
      assistantMessageId: "assistant-1",
      assistantPartId: "text-1",
      reasoningPartId: "reasoning-1",
      initialSnapshot: {
        message: {
          id: "assistant-1",
          sessionId: "session-1",
          seq: 2,
          role: "assistant",
          status: "streaming",
          activeRunId: "run-1",
          userMessageId: "user-1",
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:01.000Z",
        },
        parts: [
          {
            id: "reasoning-1",
            messageId: "assistant-1",
            runId: "run-1",
            index: 0,
            type: "reasoning",
            status: "streaming",
            content: { type: "reasoning", text: "已恢复推理" },
            createdAt: "2026-11-05T00:00:00.000Z",
            updatedAt: "2026-11-05T00:00:01.000Z",
          },
          {
            id: "text-1",
            messageId: "assistant-1",
            runId: "run-1",
            index: 1,
            type: "text",
            status: "streaming",
            content: { type: "text", text: "" },
            createdAt: "2026-11-05T00:00:00.000Z",
            updatedAt: "2026-11-05T00:00:01.000Z",
          },
        ],
      },
    });

    const snapshot = run.reasoningDelta("，继续", 5);

    expect(snapshot.parts[0]).toMatchObject({
      id: "reasoning-1",
      content: { type: "reasoning", text: "已恢复推理，继续" },
    });
  });
});
```

- [ ] **Step 2: Write the failing run-client test**

Append to `tests/e2e/unit/agent/agentRunClient.test.ts`, using the existing helpers:

```ts
  it("forwards reasoning deltas to the reasoning part only", async () => {
    const snapshots: Array<{ reasoning: string; text: string }> = [];
    const input = baseInput(() =>
      Promise.resolve(
        streamResponse([
          startFrame(1),
          `id: 2\ndata: ${JSON.stringify({
            type: "reasoning-start",
            id: "reasoning-1",
          })}\n\n`,
          `id: 3\ndata: ${JSON.stringify({
            type: "reasoning-delta",
            id: "reasoning-1",
            delta: "先分析",
          })}\n\n`,
          `id: 4\ndata: ${JSON.stringify({
            type: "reasoning-end",
            id: "reasoning-1",
          })}\n\n`,
          `id: 5\ndata: ${JSON.stringify({
            type: "text-start",
            id: "part-1",
          })}\n\n`,
          textFrame(6, "最终回答"),
          finishFrame(7),
          "data: [DONE]\n\n",
        ]),
      ),
    );
    input.materializer = createAgentRunMaterializer({
      sessionId: "session-1",
      runId: "run-1",
      userMessageId: "user-1",
      assistantMessageId: "assistant-1",
      assistantPartId: "part-1",
      reasoningPartId: "reasoning-1",
    });
    input.handlers.onSnapshot = (snapshot) => {
      snapshots.push({
        reasoning: snapshot.parts
          .filter((part) => part.type === "reasoning")
          .map((part) => part.content.text)
          .join(""),
        text: snapshot.parts
          .filter((part) => part.type === "text")
          .map((part) => part.content.text)
          .join(""),
      });
    };

    const [, error] = await consumeAgentRun(input);

    expect(error).toBeNull();
    expect(snapshots.at(-1)).toEqual({
      reasoning: "先分析",
      text: "最终回答",
    });
  });
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `pnpm vitest run tests/e2e/unit/agent/agentRunMaterializer.test.ts tests/e2e/unit/agent/agentRunClient.test.ts`
Expected: FAIL, `reasoningDelta` is not a function and no reasoning Part exists.

- [ ] **Step 4: Add the reasoning Part to the Materializer**

In `src/models/agentRunMaterializer.ts`:

```ts
export type RunMaterializer = {
  start: (handle: AgentRunHandle, timestamp: string) => AgentMessageSnapshot;
  reasoningDelta: (text: string, cursor: number) => AgentMessageSnapshot;
  delta: (text: string, cursor: number) => AgentMessageSnapshot;
  finish: (status: AgentRunStatus, timestamp: string) => AgentMessageSnapshot;
  error: (error: AgentError, timestamp: string) => AgentMessageSnapshot;
};

type MaterializerInput = {
  sessionId: string;
  runId: string;
  userMessageId: string;
  assistantMessageId: string;
  assistantPartId: string;
  reasoningPartId?: string;
  initialSnapshot?: AgentMessageSnapshot;
};
```

Replace the part initialisation with:

```ts
  let reasoningPart = initialParts?.find(
    (part): part is AgentReasoningPart => part.type === "reasoning",
  );
  let textPart = initialParts?.find(
    (part): part is Extract<AgentMessagePart, { type: "text" }> =>
      part.type === "text",
  );
  let errorPart = initialParts?.find(
    (part): part is Extract<AgentMessagePart, { type: "error" }> =>
      part.type === "error",
  );

  if (!reasoningPart && input.reasoningPartId) {
    reasoningPart = {
      id: input.reasoningPartId,
      messageId: input.assistantMessageId,
      runId: input.runId,
      index: 0,
      type: "reasoning",
      status: "streaming",
      content: { type: "reasoning", text: "" },
      createdAt: timestamp,
      updatedAt: timestamp,
    };
  }

  if (!textPart) {
    textPart = {
      id: input.assistantPartId,
      messageId: input.assistantMessageId,
      runId: input.runId,
      index: reasoningPart ? 1 : 0,
      type: "text",
      status: "streaming",
      content: { type: "text", text: "" },
      createdAt: timestamp,
      updatedAt: timestamp,
    };
  }
```

Replace `snapshot` and `setTerminalStatus` with:

```ts
  const snapshot = (): AgentMessageSnapshot => ({
    message: { ...message },
    parts: [
      ...(reasoningPart ? [clonePart(reasoningPart)] : []),
      clonePart(textPart!),
      ...(errorPart ? [clonePart(errorPart)] : []),
    ],
  });

  const setTerminalStatus = (
    status: AgentRunStatus,
    updatedAt: string,
  ) => {
    const terminalStatus = messageStatus(status);
    message = {
      ...message,
      status: terminalStatus,
      updatedAt,
    };
    textPart = { ...textPart!, status: terminalStatus, updatedAt };
    if (reasoningPart) {
      reasoningPart = { ...reasoningPart, status: terminalStatus, updatedAt };
    }
    if (errorPart) {
      errorPart = {
        ...errorPart,
        status: terminalStatus,
        updatedAt,
      };
    }
  };
```

In `start`, rebuild both parts:

```ts
    start(handle, startedAt) {
      message = {
        id: handle.assistantMessageId,
        sessionId: handle.sessionId,
        seq: message.seq,
        role: "assistant",
        status: "streaming",
        activeRunId: handle.runId,
        userMessageId: handle.userMessageId,
        createdAt: message.createdAt,
        updatedAt: startedAt,
      };
      reasoningPart = handle.reasoningPartId
        ? {
            id: handle.reasoningPartId,
            messageId: handle.assistantMessageId,
            runId: handle.runId,
            index: 0,
            type: "reasoning",
            status: "streaming",
            content: { type: "reasoning", text: "" },
            createdAt: startedAt,
            updatedAt: startedAt,
          }
        : undefined;
      textPart = {
        id: handle.assistantPartId,
        messageId: handle.assistantMessageId,
        runId: handle.runId,
        index: reasoningPart ? 1 : 0,
        type: "text",
        status: "streaming",
        content: { type: "text", text: "" },
        createdAt: startedAt,
        updatedAt: startedAt,
      };
      errorPart = undefined;
      return snapshot();
    },

    reasoningDelta(text, _cursor) {
      if (!reasoningPart) {
        return snapshot();
      }
      const updatedAt = new Date().toISOString();
      reasoningPart = {
        ...reasoningPart,
        content: {
          type: "reasoning",
          text: reasoningPart.content.text + text,
        },
        updatedAt,
      };
      message = { ...message, status: "streaming", updatedAt };
      return snapshot();
    },
```

Also change the error Part index to stay above both content parts:

```ts
        index: (textPart?.index ?? 0) + 1,
```

(unchanged expression still yields `2` when text is index `1`.)

Add `AgentReasoningPart` to the imports.

- [ ] **Step 5: Route reasoning events and handles**

In `src/models/agentRunClient.ts`:

```ts
        if (event.type === "reasoning-delta") {
          input.handlers.onSnapshot(
            input.materializer.reasoningDelta(event.delta, event.cursor),
          );
          continue;
        }
```

placed before the `text-delta` branch.

In `src/models/agentTransport.ts`, extend `runHandleSchema`:

```ts
const runHandleSchema = z.object({
  runId: z.string().min(1),
  sessionId: z.string().min(1),
  userMessageId: z.string().min(1),
  assistantMessageId: z.string().min(1),
  assistantPartId: z.string().min(1),
  reasoningPartId: z.string().min(1).optional(),
  status: z.enum(["running", "completed", "failed", "cancelled", "interrupted"]),
});
```

In `src/models/apiAgentModel.ts`:

```ts
function materializerFromHandle(handle: AgentRunHandle): RunMaterializer {
  return createAgentRunMaterializer({
    sessionId: handle.sessionId,
    runId: handle.runId,
    userMessageId: handle.userMessageId,
    assistantMessageId: handle.assistantMessageId,
    assistantPartId: handle.assistantPartId,
    reasoningPartId: handle.reasoningPartId,
  });
}

function materializerFromSnapshot(
  runId: string,
  snapshot: AgentMessageSnapshot,
): RunMaterializer {
  const textPart = snapshot.parts.find((part) => part.type === "text");
  const reasoningPart = snapshot.parts.find(
    (part) => part.type === "reasoning",
  );
  return createAgentRunMaterializer({
    sessionId: snapshot.message.sessionId,
    runId,
    userMessageId: snapshot.message.userMessageId ?? "",
    assistantMessageId: snapshot.message.id,
    assistantPartId: textPart?.id ?? "",
    reasoningPartId: reasoningPart?.id,
    initialSnapshot: snapshot,
  });
}
```

- [ ] **Step 6: Teach the fake model to stream reasoning**

In `src/models/fakeAgentModel.ts`, extend the options:

```ts
type FakeAgentModelOptions = {
  chunkDelayMs?: number;
  responseChunks?: (
    input: AgentSendInput,
    history: Array<{ role: "user" | "assistant"; content: string }>,
    signal: AbortSignal,
    runtime: {
      assistantMessageId: string;
      assistantPartId: string;
      reasoningPartId: string;
      runId: string;
    },
  ) => AsyncIterable<string>;
  reasoningChunks?: (
    input: AgentSendInput,
    runtime: {
      assistantMessageId: string;
      reasoningPartId: string;
      runId: string;
    },
  ) => AsyncIterable<string>;
};
```

In `runAssistant`, create the reasoning Part before the text Part:

```ts
    const reasoningPart: AgentReasoningPart | null = options.reasoningChunks
      ? {
          id: createId(),
          messageId: assistantMessage.id,
          runId,
          index: 0,
          type: "reasoning",
          status: "streaming",
          content: { type: "reasoning", text: "" },
          createdAt: timestamp,
          updatedAt: timestamp,
        }
      : null;
    const assistantPart: AgentTextPart = {
      id: createId(),
      messageId: assistantMessage.id,
      runId,
      index: reasoningPart ? 1 : 0,
      type: "text",
      status: "streaming",
      content: { type: "text", text: "" },
      createdAt: timestamp,
      updatedAt: timestamp,
    };
```

with imports:

```ts
import type {
  AgentError,
  AgentMessage,
  AgentMessagePart,
  AgentReasoningPart,
  AgentSendHandlers,
  AgentSendResult,
  AgentRunStatus,
  AgentSendInput,
  AgentSession,
  AgentSessionDetail,
  AgentTextPart,
  Result,
} from "@/models/agentContracts";
```

store both parts:

```ts
    store.partsByMessageId.set(assistantMessage.id, [
      ...(store.partsByMessageId.get(assistantMessage.id) ?? []),
      ...(reasoningPart ? [reasoningPart] : []),
      assistantPart,
    ]);
```

snapshot helper and reasoning stream before text:

```ts
    const currentParts = () =>
      [reasoningPart, assistantPart]
        .filter((part): part is AgentMessagePart => part !== null)
        .map(clonePart);
```

```ts
    let reasoningText = "";
    try {
      if (reasoningPart && options.reasoningChunks) {
        const reasoningRuntime = {
          assistantMessageId: assistantMessage.id,
          reasoningPartId: reasoningPart.id,
          runId,
        };
        for await (const chunk of options.reasoningChunks(
          requestInput,
          reasoningRuntime,
        )) {
          if (run.abortController.signal.aborted) {
            break;
          }
          reasoningText += chunk;
          reasoningPart.content = { type: "reasoning", text: reasoningText };
          reasoningPart.updatedAt = now();
          assistantMessage.updatedAt = reasoningPart.updatedAt;
          handlers.onSnapshot({
            message: cloneMessage(assistantMessage),
            parts: currentParts(),
          });
        }
      }
```

Replace the existing text-stream `handlers.onSnapshot({ ..., parts: [clonePart(assistantPart)] })` with
`parts: currentParts()`, and replace the terminal snapshot `finalParts` filter with:

```ts
    const finalParts = (
      store.partsByMessageId.get(assistantMessage.id) ?? []
    )
      .filter((part) => !part.runId || part.runId === runId)
      .filter((part) => part.type !== "reasoning" || reasoningText !== "")
      .map(clonePart);
```

also mark the reasoning Part terminal in the terminal block:

```ts
    if (reasoningPart) {
      reasoningPart.status = terminalPartStatus;
      reasoningPart.updatedAt = finishedAt;
    }
```

and pass `reasoningPartId: reasoningPart?.id ?? ""` into the `responseChunks` runtime.

Append to `tests/e2e/unit/agent/fakeAgentModel.test.ts`:

```ts
  it("streams reasoning before text when reasoning chunks are configured", async () => {
    const model = createFakeAgentModel({
      chunkDelayMs: 0,
      reasoningChunks: async function* () {
        yield "先分析";
      },
    });
    const [created] = await model.createSession({ clientRequestId: "create-1" });
    const parts: string[] = [];

    const [result, error] = await model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "user-message-1",
          role: "user",
          parts: [{ type: "text", text: "解释一下" }],
        },
        signal: new AbortController().signal,
      },
      {
        onStarted: () => undefined,
        onSnapshot: (snapshot) => {
          parts.push(
            snapshot.parts
              .map((part) =>
                part.type === "reasoning" || part.type === "text"
                  ? `${part.type}:${part.content.text}`
                  : part.type,
              )
              .join("|"),
          );
        },
      },
    );

    expect(error).toBeNull();
    expect(result?.status).toBe("completed");
    expect(parts.some((value) => value.includes("reasoning:先分析"))).toBe(true);

    const [detail] = await model.getSession(created!.session.id);
    const assistant = detail!.messages.at(-1)!;
    const assistantParts = detail!.messageParts.filter(
      (part) => part.messageId === assistant.id,
    );
    expect(assistantParts.map((part) => part.type)).toEqual([
      "reasoning",
      "text",
    ]);
  });
```

- [ ] **Step 7: Run the tests**

Run: `pnpm vitest run tests/e2e/unit/agent`
Expected: PASS.

Run: `pnpm typecheck`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add src/models tests/e2e/unit/agent
git commit -m "feat(agent): materialize reasoning parts on the client"
```

---

### Task 6: Render The Collapsible Reasoning Panel

**Files:**
- Create: `src/components/agent/agentReasoningPanel.tsx`
- Modify: `src/components/agent/agentMessageList.tsx`
- Test: `tests/e2e/unit/agent/agentReasoningPanel.test.tsx`
- Test: `tests/e2e/unit/agent/agentMessageList.test.tsx`

**Interfaces:**
- Consumes: `AgentReasoningPart` from Task 4 and Task 5 snapshots.
- Produces: `AgentReasoningPanel({ part, textStarted })`.

- [ ] **Step 1: Write the failing panel tests**

Create `tests/e2e/unit/agent/agentReasoningPanel.test.tsx`:

```tsx
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AgentReasoningPanel } from "@/components/agent/agentReasoningPanel";
import type { AgentReasoningPart } from "@/models/agentContracts";

function reasoningPart(
  text: string,
  status: AgentReasoningPart["status"] = "streaming",
): AgentReasoningPart {
  return {
    id: "reasoning-1",
    messageId: "assistant-1",
    runId: "run-1",
    index: 0,
    type: "reasoning",
    status,
    content: { type: "reasoning", text },
    createdAt: "2026-11-05T00:00:00.000Z",
    updatedAt: "2026-11-05T00:00:00.000Z",
  };
}

describe("AgentReasoningPanel", () => {
  it("renders nothing before reasoning content arrives", () => {
    render(<AgentReasoningPanel part={reasoningPart("")} textStarted={false} />);

    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("expands while reasoning streams", () => {
    const { rerender } = render(
      <AgentReasoningPanel part={reasoningPart("")} textStarted={false} />,
    );
    rerender(
      <AgentReasoningPanel part={reasoningPart("先分析")} textStarted={false} />,
    );

    expect(screen.getByRole("button", { name: "思考中..." })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getByText("先分析")).toBeVisible();
  });

  it("collapses automatically when the answer starts", () => {
    const { rerender } = render(
      <AgentReasoningPanel part={reasoningPart("")} textStarted={false} />,
    );
    rerender(
      <AgentReasoningPanel part={reasoningPart("先分析")} textStarted={false} />,
    );
    rerender(
      <AgentReasoningPanel part={reasoningPart("先分析")} textStarted />,
    );

    expect(screen.getByRole("button", { name: "思考中..." })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(screen.queryByText("先分析")).not.toBeInTheDocument();
  });

  it("keeps a manual expansion after the answer starts", () => {
    const { rerender } = render(
      <AgentReasoningPanel part={reasoningPart("")} textStarted={false} />,
    );
    rerender(
      <AgentReasoningPanel part={reasoningPart("先分析")} textStarted={false} />,
    );
    const toggle = screen.getByRole("button", { name: "思考中..." });
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");

    rerender(
      <AgentReasoningPanel part={reasoningPart("先分析")} textStarted />,
    );

    expect(screen.getByRole("button", { name: "思考中..." })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("starts collapsed when restored content is already present", () => {
    render(
      <AgentReasoningPanel
        part={reasoningPart("旧推理", "complete")}
        textStarted
      />,
    );

    expect(screen.getByRole("button", { name: "思考中..." })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });
});
```

Append to `tests/e2e/unit/agent/agentMessageList.test.tsx`:

```tsx
  it("announces thinking until the first text token arrives", () => {
    useAgentStore.getState().setSessionMessages(
      "session-1",
      [
        {
          id: "assistant-1",
          sessionId: "session-1",
          seq: 1,
          role: "assistant",
          status: "streaming",
          activeRunId: "run-1",
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:00.000Z",
        },
      ],
      [
        {
          id: "reasoning-1",
          messageId: "assistant-1",
          runId: "run-1",
          index: 0,
          type: "reasoning",
          status: "streaming",
          content: { type: "reasoning", text: "先分析" },
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:00.000Z",
        },
        {
          id: "text-1",
          messageId: "assistant-1",
          runId: "run-1",
          index: 1,
          type: "text",
          status: "streaming",
          content: { type: "text", text: "" },
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:00.000Z",
        },
      ],
    );

    render(<AgentMessageList sessionId="session-1" />);

    expect(screen.getByRole("status")).toHaveTextContent("Agent 正在思考");
    expect(screen.getByRole("button", { name: "思考中..." })).toBeVisible();
  });
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm vitest run tests/e2e/unit/agent/agentReasoningPanel.test.tsx tests/e2e/unit/agent/agentMessageList.test.tsx`
Expected: FAIL, `@/components/agent/agentReasoningPanel` does not exist.

- [ ] **Step 3: Create the panel component**

Create `src/components/agent/agentReasoningPanel.tsx`:

```tsx
import { useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import type { AgentReasoningPart } from "@/models/agentContracts";

export function AgentReasoningPanel({
  part,
  textStarted,
}: {
  part: AgentReasoningPart;
  textStarted: boolean;
}) {
  const [expanded, setExpanded] = useState(
    () => part.content.text.length === 0,
  );
  const userToggled = useRef(false);
  const contentRef = useRef<HTMLDivElement>(null);
  const followTail = useRef(true);

  useEffect(() => {
    if (userToggled.current || !textStarted) {
      return;
    }
    setExpanded(false);
  }, [textStarted]);

  useEffect(() => {
    const node = contentRef.current;
    if (!node || !expanded || !followTail.current) {
      return;
    }
    node.scrollTop = node.scrollHeight;
  }, [part.content.text, expanded]);

  if (part.content.text.length === 0) {
    return null;
  }

  const panelId = `reasoning-panel-${part.id}`;
  return (
    <div className="space-y-1">
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={panelId}
        onClick={() => {
          userToggled.current = true;
          setExpanded((current) => !current);
        }}
        className="flex items-center gap-1 rounded text-xs font-medium text-textSecondary transition-colors hover:text-textPrimary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40"
      >
        {expanded ? (
          <ChevronDown aria-hidden="true" size={13} />
        ) : (
          <ChevronRight aria-hidden="true" size={13} />
        )}
        <span>思考中...</span>
      </button>
      {expanded && (
        <div
          id={panelId}
          ref={contentRef}
          onScroll={(event) => {
            const node = event.currentTarget;
            followTail.current =
              node.scrollHeight - node.scrollTop - node.clientHeight < 8;
          }}
          className="max-h-[40vh] overflow-y-auto whitespace-pre-wrap break-words border-l border-border pl-2.5 text-xs leading-5 text-textSecondary"
        >
          {part.content.text}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Wire the panel into the message list**

In `src/components/agent/agentMessageList.tsx`, import:

```tsx
import { AgentReasoningPanel } from "@/components/agent/agentReasoningPanel";
import type {
  AgentMessage,
  AgentMessagePart,
  AgentReasoningPart,
} from "@/models/agentContracts";
```

Rename `runAnnouncement` to take the parts map:

```tsx
function runAnnouncement(
  messages: AgentMessage[],
  partsByMessageId: Record<string, AgentMessagePart[]>,
) {
  const streaming = messages.find(
    (message) => message.role === "assistant" && message.status === "streaming",
  );
  if (streaming) {
    const hasText = (partsByMessageId[streaming.id] ?? []).some(
      (part) => part.type === "text" && part.content.text.length > 0,
    );
    return hasText ? "Agent 正在回复" : "Agent 正在思考";
  }

  let lastAssistant: AgentMessage | undefined;
  for (const message of messages) {
    if (message.role === "assistant") {
      lastAssistant = message;
    }
  }
  if (lastAssistant?.status === "failed") {
    return "Agent 回复失败";
  }
  if (
    lastAssistant?.status === "cancelled" ||
    lastAssistant?.status === "interrupted"
  ) {
    return "Agent 回复已停止";
  }
  return "Agent 回复完成";
}
```

Replace the body render block with:

```tsx
  const reasoningParts = visibleParts.filter(
    (part): part is AgentReasoningPart => part.type === "reasoning",
  );
  const textStarted = visibleParts.some(
    (part) => part.type === "text" && part.content.text.length > 0,
  );

  if (visibleParts.length === 0) {
    return (
      <p className="animate-pulse text-sm leading-6 text-textSecondary motion-reduce:animate-none">
        正在思考…
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {reasoningParts.map((part) => (
        <AgentReasoningPanel
          key={part.id}
          part={part}
          textStarted={textStarted}
        />
      ))}
      {visibleParts.map((part) =>
        part.type === "error" ? (
          <div
            key={part.id}
            role="alert"
            className="flex items-start gap-2 text-sm leading-6 text-destructive"
          >
            <AlertCircle
              aria-hidden="true"
              size={15}
              className="mt-1 shrink-0"
            />
            <span className="min-w-0 break-words">
              {part.content.error.message}
            </span>
          </div>
        ) : part.type === "reasoning" ? null : (
          <p
            key={part.id}
            className="whitespace-pre-wrap break-words text-sm leading-6"
          >
            {part.content.text}
            {part.status === "streaming" && (
              <span
                aria-hidden="true"
                className="ml-0.5 inline-block h-4 w-0.5 animate-pulse bg-brand align-middle"
              />
            )}
          </p>
        ),
      )}
    </div>
  );
```

and update the status paragraph:

```tsx
      <p role="status" aria-atomic="true" className="sr-only">
        {runAnnouncement(messages, partsByMessageId)}
      </p>
```

- [ ] **Step 5: Run the tests**

Run: `pnpm vitest run tests/e2e/unit/agent`
Expected: PASS.

Run: `pnpm typecheck`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/components/agent tests/e2e/unit/agent
git commit -m "feat(agent): show reasoning in a collapsible panel"
```

---

### Task 7: Browser Regression For The Reasoning Panel

**Files:**
- Modify: `scripts/fakeAgentProvider.mjs`
- Create: `tests/e2e/modules/agent/reasoning.e2e.ts`

**Interfaces:**
- Consumes: the fake OpenAI-compatible provider used by `.env.e2e` and the real Go+SQLite pipeline.
- Produces: browser coverage for expand, auto-collapse, manual toggle and reload.

- [ ] **Step 1: Write the failing browser test**

Create `tests/e2e/modules/agent/reasoning.e2e.ts`:

```ts
import { expect, test } from "@playwright/test";

test("keeps reasoning above the answer across reloads", async ({ page }) => {
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  await expect(page).toHaveURL(/#\/agent\/.+/);

  const composer = page.getByRole("textbox", { name: "发送给 Agent" });
  await composer.fill("整理今天的重点");
  await page.getByRole("button", { name: "发送" }).click();

  const toggle = page.getByRole("button", { name: "思考中..." });
  await expect(toggle).toBeVisible();
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByText("先确认用户意图", { exact: false })).toBeVisible();

  const messages = page.getByRole("log", { name: "会话消息" });
  await expect(messages.getByText(/已收到：整理今天的重点/)).toBeVisible();
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByText("先确认用户意图", { exact: false })).toHaveCount(0);

  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByText("先确认用户意图", { exact: false })).toBeVisible();

  await page.reload();
  const restored = page.getByRole("button", { name: "思考中..." });
  await expect(restored).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByText("先确认用户意图", { exact: false })).toHaveCount(0);
  await restored.click();
  await expect(page.getByText("先确认用户意图", { exact: false })).toBeVisible();
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm playwright test tests/e2e/modules/agent/reasoning.e2e.ts`
Expected: FAIL, the fake provider never sends `reasoning_content` and no panel appears.

- [ ] **Step 3: Stream reasoning from the fake provider**

Replace the helpers in `scripts/fakeAgentProvider.mjs` with:

```js
function chunk(delta) {
  return `data: ${JSON.stringify({ choices: [{ delta }] })}\n\n`;
}

function writeDelta(response, delta) {
  if (response.writableEnded || response.destroyed) {
    return false;
  }
  response.write(chunk(delta));
  return true;
}

function writeReasoning(response, content) {
  return writeDelta(response, { reasoning_content: content });
}

function writeText(response, content) {
  return writeDelta(response, { content });
}
```

and replace the default response path with:

```js
    writeReasoning(response, "先确认用户意图，");
    setTimeout(() => {
      writeReasoning(response, "再组织最终回答。");
    }, 700);
    setTimeout(() => {
      writeText(response, `已收到：${prompt}`);
      setTimeout(() => {
        writeText(response, "\n\n处理完成。");
        finish(response);
      }, 700);
    }, 1_500);
```

Keep the `慢速恢复测试` branch, but rename its calls from `writeChunk` to `writeText`.

- [ ] **Step 4: Run the browser test**

Run: `pnpm playwright test tests/e2e/modules/agent/reasoning.e2e.ts`
Expected: PASS for both `desktop` and `mobile` projects.

Run: `pnpm playwright test tests/e2e/modules/agent`
Expected: PASS (existing chat and navigation flows still green).

- [ ] **Step 5: Commit**

```bash
git add scripts/fakeAgentProvider.mjs tests/e2e/modules/agent/reasoning.e2e.ts
git commit -m "test(agent): cover the reasoning panel in the browser"
```

---

### Task 8: Record The Reasoning Implementation

**Files:**
- Modify: `docs/superpowers/specs/2026-11-05-craftingtable-agent-reasoning-display-design.md`
- Create: `docs/handoffs/2026-11-05-craftingtable-agent-reasoning-handoff.md`
- Modify: `README.md` only if it lists Agent capabilities explicitly.

**Interfaces:**
- Consumes: completed Tasks 1-7.
- Produces: reviewed implementation notes and known limits.

- [ ] **Step 1: Update the design status**

Change the design header to:

```markdown
- **Status:** Implemented on 2026-11-05
- **Date:** 2026-11-05
```

- [ ] **Step 2: Write the handoff document**

Create `docs/handoffs/2026-11-05-craftingtable-agent-reasoning-handoff.md`:

````markdown
# CraftingTable Agent Reasoning 交接文档

日期：2026-11-05
分支：`leeatMvp`
状态：reasoning 流式接收、持久化、恢复和折叠展示已完成

## 1. 结论

- MiMo 的 `delta.reasoning_content` 与 `delta.content` 现在分别写入两个 MessagePart。
- Part 顺序固定为 `reasoning: 0`、`text: 1`、`error` 从 2 开始。
- 推理阶段默认展开，首个正文增量到达后自动折叠。
- 用户手动切换后，自动规则不再覆盖。
- 刷新或重新打开会话后，reasoning 保留但默认折叠。
- 没有 reasoning 的 Run 在 Finalize 时删除空 reasoning Part，不显示空面板。
- reasoning 不进入下一轮模型上下文。

## 2. 协议

```text
MiMo delta.reasoning_content
  -> Go "reasoning" provider event
  -> StreamReasoningStart / StreamReasoningDelta / StreamReasoningEnd
  -> UI Message Stream reasoning-start / reasoning-delta / reasoning-end
  -> 前端 reasoning MessagePart
```

事件顺序为 `start`、`start-step`、`reasoning-*`、`text-*`、`finish-step`、`finish`；
没有 reasoning 时跳过 `reasoning-*`。

## 3. 存储

- `003_agent_reasoning.sql` 重建 `agent_message_parts`，类型约束扩展为 `text`、`reasoning`、`error`。
- Finalize 在同一个事务中写入 reasoning 与正文，空 reasoning 会被删除。
- Interrupt 与启动恢复会把 reasoning 状态同步为 `interrupted`。
- 未持久化的 reasoning 不提供强于正文的恢复能力。

## 4. 前端

- `agentStreamSchema` 识别三个 reasoning 片段。
- `agentRunMaterializer` 独立维护 reasoning Part 与 text Part。
- `agentReasoningPanel` 负责折叠、滚动态和可访问性。
- 标题固定为 `思考中...`，纯文本展示，不做 Markdown 渲染。

## 5. 测试位置

- `tests/e2e/integration/agent_reasoning.integration_test.go`
- `tests/e2e/integration/agent_reasoning_stream.integration_test.go`
- `tests/e2e/integration/agent_reasoning_recovery.integration_test.go`
- `tests/e2e/unit/agent/agentRunMaterializer.test.ts`
- `tests/e2e/unit/agent/agentReasoningPanel.test.tsx`
- `tests/e2e/modules/agent/reasoning.e2e.ts`

## 6. 已知限制

- 只识别 `reasoning_content`，未做多供应商适配。
- reasoning 不渲染 Markdown。
- 旧的 `/api/v1/agent/stream` 端点仍忽略 reasoning。
- 服务端进程重启不会继续原 Provider 流。
````

- [ ] **Step 3: Verify the documentation only change**

Run: `git diff --stat`
Expected: only the design status, the new handoff file and (optionally) `README.md` change.

- [ ] **Step 4: Run the full validation suite**

Run: `pnpm typecheck`
Run: `pnpm vitest run`
Run: `pnpm test:go`
Run: `pnpm build`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add docs README.md
git commit -m "docs: record agent reasoning implementation"
```

---

## Self-Review Notes

- Design section 8 (domain/storage) is covered by Task 1.
- Design section 9 (migration) is covered by Task 1 Step 4 and the legacy upgrade test.
- Design sections 10.1-10.4 (parser, chunks, journal, UI stream) are covered by Task 2 Steps 3-7.
- Design section 10.5 (event order) is covered by `TestCoordinatorEmitsReasoningBeforeText`.
- Design section 11 (contracts, schema, reducer, materializer) is covered by Tasks 4 and 5.
- Design section 12 (recovery) is covered by Task 3 and Task 5's snapshot test.
- Design section 13 (UI, accessibility, auto rules) is covered by Task 6 and Task 7.
- Design section 14 (privacy, no reasoning logs) is preserved: no step adds reasoning content to logs.
- Design section 15 (test matrix) is covered across Tasks 1-7.
- Design section 16 (acceptance) is validated by Task 8 Step 4.
