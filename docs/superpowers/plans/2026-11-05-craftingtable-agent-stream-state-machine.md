# CraftingTable Agent Stream State Machine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用 Schema Parser、纯状态 Reducer 和薄 SSE Adapter 重构 Agent 前端流协议，使生命周期状态只有一个事实源。

**Architecture:** `agentStreamSchema.ts` 负责协议结构和已知/未知事件分类；`agentStreamReducer.ts` 负责 `awaiting-start -> streaming -> terminal` 状态转换；`agentStreamProtocol.ts` 只组合 SSE、JSON、Parser、Reducer 和异步迭代。`apiAgentModel.ts` 只消费已验证事件。

**Tech Stack:** TypeScript strict、React 19、Zustand、Zod、`eventsource-parser`、Vitest。

## Global Constraints

- 先设计、再计划、再失败测试、再最小实现，遵循 SDD。
- `readAgentStream(body: ReadableStream<Uint8Array>): AsyncIterable<AgentStreamEvent>` 的公开签名保持不变。
- `AgentStreamEvent` 仍只有 `text-delta`、`finish` 和 `error` 三种类型。
- 不修改 Go SSE 输出、HTTP 路由或请求体。
- 不新增 Vercel AI SDK、`aisdk-go` 或其他流框架依赖。
- `eventsource-parser` 只负责 SSE 分帧。
- 项目协议错误以 `Result<T>` 在纯函数边界返回，I/O 边界再转换为现有 `agentRequestError`。
- Zustand 仍是前端业务状态唯一事实源。
- 所有测试统一放在 `tests/e2e/unit/agent/`。
- 每个行为先写失败测试并观察失败原因。
- 每个任务独立通过定向测试后提交。
- 最终验证命令：`pnpm test`、`pnpm build`、`pnpm test:go`、`go vet ./server/...`、`gofmt -d` 和 `git diff --check`。

---

## File Structure

- Create: `src/models/agentStreamSchema.ts`
  - Zod Schema Registry、`AgentStreamPart`、`AgentStreamEvent`、已知/未知事件分类和协议错误值。
- Create: `src/models/agentStreamReducer.ts`
  - 协议状态、纯 Reducer、事件产出和 EOF 完整性检查。
- Modify: `src/models/agentStreamProtocol.ts`
  - 保留为薄 I/O 组合层，不再内嵌生命周期分支。
- Modify: `src/models/apiAgentModel.ts`
  - 删除 `finished` 标记和 EOF 猜测。
- Create: `tests/e2e/unit/agent/agentStreamSchema.test.ts`
  - Parser 对已知、未知和畸形 chunk 的分类。
- Create: `tests/e2e/unit/agent/agentStreamReducer.test.ts`
  - 状态转换矩阵和错误值。
- Modify: `tests/e2e/unit/agent/agentStreamProtocol.test.ts`
  - SSE Adapter 的 EOF、终止和未知事件行为。
- Modify: `docs/superpowers/specs/2026-11-05-craftingtable-agent-stream-state-machine-design.md`
  - 实施完成后更新状态与验证结果。

### Task 1: Protocol Schema Parser

**Files:**
- Create: `src/models/agentStreamSchema.ts`
- Create: `tests/e2e/unit/agent/agentStreamSchema.test.ts`

**Interfaces:**
- Consumes: `agentErrorSchema`、`AgentError` 和 `Result<T>` from `src/models/agentContracts.ts`.
- Produces:
  - `AgentStreamPart`
  - `AgentStreamEvent`
  - `ParsedAgentStreamPart`
  - `parseAgentStreamPart(value: unknown): Result<ParsedAgentStreamPart>`
  - `invalidProtocolError: AgentError`
  - `unexpectedEndError: AgentError`

- [ ] **Step 1: Write the failing schema parser tests**

```ts
import { describe, expect, it } from "vitest";
import { parseAgentStreamPart } from "@/models/agentStreamSchema";

describe("agentStreamSchema", () => {
  it("classifies an unknown chunk without exposing it as a domain event", () => {
    const [parsed, error] = parseAgentStreamPart({
      type: "future-warning",
      message: "not part of the current protocol",
    });

    expect(error).toBeNull();
    expect(parsed).toEqual({ kind: "unknown" });
  });

  it("returns a stable error value for a malformed known chunk", () => {
    const [parsed, error] = parseAgentStreamPart({
      type: "text-delta",
      id: "part-1",
    });

    expect(parsed).toBeNull();
    expect(error).toMatchObject({
      code: "PROTOCOL_INVALID_METADATA",
      kind: "protocol",
      retryable: false,
    });
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentStreamSchema.test.ts
```

Expected: FAIL because `@/models/agentStreamSchema` does not exist.

- [ ] **Step 3: Implement the Schema Parser**

```ts
import { z } from "zod";
import {
  agentErrorSchema,
  type AgentError,
  type Result,
} from "@/models/agentContracts";

const startPartSchema = z.object({
  type: z.literal("start"),
  messageId: z.string().min(1),
  messageMetadata: z
    .object({
      sessionId: z.string().min(1),
      runId: z.string().min(1),
      userMessageId: z.string().min(1),
      assistantMessageId: z.string().min(1).optional(),
    })
    .passthrough(),
});

const startStepPartSchema = z.object({ type: z.literal("start-step") });
const textStartPartSchema = z.object({
  type: z.literal("text-start"),
  id: z.string().min(1),
});
const textDeltaPartSchema = z.object({
  type: z.literal("text-delta"),
  id: z.string().min(1),
  delta: z.string(),
});
const textEndPartSchema = z.object({
  type: z.literal("text-end"),
  id: z.string().min(1),
});
const finishStepPartSchema = z.object({ type: z.literal("finish-step") });
const finishPartSchema = z.object({
  type: z.literal("finish"),
  finishReason: z.string().optional(),
  messageMetadata: z
    .object({
      runId: z.string().min(1),
      status: z.string().min(1),
    })
    .passthrough(),
});
const errorPartSchema = z.object({
  type: z.literal("error"),
  errorText: z.string(),
  agentError: agentErrorSchema.optional(),
});

const streamPartSchema = z.discriminatedUnion("type", [
  startPartSchema,
  startStepPartSchema,
  textStartPartSchema,
  textDeltaPartSchema,
  textEndPartSchema,
  finishStepPartSchema,
  finishPartSchema,
  errorPartSchema,
]);

const knownPartTypes = new Set([
  "start",
  "start-step",
  "text-start",
  "text-delta",
  "text-end",
  "finish-step",
  "finish",
  "error",
]);

export type AgentStreamPart = z.infer<typeof streamPartSchema>;

export type AgentStreamEvent =
  | { type: "text-delta"; id: string; delta: string }
  | {
      type: "finish";
      finishReason?: string;
      runId?: string;
      status?: string;
    }
  | { type: "error"; error: AgentError };

export type ParsedAgentStreamPart =
  | { kind: "known"; part: AgentStreamPart }
  | { kind: "unknown" };

export const invalidProtocolError: AgentError = {
  code: "PROTOCOL_INVALID_METADATA",
  kind: "protocol",
  message: "模型服务返回了无效事件",
  retryable: false,
};

export const unexpectedEndError: AgentError = {
  code: "PROTOCOL_UNEXPECTED_EOF",
  kind: "protocol",
  message: "模型响应意外中断",
  retryable: true,
};

function hasPartType(value: unknown): value is { type: string } {
  return (
    !!value &&
    typeof value === "object" &&
    "type" in value &&
    typeof value.type === "string"
  );
}

export function parseAgentStreamPart(
  value: unknown,
): Result<ParsedAgentStreamPart> {
  const parsed = streamPartSchema.safeParse(value);
  if (!parsed.success) {
    if (hasPartType(value) && !knownPartTypes.has(value.type)) {
      return [{ kind: "unknown" }, null];
    }
    return [null, invalidProtocolError];
  }
  return [{ kind: "known", part: parsed.data }, null];
}
```

- [ ] **Step 4: Run the test and verify it passes**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentStreamSchema.test.ts
```

Expected: 2 tests pass.

- [ ] **Step 5: Commit**

```powershell
git add src/models/agentStreamSchema.ts tests/e2e/unit/agent/agentStreamSchema.test.ts
git commit -m "refactor: extract agent stream schema parser"
```

### Task 2: Pure Protocol Reducer

**Files:**
- Create: `src/models/agentStreamReducer.ts`
- Create: `tests/e2e/unit/agent/agentStreamReducer.test.ts`

**Interfaces:**
- Consumes:
  - `AgentStreamPart` and `AgentStreamEvent` from Task 1.
  - `invalidProtocolError` and `unexpectedEndError` from Task 1.
  - `Result<T>` from `src/models/agentContracts.ts`.
- Produces:
  - `AgentStreamPhase`
  - `AgentStreamState`
  - `createAgentStreamState(): AgentStreamState`
  - `reduceAgentStream(state, part): Result<AgentStreamTransition>`
  - `completeAgentStream(state): Result<null>`

- [ ] **Step 1: Write the failing reducer tests**

```ts
import { describe, expect, it } from "vitest";
import type { AgentStreamPart } from "@/models/agentStreamSchema";
import {
  completeAgentStream,
  createAgentStreamState,
  reduceAgentStream,
} from "@/models/agentStreamReducer";

const startPart = {
  type: "start",
  messageId: "message-1",
  messageMetadata: {
    sessionId: "session-1",
    runId: "run-1",
    userMessageId: "user-1",
  },
} satisfies AgentStreamPart;

describe("agentStreamReducer", () => {
  it("moves from awaiting-start to streaming on start", () => {
    const [transition, error] = reduceAgentStream(
      createAgentStreamState(),
      startPart,
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "streaming" },
      events: [],
    });
  });

  it("emits text deltas while remaining in streaming", () => {
    const [transition, error] = reduceAgentStream(
      { phase: "streaming" },
      { type: "text-delta", id: "part-1", delta: "你好" },
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "streaming" },
      events: [{ type: "text-delta", id: "part-1", delta: "你好" }],
    });
  });

  it("moves to terminal and emits finish", () => {
    const [transition, error] = reduceAgentStream(
      { phase: "streaming" },
      {
        type: "finish",
        finishReason: "stop",
        messageMetadata: { runId: "run-1", status: "completed" },
      },
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "terminal" },
      events: [
        {
          type: "finish",
          finishReason: "stop",
          runId: "run-1",
          status: "completed",
        },
      ],
    });
  });

  it("rejects a second terminal event", () => {
    const [, error] = reduceAgentStream(
      { phase: "terminal" },
      {
        type: "finish",
        finishReason: "stop",
        messageMetadata: { runId: "run-1", status: "completed" },
      },
    );

    expect(error).toMatchObject({
      code: "PROTOCOL_INVALID_METADATA",
      kind: "protocol",
      retryable: false,
    });
  });

  it("reports unexpected EOF unless terminal was reached", () => {
    expect(completeAgentStream({ phase: "streaming" })).toEqual([
      null,
      expect.objectContaining({ code: "PROTOCOL_UNEXPECTED_EOF" }),
    ]);
    expect(completeAgentStream({ phase: "terminal" })).toEqual([null, null]);
  });

  it("maps a structured error chunk to a terminal error event", () => {
    const [transition, error] = reduceAgentStream(
      { phase: "streaming" },
      {
        type: "error",
        errorText: "模型生成失败",
        agentError: {
          code: "AGENT_PROVIDER_FAILED",
          kind: "server",
          message: "模型生成失败",
          retryable: true,
        },
      },
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "terminal" },
      events: [
        {
          type: "error",
          error: {
            code: "AGENT_PROVIDER_FAILED",
            kind: "server",
            message: "模型生成失败",
            retryable: true,
          },
        },
      ],
    });
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentStreamReducer.test.ts
```

Expected: FAIL because `@/models/agentStreamReducer` does not exist.

- [ ] **Step 3: Implement the pure Reducer**

```ts
import type { Result } from "@/models/agentContracts";
import {
  invalidProtocolError,
  unexpectedEndError,
  type AgentStreamEvent,
  type AgentStreamPart,
} from "@/models/agentStreamSchema";

export type AgentStreamPhase =
  | "awaiting-start"
  | "streaming"
  | "terminal";

export type AgentStreamState = {
  phase: AgentStreamPhase;
};

export type AgentStreamTransition = {
  state: AgentStreamState;
  events: AgentStreamEvent[];
};

export function createAgentStreamState(): AgentStreamState {
  return { phase: "awaiting-start" };
}

function invalid<T>(): Result<T> {
  return [null, invalidProtocolError];
}

export function reduceAgentStream(
  state: AgentStreamState,
  part: AgentStreamPart,
): Result<AgentStreamTransition> {
  if (state.phase === "terminal") {
    return invalid();
  }

  if (state.phase === "awaiting-start") {
    if (part.type !== "start") {
      return invalid();
    }
    return [{ state: { phase: "streaming" }, events: [] }, null];
  }

  switch (part.type) {
    case "start":
      return invalid();
    case "start-step":
    case "text-start":
    case "text-end":
    case "finish-step":
      return [{ state, events: [] }, null];
    case "text-delta":
      return [
        {
          state,
          events: [
            { type: "text-delta", id: part.id, delta: part.delta },
          ],
        },
        null,
      ];
    case "finish":
      return [
        {
          state: { phase: "terminal" },
          events: [
            {
              type: "finish",
              finishReason: part.finishReason,
              runId: part.messageMetadata.runId,
              status: part.messageMetadata.status,
            },
          ],
        },
        null,
      ];
    case "error":
      return [
        {
          state: { phase: "terminal" },
          events: [
            {
              type: "error",
              error: part.agentError ?? {
                code: "AGENT_PROVIDER_FAILED",
                kind: "server",
                message: part.errorText,
                retryable: true,
              },
            },
          ],
        },
        null,
      ];
  }
}

export function completeAgentStream(
  state: AgentStreamState,
): Result<null> {
  return state.phase === "terminal"
    ? [null, null]
    : [null, unexpectedEndError];
}
```

- [ ] **Step 4: Run the test and verify it passes**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentStreamReducer.test.ts
```

Expected: 6 tests pass.

- [ ] **Step 5: Commit**

```powershell
git add src/models/agentStreamReducer.ts tests/e2e/unit/agent/agentStreamReducer.test.ts
git commit -m "refactor: add pure agent stream reducer"
```

### Task 3: Compose SSE Adapter and Simplify API Model

**Files:**
- Modify: `src/models/agentStreamProtocol.ts`
- Modify: `src/models/apiAgentModel.ts`
- Modify: `tests/e2e/unit/agent/agentStreamProtocol.test.ts`

**Interfaces:**
- Consumes:
  - `parseAgentStreamPart` from Task 1.
  - `createAgentStreamState`、`reduceAgentStream`、`completeAgentStream` from Task 2.
- Produces:
  - Unchanged `readAgentStream(body): AsyncIterable<AgentStreamEvent>`.
  - `apiAgentModel` no longer contains a local `finished` flag.

- [ ] **Step 1: Add failing EOF and `[DONE]` tests**

Append to `tests/e2e/unit/agent/agentStreamProtocol.test.ts`:

```ts
it("rejects [DONE] before a terminal event", async () => {
  const stream = streamOf(
    [
      'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}',
      "data: [DONE]",
      "",
      "",
    ].join("\n\n"),
  );

  await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
    agentError: {
      code: "PROTOCOL_UNEXPECTED_EOF",
      kind: "protocol",
      retryable: true,
    },
  });
});

it("rejects EOF before a terminal event", async () => {
  const stream = streamOf(
    'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}\n\n',
  );

  await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
    agentError: {
      code: "PROTOCOL_UNEXPECTED_EOF",
      kind: "protocol",
      retryable: true,
    },
  });
});
```

- [ ] **Step 2: Run the tests and verify the new cases fail**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentStreamProtocol.test.ts
```

Expected: the two new tests FAIL because `readAgentStream` currently returns normally without a terminal event.

- [ ] **Step 3: Replace the protocol implementation**

```ts
import { EventSourceParserStream } from "eventsource-parser/stream";
import { agentRequestError } from "@/models/agentTransport";
import {
  completeAgentStream,
  createAgentStreamState,
  reduceAgentStream,
} from "@/models/agentStreamReducer";
import {
  invalidProtocolError,
  parseAgentStreamPart,
  type AgentStreamEvent,
} from "@/models/agentStreamSchema";

export type { AgentStreamEvent } from "@/models/agentStreamSchema";

export async function* readAgentStream(
  body: ReadableStream<Uint8Array>,
): AsyncIterable<AgentStreamEvent> {
  let state = createAgentStreamState();
  const eventStream = body
    .pipeThrough(
      new TextDecoderStream() as unknown as TransformStream<
        Uint8Array,
        string
      >,
    )
    .pipeThrough(
      new EventSourceParserStream({
        maxBufferSize: 1_048_576,
        onError: "terminate",
      }),
    )
    .getReader();

  try {
    while (true) {
      const { value, done } = await eventStream.read();
      if (done || value?.data === "[DONE]") {
        const [, completionError] = completeAgentStream(state);
        if (completionError) {
          throw agentRequestError(completionError);
        }
        return;
      }
      if (!value) {
        continue;
      }

      let data: unknown;
      try {
        data = JSON.parse(value.data);
      } catch {
        throw agentRequestError(invalidProtocolError);
      }

      const [parsed, parseError] = parseAgentStreamPart(data);
      if (parseError) {
        throw agentRequestError(parseError);
      }
      if (parsed.kind === "unknown") {
        continue;
      }

      const [transition, reduceError] = reduceAgentStream(
        state,
        parsed.part,
      );
      if (reduceError) {
        throw agentRequestError(reduceError);
      }

      state = transition.state;
      for (const event of transition.events) {
        yield event;
      }
    }
  } finally {
    eventStream.releaseLock();
  }
}
```

- [ ] **Step 4: Run the protocol test and verify it passes**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentStreamProtocol.test.ts
```

Expected: all existing and new protocol tests pass.

- [ ] **Step 5: Write a failing API Model expectation for protocol-owned EOF**

Append to `tests/e2e/unit/agent/apiAgentModel.test.ts`:

```ts
it("maps protocol EOF to a stable model error without local terminal tracking", async () => {
  const encoder = new TextEncoder();
  const model = createApiAgentModel({
    baseUrl: "/api/v1",
    fetchImpl: async (_input, init) => {
      const requestBody = JSON.parse(String(init?.body)) as {
        assistantMessageId: string;
        sessionId: string;
        userMessageId: string;
        runId: string;
      };
      return new Response(
        new ReadableStream({
          start(controller) {
            controller.enqueue(
              encoder.encode(
                `data: ${JSON.stringify({
                  type: "start",
                  messageId: requestBody.assistantMessageId,
                  messageMetadata: {
                    sessionId: requestBody.sessionId,
                    runId: requestBody.runId,
                    userMessageId: requestBody.userMessageId,
                  },
                })}\n\n`,
              ),
            );
            controller.close();
          },
        }),
        { status: 200 },
      );
    },
  });
  const [created] = await model.createSession({ clientRequestId: "request-1" });

  const [result, error] = await model.sendMessage(
    {
      sessionId: created!.session.id,
      clientRequestId: "send-1",
      message: {
        id: "message-1",
        role: "user",
        parts: [{ type: "text", text: "你好" }],
      },
      signal: new AbortController().signal,
    },
    { onStarted: () => undefined, onSnapshot: () => undefined },
  );

  expect(result).toBeNull();
  expect(error).toMatchObject({
    code: "PROTOCOL_UNEXPECTED_EOF",
    kind: "protocol",
    retryable: true,
  });
});
```

- [ ] **Step 6: Run the API Model test and verify it fails**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/apiAgentModel.test.ts
```

Expected: FAIL because `apiAgentModel.ts` still imports and calls `agentStreamUnexpectedEnd`.

- [ ] **Step 7: Remove the local `finished` flag and rely on the protocol**

Replace the stream loop in `apiAgentModel.ts` with:

```ts
for await (const event of readAgentStream(body)) {
  if (event.type === "text-delta") {
    yield event.delta;
  } else if (event.type === "error") {
    throw agentRequestError(event.error);
  }
}
```

Remove `agentStreamUnexpectedEnd` from imports and delete the `let finished = false` block.

- [ ] **Step 8: Run the focused and full frontend tests**

Run:

```powershell
pnpm vitest run tests/e2e/unit/agent/agentStreamSchema.test.ts tests/e2e/unit/agent/agentStreamReducer.test.ts tests/e2e/unit/agent/agentStreamProtocol.test.ts tests/e2e/unit/agent/apiAgentModel.test.ts
pnpm test
```

Expected: all tests pass.

- [ ] **Step 9: Commit**

```powershell
git add src/models/agentStreamProtocol.ts src/models/apiAgentModel.ts tests/e2e/unit/agent/agentStreamProtocol.test.ts tests/e2e/unit/agent/apiAgentModel.test.ts
git commit -m "refactor: centralize agent stream lifecycle"
```

### Task 4: Documentation Status and Full Verification

**Files:**
- Modify: `docs/superpowers/specs/2026-11-05-craftingtable-agent-stream-state-machine-design.md`

**Interfaces:**
- Consumes: the implemented modules and passing tests from Tasks 1-3.
- Produces: design status `Implemented`, verification record, and clean final worktree.

- [ ] **Step 1: Update the design status**

Change:

```markdown
- **Status:** Proposed for review
```

to:

```markdown
- **Status:** Implemented and verified on 2026-11-05
```

Add a final section:

```markdown
## 16. Implementation Record

- Schema Parser: `src/models/agentStreamSchema.ts`.
- Pure Reducer: `src/models/agentStreamReducer.ts`.
- SSE Adapter: `src/models/agentStreamProtocol.ts`.
- Domain adapter: `src/models/apiAgentModel.ts`.
- Protocol tests: `tests/e2e/unit/agent/agentStreamSchema.test.ts`, `agentStreamReducer.test.ts`, `agentStreamProtocol.test.ts`.
- Full verification passed with `pnpm test`, `pnpm build`, `pnpm test:go`, `go vet ./server/...`, `gofmt -d`, and `git diff --check`.
```

- [ ] **Step 2: Run full verification**

Run:

```powershell
pnpm test
pnpm build
pnpm test:go
go vet ./server/...
gofmt -d server/internal/agent/access/stream_controller.go tests/e2e/integration/agent.integration_test.go
git diff --check
```

Expected:

- Every command exits with code 0.
- `gofmt -d` prints nothing.
- `git diff --check` prints no whitespace errors.
- Vite may retain the existing chunk-size warning.

- [ ] **Step 3: Commit**

```powershell
git add docs/superpowers/specs/2026-11-05-craftingtable-agent-stream-state-machine-design.md
git commit -m "docs: record agent stream state machine implementation"
```

## Self-Review

- Spec coverage: Schema Registry、State/Reducer、EOF 完整性、错误值、`apiAgentModel` 职责收缩和 TDD 均有对应任务。
- Placeholder scan: 所有任务包含具体文件、失败命令、预期失败原因、实现代码和通过命令。
- Type consistency: `AgentStreamPart` 在 Task 1 定义；Reducer 在 Task 2 消费；Adapter 在 Task 3 组合；所有名称与设计文档一致。
