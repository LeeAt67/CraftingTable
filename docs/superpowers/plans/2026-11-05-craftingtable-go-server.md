# CraftingTable Go Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the temporary Fastify server with a Go modular-monolith server while preserving every current frontend API and SSE contract.

**Architecture:** A single Go module at the repository root owns `server/` and `tests/e2e/integration/`. `server/cmd/craftingtabled` is the composition root; HTTP and SQLite live under `server/internal/platform`, Today owns its domain and use cases under `server/internal/today`, and Agent transport/provider code lives under `server/internal/agent`.

**Tech Stack:** Go 1.26, standard-library `net/http`, `database/sql`, `modernc.org/sqlite`, `log/slog`, and black-box integration tests under `tests/e2e/integration`.

**Status:** Go service migration implemented and verified on 2026-11-05. The Agent endpoint now emits a project-owned, UI Message Stream-compatible SSE subset.

## Global Constraints

- API paths and JSON field names remain unchanged.
- API success responses remain `{ "data": ... }`.
- API failures remain `{ "error": { code, kind, message, retryable, ... } }`.
- Agent SSE is a project-owned subset of UI Message Stream. Follow [Agent 设计 11.1](../specs/2026-11-05-craftingtable-agent-design.md#111-协议选择与三层-pipe-through) for the current contract.
- Treat Vercel AI SDK, `mimo-chat`, and `aisdk-go` as design and test references. Do not import their business layers or complete Agent architecture.
- Use generic packages only for single-purpose infrastructure. `eventsource-parser` may clean SSE; project-owned Adapters must map protocol events and domain state.
- Do not add a general-purpose frontend SSE parser or a third-party message Store.
- Domain errors are values checked with `errors.Is` or `errors.As`; handlers never branch on error strings.
- Every external call accepts `context.Context`.
- Model credentials stay server-side and never enter logs, responses, source files, or frontend bundles.
- Go files use lower_snake_case names; database objects use snake_case.
- `gofmt` and `go vet` are mandatory.
- No real model is called by automated tests.

---

### Task 1: Go Module, Composition Root, and Platform HTTP

**Files:**
- Create: `go.mod`
- Create: `server/cmd/craftingtabled/main.go`
- Create: `server/internal/platform/httpapi/response.go`
- Create: `server/internal/platform/httpapi/request.go`
- Create: `server/internal/platform/httpapi/router.go`
- Create: `server/internal/platform/httpapi/logging.go`

**Interfaces:**
- Consumes: environment variables and injected route registrars.
- Produces: `httpapi.NewRouter`, `httpapi.OK`, `httpapi.Fail`, `httpapi.ReadJSON`, and `httpapi.RequestID`.

- [ ] **Step 1: Create the root module and define the composition-root package.**

```go
module github.com/leeat/craftingtable

go 1.26
```

- [ ] **Step 2: Implement stable JSON envelopes and bounded JSON request decoding.**

```go
type ErrorBody struct {
    Code      string         `json:"code"`
    Kind      string         `json:"kind"`
    Message   string         `json:"message"`
    Retryable bool           `json:"retryable"`
    RequestID string         `json:"requestId,omitempty"`
    Details   map[string]any `json:"details,omitempty"`
}
```

- [ ] **Step 3: Add `/healthz`, request IDs, panic recovery, and request logging.**

- [ ] **Step 4: Run `gofmt` and `go test ./server/...`.**

### Task 2: SQLite Today Repository and HTTP Endpoints

**Files:**
- Create: `server/internal/today/item.go`
- Create: `server/internal/today/repository.go`
- Create: `server/internal/today/controller.go`
- Create: `server/internal/platform/store/sqlite_store.go`
- Create: `server/internal/platform/store/migrations/001_initial.sql`

**Interfaces:**
- Consumes: `context.Context` and validated Today inputs.
- Produces: `today.Repository`, `today.NewController`, and `store.OpenSQLite`.

- [ ] **Step 1: Define domain types and sentinel errors.**

```go
var (
    ErrItemNotFound = errors.New("item not found")
    ErrInvalidInput = errors.New("invalid item input")
)
```

- [ ] **Step 2: Port the existing SQLite schema and seed categories unchanged.**

- [ ] **Step 3: Implement Today, category, create, update, complete, and uncomplete repository operations.**

- [ ] **Step 4: Implement the HTTP controller with the existing camelCase contract and validation limits.**

- [ ] **Step 5: Add pure package tests for status selection and timestamp-independent mapping behavior.**

- [ ] **Step 6: Run `go test ./server/...` and `go vet ./server/...`.**

### Task 3: OpenAI-Compatible Agent Stream

**Files:**
- Create: `server/internal/agent/domain/error.go`
- Create: `server/internal/agent/provider/openai_compatible.go`
- Create: `server/internal/agent/core/stream_parser.go`
- Create: `server/internal/agent/access/stream_controller.go`
- Create: `server/internal/agent/core/stream_parser_test.go`

**Interfaces:**
- Consumes: bounded chat messages, request context, and provider configuration.
- Produces: normalized SSE through `POST /api/v1/agent/stream`.

- [ ] **Step 1: Define provider configuration aliases.**

```text
CRAFTINGTABLE_AGENT_BASE_URL
CRAFTINGTABLE_AGENT_API_KEY
CRAFTINGTABLE_AGENT_MODEL
CRAFTINGTABLE_AGENT_SYSTEM_PROMPT
```

The legacy `AGENT_API_BASE_URL`, `AGENT_API_KEY`, and `AGENT_MODEL` names remain supported.

- [ ] **Step 2: Implement an OpenAI-compatible streaming provider using `http.NewRequestWithContext`.**

- [ ] **Step 3: Parse provider SSE incrementally, including `[DONE]`, `finish_reason`, and a final event without a blank line.**

- [ ] **Step 4: Emit the project-owned UI Message Stream subset: `start`, step events, `text-*`, `finish`, structured `error`, and `[DONE]`.**

- [ ] **Step 5: Keep request-context cancellation connected to the upstream request so client disconnect cancels generation.**

- [ ] **Step 6: Test normal completion, final event without a blank line, finish reason, malformed JSON, structured errors, and unexpected EOF.**

- [ ] **Step 7: Run `go test ./server/...` and `go vet ./server/...`.**

### Task 4: Composition and Development Startup

**Files:**
- Modify: `server/cmd/craftingtabled/main.go`
- Modify: `package.json`
- Modify: `.env.example`
- Modify: `.env.e2e`
- Modify: `README.md`
- Delete after parity: `api/src/**`, `api/e2eServer.ts`, `api/migrations/001Initial.sql`

**Interfaces:**
- Consumes: `API_PORT`, `CRAFTINGTABLE_DATABASE_PATH` (with `LEEAT_DATABASE_PATH` fallback), and Agent environment variables.
- Produces: one process on `127.0.0.1` for Vite and Playwright.

- [ ] **Step 1: Compose the SQLite store, Today controller, and Agent controller in `main.go`.**

- [ ] **Step 2: Switch `dev:api` and `e2e:api` to `go run ./server/cmd/craftingtabled`.**

- [ ] **Step 3: Recreate the E2E database before every test server start.**

- [ ] **Step 4: Remove Fastify, the old Node API source, and the now-unused `node:sqlite` test.**

- [ ] **Step 5: Remove Fastify and `tsx` from package dependencies after `pnpm typecheck` and Vitest pass.**

### Task 5: Black-Box Integration Verification

**Files:**
- Create: `tests/e2e/integration/support_test.go`
- Create: `tests/e2e/integration/today.integration_test.go`
- Create: `tests/e2e/integration/agent.integration_test.go`

**Interfaces:**
- Consumes: real `craftingtabled` executable over loopback HTTP.
- Produces: proof of Today persistence and Agent stream behavior without importing `server/internal`.

- [ ] **Step 1: Build one temporary server binary for the integration package.**

- [ ] **Step 2: Start each test server on a free loopback port with a temporary SQLite path.**

- [ ] **Step 3: Exercise all Today endpoints through HTTP and assert camelCase responses.**

- [ ] **Step 4: Use a loopback fake OpenAI-compatible provider to assert normalized SSE.**

- [ ] **Step 5: Disconnect a client during streaming and assert the fake provider request context is cancelled.**

- [ ] **Step 6: Run the full validation matrix.**

```text
gofmt -w server tests/e2e/integration
go test ./server/... ./tests/e2e/integration/...
go vet ./server/... ./tests/e2e/integration/...
pnpm typecheck
pnpm test
pnpm test:e2e
pnpm build
```
