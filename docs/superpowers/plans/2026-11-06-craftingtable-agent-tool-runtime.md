# CraftingTable Agent Tool Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an immutable read-only tool runtime and the minimum real-provider continuation loop that lets the Agent call `today_search`, `today_get_item`, and `today_summary`, persist every call, and answer from the returned results.

**Architecture:** Keep provider-neutral tool contracts, registry, validation, and execution in `agent/core`; adapt Today through a read-only port in `agent/tools`; extend the existing OpenAI-compatible adapter to send tools and parse streamed tool calls; persist tool steps and invocations in SQLite; run a bounded serial model/tool loop in the Coordinator without adding frontend tool UI.

**Tech Stack:** Go 1.26, SQLite, `github.com/santhosh-tekuri/jsonschema/v6`, OpenAI-compatible SSE, React/Vitest regression coverage.

**Spec:** `docs/superpowers/specs/2026-11-06-craftingtable-agent-tool-runtime-design.md`

## Global Constraints

- Tools are static at startup. No dynamic registration, MCP, Skills, Subagent, Shell, filesystem writes, approval, or sandbox in this change.
- The first version supports only `ToolChoice: "auto"`.
- Maximum model steps is 4. Maximum tool calls per run is 8. Tools execute serially with no automatic retries.
- `CRAFTINGTABLE_AGENT_TOOLS_ENABLED` defaults to `true`; `false` restores pure text behavior.
- Invalid tool-runtime environment values prevent server startup.
- Unknown tool, invalid arguments, handler panic, and oversized result become `ToolResult{IsError:true}`.
- Provider cancellation and Run deadline cancellation propagate as errors and do not become ToolResults.
- Tool results never expose handler stacks, SQL, internal paths, or raw exceptions to the model or frontend.
- Only `today_search`, `today_get_item`, and `today_summary` are registered. All are read-only and idempotent.
- Provider normalization is deferred. Do not add provider-specific fields to frontend or tool contracts.
- The frontend continues to consume only existing text/reasoning events and must not gain tool UI in this change.
- Migration files are append-only. Add `005_agent_tools.sql`; never edit migrations 001-004.
- Use real providers in production. Fake providers are only for tests and never a production fallback.
- Follow RED-GREEN-REFACTOR for every behavior. Run the named focused test after each RED and GREEN step.

## Review Focus

- A model emits fragmented or duplicate `tool_calls` fields: ordering, ID, name, and arguments must remain deterministic or fail as a protocol error.
- A tool is unknown, receives invalid JSON, panics, times out, or returns too much data: the model must receive a stable error result instead of an internal exception.
- A Run is cancelled or reaches its deadline while a tool is executing: cancellation must terminate the Run, not be converted into a successful ToolResult.
- A model requests more tools than the budget or keeps requesting tools after the last Model Step: the Run must fail with a stable retryable limit error and leave no running Step.
- Today date/timezone values are absent, invalid, duplicated, or cross a DST boundary: results must use one resolved natural day and never silently mix server-local and requested timezone days.

---

### Task 1: Tool Contracts, Registry, Schema Validation, and Executor

**Files:**
- Create: `server/internal/agent/core/tool_contracts.go`
- Create: `server/internal/agent/core/tool_registry.go`
- Create: `server/internal/agent/core/tool_executor.go`
- Create: `server/internal/agent/core/tool_registry_test.go`
- Create: `server/internal/agent/core/tool_executor_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: standard `context`, `encoding/json`, `time`.
- Produces:
  - `type ToolDefinition struct { Name, Namespace, Version, Title, Description string; InputSchema json.RawMessage; ReadOnly, Idempotent bool; Timeout time.Duration; MaxInputBytes, MaxResultBytes int }`
  - `type ToolHandler func(context.Context, json.RawMessage) (ToolResult, error)`
  - `type Tool struct { Definition ToolDefinition; Handler ToolHandler }`
  - `type ToolCall struct { ID, ProviderCallID, RunID, StepID, Name, Version string; Arguments json.RawMessage; ArgumentsHash string }`
  - `type ToolResult struct { CallID string; Content []ToolResultContent; IsError, Truncated bool; Error *ToolError; EncodedBytes int; CreatedAt string }`
  - `type ToolError struct { Code, Kind, Message, Hint string; Retryable bool }`, implementing `error`.
  - `func NewToolRegistry([]Tool) (*ToolRegistry, error)`
  - `func (*ToolRegistry) Catalog() []ToolDefinition`
  - `func (*ToolRegistry) ProviderTools() []ProviderToolDefinition`
  - `func (*ToolRegistry) Resolve(name, version string) (Tool, bool)`
  - `func (*ToolRegistry) Validate(name, version string, arguments json.RawMessage) (json.RawMessage, *ToolError)`
  - `func NewToolExecutor(*ToolRegistry) *ToolExecutor`
  - `func (*ToolExecutor) Execute(context.Context, ToolCall) (ToolResult, error)`

- [ ] **Step 1: Add registry behavior tests**

Tests must assert: duplicate names fail; schemas compile at construction; top-level non-object schemas fail; non-empty external `$ref` fails; catalog is sorted and immutable; `ProviderTools()` contains `type: "function"` plus name/description/parameters; exact version lookup works.

```go
func TestNewToolRegistryRejectsDuplicateToolNames(t *testing.T)
func TestNewToolRegistryRejectsExternalSchemaReference(t *testing.T)
func TestToolRegistryCatalogIsSortedAndCannotBeMutatedByCaller(t *testing.T)
```

- [ ] **Step 2: Run registry tests to verify RED**

Run: `go test ./server/internal/agent/core -run 'TestNewToolRegistry|TestToolRegistryCatalog' -count=1`

Expected: FAIL because `NewToolRegistry` and related contracts do not exist.

- [ ] **Step 3: Implement tool contracts and immutable registry**

Add Draft 2020-12 compilation with a compiler loader that rejects every external reference. Validate `Name`, version, top-level object schema, `additionalProperties:false`, timeout and byte limits at construction. Canonicalize validated JSON before hashing or execution.

- [ ] **Step 4: Run registry tests to verify GREEN**

Run: `go test ./server/internal/agent/core -run 'TestNewToolRegistry|TestToolRegistryCatalog' -count=1`

Expected: PASS.

- [ ] **Step 5: Add executor behavior tests**

Tests must assert: Schema validation happens before Handler; unknown tool and invalid arguments return stable error results; handler panic maps to `internal`; successful result gets encoded byte count; oversized result becomes a truncated summary; Run context cancellation propagates; handler timeout returns `timeout`.

```go
func TestToolExecutorValidatesBeforeInvokingHandler(t *testing.T)
func TestToolExecutorRecoversHandlerPanicAsInternalResult(t *testing.T)
func TestToolExecutorPropagatesRunCancellation(t *testing.T)
func TestToolExecutorReturnsTimeoutForToolDeadline(t *testing.T)
func TestToolExecutorTruncatesOversizedResult(t *testing.T)
```

- [ ] **Step 6: Run executor tests to verify RED**

Run: `go test ./server/internal/agent/core -run 'TestToolExecutor' -count=1`

Expected: FAIL because `ToolExecutor` does not exist.

- [ ] **Step 7: Implement the executor**

Resolve the tool, enforce input size, validate and canonicalize arguments, invoke with a tool-scoped timeout, recover panic, encode the result, enforce `MaxResultBytes`, and map stable `ToolError` values. Return parent context cancellation/deadline directly.

- [ ] **Step 8: Run focused tool tests to verify GREEN**

Run: `go test ./server/internal/agent/core -run 'TestNewToolRegistry|TestToolRegistry|TestToolExecutor' -count=1`

Expected: PASS.

- [ ] **Step 9: Commit**

```powershell
git add go.mod go.sum server/internal/agent/core
git commit -m "feat(agent): add validated tool registry and executor"
```

---

### Task 2: Today Read Port, Search, Detail, and Summary

**Files:**
- Modify: `server/internal/today/item.go`
- Modify: `server/internal/platform/store/sqlite_store.go`
- Create: `server/internal/today/query.go`
- Create: `server/internal/today/query_test.go`
- Create: `server/internal/agent/tools/today_tools.go`
- Create: `server/internal/agent/tools/today_tools_test.go`

**Interfaces:**
- Consumes: Task 1 `Tool`, `ToolDefinition`, `ToolHandler`, `ToolResult`, `ToolResultContent`, `ToolError`.
- Produces:
  - `today.Repository.GetItem(context.Context, string) (Item, error)`
  - `type today.Reader interface { ListToday(...); GetItem(...); ListCategories(...) }`
  - `func today.SearchItems([]Item, SearchInput) []SearchResult`
  - `func today.SummarizeItems([]Item, []Category) Summary`
  - `func tools.NewTodayTools(today.Reader, func() time.Time) ([]core.Tool, error)`
  - Tool names exactly `today_search`, `today_get_item`, `today_summary`, version `1.0.0`, namespace `today`.

- [ ] **Step 1: Add Today query tests**

Tests must assert: date and timezone are resolved once; absent date uses the injected clock in the requested timezone; search case-insensitively matches title and description; status/category/query filters compose; default limit is 20 and max is 50; reduced search rows omit description; detail returns the full Item; summary counts statuses and category completions; scheduled duration sums valid start/end ranges; deleted item maps to `not_found`.

```go
func TestSearchItemsFiltersTitleDescriptionAndLimit(t *testing.T)
func TestSummarizeItemsCountsStatusesCategoriesAndScheduledDuration(t *testing.T)
func TestTodayToolsResolveDateAndTimezoneOnceForSearch(t *testing.T)
func TestTodayGetItemMapsDeletedItemToNotFound(t *testing.T)
```

- [ ] **Step 2: Run Today tool tests to verify RED**

Run: `go test ./server/internal/today ./server/internal/agent/tools -run 'TestSearchItems|TestSummarizeItems|TestToday' -count=1`

Expected: FAIL because `GetItem`, query functions, and the Today adapters do not exist.

- [ ] **Step 3: Implement Today read methods and adapters**

Expose `GetItem` by delegating to SQLite's existing private `getItem`. Keep all Agent imports out of `today`. Register strict Draft 2020-12 schemas with `additionalProperties:false`. Return reduced JSON for search, full JSON for detail, and summary JSON without focus duration.

- [ ] **Step 4: Run focused Today tests to verify GREEN**

Run: `go test ./server/internal/today ./server/internal/agent/tools -run 'TestSearchItems|TestSummarizeItems|TestToday' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add server/internal/today server/internal/agent/tools server/internal/platform/store/sqlite_store.go
git commit -m "feat(today): expose read-only agent tools"
```

---

### Task 3: Provider Tool Request and Streaming Parser

**Files:**
- Modify: `server/internal/agent/core/ports.go`
- Modify: `server/internal/agent/core/stream_parser.go`
- Modify: `server/internal/agent/core/stream_parser_test.go`
- Modify: `server/internal/agent/provider/openai_compatible.go`
- Modify: `server/testkit/agent.go`
- Modify: `tests/e2e/unit/agent/openai_provider_test.go`

**Interfaces:**

- Consumes: Task 1 `ProviderToolDefinition` equivalent and registry catalog shape.
- Produces:
  - `type ModelRequest struct { Messages []ProviderMessage; Tools []ProviderToolDefinition; ToolChoice string }`
  - `StreamProvider.Stream(context.Context, ModelRequest) (io.ReadCloser, error)`
  - `ProviderMessage{ Role, Content string; ToolCalls []ProviderToolCall; ToolCallID string }`
  - `ProviderToolCall{ ID, Type string; Function ProviderFunctionCall }`
  - `type ProviderTurn struct { Text, Reasoning, FinishReason string; ToolCalls []ProviderToolCall }`
  - `func CollectProviderTurn(context.Context, io.Reader, EventSink, func()) (ProviderTurn, error)`
  - `ErrOutputTruncated` and `ErrInvalidToolCall` sentinels.

- [ ] **Step 1: Add parser tests**

Tests must assert fragments concatenate by index; multiple calls preserve provider index order; missing ID, missing name, conflicting ID/name, and invalid concatenated JSON fail as protocol errors; `stop`, `tool_calls`, `length`, unknown reason, and `[DONE]` mapping; plain text and reasoning event order does not regress.

```go
func TestCollectProviderTurnAccumulatesFragmentedToolCalls(t *testing.T)
func TestCollectProviderTurnPreservesMultipleToolCallOrder(t *testing.T)
func TestCollectProviderTurnRejectsInvalidToolArguments(t *testing.T)
func TestCollectProviderTurnMapsFinishReasons(t *testing.T)
```

- [ ] **Step 2: Run parser tests to verify RED**

Run: `go test ./server/internal/agent/core -run 'TestCollectProviderTurn|TestRelayProviderStream' -count=1`

Expected: FAIL because `CollectProviderTurn` and tool-call wire types do not exist.

- [ ] **Step 3: Extend provider protocol and parser**

Parse `delta.tool_calls`, accumulate function arguments as strings, parse each argument JSON only after aggregation, reject conflicts, and return a provider-neutral turn. Keep `RelayProviderStream` as a compatibility wrapper around `CollectProviderTurn`.

- [ ] **Step 4: Add provider request wire tests**

Assert the OpenAI-compatible body contains `tools`, `tool_choice:"auto"`, assistant `tool_calls`, and `role:"tool"` plus `tool_call_id`; existing plain text wire shape remains unchanged when no tools are sent.

```go
func TestOpenAICompatibleProviderSendsToolsAndFollowUpMessages(t *testing.T)
func TestOpenAICompatibleProviderOmitsToolFieldsForTextOnlyRequest(t *testing.T)
```

- [ ] **Step 5: Run provider tests to verify RED, then implement**

Run RED: `go test ./tests/e2e/unit/agent -run 'TestOpenAICompatibleProvider' -count=1`

Expected before implementation: FAIL because `Stream` still takes only messages.

Implement request marshaling in `OpenAICompatibleProvider`, update all test providers to accept `ModelRequest`, then run:

`go test ./tests/e2e/unit/agent -run 'TestOpenAICompatibleProvider' -count=1`

Expected: PASS.

- [ ] **Step 6: Run focused parser/provider tests to verify GREEN**

Run: `go test ./server/internal/agent/core ./tests/e2e/unit/agent -run 'TestCollectProviderTurn|TestRelayProviderStream|TestOpenAICompatibleProvider' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add server/internal/agent/core server/internal/agent/provider/openai_compatible.go server/testkit tests/e2e/unit/agent/openai_provider_test.go
git commit -m "feat(agent): support streamed provider tool calls"
```

---

### Task 4: Tool Step and Invocation Persistence

**Files:**
- Create: `server/internal/platform/store/migrations/005_agent_tools.sql`
- Create: `server/internal/platform/store/tool_store.go`
- Modify: `server/internal/agent/core/repository.go`
- Modify: `server/internal/agent/domain/run.go`
- Modify: `server/internal/platform/store/agent_store.go`
- Modify: `server/testkit/agent.go`
- Create: `tests/e2e/integration/agent_tools_persistence.integration_test.go`
- Modify: `tests/e2e/integration/migrations.integration_test.go`

**Interfaces:**

- Consumes: Task 1 `ToolCall`, `ToolResult`; Task 3 step lifecycle.
- Produces:
  - `RunExecution.ModelStepID`
  - `BeginAgentToolCallRecord` and `CompleteAgentToolCallRecord`
  - `CreateAgentModelStepRecord` and `CompleteAgentModelStepRecord`
  - Repository methods:
    - `CreateAgentModelStep(context.Context, CreateAgentModelStepRecord) (domain.AgentStep, error)`
    - `CompleteAgentModelStep(context.Context, CompleteAgentModelStepRecord) error`
    - `CreateAgentToolCall(context.Context, BeginAgentToolCallRecord) (domain.AgentStep, error)`
    - `CompleteAgentToolCall(context.Context, CompleteAgentToolCallRecord) error`

- [ ] **Step 1: Add migration and transaction tests**

Tests must assert: migration 005 is checksum-ledgered; Step `kind` defaults to `model` for old rows; tool Step + invocation are inserted atomically; unique `step_id` and `(run_id, provider_call_id)` are enforced; result + terminal tool Step commit atomically; Run finalize and recovery close all running Steps.

```go
func TestMigration005AddsToolStepsAndInvocations(t *testing.T)
func TestCreateAgentToolCallPersistsStepAndInvocationAtomically(t *testing.T)
func TestCompleteAgentToolCallPersistsResultAndTerminalStepAtomically(t *testing.T)
func TestFinalizeAndRecoveryCloseAllRunningToolSteps(t *testing.T)
```

- [ ] **Step 2: Run persistence tests to verify RED**

Run: `go test ./tests/e2e/integration -run 'TestMigration005|TestCreateAgentToolCall|TestCompleteAgentToolCall|TestFinalizeAndRecoveryCloseAllRunningToolSteps' -count=1`

Expected: FAIL because migration 005 and repository methods do not exist.

- [ ] **Step 3: Implement migration and SQLite persistence**

Add `kind`, `parent_step_id`, and `error_code` to `agent_steps`; add `agent_tool_invocations` with both unique constraints. Allocate the next Step index inside the write transaction. Store canonical arguments, SHA-256 hash, encoded result, error/truncation flags, and byte count. Change finalize to close every running Step, not only index 0.

- [ ] **Step 4: Run persistence tests to verify GREEN**

Run: `go test ./tests/e2e/integration -run 'TestMigration005|TestCreateAgentToolCall|TestCompleteAgentToolCall|TestFinalizeAndRecoveryCloseAllRunningToolSteps' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add server/internal/platform/store server/internal/agent/core/repository.go server/internal/agent/domain/run.go server/testkit tests/e2e/integration
git commit -m "feat(agent): persist tool steps and invocations"
```

---

### Task 5: Coordinator Tool Loop and Runtime Configuration

**Files:**
- Modify: `server/internal/agent/core/runtime_config.go`
- Modify: `server/internal/agent/core/coordinator.go`
- Modify: `server/internal/agent/core/service.go`
- Modify: `server/internal/agent/core/ports.go`
- Modify: `server/testkit/agent.go`
- Create: `tests/e2e/integration/agent_tools.integration_test.go`

**Interfaces:**

- Consumes: Tasks 1-4 contracts, executor, Today tools, provider turns, and persistence.
- Produces:
  - `RuntimeConfig.ToolsEnabled bool`
  - `RuntimeConfig.MaxModelSteps int`
  - `RuntimeConfig.MaxToolCalls int`
  - `func NewServiceWithTools(repository Repository, provider StreamProvider, registry *ToolRegistry, logger *slog.Logger, config RuntimeConfig) *Service`
  - Error mappings: `AGENT_TOOL_LIMIT_EXCEEDED`, `AGENT_MODEL_STEP_LIMIT_EXCEEDED`, `AGENT_TOOL_CALL_INVALID`, `AGENT_TOOL_EXECUTION_FAILED`, `AGENT_OUTPUT_TRUNCATED`.

- [ ] **Step 1: Add loop integration tests**

Use loopback fake providers as transport fixtures while running the real Registry, Executor, Today adapters, SQLiteStore, and Coordinator. Assert one tool then final answer; all three tools can execute; model receives assistant tool calls followed by tool messages in provider order; tool business error is fed back and the model can answer; cancellation during a tool cancels the Run; model/tool budget failures are stable.

```go
func TestCoordinatorExecutesTodaySearchAndContinuesModel(t *testing.T)
func TestCoordinatorExecutesEveryTodayTool(t *testing.T)
func TestCoordinatorFeedsToolErrorBackToModel(t *testing.T)
func TestCoordinatorEnforcesModelAndToolBudgets(t *testing.T)
func TestCoordinatorCancellationDuringToolDoesNotBecomeToolResult(t *testing.T)
```

- [ ] **Step 2: Run loop tests to verify RED**

Run: `go test ./tests/e2e/integration -run 'TestCoordinator.*Today|TestCoordinatorFeedsToolError|TestCoordinatorEnforces|TestCoordinatorCancellationDuringTool' -count=1`

Expected: FAIL because the Coordinator still performs exactly one provider call and has no tool executor.

- [ ] **Step 3: Implement the bounded serial loop**

Call the model with the registry catalog only when tools are enabled. Count the initial and follow-up calls against `MaxModelSteps`. For each returned call, enforce the total tool budget, create the persisted Step/Invocation, execute serially, persist the result, append `assistant(tool_calls)` and `tool(tool_call_id, content)`, then continue. Stop only on `finish_reason=stop`. Map tool business failures into context and fail the Run only for protocol/persistence/cancellation/budget/internal failures.

- [ ] **Step 4: Run focused loop tests to verify GREEN**

Run: `go test ./tests/e2e/integration -run 'TestCoordinator.*Today|TestCoordinatorFeedsToolError|TestCoordinatorEnforces|TestCoordinatorCancellationDuringTool' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add server/internal/agent/core server/testkit tests/e2e/integration/agent_tools.integration_test.go
git commit -m "feat(agent): execute bounded tool continuation loop"
```

---

### Task 6: Composition Root, Configuration, Regression, and Review

**Files:**
- Modify: `server/cmd/craftingtabled/main.go`
- Modify: `.env.example`
- Modify: `server/testkit/agent.go`
- Modify: `tests/e2e/integration/support_test.go`
- Modify: `docs/requirements.md`

**Interfaces:**

- Consumes: Tasks 1-5.
- Produces: startup-validated environment flags and a fully wired production server.

- [ ] **Step 1: Add configuration tests**

Assert defaults enable tools with limits 4/8; false disables tools; zero, negative, overflow, and non-integer limits reject startup before listening.

```go
func TestAgentToolConfigurationDefaultsAndOverrides(t *testing.T)
func TestAgentToolConfigurationRejectsInvalidValues(t *testing.T)
```

- [ ] **Step 2: Run configuration tests to verify RED**

Run: `go test ./tests/e2e/integration -run 'TestAgentToolConfiguration' -count=1`

Expected: FAIL because the runtime flags and startup parser do not exist.

- [ ] **Step 3: Implement composition and environment parsing**

Create the Today tools before the Agent Service, build the immutable Registry at startup, fail startup on Registry or environment error, pass the real executor into the service, and log whether tools are enabled. Add all three environment variables to `.env.example` and isolated integration environments.

- [ ] **Step 4: Run configuration tests to verify GREEN**

Run: `go test ./tests/e2e/integration -run 'TestAgentToolConfiguration' -count=1`

Expected: PASS.

- [ ] **Step 5: Run backend regression suites**

Run:

```powershell
go test ./server/... ./tests/e2e/integration/... -count=1
go build ./...
go vet ./...
```

Expected: all commands exit 0.

- [ ] **Step 6: Run frontend protocol regression**

Run:

```powershell
pnpm test
pnpm typecheck
```

Expected: Vitest all pass and TypeScript exits 0. No new tool UI is expected.

- [ ] **Step 7: Update requirement status**

Change only AG-01's status to implemented after all focused and full tests pass. Leave the Markdown-rendering TODO and provider-normalization TODO untouched.

- [ ] **Step 8: Final whole-branch review**

Review from the branch merge base. Check each Review Focus item against a named test, inspect migration transaction boundaries, verify provider secrets/internal errors cannot enter context or stream output, and run the full suites once more after any fix.

- [ ] **Step 9: Commit**

```powershell
git add server/cmd/craftingtabled/main.go .env.example server/testkit tests/e2e/integration/support_test.go docs/requirements.md
git commit -m "feat(agent): wire read-only tools into runtime"
```

## Self-Review

- Spec coverage: Registry, schema, executor errors, three Today tools, Provider request/parser, persistence, minimum continuation loop, configuration, integration and frontend regression all have owning tasks.
- Step scan: Each behavior has a named failing test, a focused RED command, a minimal implementation boundary, and a GREEN command.
- Type consistency: `ToolDefinition`, `ToolResult`, `ToolCall`, `ProviderTurn`, Step/Invocation records, and constructor names are defined before consumers.
- Review Focus coverage: fragmented calls are pinned in Task 3; tool failures in Task 1 and Task 5; cancellation in Task 1 and Task 5; budgets in Task 5; timezone/DST in Task 2.
- Deferred scope: approval UI, full budgets, sandbox, provider normalization, Markdown rendering, usage/token display, and provider-specific reasoning metadata remain outside AG-01.
