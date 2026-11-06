package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/leeat/craftingtable/server/testkit"
)

func createPersistenceTestRun(
	t *testing.T,
	db *testkit.SQLiteStore,
	prefix string,
) testkit.RunExecution {
	t.Helper()
	session, err := db.CreateAgentSession(
		context.Background(),
		testkit.CreateAgentSessionInput{
			ClientRequestID: prefix + "-session",
			RequestHash:     prefix + "-session-hash",
		},
	)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	execution, err := db.CreateAgentSendRun(
		context.Background(),
		testkit.CreateAgentSendRunInput{
			SessionID:       session.ID,
			ClientRequestID: prefix + "-send",
			RequestHash:     prefix + "-send-hash",
			UserMessageID:   prefix + "-user",
			UserParts:       []string{"tool persistence"},
		},
	)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return execution
}

func TestMigration005AddsToolStepsAndInvocations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tool-migration.sqlite")
	store, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	columns := map[string]bool{}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info('agent_steps')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(
			&cid,
			&name,
			&columnType,
			&notNull,
			&defaultVal,
			&primaryKey,
		); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"kind", "parent_step_id", "error_code"} {
		if !columns[name] {
			t.Fatalf("agent_steps missing column %q", name)
		}
	}
	var invocationTable int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type = 'table' AND name = 'agent_tool_invocations'`,
	).Scan(&invocationTable); err != nil {
		t.Fatal(err)
	}
	if invocationTable != 1 {
		t.Fatal("agent_tool_invocations table was not created")
	}
	var checksum string
	if err := db.QueryRowContext(
		ctx,
		`SELECT checksum FROM schema_migrations WHERE name = '005_agent_tools.sql'`,
	).Scan(&checksum); err != nil {
		t.Fatal(err)
	}
	if len(checksum) != 64 {
		t.Fatalf("migration checksum = %q", checksum)
	}
}

func TestCreateAgentToolCallPersistsStepAndInvocationAtomically(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "create-tool-call.sqlite")
	db, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	execution := createPersistenceTestRun(t, db, "create-tool")

	if execution.ModelStepID == "" {
		t.Fatal("RunExecution.ModelStepID is empty")
	}
	input := testkit.BeginAgentToolCallRecord{
		RunID:          execution.Run.ID,
		ParentStepID:   execution.ModelStepID,
		ProviderCallID: "provider-call-1",
		ToolName:       "today_search",
		ToolVersion:    "1.0.0",
		Arguments:      json.RawMessage(`{"query":"agent"}`),
		ArgumentsHash:  "hash-1",
	}
	step, err := db.CreateAgentToolCall(ctx, input)
	if err != nil {
		t.Fatalf("CreateAgentToolCall() error = %v", err)
	}
	if step.ID == "" || step.Index != 1 || step.Kind != "tool" {
		t.Fatalf("tool step = %#v", step)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var (
		kind           string
		parentStepID   string
		invocationStep string
		toolName       string
	)
	if err := raw.QueryRowContext(
		ctx,
		`SELECT kind, parent_step_id FROM agent_steps WHERE id = ?`,
		step.ID,
	).Scan(&kind, &parentStepID); err != nil {
		t.Fatal(err)
	}
	if kind != "tool" || parentStepID != execution.ModelStepID {
		t.Fatalf("stored step kind=%q parent=%q", kind, parentStepID)
	}
	if err := raw.QueryRowContext(
		ctx,
		`SELECT step_id, tool_name FROM agent_tool_invocations
		 WHERE run_id = ? AND provider_call_id = ?`,
		execution.Run.ID,
		input.ProviderCallID,
	).Scan(&invocationStep, &toolName); err != nil {
		t.Fatal(err)
	}
	if invocationStep != step.ID || toolName != "today_search" {
		t.Fatalf("invocation step=%q tool=%q", invocationStep, toolName)
	}

	_, err = db.CreateAgentToolCall(ctx, input)
	if err == nil {
		t.Fatal("duplicate provider call ID was accepted")
	}
	var stepCount int
	if err := raw.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM agent_steps WHERE run_id = ?`,
		execution.Run.ID,
	).Scan(&stepCount); err != nil {
		t.Fatal(err)
	}
	if stepCount != 2 {
		t.Fatalf("step count after duplicate = %d, want 2", stepCount)
	}
}

func TestCompleteAgentToolCallPersistsResultAndTerminalStepAtomically(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "complete-tool-call.sqlite")
	db, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	execution := createPersistenceTestRun(t, db, "complete-tool")
	step, err := db.CreateAgentToolCall(ctx, testkit.BeginAgentToolCallRecord{
		RunID:          execution.Run.ID,
		ParentStepID:   execution.ModelStepID,
		ProviderCallID: "provider-call-2",
		ToolName:       "today_get_item",
		ToolVersion:    "1.0.0",
		Arguments:      json.RawMessage(`{"id":"item-1"}`),
		ArgumentsHash:  "hash-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	result := testkit.ToolResult{
		CallID: "call-2",
		Content: []testkit.ToolResultContent{{
			Type: "json",
			JSON: map[string]any{"id": "item-1"},
		}},
		EncodedBytes: 42,
		CreatedAt:    "2026-11-06T00:00:00Z",
	}
	if err := db.CompleteAgentToolCall(
		ctx,
		testkit.CompleteAgentToolCallRecord{
			RunID:        execution.Run.ID,
			StepID:       step.ID,
			Result:       result,
			EncodedBytes: 42,
		},
	); err != nil {
		t.Fatalf("CompleteAgentToolCall() error = %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var (
		resultJSON      string
		resultIsError   bool
		resultTruncated bool
		encodedBytes    int
		stepStatus      string
	)
	if err := raw.QueryRowContext(
		ctx,
		`SELECT result_json, result_is_error, result_truncated, encoded_bytes
		 FROM agent_tool_invocations WHERE step_id = ?`,
		step.ID,
	).Scan(
		&resultJSON,
		&resultIsError,
		&resultTruncated,
		&encodedBytes,
	); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRowContext(
		ctx,
		`SELECT status FROM agent_steps WHERE id = ?`,
		step.ID,
	).Scan(&stepStatus); err != nil {
		t.Fatal(err)
	}
	if resultIsError || resultTruncated || encodedBytes != 42 ||
		stepStatus != "completed" {
		t.Fatalf(
			"stored result error=%v truncated=%v bytes=%d step=%q",
			resultIsError,
			resultTruncated,
			encodedBytes,
			stepStatus,
		)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(resultJSON), &decoded); err != nil {
		t.Fatalf("result_json is invalid: %v", err)
	}
}

func TestFinalizeAndRecoveryCloseAllRunningToolSteps(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "close-tool-steps.sqlite")
	db, err := testkit.OpenSQLite(
		ctx,
		path,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	execution := createPersistenceTestRun(t, db, "close-tool")
	modelStep, err := db.CreateAgentModelStep(
		ctx,
		testkit.CreateAgentModelStepRecord{RunID: execution.Run.ID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateAgentToolCall(ctx, testkit.BeginAgentToolCallRecord{
		RunID:          execution.Run.ID,
		ParentStepID:   modelStep.ID,
		ProviderCallID: "provider-call-3",
		ToolName:       "today_summary",
		ToolVersion:    "1.0.0",
		Arguments:      json.RawMessage(`{}`),
		ArgumentsHash:  "hash-3",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinalizeAgentRun(
		ctx,
		testkit.FinalizeAgentRunInput{
			RunID:           execution.Run.ID,
			Status:          testkit.RunCompleted,
			AssistantPartID: execution.AssistantPartID,
			AssistantText:   "done",
			MessageStatus:   testkit.MessageComplete,
			PartStatus:      testkit.PartComplete,
		},
	); err != nil {
		t.Fatal(err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var running int
	if err := raw.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM agent_steps
		 WHERE run_id = ? AND status = 'running'`,
		execution.Run.ID,
	).Scan(&running); err != nil {
		t.Fatal(err)
	}
	if running != 0 {
		t.Fatalf("running steps after finalize = %d, want 0", running)
	}
	var terminal int
	if err := raw.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM agent_steps
		 WHERE run_id = ? AND status = 'completed'`,
		execution.Run.ID,
	).Scan(&terminal); err != nil {
		t.Fatal(err)
	}
	if terminal != 3 {
		t.Fatalf("completed steps = %d, want 3", terminal)
	}

	recoveryRun := createPersistenceTestRun(t, db, "recovery-tool")
	if _, err := db.CreateAgentToolCall(ctx, testkit.BeginAgentToolCallRecord{
		RunID:          recoveryRun.Run.ID,
		ParentStepID:   recoveryRun.ModelStepID,
		ProviderCallID: "provider-call-4",
		ToolName:       "today_summary",
		ToolVersion:    "1.0.0",
		Arguments:      json.RawMessage(`{}`),
		ArgumentsHash:  "hash-4",
	}); err != nil {
		t.Fatal(err)
	}
	count, err := db.RecoverInterruptedAgentRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("recovery count = %d, want 1", count)
	}
	if err := raw.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM agent_steps
		 WHERE run_id = ? AND status = 'running'`,
		recoveryRun.Run.ID,
	).Scan(&running); err != nil {
		t.Fatal(err)
	}
	if running != 0 {
		t.Fatalf("running steps after recovery = %d, want 0", running)
	}
}
