# CraftingTable Agent UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付可交互的 CraftingTable Agent 工作区，包含一级入口、常驻会话侧栏、会话页面、消息输入与可替换的流式 AgentModel。

**Architecture:** 保持现有 React、Zustand 和 MVC 边界，View 只读取 Store 并调用 Effect，Effect 通过 AgentModel 获取数据。首阶段使用进程内 FakeAgentModel 验证 UI、交互和状态机，模型接口与后续 Go sidecar、AI SDK Transport 保持相同 Result 语义。

**Tech Stack:** React 19、TypeScript strict、Vite、ShadcnUI、Tailwind CSS、Zustand、React Router、Lucide React、Vitest、Testing Library、Playwright。

## Global Constraints

- 项目名固定为 `CraftingTable`；`leeat` 只表示用户身份。
- 一级导航固定为首页、Agent、设置。
- Agent 页面必须有独立且常驻的二级会话侧栏。
- Zustand 是前端业务状态唯一事实源，不使用 `useChat` 保存业务状态。
- Model 和 Effect 边界使用 `Result<T>`，不把网络或协议异常抛给 View。
- Store action 只执行同步状态变更；请求、流消费、取消和 Toast 由 Effect 处理。
- 颜色只使用 `src/styles/tokens.css` 的语义 Token。
- 390px、768px、1024px 和 1440px 不出现全局横向滚动。
- 产品行为、API、契约和桌面验收测试统一放在 `tests/e2e`。
- 本计划只交付 Agent 前端纵向切片；Go sidecar、SQLite、Electron IPC 和真实 SSE 不在本计划内。
- FakeAgentModel 只能作为开发和测试数据源，不能成为生产默认配置。

## File Structure

```text
src/
  components/
    agent/
      agentComposer.tsx
      agentEmptyState.tsx
      agentMessageList.tsx
      agentSidebar.tsx
  layouts/
    agentWorkspaceLayout.tsx
    todayWorkspaceLayout.tsx
  pages/
    agent/
      index.tsx
  controllers/
    effects/
      useAgentEffect.ts
    stores/
      agentStore.ts
  models/
    agentContracts.ts
    agentModel.ts
    fakeAgentModel.ts
tests/e2e/
  unit/
    agent/
      agentStore.test.ts
      fakeAgentModel.test.ts
    app/
      appRouter.test.tsx
  modules/
    agent/
      navigation.e2e.ts
      sessionSidebar.e2e.ts
      chat.e2e.ts
      responsive.e2e.ts
```

## Task 1: Agent Domain Contracts and Fake Model

**Files:**
- Create: `src/models/agentContracts.ts`
- Create: `src/models/agentModel.ts`
- Create: `src/models/fakeAgentModel.ts`
- Test: `tests/e2e/unit/agent/fakeAgentModel.test.ts`

**Interfaces:**

```ts
type Result<T> =
  | readonly [data: T, error: null]
  | readonly [data: null, error: AgentError]

type AgentModel = {
  listSessions(): Promise<Result<AgentSession[]>>
  getSession(sessionId: string): Promise<Result<AgentSessionDetail>>
  createSession(input: {
    clientRequestId: string
  }): Promise<Result<{ session: AgentSession }>>
  sendMessage(
    input: AgentSendInput,
    handlers: AgentSendHandlers,
  ): Promise<Result<AgentSendResult>>
  cancelRun(input: {
    sessionId: string
    runId: string
  }): Promise<Result<{ runId: string; status: AgentRunStatus }>>
}
```

- [ ] **Step 1: Write the failing model tests**

```ts
import { describe, expect, it } from "vitest";
import { createFakeAgentModel } from "@/models/fakeAgentModel";

describe("FakeAgentModel", () => {
  it("creates a session and returns it in the session list", async () => {
    const model = createFakeAgentModel();
    const [created, createError] = await model.createSession({
      clientRequestId: "create-1",
    });

    expect(createError).toBeNull();
    expect(created?.session.title).toBe("新会话");

    const [sessions, listError] = await model.listSessions();
    expect(listError).toBeNull();
    expect(sessions?.map((session) => session.id)).toContain(
      created?.session.id,
    );
  });

  it("streams assistant text and persists the completed message", async () => {
    const model = createFakeAgentModel({ chunkDelayMs: 0 });
    const [created] = await model.createSession({ clientRequestId: "create-1" });
    const snapshots: string[] = [];

    const [result, error] = await model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "user-message-1",
          role: "user",
          parts: [{ type: "text", text: "整理今天的重点" }],
        },
        signal: new AbortController().signal,
      },
      {
        onStarted: () => undefined,
        onSnapshot: (snapshot) => {
          snapshots.push(snapshot.parts.map((part) => part.content.text).join(""));
        },
      },
    );

    expect(error).toBeNull();
    expect(result?.status).toBe("completed");
    expect(snapshots.at(-1)).toContain("整理今天的重点");

    const [detail, detailError] = await model.getSession(created!.session.id);
    expect(detailError).toBeNull();
    expect(detail?.messages.at(-1)?.role).toBe("assistant");
  });
});
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `pnpm vitest run tests/e2e/unit/agent/fakeAgentModel.test.ts`

Expected: FAIL because `@/models/fakeAgentModel` does not exist.

- [ ] **Step 3: Implement the domain types and model interface**

Create `AgentSession`, `AgentMessage`, `AgentMessagePart`, `AgentRunStatus`, `AgentSendInput`, `AgentSendHandlers`, `AgentSendResult`, `AgentSessionDetail`, `AgentError`, `Result<T>` and `AgentModel` in `src/models/agentContracts.ts` and `src/models/agentModel.ts`.

- [ ] **Step 4: Implement the in-memory FakeAgentModel**

The model must:

- Keep sessions and messages in memory.
- Use `chunkDelayMs=12` by default and allow tests to set `0`.
- Persist the user message before emitting assistant snapshots.
- Emit at least three assistant text chunks.
- Return `completed`, `cancelled` or `failed` in `AgentSendResult`.
- Respect the supplied `AbortSignal`.
- Return `[data, null]` or `[null, AgentError]`, never throw for expected domain failures.

- [ ] **Step 5: Run the model tests and verify GREEN**

Run: `pnpm vitest run tests/e2e/unit/agent/fakeAgentModel.test.ts`

Expected: PASS.

## Task 2: Agent Store

**Files:**
- Create: `src/controllers/stores/agentStore.ts`
- Test: `tests/e2e/unit/agent/agentStore.test.ts`

**Interfaces:**

```ts
type AgentState = {
  sessions: AgentSession[]
  activeSessionId: string | null
  messagesBySessionId: Record<string, AgentMessage[]>
  partsByMessageId: Record<string, AgentMessagePart[]>
  sendPhase: "idle" | "submitting" | "streaming" | "error"
  liveRun: { sessionId: string; runId: string } | null
  error: AgentError | null
}
```

- [ ] **Step 1: Write the failing Store tests**

```ts
import { beforeEach, describe, expect, it } from "vitest";
import {
  resetAgentStore,
  useAgentStore,
} from "@/controllers/stores/agentStore";

describe("AgentStore", () => {
  beforeEach(resetAgentStore);

  it("stores messages per session and merges parts by message and part id", () => {
    useAgentStore.getState().appendMessage("session-1", {
      id: "message-1",
      sessionId: "session-1",
      seq: 1,
      role: "assistant",
      status: "streaming",
      createdAt: "2026-11-05T00:00:00.000Z",
      updatedAt: "2026-11-05T00:00:00.000Z",
    });
    useAgentStore.getState().upsertMessagePart("message-1", {
      id: "part-1",
      messageId: "message-1",
      runId: "run-1",
      index: 0,
      type: "text",
      status: "streaming",
      content: { type: "text", text: "你" },
      createdAt: "2026-11-05T00:00:00.000Z",
      updatedAt: "2026-11-05T00:00:00.000Z",
    });
    useAgentStore.getState().upsertMessagePart("message-1", {
      id: "part-1",
      messageId: "message-1",
      runId: "run-1",
      index: 0,
      type: "text",
      status: "complete",
      content: { type: "text", text: "你好" },
      createdAt: "2026-11-05T00:00:00.000Z",
      updatedAt: "2026-11-05T00:00:01.000Z",
    });

    expect(
      useAgentStore.getState().partsByMessageId["message-1"],
    ).toHaveLength(1);
    expect(
      useAgentStore.getState().partsByMessageId["message-1"][0].content.text,
    ).toBe("你好");
  });
});
```

- [ ] **Step 2: Run the test and verify RED**

Run: `pnpm vitest run tests/e2e/unit/agent/agentStore.test.ts`

Expected: FAIL because `@/controllers/stores/agentStore` does not exist.

- [ ] **Step 3: Implement synchronous Store actions**

Add `setSessions`, `upsertSession`, `setActiveSession`, `setSessionMessages`, `appendMessage`, `upsertMessage`, `upsertMessagePart`, `setSendPhase`, `setLiveRun`, `setError` and `resetAgentStore`.

- [ ] **Step 4: Run the Store test and verify GREEN**

Run: `pnpm vitest run tests/e2e/unit/agent/agentStore.test.ts`

Expected: PASS.

## Task 3: Agent Effect

**Files:**
- Create: `src/controllers/effects/useAgentEffect.ts`
- Modify: `src/models/agentModel.ts`
- Test: `tests/e2e/unit/agent/agentEffect.test.ts`

**Interfaces:**

- Consumes: `AgentModel` from Task 1 and Store actions from Task 2.
- Produces: `loadSessions()`, `createSession()`, `loadSession(sessionId)`, `sendMessage(text)` and `cancelRun()`.

- [ ] **Step 1: Write the failing Effect test**

```ts
import { beforeEach, describe, expect, it } from "vitest";
import { createAgentEffect } from "@/controllers/effects/useAgentEffect";
import { resetAgentStore, useAgentStore } from "@/controllers/stores/agentStore";
import { createFakeAgentModel } from "@/models/fakeAgentModel";

describe("AgentEffect", () => {
  beforeEach(resetAgentStore);

  it("creates a session, sends a message and writes snapshots to the Store", async () => {
    const effect = createAgentEffect(
      createFakeAgentModel({ chunkDelayMs: 0 }),
      { success: () => undefined, error: () => undefined },
    );

    const [created, createError] = await effect.createSession();
    expect(createError).toBeNull();

    useAgentStore.getState().setActiveSession(created!.session.id);
    const [sent, sendError] = await effect.sendMessage("整理今天的重点");

    expect(sendError).toBeNull();
    expect(sent?.status).toBe("completed");
    expect(
      useAgentStore.getState().messagesBySessionId[created!.session.id],
    ).toHaveLength(2);
    expect(useAgentStore.getState().liveRun).toBeNull();
  });
});
```

- [ ] **Step 2: Run the test and verify RED**

Run: `pnpm vitest run tests/e2e/unit/agent/agentEffect.test.ts`

Expected: FAIL because `createAgentEffect` does not exist.

- [ ] **Step 3: Implement the Effect with Result-based control flow**

The Effect must load sessions once, create sessions with a UUID `clientRequestId`, append the user message before starting the stream, map snapshots into Store actions, set `liveRun` while streaming and clear it on every terminal status.

- [ ] **Step 4: Run the Effect test and verify GREEN**

Run: `pnpm vitest run tests/e2e/unit/agent/agentEffect.test.ts`

Expected: PASS.

## Task 4: App Shell and Agent Routing

**Files:**
- Modify: `src/components/shell/appShell.tsx`
- Modify: `src/layouts/appLayout.tsx`
- Create: `src/layouts/todayWorkspaceLayout.tsx`
- Create: `src/layouts/agentWorkspaceLayout.tsx`
- Create: `src/components/agent/agentSidebar.tsx`
- Modify: `src/components/shell/navRail.tsx`
- Modify: `src/router/index.tsx`
- Modify: `tests/e2e/unit/app/appRouter.test.tsx`

**Interfaces:**

- AppShell renders only the primary rail and the routed workspace.
- TodayWorkspaceLayout renders the existing Today ContextSidebar.
- AgentWorkspaceLayout renders AgentSidebar and the routed Agent page.
- NavRail adds an Agent link with accessible name `Agent`.

- [ ] **Step 1: Add failing router assertions**

```tsx
it("opens the Agent workspace with its persistent session sidebar", () => {
  const router = createMemoryRouter(routes, { initialEntries: ["/agent"] });
  render(<RouterProvider router={router} />);

  expect(screen.getByRole("heading", { name: "Agent" })).toBeInTheDocument();
  expect(
    screen.getByRole("complementary", { name: "Agent 会话" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Agent" })).toHaveAttribute(
    "aria-current",
    "page",
  );
});
```

- [ ] **Step 2: Run the router test and verify RED**

Run: `pnpm vitest run tests/e2e/unit/app/appRouter.test.tsx`

Expected: FAIL because no Agent route or navigation link exists.

- [ ] **Step 3: Refactor the shell without changing Today behavior**

Move ContextSidebar ownership from AppShell into TodayWorkspaceLayout. Add the Agent route tree and a Bot icon link in NavRail. Agent sidebar remains visible at every viewport.

- [ ] **Step 4: Run router and existing layout tests**

Run: `pnpm vitest run tests/e2e/unit/app/appRouter.test.tsx`

Run: `pnpm playwright test tests/e2e/modules/layout/responsive.e2e.ts`

Expected: PASS.

## Task 5: Agent Page and Chat Flow

**Files:**
- Create: `src/pages/agent/index.tsx`
- Create: `src/components/agent/agentEmptyState.tsx`
- Create: `src/components/agent/agentMessageList.tsx`
- Create: `src/components/agent/agentComposer.tsx`
- Create: `tests/e2e/modules/agent/navigation.e2e.ts`
- Create: `tests/e2e/modules/agent/chat.e2e.ts`

**Interfaces:**

- Consumes: `useAgentEffect`, `useAgentStore`.
- Produces: `AgentPage` used by both `/agent` and `/agent/:sessionId`.

- [ ] **Step 1: Write the failing Agent flow test**

```ts
import { expect, test } from "@playwright/test";

test("creates a session and completes a message", async ({ page }) => {
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  await expect(page).toHaveURL(/#\/agent\/.+/);

  const composer = page.getByRole("textbox", { name: "发送给 Agent" });
  await composer.fill("整理今天的重点");
  await page.getByRole("button", { name: "发送" }).click();

  await expect(page.getByText("整理今天的重点")).toBeVisible();
  await expect(page.getByText(/收到你的请求/)).toBeVisible();
  await expect(page.getByRole("button", { name: "发送" })).toBeEnabled();
});
```

- [ ] **Step 2: Run the test and verify RED**

Run: `pnpm playwright test tests/e2e/modules/agent/chat.e2e.ts --project=desktop`

Expected: FAIL because the Agent page and composer do not exist.

- [ ] **Step 3: Implement Agent view components**

Implement:

- Empty state with a visible `新建会话` command.
- Session title and update time.
- User messages right aligned; assistant messages left aligned.
- Streaming cursor on the active assistant part.
- Composer with Enter send, Shift+Enter newline, IME composition guard and whitespace rejection.
- Send button becomes `停止` while streaming.
- Error state with retry action.

- [ ] **Step 4: Run Agent flow tests and verify GREEN**

Run: `pnpm playwright test tests/e2e/modules/agent/chat.e2e.ts --project=desktop`

Expected: PASS.

## Task 6: Responsive, Theme and Accessibility Verification

**Files:**
- Create: `tests/e2e/modules/agent/responsive.e2e.ts`
- Modify: `src/components/agent/agentSidebar.tsx`
- Modify: `src/components/agent/agentComposer.tsx`
- Modify: `src/components/agent/agentMessageList.tsx`

- [ ] **Step 1: Write the failing responsive test**

```ts
import { expect, test } from "@playwright/test";
import { expectNoHorizontalOverflow } from "../../support/assertions";

test("Agent keeps its session sidebar at every target width", async ({ page }) => {
  for (const width of [390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/#/agent");
    await expect(
      page.getByRole("complementary", { name: "Agent 会话" }),
    ).toBeVisible();
    await expectNoHorizontalOverflow(page);
  }
});
```

- [ ] **Step 2: Run the test and verify RED**

Run: `pnpm playwright test tests/e2e/modules/agent/responsive.e2e.ts`

Expected: FAIL before the responsive Agent layout is complete.

- [ ] **Step 3: Refine responsive and accessible behavior**

Ensure:

- The Agent sidebar never disappears.
- New-session button uses an icon on narrow screens and has `aria-label="新建会话"`.
- Message area scrolls independently while composer remains reachable.
- `role="log"` and restrained `aria-live="polite"` are applied to messages.
- Current navigation uses `aria-current="page"`.
- Focus rings and reduced-motion behavior follow existing tokens.

- [ ] **Step 4: Run focused and full verification**

Run: `pnpm typecheck`

Run: `pnpm test`

Run: `pnpm playwright test tests/e2e/modules/agent --project=desktop --project=mobile`

Run: `pnpm playwright test`

Expected: all checks PASS.

## Task 7: Documentation and Migration Note

**Files:**
- Modify: `README.md`
- Modify: `docs/glossary.md`

- [ ] **Step 1: Add a short implementation-status note**

Record that Agent UI uses FakeAgentModel only for development and test. State explicitly that Go sidecar, SQLite persistence and real SSE remain unimplemented.

- [ ] **Step 2: Add new UI terms only when they enter the public contract**

Do not add extra synonyms for session, run or message. Reuse the existing glossary entries.

- [ ] **Step 3: Verify documentation**

Run: `git diff --check`

Run the relative Markdown link checker used by the repository.

Expected: no broken links, placeholders or contradictory completion claims.

## Self-Review

- Spec coverage for this slice: navigation, persistent Agent sidebar, session list, session creation, message display, streaming state, cancel affordance, responsive layout, accessibility and Fake Runtime test path.
- Deferred by the approved spec: Go sidecar, Electron bridge, SQLite, PostgreSQL, real SSE recovery, tools, Skills, MCP, multi-Agent and Redis.
- Production default must remain the existing API-backed Today model. FakeAgentModel is reachable only through an explicit development/test switch.
