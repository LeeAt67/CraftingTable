# CraftingTable Agent Persistence And Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist Agent sessions and messages in SQLite, own Run execution outside HTTP request lifetimes, and recover partial Agent output after browser refresh or network reconnect.

**Architecture:** SQLite stores Session, Message, Part, Run, Step, lifecycle RunEvent and idempotency facts. A single-process Run Coordinator owns model execution and keeps a bounded in-memory UI chunk journal; SSE reconnect uses a monotonic `after` cursor while the database remains the final content source. React keeps Zustand as the only business state source and turns the existing API model into a session/run HTTP adapter.

**Tech Stack:** Go 1.26, `net/http`, `database/sql`, `modernc.org/sqlite`, React 19, TypeScript, Zustand, Zod, `eventsource-parser`, Vitest, Playwright.

## Global Constraints

- The approved design is `docs/superpowers/specs/2026-11-05-craftingtable-agent-persistence-recovery-design.md`.
- HTTP disconnection must not cancel a Run.
- `text-delta` must not be written to SQLite per token.
- SQLite remains the durable source for Session, Message, Part and Run state.
- The in-memory Run stream journal is a recovery cache, not a durable event source.
- A server restart converts every persisted `running` Run to `interrupted`.
- `after` cursors are monotonic and scoped to one Run.
- Cursor expiry returns `AGENT_STREAM_CURSOR_EXPIRED`; clients then read Session detail.
- Zustand remains the only front-end source for Agent business state.
- All new tests live under `tests/e2e`, grouped by module.
- Do not introduce Redis, Vercel AI SDK, LangChain or a new Agent framework.
- Do not store `AGENT_API_KEY` in source, SQLite, browser state, fixtures or logs.
- Preserve existing Today CRUD behavior and current Agent visual layout.
- Every task starts with a failing test and ends with focused tests plus a commit.

---

## File Responsibility Map

```text
server/internal/agent/domain/
  session.go       Session, Message, Part and their state constants
  run.go           Run, RunHandle, SessionDetail and context messages

server/internal/agent/core/
  repository.go    Persistence Port
  ports.go         Provider and Clock Ports
  service.go       Session and Run use cases
  coordinator.go   Background execution ownership
  event_journal.go Bounded replayable UI chunk journal
  stream_chunk.go  Transport-neutral UI stream chunks

server/internal/agent/access/
  session_controller.go  Session JSON routes
  run_controller.go      Run stream and cancel routes
  ui_stream.go           StreamChunk to UI Message Stream SSE encoding

server/internal/platform/store/
  sqlite_store.go        Database opening and ordered migrations
  agent_store.go         SQLite implementation of core.Repository
  migrations/002_agent.sql

server/cmd/craftingtabled/
  main.go                Composition root and startup recovery

src/models/
  agentContracts.ts      Front-end Agent contracts
  agentTransport.ts      Typed HTTP calls and stream response
  agentRunClient.ts      Run creation, subscription and reconnect
  agentRunMaterializer.ts Protocol event to Message/Part snapshots
  apiAgentModel.ts       AgentModel implementation backed by Go API
  agentModel.ts          AgentModel interface and production selection
  agentStreamSchema.ts   Per-chunk schema
  agentStreamProtocol.ts SSE parsing with cursor output
  agentStreamReducer.ts  Protocol state machine with cursor

src/controllers/
  effects/useAgentEffect.ts          Resume and send orchestration
  stores/agentStore.ts               Store additions only where business data requires it

tests/e2e/
  integration/agent_*.go             Persistence and HTTP recovery
  unit/agent/*.test.ts               Protocol, model and Effect behavior
  modules/agent/*.e2e.ts             Browser regressions and page reload
```

---

## Task 1: Add SQLite Migrations And Agent Persistence

**Files:**
- Create: `server/internal/agent/domain/session.go`
- Create: `server/internal/agent/domain/run.go`
- Create: `server/internal/agent/core/repository.go`
- Create: `server/internal/platform/store/agent_store.go`
- Create: `server/internal/platform/store/migrations/002_agent.sql`
- Modify: `server/internal/platform/store/sqlite_store.go`
- Test: `tests/e2e/integration/agent_persistence.integration_test.go`

**Interfaces:**
- Consumes: existing `store.OpenSQLite(ctx, filename)` and `SQLiteStore.Close()`.
- Produces: `core.Repository`; `domain.Session`, `domain.Message`, `domain.MessagePart`, `domain.Run`, `domain.SessionDetail`; `SQLiteStore.CreateAgentSession`, `ListAgentSessions`, `GetAgentSession`, `CreateAgentSendRun`, `CreateAgentRetryRun`, `FinalizeAgentRun`, `GetAgentRun`, `RecoverInterruptedAgentRuns`.

- [ ] **Step 1: Write the failing persistence test**

Create `tests/e2e/integration/agent_persistence.integration_test.go`:

```go
package integration

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
	"github.com/leeat/craftingtable/server/internal/platform/store"
)

func TestAgentSessionAndCompletedMessageSurviveReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agent-persistence.sqlite")

	first, err := store.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	session, err := first.CreateAgentSession(ctx, store.CreateAgentSessionInput{
		ClientRequestID: "session-request-1",
		RequestHash:     "hash-1",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	execution, err := first.CreateAgentSendRun(ctx, store.CreateAgentSendRunInput{
		SessionID:       session.ID,
		ClientRequestID: "send-request-1",
		RequestHash:     "send-hash-1",
		UserMessageID:   "user-message-1",
		UserParts:       []string{"你好"},
	})
	if err != nil {
		t.Fatalf("create send run: %v", err)
	}
	err = first.FinalizeAgentRun(ctx, store.FinalizeAgentRunInput{
		RunID:              execution.Run.ID,
		Status:             domain.RunCompleted,
		AssistantPartID:    execution.AssistantPartID,
		AssistantText:      "你好，Leeat",
		MessageStatus:      domain.MessageComplete,
		PartStatus:         domain.PartComplete,
		CompletedEventType: "run.completed",
	})
	if err != nil {
		t.Fatalf("finalize run: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	second, err := store.OpenSQLite(ctx, path)
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
		if part.Type == domain.PartText && part.RunID == execution.Run.ID {
			assistantText = part.Text
		}
	}
	if assistantText != "你好，Leeat" {
		t.Fatalf("assistant text = %q", assistantText)
	}
}
```

- [ ] **Step 2: Run the test and verify it fails for missing APIs**

Run:

```powershell
go test ./tests/e2e/integration -run TestAgentSessionAndCompletedMessageSurviveReopen -v
```

Expected: FAIL because `CreateAgentSession`, `CreateAgentSendRun`, `FinalizeAgentRun`, `GetAgentSession` and domain types do not exist.

- [ ] **Step 3: Add the Agent domain types**

Create `server/internal/agent/domain/session.go`:

```go
package domain

type MessageRole string

const (
	MessageUser      MessageRole = "user"
	MessageAssistant MessageRole = "assistant"
)

type MessageStatus string

const (
	MessageStreaming   MessageStatus = "streaming"
	MessageComplete    MessageStatus = "complete"
	MessageFailed      MessageStatus = "failed"
	MessageCancelled   MessageStatus = "cancelled"
	MessageInterrupted MessageStatus = "interrupted"
)

type PartType string

const (
	PartText  PartType = "text"
	PartError PartType = "error"
)

type PartStatus string

const (
	PartStreaming   PartStatus = "streaming"
	PartComplete    PartStatus = "complete"
	PartFailed      PartStatus = "failed"
	PartCancelled   PartStatus = "cancelled"
	PartInterrupted PartStatus = "interrupted"
)

type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Message struct {
	ID            string        `json:"id"`
	SessionID     string        `json:"sessionId"`
	Seq           int           `json:"seq"`
	Role          MessageRole   `json:"role"`
	Status        MessageStatus `json:"status"`
	ActiveRunID   *string       `json:"activeRunId,omitempty"`
	UserMessageID *string       `json:"userMessageId,omitempty"`
	CreatedAt     string        `json:"createdAt"`
	UpdatedAt     string        `json:"updatedAt"`
}

type MessagePart struct {
	ID        string     `json:"id"`
	MessageID string     `json:"messageId"`
	RunID     *string    `json:"runId,omitempty"`
	Index     int        `json:"index"`
	Type      PartType   `json:"type"`
	Status    PartStatus `json:"status"`
	Text      string     `json:"-"`
	Error     *Error     `json:"error,omitempty"`
	CreatedAt string     `json:"createdAt"`
	UpdatedAt string     `json:"updatedAt"`
}
```

Create `server/internal/agent/domain/run.go`:

```go
package domain

type RunStatus string

const (
	RunRunning     RunStatus = "running"
	RunCompleted   RunStatus = "completed"
	RunFailed      RunStatus = "failed"
	RunCancelled   RunStatus = "cancelled"
	RunInterrupted RunStatus = "interrupted"
)

type Run struct {
	ID                 string    `json:"id"`
	SessionID          string    `json:"sessionId"`
	UserMessageID      string    `json:"userMessageId"`
	AssistantMessageID string    `json:"assistantMessageId"`
	Status             RunStatus `json:"status"`
	NextEventSequence  int       `json:"-"`
	CreatedAt          string    `json:"createdAt"`
	UpdatedAt          string    `json:"updatedAt"`
}

type ContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RunExecution struct {
	Run              Run
	AssistantPartID  string
	Messages         []ContextMessage
}

type RunHandle struct {
	RunID              string    `json:"runId"`
	SessionID          string    `json:"sessionId"`
	UserMessageID      string    `json:"userMessageId"`
	AssistantMessageID string    `json:"assistantMessageId"`
	AssistantPartID    string    `json:"assistantPartId"`
	Status             RunStatus `json:"status"`
}

type ActiveRun struct {
	RunID              string    `json:"runId"`
	UserMessageID      string    `json:"userMessageId"`
	AssistantMessageID string    `json:"assistantMessageId"`
	Status             RunStatus `json:"status"`
	LastSequence       int       `json:"lastSequence"`
}

type SessionDetail struct {
	Session           Session       `json:"session"`
	Messages          []Message     `json:"messages"`
	MessageParts      []MessagePart `json:"messageParts"`
	ActiveRun         *ActiveRun    `json:"activeRun"`
	NextMessageCursor *string       `json:"nextMessageCursor"`
}
```

Create `server/internal/agent/core/repository.go`:

```go
package core

import (
	"context"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

type CreateSessionRecord struct {
	ClientRequestID string
	RequestHash     string
}

type CreateSendRunRecord struct {
	SessionID       string
	ClientRequestID string
	RequestHash     string
	UserMessageID   string
	UserParts       []string
}

type CreateRetryRunRecord struct {
	SessionID          string
	AssistantMessageID string
	ClientRequestID    string
	RequestHash        string
}

type FinalizeRunRecord struct {
	RunID              string
	Status             domain.RunStatus
	AssistantPartID    string
	AssistantText      string
	MessageStatus      domain.MessageStatus
	PartStatus         domain.PartStatus
	AgentError         *domain.Error
	TerminalEventType  string
}

type Repository interface {
	CreateAgentSession(context.Context, CreateSessionRecord) (domain.Session, error)
	ListAgentSessions(context.Context) ([]domain.Session, error)
	GetAgentSession(context.Context, string) (domain.SessionDetail, error)
	CreateAgentSendRun(context.Context, CreateSendRunRecord) (domain.RunExecution, error)
	CreateAgentRetryRun(context.Context, CreateRetryRunRecord) (domain.RunExecution, error)
	FinalizeAgentRun(context.Context, FinalizeRunRecord) error
	GetAgentRun(context.Context, string) (domain.Run, error)
	RecoverInterruptedAgentRuns(context.Context) (int, error)
}
```

- [ ] **Step 4: Add ordered migration execution**

Create `server/internal/platform/store/migrations/002_agent.sql` with the seven tables from the approved design. Include these non-negotiable constraints:

```sql
CREATE TABLE IF NOT EXISTS agent_sessions (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  client_request_id TEXT NOT NULL UNIQUE,
  activity_seq INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_messages (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
  status TEXT NOT NULL,
  active_run_id TEXT,
  user_message_id TEXT REFERENCES agent_messages(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(session_id, seq)
);

CREATE TABLE IF NOT EXISTS agent_runs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
  user_message_id TEXT NOT NULL REFERENCES agent_messages(id),
  assistant_message_id TEXT NOT NULL REFERENCES agent_messages(id),
  status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed', 'cancelled', 'interrupted')),
  client_request_id TEXT NOT NULL,
  error_json TEXT,
  next_event_sequence INTEGER NOT NULL DEFAULT 1,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_message_parts (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL REFERENCES agent_messages(id) ON DELETE CASCADE,
  run_id TEXT REFERENCES agent_runs(id),
  part_index INTEGER NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('text', 'error')),
  status TEXT NOT NULL,
  text_content TEXT NOT NULL DEFAULT '',
  error_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_steps (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  step_index INTEGER NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  finished_at TEXT,
  UNIQUE(run_id, step_index)
);

CREATE TABLE IF NOT EXISTS agent_run_events (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  sequence INTEGER NOT NULL,
  event_type TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(run_id, sequence)
);

CREATE TABLE IF NOT EXISTS agent_idempotency_keys (
  operation TEXT NOT NULL,
  client_request_id TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  session_id TEXT,
  run_id TEXT,
  created_at TEXT NOT NULL,
  PRIMARY KEY(operation, client_request_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_one_running_agent_run_per_session
  ON agent_runs(session_id) WHERE status = 'running';
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_message_parts
  ON agent_message_parts(message_id, part_index) WHERE run_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_assistant_run_parts
  ON agent_message_parts(message_id, run_id, part_index) WHERE run_id IS NOT NULL;
```

Modify `server/internal/platform/store/sqlite_store.go`:

```go
//go:embed migrations/*.sql
var migrationFiles embed.FS

func applyMigrations(ctx context.Context, db *sql.DB) error {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}
```

Replace the single embedded migration execution with `applyMigrations(ctx, db)`.

- [ ] **Step 5: Implement the SQLite Agent repository**

Create `server/internal/platform/store/agent_store.go`. Implement a transaction helper:

```go
func (s *SQLiteStore) withTx(
	ctx context.Context,
	fn func(*sql.Tx) error,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent transaction: %w", err)
	}
	return nil
}
```

Implement idempotency lookup in the same transaction that creates Session or Run. The successful `CreateAgentSendRun` transaction must insert in this order:

```text
user message
user text part
running run
run.started event
assistant message with active_run_id
assistant empty text part
step 0
step.started event
message.part.started event
idempotency key
session activity update
```

Use the existing provider request history rules:

```go
func contextMessages(messages []domain.Message, parts []domain.MessagePart) []domain.ContextMessage {
	result := make([]domain.ContextMessage, 0, len(messages))
	for _, message := range messages {
		var text strings.Builder
		for _, part := range parts {
			if part.MessageID == message.ID && part.Type == domain.PartText {
				text.WriteString(part.Text)
			}
		}
		if text.Len() > 0 {
			result = append(result, domain.ContextMessage{
				Role: string(message.Role), Content: text.String(),
			})
		}
	}
	return result
}
```

`GetAgentSession` returns all messages, but only Parts for a user message or the assistant message's current `active_run_id`.

- [ ] **Step 6: Run the focused persistence test**

Run:

```powershell
go test ./tests/e2e/integration -run TestAgentSessionAndCompletedMessageSurviveReopen -v
```

Expected: PASS.

- [ ] **Step 7: Add recovery and idempotency repository tests**

Append tests to `agent_persistence.integration_test.go`:

```go
func TestRecoverInterruptedAgentRunsMarksOnlyRunningRuns(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenSQLite(ctx, filepath.Join(t.TempDir(), "recovery.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	session, _ := db.CreateAgentSession(ctx, store.CreateAgentSessionInput{
		ClientRequestID: "session-1", RequestHash: "hash-1",
	})
	execution, _ := db.CreateAgentSendRun(ctx, store.CreateAgentSendRunInput{
		SessionID: session.ID, ClientRequestID: "send-1", RequestHash: "send-hash-1",
		UserMessageID: "user-1", UserParts: []string{"hello"},
	})
	count, err := db.RecoverInterruptedAgentRuns(ctx)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	run, err := db.GetAgentRun(ctx, execution.Run.ID)
	if err != nil || run.Status != domain.RunInterrupted {
		t.Fatalf("run=%#v err=%v", run, err)
	}
}

func TestDuplicateSendRequestReturnsSameRun(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenSQLite(ctx, filepath.Join(t.TempDir(), "idempotency.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	session, _ := db.CreateAgentSession(ctx, store.CreateAgentSessionInput{
		ClientRequestID: "session-2", RequestHash: "hash-2",
	})
	input := store.CreateAgentSendRunInput{
		SessionID: session.ID, ClientRequestID: "send-2", RequestHash: "send-hash-2",
		UserMessageID: "user-2", UserParts: []string{"duplicate"},
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
}
```

- [ ] **Step 8: Run the full persistence test file**

Run:

```powershell
go test ./tests/e2e/integration -run "TestAgent|TestRecover|TestDuplicate" -v
```

Expected: PASS.

- [ ] **Step 9: Format and commit**

Run:

```powershell
go fmt ./server/...
gofmt -w tests/e2e/integration/agent_persistence.integration_test.go
go test ./tests/e2e/integration -run "TestAgent|TestRecover|TestDuplicate" -v
git add server/internal/agent/domain server/internal/agent/core/repository.go server/internal/platform/store tests/e2e/integration/agent_persistence.integration_test.go
git commit -m "feat: persist agent sessions and messages"
```

---

## Task 2: Add Bounded Run Event Journal

**Files:**
- Create: `server/internal/agent/core/stream_chunk.go`
- Create: `server/internal/agent/core/event_journal.go`
- Test: `tests/e2e/unit/agent/event_journal_test.go`

**Interfaces:**
- Consumes: `domain.RunStatus` and `domain.Error`.
- Produces: `core.StreamChunk`, `core.JournalSnapshot`, `core.NewEventJournal()`, `Journal.Append(StreamChunk) error`, `Journal.Snapshot() JournalSnapshot`, `Journal.Subscribe(after int) (<-chan StreamChunk, func(), error)`, `Journal.Close()`.

- [ ] **Step 1: Write the failing replay test**

Create `tests/e2e/unit/agent/event_journal_test.go`:

```go
package agent_test

import (
	"testing"

	"github.com/leeat/craftingtable/server/internal/agent/core"
)

func TestEventJournalReplaysOnlyEventsAfterCursor(t *testing.T) {
	journal := core.NewEventJournal(1024)
	for _, text := range []string{"你", "好"} {
		if err := journal.Append(core.StreamChunk{
			Kind: core.StreamTextDelta, Text: text,
		}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := journal.Snapshot()
	if len(snapshot.Chunks) != 2 || snapshot.LastSequence != 2 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	events, cancel, err := journal.Subscribe(1)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	replayed := <-events
	if replayed.Sequence != 2 || replayed.Text != "好" {
		t.Fatalf("event = %#v", replayed)
	}
}
```

- [ ] **Step 2: Run test and verify missing package API**

Run:

```powershell
go test ./tests/e2e/unit/agent -run TestEventJournalReplaysOnlyEventsAfterCursor -v
```

Expected: FAIL because `core.NewEventJournal` and `StreamChunk` do not exist.

- [ ] **Step 3: Implement stream chunks and journal**

Create `server/internal/agent/core/stream_chunk.go`:

```go
package core

import "github.com/leeat/craftingtable/server/internal/agent/domain"

type StreamChunkKind string

const (
	StreamStart      StreamChunkKind = "start"
	StreamStartStep  StreamChunkKind = "start-step"
	StreamTextStart  StreamChunkKind = "text-start"
	StreamTextDelta  StreamChunkKind = "text-delta"
	StreamTextEnd    StreamChunkKind = "text-end"
	StreamFinishStep StreamChunkKind = "finish-step"
	StreamFinish     StreamChunkKind = "finish"
	StreamError      StreamChunkKind = "error"
)

type StreamChunk struct {
	Sequence int
	Kind     StreamChunkKind
	RunID    string
	SessionID string
	UserMessageID string
	AssistantMessageID string
	PartID   string
	Text     string
	Status   domain.RunStatus
	Error    *domain.Error
}
```

Create `server/internal/agent/core/event_journal.go`. Allocate sequence inside `Append` under one mutex, estimate size with `len(Text)+128`, and never block the publisher:

```go
func (j *EventJournal) Append(chunk StreamChunk) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return ErrJournalClosed
	}
	size := len(chunk.Text) + 128
	if j.bytes+size > j.maxBytes {
		return ErrJournalFull
	}
	chunk.Sequence = j.nextSequence
	j.nextSequence++
	j.bytes += size
	j.chunks = append(j.chunks, chunk)
	for id, subscriber := range j.subscribers {
		select {
		case subscriber <- chunk:
		default:
			delete(j.subscribers, id)
			close(subscriber)
		}
	}
	return nil
}
```

`Subscribe(after)` validates `after <= lastSequence`, creates a buffered channel, registers it under the mutex, and sends existing chunks after the cursor non-blockingly.

- [ ] **Step 4: Add tests for full journal and slow subscriber**

Append to `event_journal_test.go`:

```go
func TestEventJournalRejectsGrowthPastLimit(t *testing.T) {
	journal := core.NewEventJournal(200)
	if err := journal.Append(core.StreamChunk{
		Kind: core.StreamTextDelta,
		Text: string(make([]byte, 300)),
	}); err == nil {
		t.Fatal("expected journal full error")
	}
}

func TestEventJournalDropsSlowSubscriberWithoutBlocking(t *testing.T) {
	journal := core.NewEventJournal(4096)
	events, cancel, err := journal.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	for index := 0; index <= cap(events); index++ {
		if err := journal.Append(core.StreamChunk{Kind: core.StreamTextDelta, Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := journal.Snapshot()
	if snapshot.LastSequence != cap(events)+1 {
		t.Fatalf("last sequence = %d", snapshot.LastSequence)
	}
}
```

- [ ] **Step 5: Run journal tests**

Run:

```powershell
go test ./tests/e2e/unit/agent -run TestEventJournal -v
```

Expected: PASS.

- [ ] **Step 6: Format and commit**

Run:

```powershell
gofmt -w server/internal/agent/core/event_journal.go server/internal/agent/core/stream_chunk.go tests/e2e/unit/agent/event_journal_test.go
go test ./tests/e2e/unit/agent -run TestEventJournal -v
git add server/internal/agent/core/event_journal.go server/internal/agent/core/stream_chunk.go tests/e2e/unit/agent/event_journal_test.go
git commit -m "feat: add replayable agent run journal"
```

---

## Task 3: Add Agent Service And Run Coordinator

**Files:**
- Create: `server/internal/agent/core/ports.go`
- Create: `server/internal/agent/core/service.go`
- Create: `server/internal/agent/core/coordinator.go`
- Modify: `server/internal/agent/provider/openai_compatible.go`
- Test: `tests/e2e/integration/agent_recovery.integration_test.go`

**Interfaces:**
- Consumes: `core.Repository`, `EventJournal`, existing Provider SSE parser.
- Produces: `core.ProviderMessage`, `core.StreamProvider`, `core.RunSnapshot`, `core.NewService(repository, provider, logger)`, `Service.CreateSession`, `ListSessions`, `GetSession`, `SendMessage`, `RetryMessage`, `CancelRun`, `SubscribeRun`, `Coordinator.Start(domain.RunExecution) error`, `Coordinator.Snapshot(runID) (RunSnapshot, bool)`.

- [ ] **Step 1: Write the failing browser-disconnect test**

Create `tests/e2e/integration/agent_recovery.integration_test.go` with a fake provider that emits two slow chunks:

```go
func TestAgentRunContinuesAfterSubscriberDisconnects(t *testing.T) {
	provider := startFakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"第一段\"}}]}\n\n")
		flusher.Flush()
		time.Sleep(150 * time.Millisecond)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"第二段\"}}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	server := startServer(t, provider.URL+"/v1")
	status, created := requestJSON[struct {
		Data struct {
			Session struct {
				ID string `json:"id"`
			} `json:"session"`
		} `json:"data"`
	}](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/session/create"),
		`{"clientRequestId":"recovery-session-request"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create session status = %d", status)
	}

	status, started := requestJSON[struct {
		Data struct {
			RunID              string `json:"runId"`
			SessionID          string `json:"sessionId"`
			UserMessageID      string `json:"userMessageId"`
			AssistantMessageID string `json:"assistantMessageId"`
			AssistantPartID    string `json:"assistantPartId"`
			Status             string `json:"status"`
		} `json:"data"`
	}](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/message/send"),
		fmt.Sprintf(`{
			"sessionId":%q,
			"clientRequestId":"recovery-send-request",
			"message":{
				"id":"recovery-user-message",
				"role":"user",
				"parts":[{"type":"text","text":"恢复测试"}]
			}
		}`, created.Data.Session.ID),
	)
	if status != http.StatusOK {
		t.Fatalf("start run status = %d", status)
	}

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		server.URL("/api/v1/agent/run/stream?runId="+started.Data.RunID+"&after=0"),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read first stream event: %v", err)
		}
		if strings.Contains(line, `"text-delta"`) {
			break
		}
	}
	cancel()
	_ = response.Body.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, detail := requestJSON[struct {
			Data struct {
				MessageParts []struct {
					RunID string `json:"runId"`
					Text  string `json:"text"`
				} `json:"messageParts"`
				ActiveRun *struct {
					Status string `json:"status"`
				} `json:"activeRun"`
			} `json:"data"`
		}](
			t,
			http.MethodGet,
			server.URL("/api/v1/agent/session/get?sessionId="+created.Data.Session.ID),
			"",
		)
		if status != http.StatusOK {
			t.Fatalf("get session status = %d", status)
		}
		text := ""
		for _, part := range detail.Data.MessageParts {
			if part.RunID == started.Data.RunID {
				text = part.Text
			}
		}
		if text == "第一段第二段" {
			if detail.Data.ActiveRun != nil {
				t.Fatalf("active run = %#v, want null", detail.Data.ActiveRun)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("session detail never reached completed text")
}
```

Add `bufio`, `fmt`, and `strings` to the test file imports.

- [ ] **Step 2: Run test and verify routes are missing**

Run:

```powershell
go test ./tests/e2e/integration -run TestAgentRunContinuesAfterSubscriberDisconnects -v
```

Expected: FAIL with HTTP 404 for the new routes.

- [ ] **Step 3: Add Provider Port and adapt the provider adapter**

Create `server/internal/agent/core/ports.go`:

```go
package core

import (
	"context"
	"io"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

type ProviderMessage struct {
	Role    string
	Content string
}

type StreamProvider interface {
	Stream(context.Context, []ProviderMessage) (io.ReadCloser, error)
}

func ContextMessages(messages []domain.ContextMessage) []ProviderMessage {
	result := make([]ProviderMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, ProviderMessage{
			Role: message.Role, Content: message.Content,
		})
	}
	return result
}
```

Modify `provider.NewOpenAICompatible` to implement `Stream(context.Context, []core.ProviderMessage)`. Keep the Provider's key, URL and SSE parsing hidden inside the adapter.

- [ ] **Step 4: Implement Service use cases**

Create `server/internal/agent/core/service.go`. Public behavior:

```go
func (s *Service) SendMessage(
	ctx context.Context,
	input CreateSendRunRecord,
) (domain.RunHandle, error) {
	execution, err := s.repository.CreateAgentSendRun(ctx, input)
	if err != nil {
		return domain.RunHandle{}, err
	}
	if err := s.coordinator.Start(execution); err != nil {
		_ = s.repository.FinalizeAgentRun(ctx, FinalizeRunRecord{
			RunID: execution.Run.ID, Status: domain.RunFailed,
			AssistantPartID: execution.AssistantPartID,
			MessageStatus: domain.MessageFailed, PartStatus: domain.PartFailed,
			AgentError: &domain.Error{
				Code: "AGENT_RUN_START_FAILED", Kind: "server",
				Message: "Agent 执行启动失败", Retryable: true,
			},
			TerminalEventType: "run.failed",
		})
		return domain.RunHandle{}, err
	}
	return handleFromExecution(execution), nil
}
```

`GetSession` must:

```text
load persisted detail
if detail.ActiveRun is running
  snapshot := coordinator.Snapshot(activeRun.RunID)
  replace matching assistant text part with snapshot.Text
  set activeRun.LastSequence = snapshot.LastSequence
```

`RunSnapshot` is:

```go
type RunSnapshot struct {
	Text         string
	LastSequence int
	Status       domain.RunStatus
}
```

`CancelRun` validates Session/Run ownership and calls Coordinator. It must not mutate a terminal Run back to cancelled.

- [ ] **Step 5: Implement Run Coordinator**

Create `server/internal/agent/core/coordinator.go`. Start uses an application-owned context:

```go
func (c *Coordinator) Start(execution domain.RunExecution) error {
	c.mu.Lock()
	if _, exists := c.runs[execution.Run.ID]; exists {
		c.mu.Unlock()
		return ErrRunAlreadyRegistered
	}
	ctx, cancel := context.WithCancel(c.baseContext)
	journal := NewEventJournal(8 * 1024 * 1024)
	active := &activeRun{
		execution: execution, ctx: ctx, cancel: cancel, journal: journal,
	}
	c.runs[execution.Run.ID] = active
	c.mu.Unlock()
	go c.execute(active)
	return nil
}
```

`execute` must:

1. Append `StreamStart`, `StreamStartStep`, `StreamTextStart`.
2. Call `provider.Stream(active.ctx, core.ContextMessages(execution.Messages))`.
3. Relay provider deltas with the existing parser.
4. Append every delta to the journal.
5. Call `FinalizeAgentRun` before appending `StreamTextEnd`, `StreamFinishStep`, `StreamFinish`.
6. On provider or protocol failure, persist `failed`, append `StreamError`, and retain partial text.
7. On cancellation, persist `cancelled` and append the cancellation terminal sequence.
8. Remove the active coordinator entry only after the journal terminal event is published.

- [ ] **Step 6: Add startup recovery**

Expose:

```go
func (c *Coordinator) Recover(ctx context.Context) error {
	_, err := c.repository.RecoverInterruptedAgentRuns(ctx)
	return err
}
```

The composition root calls `Recover` before registering HTTP routes.

- [ ] **Step 7: Run recovery test**

Run:

```powershell
go test ./tests/e2e/integration -run TestAgentRunContinuesAfterSubscriberDisconnects -v
```

Expected: PASS.

- [ ] **Step 8: Format and commit**

Run:

```powershell
go fmt ./server/...
gofmt -w tests/e2e/integration/agent_recovery.integration_test.go
go test ./tests/e2e/integration -run TestAgentRunContinuesAfterSubscriberDisconnects -v
git add server/internal/agent/core server/internal/agent/provider server/cmd tests/e2e/integration/agent_recovery.integration_test.go
git commit -m "feat: own agent runs outside http requests"
```

---

## Task 4: Add Session, Message And Run HTTP Controllers

**Files:**
- Create: `server/internal/agent/access/session_controller.go`
- Create: `server/internal/agent/access/run_controller.go`
- Create: `server/internal/agent/access/ui_stream.go`
- Modify: `server/internal/agent/access/stream_controller.go`
- Modify: `server/cmd/craftingtabled/main.go`
- Test: `tests/e2e/integration/agent_integration_api.integration_test.go`

**Interfaces:**
- Consumes: `core.Service`, `core.Coordinator`, `httpapi.Router`.
- Produces: all seven approved Agent routes and stable JSON error mapping.

- [ ] **Step 1: Write failing HTTP contract tests**

Create `tests/e2e/integration/agent_integration_api.integration_test.go`:

```go
func TestAgentSessionApiPersistsAcrossServerRestart(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "api-restart.sqlite")
	first := startServerWithDatabase(t, "", databasePath)

	status, created := requestJSON[struct {
		Data struct {
			Session struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"session"`
		} `json:"data"`
	}](
		t,
		http.MethodPost,
		first.URL("/api/v1/agent/session/create"),
		`{"clientRequestId":"api-restart-session"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create status = %d", status)
	}
	first.Close()

	second := startServerWithDatabase(t, "", databasePath)
	defer second.Close()
	status, listed := requestJSON[struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}](
		t,
		http.MethodGet,
		second.URL("/api/v1/agent/session/list"),
		"",
	)
	if status != http.StatusOK || len(listed.Data) != 1 {
		t.Fatalf("list status=%d body=%#v", status, listed)
	}
	if listed.Data[0].ID != created.Data.Session.ID {
		t.Fatalf("session = %q, want %q", listed.Data[0].ID, created.Data.Session.ID)
	}
}

func TestAgentRunStreamReplaysAfterCursor(t *testing.T) {
	provider := startFakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"B\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	server := startServer(t, provider.URL+"/v1")
	_, runID := createSessionAndRun(t, server, "cursor replay")
	response, err := http.Get(
		server.URL("/api/v1/agent/run/stream?runId=" + runID + "&after=1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `"type":"start"`) ||
		strings.Contains(text, `"delta":"A"`) ||
		!strings.Contains(text, `"delta":"B"`) ||
		!strings.Contains(text, "data: [DONE]") {
		t.Fatalf("stream = %s", text)
	}
}

func TestAgentRunStreamReturnsCursorExpiredAfterJournalRemoval(t *testing.T) {
	provider := startFakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	server := startServer(t, provider.URL+"/v1")
	_, runID := createSessionAndRun(t, server, "cursor expiry")
	waitForRunTerminal(t, server, runID)
	removeRunJournalAfterTerminal(t, server, runID)

	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/agent/run/stream?runId="+runID+"&after=1"),
		"",
	)
	if status != http.StatusConflict ||
		failure.Error.Code != "AGENT_STREAM_CURSOR_EXPIRED" {
		t.Fatalf("status=%d error=%#v", status, failure.Error)
	}
}
```

Add these helpers to `support_test.go`:

```go
type testServer struct {
	baseURL string
	command *exec.Cmd
}

func (s *testServer) URL(path string) string {
	return s.baseURL + path
}

func (s *testServer) Close() {
	if s.command != nil && s.command.Process != nil {
		_ = s.command.Process.Kill()
		_ = s.command.Wait()
	}
}

func startServerWithDatabase(
	t *testing.T,
	providerURL string,
	databasePath string,
) *testServer {
	t.Helper()
	port := freePort(t)
	command := exec.Command(
		serverBinary,
		"-env-file="+filepath.Join(t.TempDir(), "missing.env"),
		"-reset-database",
	)
	command.Dir = repositoryRoot()
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	overrides := map[string]string{
		"API_PORT":                    fmt.Sprintf("%d", port),
		"CRAFTINGTABLE_DATABASE_PATH": databasePath,
		"CRAFTINGTABLE_TEST_HOOKS":    "1",
	}
	if providerURL != "" {
		overrides["CRAFTINGTABLE_AGENT_BASE_URL"] = providerURL
		overrides["CRAFTINGTABLE_AGENT_API_KEY"] = "integration-test-key"
		overrides["CRAFTINGTABLE_AGENT_MODEL"] = "integration-model"
	}
	command.Env = isolatedEnvironment(overrides)
	if err := command.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	server := &testServer{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		command: command,
	}
	t.Cleanup(server.Close)
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

var integrationRequestSequence atomic.Int64

func createSessionAndRun(
	t *testing.T,
	server *testServer,
	prompt string,
) (string, string) {
	t.Helper()
	sequence := integrationRequestSequence.Add(1)
	status, created := requestJSON[struct {
		Data struct {
			Session struct {
				ID string `json:"id"`
			} `json:"session"`
		} `json:"data"`
	}](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/session/create"),
		fmt.Sprintf(`{"clientRequestId":"session-%d"}`, sequence),
	)
	if status != http.StatusOK {
		t.Fatalf("create session status = %d", status)
	}
	status, started := requestJSON[struct {
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/message/send"),
		fmt.Sprintf(`{
			"sessionId":%q,
			"clientRequestId":"send-%d",
			"message":{
				"id":"message-%d",
				"role":"user",
				"parts":[{"type":"text","text":%q}]
			}
		}`, created.Data.Session.ID, sequence, sequence, prompt),
	)
	if status != http.StatusOK {
		t.Fatalf("start run status = %d", status)
	}
	return created.Data.Session.ID, started.Data.RunID
}

func waitForRunTerminal(
	t *testing.T,
	server *testServer,
	runID string,
) {
	t.Helper()
	response, err := http.Get(
		server.URL("/api/v1/agent/run/stream?runId=" + runID + "&after=0"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "data: [DONE]") {
		t.Fatalf("run did not terminate: %s", body)
	}
}

func removeRunJournalAfterTerminal(
	t *testing.T,
	server *testServer,
	runID string,
) {
	t.Helper()
	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/test/agent/run/remove-journal"),
		fmt.Sprintf(`{"runId":%q}`, runID),
	)
	if status != http.StatusOK {
		t.Fatalf("remove journal status=%d error=%#v", status, failure.Error)
	}
}
```

`removeRunJournalAfterTerminal` calls a test-only route that is registered only when `CRAFTINGTABLE_TEST_HOOKS=1`. The route deletes the in-memory journal and never deletes SQLite data.

- [ ] **Step 2: Run tests and verify 404**

Run:

```powershell
go test ./tests/e2e/integration -run "TestAgentSessionApi|TestAgentRunStream" -v
```

Expected: FAIL because the new routes are not registered.

- [ ] **Step 3: Implement Session Controller**

Create `server/internal/agent/access/session_controller.go`. Register:

```go
func (c *SessionController) Register(router *httpapi.Router) {
	router.Handle("POST /api/v1/agent/session/create", c.createSession)
	router.Handle("GET /api/v1/agent/session/list", c.listSessions)
	router.Handle("GET /api/v1/agent/session/get", c.getSession)
	router.Handle("POST /api/v1/agent/message/send", c.sendMessage)
	router.Handle("POST /api/v1/agent/message/retry", c.retryMessage)
}
```

Request shapes must exactly match the approved design. Compute the request hash in the Service from canonical JSON, not from untrusted headers.

- [ ] **Step 4: Implement Run Controller and SSE encoder**

Create `server/internal/agent/access/run_controller.go`:

```go
func (c *RunController) Register(router *httpapi.Router) {
	router.Handle("GET /api/v1/agent/run/stream", c.stream)
	router.Handle("POST /api/v1/agent/run/cancel", c.cancel)
}
```

The stream handler:

```text
parse runId and after
subscribe to Coordinator
write SSE headers
always write the original start chunk first
write replay and live chunks with id: <sequence>
skip duplicate sequences and sequences <= after
close subscription when the request context ends
```

Move existing UI chunk structs into `ui_stream.go`. Encode `StreamChunk` into the current `start`, `start-step`, `text-start`, `text-delta`, `text-end`, `finish-step`, `finish` and `error` JSON shapes.

- [ ] **Step 5: Map stable errors**

Use these mappings:

```go
AGENT_SESSION_NOT_FOUND       404
AGENT_RUN_NOT_FOUND           404
AGENT_SESSION_BUSY            409
AGENT_IDEMPOTENCY_CONFLICT    409
AGENT_MESSAGE_NOT_RETRYABLE   409
AGENT_STREAM_CURSOR_EXPIRED   409
AGENT_NOT_CONFIGURED          503
AGENT_PERSISTENCE_FAILED      500
```

Never return raw SQLite errors.

- [ ] **Step 6: Wire the composition root**

Modify `server/cmd/craftingtabled/main.go`:

```go
sqliteStore, err := store.OpenSQLite(openContext, databasePath)
if err != nil {
	return err
}

var streamProvider core.StreamProvider
if agentConfig.Configured() {
	streamProvider = provider.NewOpenAICompatible(agentConfig)
}

agentService := core.NewService(sqliteStore, streamProvider, logger)
if err := agentService.Recover(context.Background()); err != nil {
	return fmt.Errorf("recover agent runs: %w", err)
}

access.NewSessionController(agentService, logger).Register(router)
access.NewRunController(agentService, logger).Register(router)
```

Keep Today registration and route behavior unchanged.

- [ ] **Step 7: Run HTTP and existing Go tests**

Run:

```powershell
go test ./tests/e2e/integration -run "TestAgent" -v
go test ./server/... ./tests/e2e/integration/...
```

Expected: PASS, including existing Today tests.

- [ ] **Step 8: Format and commit**

Run:

```powershell
go fmt ./server/...
go fmt ./tests/e2e/integration/...
go test ./server/... ./tests/e2e/integration/...
git add server/internal/agent/access server/cmd/craftingtabled tests/e2e/integration
git commit -m "feat: expose persistent agent session api"
```

---

## Task 5: Add SSE Cursor To Front-End Protocol

**Files:**
- Modify: `src/models/agentStreamSchema.ts`
- Modify: `src/models/agentStreamReducer.ts`
- Modify: `src/models/agentStreamProtocol.ts`
- Test: `tests/e2e/unit/agent/agentStreamProtocol.test.ts`
- Test: `tests/e2e/unit/agent/agentStreamReducer.test.ts`

**Interfaces:**
- Consumes: existing SSE parser and Zod schemas.
- Produces: `AgentStreamEvent.cursor: number`; `readAgentStream(body)` remains an `AsyncIterable<AgentStreamEvent>`.

- [ ] **Step 1: Write failing cursor tests**

Add to `agentStreamProtocol.test.ts`:

```ts
it("attaches and validates the SSE cursor", async () => {
  const stream = sseStream([
    'id: 1\ndata: {"type":"start","messageId":"assistant-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}\n\n',
    'id: 2\ndata: {"type":"text-delta","id":"part-1","delta":"你好"}\n\n',
    'id: 3\ndata: {"type":"finish","messageMetadata":{"runId":"run-1","status":"completed"}}\n\n',
    "data: [DONE]\n\n",
  ]);

  const events = [];
  for await (const event of readAgentStream(stream)) {
    events.push(event);
  }
  expect(events).toEqual([
    expect.objectContaining({ type: "text-delta", cursor: 2 }),
    expect.objectContaining({ type: "finish", cursor: 3 }),
  ]);
});

it("rejects a missing, non-numeric, or regressing cursor", async () => {
  await expect(readEvents([
    'data: {"type":"start","messageId":"a","messageMetadata":{"sessionId":"s","runId":"r","userMessageId":"u"}}\n\n',
  ])).rejects.toMatchObject({
    agentError: { code: "PROTOCOL_INVALID_METADATA" },
  });
});
```

- [ ] **Step 2: Run focused tests and verify failure**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentStreamProtocol.test.ts
```

Expected: FAIL because cursor is not attached.

- [ ] **Step 3: Carry cursor through the reducer**

Change the protocol transition input to include a validated cursor:

```ts
type AgentStreamFrame = {
  cursor: number
  part: AgentStreamPart
}

function reduceAgentStream(
  state: AgentStreamState,
  frame: AgentStreamFrame,
): Result<AgentStreamTransition>
```

Every emitted `text-delta`, `finish` and `error` includes `cursor: frame.cursor`.

`AgentStreamState` becomes:

```ts
export type AgentStreamState = {
  phase: AgentStreamPhase
  lastCursor: number
}
```

If `frame.cursor <= state.lastCursor`, return `PROTOCOL_INVALID_METADATA`.

- [ ] **Step 4: Parse SSE ids in the protocol adapter**

In `readAgentStream`, parse each SSE message's `id`:

```ts
const cursor = Number.parseInt(value.id ?? "", 10)
if (!Number.isSafeInteger(cursor) || cursor < 1) {
  throw agentRequestError(invalidProtocolError)
}
```

The `[DONE]` sentinel has no semantic cursor and does not change `lastCursor`.

- [ ] **Step 5: Run protocol tests**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentStreamProtocol.test.ts tests/e2e/unit/agent/agentStreamReducer.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentStreamProtocol.test.ts tests/e2e/unit/agent/agentStreamReducer.test.ts
git add src/models/agentStreamSchema.ts src/models/agentStreamReducer.ts src/models/agentStreamProtocol.ts tests/e2e/unit/agent
git commit -m "feat: carry sse cursors through agent protocol"
```

---

## Task 6: Replace The In-Memory API Model With Persistent Agent Calls

**Files:**
- Modify: `src/models/agentContracts.ts`
- Modify: `src/models/agentTransport.ts`
- Create: `src/models/agentRunClient.ts`
- Create: `src/models/agentRunMaterializer.ts`
- Modify: `src/models/apiAgentModel.ts`
- Modify: `src/models/agentModel.ts`
- Test: `tests/e2e/unit/agent/apiAgentModel.test.ts`
- Test: `tests/e2e/unit/agent/agentRunClient.test.ts`

**Interfaces:**
- Consumes: HTTP APIs from Task 4 and cursor events from Task 5.
- Produces: `createApiAgentModel({ baseUrl, fetchImpl })` that no longer imports or delegates to `createFakeAgentModel`; new `AgentModel.resumeRun(input, handlers)`.

- [ ] **Step 1: Extend front-end contracts with failing tests**

Update `apiAgentModel.test.ts` first:

```ts
it("creates sessions through the session API instead of local maps", async () => {
  const calls: string[] = [];
  const model = createApiAgentModel({
    baseUrl: "/api/v1",
    fetchImpl: async (input) => {
      calls.push(String(input));
      return jsonResponse({
        data: {
          session: {
            id: "session-1",
            title: "新会话",
            createdAt: "2026-11-05T00:00:00Z",
            updatedAt: "2026-11-05T00:00:00Z",
          },
        },
      });
    },
  });

  const [created, error] = await model.createSession({
    clientRequestId: "request-1",
  });
  expect(error).toBeNull();
  expect(created?.session.id).toBe("session-1");
  expect(calls).toContain("/api/v1/agent/session/create");
});
```

Add a resume test:

```ts
it("resumes the active run after the supplied cursor", async () => {
  const requestedUrls: string[] = [];
  const model = createApiAgentModel({
    baseUrl: "/api/v1",
    fetchImpl: async (input) => {
      const url = String(input);
      requestedUrls.push(url);
      return streamResponse([
        'id: 4\ndata: {"type":"start","messageId":"assistant-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}\n\n',
        'id: 5\ndata: {"type":"text-delta","id":"part-1","delta":"后续"}\n\n',
        'id: 6\ndata: {"type":"finish","messageMetadata":{"runId":"run-1","status":"completed"}}\n\n',
        "data: [DONE]\n\n",
      ]);
    },
  });

  const result = await model.resumeRun(
    {
      sessionId: "session-1",
      runId: "run-1",
      afterSequence: 4,
      signal: new AbortController().signal,
    },
    { onStarted: vi.fn(), onSnapshot: vi.fn() },
  );
  expect(result[1]).toBeNull();
  expect(requestedUrls[0]).toContain("after=4");
});
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/apiAgentModel.test.ts
```

Expected: FAIL because the API model still calls `session/create` through the fake model and `resumeRun` is missing.

- [ ] **Step 3: Update contracts**

In `src/models/agentContracts.ts`:

```ts
export type AgentRunHandle = {
  runId: string
  sessionId: string
  userMessageId: string
  assistantMessageId: string
  assistantPartId: string
  status: AgentRunStatus
}

export type AgentActiveRun = {
  runId: string
  status: AgentRunStatus
  lastSequence: number
}

export type AgentSessionDetail = {
  session: AgentSession
  messages: AgentMessage[]
  messageParts: AgentMessagePart[]
  activeRun: AgentActiveRun | null
  nextMessageCursor: string | null
}
```

Add to `AgentModel`:

```ts
resumeRun(
  input: {
    sessionId: string
    runId: string
    afterSequence: number
    signal: AbortSignal
  },
  handlers: AgentSendHandlers,
): Promise<Result<AgentSendResult>>
```

- [ ] **Step 4: Implement typed HTTP transport**

Add to `agentTransport.ts`:

```ts
type Envelope<T> = { data: T } | { error: AgentError }

async function readAgentEnvelope<T>(response: Response): Promise<T> {
  const body = (await response.json()) as Envelope<T>
  if ("error" in body) {
    throw agentRequestError(body.error)
  }
  return body.data
}

export async function createAgentSession(
  baseUrl: string,
  clientRequestId: string,
  fetchImpl: AgentFetch,
) {
  const response = await fetchImpl(`${baseUrl}/agent/session/create`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ clientRequestId }),
  })
  return readAgentEnvelope<{ session: AgentSession }>(response)
}
```

Implement matching `listAgentSessions`, `getAgentSession`, `startAgentRun`, `retryAgentRun`, `cancelAgentRun` and `openAgentRunStream`.

`openAgentRunStream` must request:

```ts
`${baseUrl}/agent/run/stream?runId=${encodeURIComponent(runId)}&after=${afterSequence}`
```

- [ ] **Step 5: Implement the run materializer**

Create `agentRunMaterializer.ts` with one state object:

```ts
type RunMaterializer = {
  start: (handle: AgentRunHandle, timestamp: string) => AgentMessageSnapshot
  delta: (text: string, cursor: number) => AgentMessageSnapshot
  finish: (status: AgentRunStatus, timestamp: string) => AgentMessageSnapshot
  error: (error: AgentError, timestamp: string) => AgentMessageSnapshot
}
```

It owns only one assistant message and one text part, plus an optional error part. It must never mutate Store directly.

- [ ] **Step 6: Implement `agentRunClient.ts`**

The client owns the reconnect loop and cursor:

```ts
export async function consumeAgentRun(input: {
  baseUrl: string
  fetchImpl: AgentFetch
  sessionId: string
  runId: string
  assistantMessageId: string
  afterSequence: number
  signal: AbortSignal
  materializer: RunMaterializer
  handlers: AgentSendHandlers
}): Promise<Result<AgentSendResult>> {
  let cursor = input.afterSequence
  let attempt = 0
  while (!input.signal.aborted) {
    try {
      const body = await openAgentRunStream(
        input.baseUrl, input.runId, cursor, input.signal, input.fetchImpl,
      )
      for await (const event of readAgentStream(body)) {
        cursor = Math.max(cursor, event.cursor)
        // map text-delta, finish and error through materializer
      }
      if (terminal) return [result, null]
    } catch (cause) {
      if (input.signal.aborted) return cancellationResult()
      const error = embeddedAgentError(cause)
      if (error?.code === "AGENT_STREAM_CURSOR_EXPIRED") {
        const detail = await getAgentSession(
          input.baseUrl, input.sessionId, input.fetchImpl,
        )
        const message = detail.messages.find(
          (candidate) => candidate.id === input.assistantMessageId,
        )
        const parts = detail.messageParts.filter(
          (part) => part.messageId === input.assistantMessageId,
        )
        if (!message) {
          return [null, {
            code: "AGENT_MESSAGE_NOT_RETRYABLE",
            kind: "conflict",
            message: "恢复后的助手消息不存在",
            retryable: false,
          }]
        }
        input.handlers.onSnapshot({ message, parts })
        return [{
          assistantMessageId: input.assistantMessageId,
          runId: input.runId,
          status: statusFromPersistedMessage(message),
        }, null]
      }
      attempt++
      if (attempt > 5) return [null, error ?? internalRunError(cause)]
      await delay(Math.min(250 * 2 ** attempt, 4000), input.signal)
    }
  }
  return [null, cursorExpiredOrAbortResult()]
}
```

No backoff happens for non-retryable protocol errors or not-found errors.

- [ ] **Step 7: Rewrite `apiAgentModel.ts`**

Remove this import:

```ts
import { createFakeAgentModel } from "@/models/fakeAgentModel";
```

`createSession`, `listSessions` and `getSession` call the new transport functions. `sendMessage` calls `startAgentRun`, invokes `onStarted` from the returned handle, then consumes the stream. `retryMessage` calls `retryAgentRun` and emits snapshots for the existing assistant message ID. `resumeRun` consumes from the supplied cursor.

`cancelRun` calls the server API and does not treat local fetch abort as server cancellation.

- [ ] **Step 8: Run model tests**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/apiAgentModel.test.ts tests/e2e/unit/agent/agentRunClient.test.ts
```

Expected: PASS.

- [ ] **Step 9: Commit**

Run:

```powershell
pnpm typecheck
pnpm vitest run tests/e2e/unit/agent/apiAgentModel.test.ts tests/e2e/unit/agent/agentRunClient.test.ts
git add src/models tests/e2e/unit/agent
git commit -m "feat: persist agent model through session api"
```

---

## Task 7: Resume Active Runs From The Agent Effect

**Files:**
- Modify: `src/controllers/effects/useAgentEffect.ts`
- Modify: `src/controllers/stores/agentStore.ts`
- Test: `tests/e2e/unit/agent/agentEffect.test.ts`
- Test: `tests/e2e/unit/agent/agentStore.test.ts`

**Interfaces:**
- Consumes: `AgentModel.resumeRun`, `AgentSessionDetail.activeRun.lastSequence`.
- Produces: `loadSession()` automatically subscribes to a running Run; duplicate subscriptions are suppressed.

- [ ] **Step 1: Write failing Effect resume test**

Add to `agentEffect.test.ts`:

```ts
it("resumes a running session after loading persisted detail", async () => {
  const resumed = vi.fn(async (_input, handlers) => {
    handlers.onSnapshot({
      message: {
        id: "assistant-1",
        sessionId: "session-1",
        seq: 2,
        role: "assistant",
        status: "streaming",
        activeRunId: "run-1",
        userMessageId: "user-1",
        createdAt: "2026-11-05T00:00:00Z",
        updatedAt: "2026-11-05T00:00:01Z",
      },
      parts: [{
        id: "part-1",
        messageId: "assistant-1",
        runId: "run-1",
        index: 0,
        type: "text",
        status: "streaming",
        content: { type: "text", text: "恢复内容" },
        createdAt: "2026-11-05T00:00:00Z",
        updatedAt: "2026-11-05T00:00:01Z",
      }],
    })
    return [{ assistantMessageId: "assistant-1", runId: "run-1", status: "completed" }, null]
  })
  const model = {
    ...createFakeAgentModel({ chunkDelayMs: 0 }),
    getSession: async () => [{
      session: { id: "session-1", title: "恢复", createdAt: "x", updatedAt: "y" },
      messages: [makeUserMessage()],
      messageParts: [makeUserPart()],
      activeRun: { runId: "run-1", status: "running", lastSequence: 7 },
      nextMessageCursor: null,
    }, null],
    resumeRun: resumed,
  }
  const effect = createAgentEffect(model)

  await effect.loadSessions()
  await effect.loadSession("session-1")
  await vi.waitFor(() => {
    expect(resumed).toHaveBeenCalledWith(
      expect.objectContaining({
        sessionId: "session-1",
        runId: "run-1",
        afterSequence: 7,
      }),
      expect.any(Object),
    )
  })
  expect(lastAssistantText("session-1")).toBe("恢复内容")
})
```

- [ ] **Step 2: Run test and verify no resume call**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentEffect.test.ts
```

Expected: FAIL because `loadSession` only sets liveRun and never subscribes.

- [ ] **Step 3: Add subscription ownership to Effect**

Inside `createAgentEffect`:

```ts
const activeSubscriptions = new Map<string, AbortController>()

const stopSubscription = (runId: string) => {
  activeSubscriptions.get(runId)?.abort()
  activeSubscriptions.delete(runId)
}

const startRunSubscription = (
  detail: AgentSessionDetail,
  activeRun: NonNullable<AgentSessionDetail["activeRun"]>,
) => {
  if (activeSubscriptions.has(activeRun.runId)) return
  const controller = new AbortController()
  activeSubscriptions.set(activeRun.runId, controller)
  void executeRun(
    detail.session.id,
    (signal, handlers) =>
      model.resumeRun(
        {
          sessionId: detail.session.id,
          runId: activeRun.runId,
          afterSequence: activeRun.lastSequence,
          signal,
        },
        handlers,
      ),
    () => undefined,
    controller,
  ).finally(() => stopSubscription(activeRun.runId))
}
```

Refactor `executeRun` to accept an externally supplied AbortController for resumed runs. Keep store updates identical to normal sends.

- [ ] **Step 4: Call resume after persisted detail is applied**

In `loadSession`:

```ts
if (detail.activeRun?.status === "running") {
  state.setSendPhase("streaming")
  state.setLiveRun({
    sessionId: detail.session.id,
    runId: detail.activeRun.runId,
  })
  startRunSubscription(detail, detail.activeRun)
} else {
  stopSubscriptionsForSession(detail.session.id)
  state.setSendPhase("idle")
  state.setLiveRun(null)
}
```

- [ ] **Step 5: Add duplicate-load guard test**

Call `effect.loadSession("session-1")` twice before the first resumed stream completes and assert `resumeRun` is called once.

- [ ] **Step 6: Run Effect and Store tests**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentEffect.test.ts tests/e2e/unit/agent/agentStore.test.ts
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```powershell
pnpm typecheck
pnpm vitest run tests/e2e/unit/agent/agentEffect.test.ts tests/e2e/unit/agent/agentStore.test.ts
git add src/controllers tests/e2e/unit/agent
git commit -m "feat: resume active agent runs from zustand effect"
```

---

## Task 8: Add Browser Reload And Recovery Regression Tests

**Files:**
- Create: `scripts/fakeAgentProvider.mjs`
- Modify: `tests/e2e/modules/agent/navigation.e2e.ts`
- Modify: `tests/e2e/modules/agent/chat.e2e.ts`
- Modify: `tests/e2e/support/data.ts`
- Modify: `playwright.config.ts`
- Modify: `.env.e2e`

- [ ] **Step 1: Add the failing page reload E2E test**

In `navigation.e2e.ts`:

```ts
test("agent history survives a direct URL reload", async ({ page }) => {
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  await expect(page).toHaveURL(/#\/agent\/[0-9a-f-]+/)
  await page.getByRole("textbox", { name: "发送给 Agent" }).fill("持久化测试")
  await page.getByRole("button", { name: "发送" }).click()
  await expect(page.getByText("持久化测试", { exact: true })).toBeVisible()
  const url = page.url()
  await page.reload()
  await expect(page).toHaveURL(url)
  await expect(page.getByText("持久化测试", { exact: true })).toBeVisible()
  await expect(page.getByText("会话不存在或已被删除")).toHaveCount(0)
})
```

- [ ] **Step 2: Add the reconnect E2E test**

Use the fake provider fixture with a delayed second chunk:

```ts
test("agent continues after a page reload during generation", async ({ page }) => {
  await page.goto("/#/agent")
  await page.getByRole("button", { name: "新建会话" }).click()
  await page.getByRole("textbox", { name: "发送给 Agent" }).fill("慢速恢复测试")
  await page.getByRole("button", { name: "发送" }).click()
  await expect(page.getByText("第一段")).toBeVisible({ timeout: 10_000 })
  await page.reload()
  await expect(page.getByText("第一段")).toBeVisible()
  await expect(page.getByText("第一段第二段")).toBeVisible({ timeout: 10_000 })
})
```

- [ ] **Step 3: Run E2E and verify failure**

Run:

```powershell
pnpm test:e2e -- tests/e2e/modules/agent/navigation.e2e.ts tests/e2e/modules/agent/chat.e2e.ts
```

Expected: FAIL before the persistent test server and delayed provider fixture are wired.

- [ ] **Step 4: Add a deterministic OpenAI-compatible provider**

Create `scripts/fakeAgentProvider.mjs`:

```js
import { createServer } from "node:http";

const port = Number.parseInt(process.env.FAKE_AGENT_PORT ?? "3201", 10);

function chunk(content) {
  return `data: ${JSON.stringify({
    choices: [{ delta: { content } }],
  })}\n\n`;
}

const server = createServer((request, response) => {
  if (request.method === "GET" && request.url === "/healthz") {
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end('{"status":"ok"}');
    return;
  }

  if (request.method !== "POST" || request.url !== "/v1/chat/completions") {
    response.writeHead(404);
    response.end();
    return;
  }

  let body = "";
  request.setEncoding("utf8");
  request.on("data", (value) => {
    body += value;
  });
  request.on("end", () => {
    const payload = JSON.parse(body);
    const prompt = [...payload.messages]
      .reverse()
      .find((message) => message.role === "user")?.content ?? "";

    response.writeHead(200, {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    });

    if (prompt.includes("慢速恢复测试")) {
      response.write(chunk("第一段"));
      setTimeout(() => {
        response.write(chunk("第二段"));
        response.end("data: [DONE]\n\n");
      }, 1200);
      return;
    }

    response.write(chunk(`已收到：${prompt}`));
    response.end("data: [DONE]\n\n");
  });
});

server.listen(port, "127.0.0.1");
```

- [ ] **Step 5: Point E2E at the persistent API**

Update `.env.e2e`:

```dotenv
VITE_DATA_SOURCE=api
VITE_AGENT_SOURCE=api
VITE_API_BASE=/api/v1
VITE_API_PROXY_TARGET=http://127.0.0.1:3101
API_PORT=3101
CRAFTINGTABLE_DATABASE_PATH=./data/craftingtable-e2e.sqlite
CRAFTINGTABLE_AGENT_BASE_URL=http://127.0.0.1:3201/v1
CRAFTINGTABLE_AGENT_API_KEY=e2e-test-key
CRAFTINGTABLE_AGENT_MODEL=e2e-fake
```

Update `playwright.config.ts`:

```ts
webServer: [
  {
    command: "node scripts/fakeAgentProvider.mjs",
    url: "http://127.0.0.1:3201/healthz",
    reuseExistingServer: false,
  },
  {
    command: "pnpm e2e:api",
    url: "http://127.0.0.1:3101/api/v1/categories",
    reuseExistingServer: false,
  },
  {
    command: "pnpm e2e:web",
    url: "http://127.0.0.1:4174",
    reuseExistingServer: false,
  },
]
```

Change `agentModel` selection so an explicit API source wins even in `e2e` mode:

```ts
export const agentModel: AgentModel =
  import.meta.env.VITE_AGENT_SOURCE === "api"
    ? createApiAgentModel({
        baseUrl: import.meta.env.VITE_API_BASE ?? "/api/v1",
      })
    : import.meta.env.MODE === "test" ||
        import.meta.env.MODE === "e2e" ||
        import.meta.env.VITE_AGENT_SOURCE === "fake"
      ? createFakeAgentModel(fakeModelOptions())
      : import.meta.env.MODE === "development"
        ? createApiAgentModel({
            baseUrl: import.meta.env.VITE_API_BASE ?? "/api/v1",
          })
        : notConfiguredAgentModel;
```

- [ ] **Step 6: Run E2E tests**

Run:

```powershell
pnpm test:e2e -- tests/e2e/modules/agent/navigation.e2e.ts tests/e2e/modules/agent/chat.e2e.ts
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```powershell
git add scripts/fakeAgentProvider.mjs tests/e2e/modules/agent tests/e2e/support playwright.config.ts .env.e2e src/models/agentModel.ts
git commit -m "test: cover agent persistence across page reload"
```

---

## Task 9: Update Current-State Documentation And Run Full Verification

**Files:**
- Modify: `docs/superpowers/specs/2026-11-05-craftingtable-agent-persistence-recovery-design.md`
- Modify: `docs/superpowers/specs/2026-11-05-craftingtable-agent-design.md`
- Modify: `docs/architecture/ADR-001-agent-run-recovery.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: completed implementation and verification results.
- Produces: documents that describe current behavior without claiming unsupported multi-instance recovery.

- [ ] **Step 1: Update statuses from Proposed to Accepted or Implemented**

Use:

```text
Design spec: Implemented and verified on <date>
ADR: Accepted
README: Agent sessions persist in local SQLite; reconnect works while the Go process is alive.
```

Add an implementation record with exact test files and commands.

- [ ] **Step 2: Document the server restart boundary**

README must state:

```text
Browser refresh and transient disconnect recovery are supported.
If the Go process restarts, running Agent runs become interrupted.
No token origin is fabricated after restart.
```

- [ ] **Step 3: Run the complete verification matrix**

Run each command separately:

```powershell
pnpm typecheck
pnpm test
go test ./server/... ./tests/e2e/integration/...
go vet ./server/...
$goFiles = rg --files -g "*.go"
gofmt -d $goFiles
pnpm build
pnpm test:e2e
git diff --check
```

Expected: every command exits 0. `gofmt -d` produces no output.

- [ ] **Step 4: Manually verify the real page**

Start a clean Go API on `:3102` and Vite on an unused port with `VITE_API_PROXY_TARGET=http://127.0.0.1:3102`.

Verify:

```text
1. Create a session.
2. Send a prompt that produces multiple chunks.
3. Reload while generation is still active.
4. Observe the partial response remains.
5. Observe the remaining response arrives.
6. Reload after completion and observe the full history.
7. Stop an active Run and observe cancelled status after reload.
```

- [ ] **Step 5: Commit documentation**

Run:

```powershell
git add README.md docs
git commit -m "docs: record agent persistence implementation"
```

---

## Final Acceptance Checklist

- [ ] New Agent tables exist on an existing SQLite database.
- [ ] Existing Today data remains readable.
- [ ] Direct `/agent/:id` refresh loads persisted history.
- [ ] Running response survives browser subscriber disconnect.
- [ ] Reconnect uses `after` and does not duplicate text.
- [ ] Expired cursor returns `AGENT_STREAM_CURSOR_EXPIRED`.
- [ ] Server restart converts running Runs to interrupted.
- [ ] Sending the same idempotency request returns the same Run.
- [ ] Retry preserves old Run history and displays only the active attempt.
- [ ] Cancel persists partial text and `cancelled`.
- [ ] Provider failures persist partial text and structured errors.
- [ ] Zustand remains the only Agent business state source.
- [ ] No API key reaches client, logs or SQLite.
- [ ] All tests, type checks, build, Go tests, vet and Playwright pass.
