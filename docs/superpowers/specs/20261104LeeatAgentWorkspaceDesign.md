# Leeat Agent 工作区设计规范

日期：2026-11-04
状态：已取代
范围：单 Agent 会话工作区
架构：前端 MVC，Vercel AI SDK 负责前端流消费，LangChain.js 负责服务端推理
术语基线：[Leeat 术语表](../../glossary.md)

> **替代说明**：本文已由 [CraftingTable Agent 设计](./2026-11-05-craftingtable-agent-design.md) 取代。本文中的 Fastify、Node.js SQLite 和 LangChain.js 方案只保留为历史设计记录。

## 1. 产品定义

Leeat 新增一个独立的 Agent 工作区，用于与单个个人 Agent 进行多轮对话。

Agent 工作区与今日工作区平级：

- 今日：`/`
- Agent：`/agent`
- Agent 会话：`/agent/:sessionId`
- 设置：`/settings`

Agent 工作区包含两级侧边栏：

1. 一级侧边栏继续承担全局导航。
2. 二级侧边栏用于新建会话、查看会话列表和切换当前会话。

主内容区展示当前会话的消息、流式回复和输入框。

本规范覆盖真实会话创建、消息持久化、模型调用、流式传输和多轮上下文。当前阶段不实现多 Agent，也不接入工作台工具。

本规范将取代基础 MVP 规范中“不实现 Agent”的旧非目标；今日页和设置页的既有行为保持不变。

## 2. 目标

- 提供独立的 Agent 一级入口。
- 提供专用二级侧边栏，用于新建会话和会话管理。
- 新建会话后立即进入真实会话路由。
- 支持发送消息并流式显示 Agent 回复。
- 支持同一会话内多轮上下文。
- 会话和消息写入 SQLite，刷新页面后可以恢复。
- 前端使用 Vercel AI SDK 的客户端 transport 和 UI Message Stream 能力。
- 服务端使用 LangChain.js 调用 OpenAI-compatible 模型。
- 保持 Light / Dark、响应式布局和现有视觉 token 一致。
- 保持现有 MVC 边界，页面和 Store 不直接请求 API。
- 为后续工具调用保留扩展点，但本阶段 `tools` 固定为空数组。

## 3. 非目标

- 不实现多 Agent、子 Agent 编排或 Agent 之间通信。
- 不实现工具注册、工具调用、工作台数据读写或 MCP。
- 不实现模型选择、参数调节、系统提示词编辑或密钥设置页面。
- 不实现会话重命名、删除、归档、搜索和分享。
- 不实现消息编辑、重新生成、分支会话和人工评分。
- 不实现附件、图片、语音、文件上传和多模态输入。
- 不实现用户账号、权限、多人协作和云同步。
- 不实现断线自动重连；失败后由用户手动重试。
- 不改变今日页数据模型和 API。
- 不接入 Electron IPC；浏览器继续通过 Fastify API 访问服务端。

## 4. 信息架构

### 4.1 一级侧边栏

一级侧边栏条目固定为：

1. 首页
2. Agent
3. 设置
4. Light / Dark 切换

Agent 使用 `Bot` 或 `MessageSquare` 类图标，带有 `aria-label="Agent"`。

当前路由为 `/agent` 或 `/agent/:sessionId` 时，Agent 入口显示选中态，并设置 `aria-current="page"`。

### 4.2 Agent 二级侧边栏

Agent 路由下显示 `AgentSessionSidebar`，替代今日页的 `ContextSidebar`。

结构从上到下为：

1. 标题 `Agent`
2. 新建会话按钮
3. 会话数量摘要
4. 按“今天 / 更早”分组的会话列表
5. 底部服务状态

新建会话按钮：

- 桌面端显示图标和文字。
- 窄屏只显示图标。
- 请求进行中时进入禁用态，避免重复创建。

会话列表：

- 按 `updatedAt` 倒序排列。
- 当前会话使用 `brandSoft` 背景和 `brand` 文字。
- 非当前会话使用 `textSecondary`，悬停使用 `surface`。
- 标题必须 `truncate`，不能撑破侧边栏。
- 当前会话设置 `aria-current="page"`。
- 空列表显示“还没有会话”，不显示示例数据。
- 本阶段不提供会话右键菜单。

服务状态：

- 正常配置时显示“Agent 已连接”。
- 模型配置缺失时显示“Agent 未配置”。

### 4.3 主内容区

未选择会话时，主内容区显示会话选择空状态和一个突出显示的新建会话按钮。

当前会话存在时，主内容区从上到下为：

1. 会话标题和最近更新时间
2. 可滚动的消息记录
3. 固定在主内容区底部的输入框和发送按钮

消息区：

- 用户消息右对齐。
- Agent 消息左对齐。
- Agent 回复支持 GitHub Flavored Markdown。
- Markdown 代码块使用等宽字体，支持横向滚动。
- 不渲染原始 HTML。
- 流式回复期间显示“正在回复”状态。
- 流式回复完成后状态恢复为“就绪”。

输入区：

- `Enter` 发送。
- `Shift + Enter` 换行。
- 空白内容不能发送。
- 流式回复期间发送按钮变为停止按钮。
- 输入框高度随内容增长，最多显示约 6 行。

## 5. 路由与页面状态

### 5.1 路由

使用现有 `HashRouter`：

- `/agent`：显示会话选择空状态。
- `/agent/:sessionId`：加载并显示指定会话。

不增加 `/agent/new`。新建会话必须先调用服务端创建会话，再导航到 `/agent/:sessionId`。

未知 `sessionId` 显示“会话不存在或已删除”，并提供返回 Agent 首页的按钮。

### 5.2 页面状态

Agent 页面必须支持：

- Loading：会话列表或当前会话加载时显示 Skeleton。
- Empty：没有会话或会话没有消息时显示空状态。
- Ready：显示历史消息和输入区。
- Streaming：显示增量回复和停止按钮。
- Error：在失败的 Agent 消息中显示错误说明；可重试错误在同一消息内显示重试入口。
- NotConfigured：模型环境变量缺失时显示清晰提示。

### 5.3 新建会话

点击“新建会话”：

1. Effect 调用 `POST /api/v1/agent/session/create`。
2. Store 插入返回的会话。
3. 导航到 `/agent/:sessionId`。
4. 主内容区显示空会话状态。

创建失败时保留当前页面，显示 Toast，不插入本地临时会话。

### 5.4 发送消息

点击发送或按 `Enter`：

1. Effect 读取当前会话和历史消息。
2. 把用户消息加入 Store，状态为已提交。
3. 调用 Agent Model 发起流式请求。
4. 每次收到 AI SDK `UIMessage` 快照时更新当前 Agent 消息。
5. 流结束后刷新会话列表，以更新标题和排序。
6. 失败时保留用户消息，显示错误状态和重试按钮。

发送中的用户消息不能在服务端确认前被修改。

### 5.5 停止生成

点击停止按钮：

1. Effect 触发当前请求的 `AbortController`。
2. 前端保留已经收到的部分文本。
3. 服务端把该 Assistant 消息标记为 `cancelled`。
4. UI 状态回到可发送。

## 6. 技术边界

### 6.1 前端

- React + TypeScript + Vite。
- Tailwind CSS + shadcn/ui。
- React Router。
- Zustand。
- Lucide React。
- Zod。
- Vercel AI SDK 包 `ai`。
- `react-markdown` + `remark-gfm`。
- Vitest + Testing Library。
- Playwright。

Vercel AI SDK 只允许出现在 `src/` 前端代码中，用于：

- `UIMessage`、`UIMessageChunk` 类型。
- `DefaultChatTransport`。
- `readUIMessageStream`。
- UI Message Stream 消费。

前端不使用 `useChat` 作为业务状态源。消息、会话和流式状态统一由 Zustand Store 持有，避免 AI SDK Hook 内部状态与 MVC Store 出现双源。

### 6.2 服务端

- Node.js 24 LTS。
- TypeScript 严格模式。
- Fastify 5。
- Node.js 内置 `node:sqlite`。
- Zod。
- LangChain.js 包 `langchain`。
- `@langchain/openai`。
- `@langchain/core`。
- Vitest + Fastify `inject()`。

LangChain.js 只允许出现在 `api/` 服务端代码中，用于：

- 构造 `ChatOpenAI`。
- 构造单 Agent runtime。
- 转换会话消息。
- 调用 `streamEvents`。

服务端不得依赖 `ai` 或 `@ai-sdk/*`。服务端只输出兼容 Vercel AI SDK UI Message Stream 的 SSE 数据。

### 6.3 模型配置

配置通过环境变量读取：

- `AGENT_BASE_URL`
- `AGENT_API_KEY`
- `AGENT_MODEL`
- `AGENT_SYSTEM_PROMPT`

`AGENT_SYSTEM_PROMPT` 可选。未设置时使用：

> 你是 Leeat 的个人工作助手。回答应准确、简洁，并优先使用中文。

缺少必填配置时，服务端其他 API 继续工作，Agent 消息接口返回 `503 AGENT_NOT_CONFIGURED`。

## 7. MVC 边界

### 7.1 前端 Model

新增文件：

- `src/models/agentContracts.ts`
- `src/models/agentModel.ts`
- `src/models/mockAgentModel.ts`

`agentContracts.ts` 只定义普通领域类型和接口，不导入 AI SDK。

`agentModel.ts` 负责：

- 会话列表和会话详情请求。
- 创建会话。
- 使用 `DefaultChatTransport` 发起流式消息请求。
- 使用 `readUIMessageStream` 把 SSE 还原为 `UIMessage` 快照。
- 把持久化消息转换为 AI SDK `UIMessage`。

Model 可以调用 `fetch`，但不得读取或写入 Zustand，不得显示 Toast，不得渲染 UI。

`mockAgentModel.ts` 只用于单元测试或显式 Mock 模式，不作为生产默认数据源。

### 7.2 前端 Store

新增：

- `src/controllers/stores/agentStore.ts`

Store 至少保存：

- `sessions`
- `activeSession`
- `messages`
- `agentConfigured`
- `listPhase`
- `sessionPhase`
- `sendPhase`
- `errorMessage`

`sendPhase` 取值：

- `idle`
- `submitting`
- `streaming`
- `error`

Store 只提供同步动作：

- `setListLoading`
- `setSessions`
- `setSessionLoading`
- `setActiveSession`
- `setMessages`
- `setAgentConfigured`
- `appendMessage`
- `upsertMessage`
- `setSendPhase`
- `setError`
- `resetAgentStore`

Store 不调用 Model，不调用 AI SDK，不创建 `AbortController`，不直接执行 `fetch`。

### 7.3 前端 Effect

新增：

- `src/controllers/effects/useAgentEffect.ts`

Effect 暴露：

- `loadSessions`
- `loadSession`
- `createSession`
- `sendMessage`
- `stopStreaming`
- `retrySend`

Effect 负责：

- 调用 Agent Model。
- 维护当前请求的 `AbortController`，只保存在 Effect 闭包或 ref 中。
- 把 Model 结果写入 Store。
- 把流式快照写入 Store。
- 处理 Toast。
- 对流结束后的会话列表做一次温和刷新。

Effect 不保存长期业务状态。

`useAgentEffect` 在 `sessionId` 变化时加载会话。页面只读取 Store，只调用 Effect 暴露的动作。

### 7.4 前端 View

新增文件：

- `src/pages/agent/index.tsx`
- `src/components/agent/agentSessionSidebar.tsx`
- `src/components/agent/agentThread.tsx`
- `src/components/agent/agentMessage.tsx`
- `src/components/agent/agentComposer.tsx`

布局修改：

- `AppShell` 接受二级侧边栏插槽。
- `AppLayout` 根据路径渲染 `ContextSidebar` 或 `AgentSessionSidebar`。
- `NavRail` 增加 Agent 入口。
- 路由增加 `/agent` 和 `/agent/:sessionId`。

View 不直接执行 `fetch`，不导入 Model，不访问 SQLite。

### 7.5 服务端 Model

新增：

- `api/migrations/002Agent.sql`
- `api/src/models/agentModel.ts`

职责：

- 创建会话。
- 列出会话。
- 读取会话和消息。
- 创建用户消息。
- 创建 Assistant 占位消息。
- 更新 Assistant 内容、状态和错误。
- 更新会话标题和最近更新时间。

### 7.6 服务端 Controller

新增：

- `api/src/controllers/agentController.ts`

职责：

- 校验请求。
- 检查会话存在。
- 检查模型配置。
- 读取历史消息。
- 持久化用户消息。
- 调用 Agent runtime。
- 通过 Stream View 返回 SSE。
- 处理完成、失败和中断。

### 7.7 服务端 Runtime 与 View

新增：

- `api/src/agent/agentRuntime.ts`
- `api/src/views/agentStreamView.ts`

`agentRuntime.ts` 封装 LangChain.js，不访问数据库。

`agentStreamView.ts` 只负责 SSE header、chunk 序列化和 `[DONE]` 结束标记，不导入 AI SDK。

## 8. 数据模型

### 8.1 领域类型

```ts
type AgentSession = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
};

type AgentMessage = {
  id: string;
  sessionId: string;
  role: "user" | "assistant";
  content: string;
  status: "complete" | "streaming" | "failed" | "cancelled";
  errorMessage?: string;
  model?: string;
  createdAt: string;
  updatedAt: string;
};

type AgentSessionDetail = {
  session: AgentSession;
  messages: AgentMessage[];
};
```

时间使用 UTC ISO 8601。

### 8.2 SQLite 表

```sql
CREATE TABLE IF NOT EXISTS agent_sessions (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_messages (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
  content TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (
    status IN ('complete', 'streaming', 'failed', 'cancelled')
  ),
  error_message TEXT,
  model TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_updated_at
  ON agent_sessions(updated_at);

CREATE INDEX IF NOT EXISTS idx_agent_messages_session_created
  ON agent_messages(session_id, created_at);
```

新会话标题为 `新会话`。首个用户消息写入后，标题更新为消息纯文本的前 32 个字符。

`agent_sessions.updated_at` 在消息写入、流开始和流结束时更新。

## 9. API

API 前缀保持 `/api/v1`。

只使用 `GET` 和 `POST`：

```text
GET  /api/v1/agent/sessions
GET  /api/v1/agent/session?sessionId=...
POST /api/v1/agent/session/create
POST /api/v1/agent/message/send
POST /api/v1/agent/message/retry
```

### 9.1 会话列表

`GET /api/v1/agent/sessions`

响应：

```json
{
  "data": {
    "sessions": [],
    "configured": true
  }
}
```

`configured` 只表示服务端是否具备调用模型所需配置，不表示模型供应商当前可用。

### 9.2 会话详情

`GET /api/v1/agent/session?sessionId=...`

响应：

```json
{
  "data": {
    "session": {},
    "messages": []
  }
}
```

不存在时返回 `404 AGENT_SESSION_NOT_FOUND`。

### 9.3 创建会话

`POST /api/v1/agent/session/create`

请求体可以为空。

响应：

```json
{
  "data": {
    "session": {}
  }
}
```

### 9.4 发送消息

`POST /api/v1/agent/message/send`

请求：

```json
{
  "sessionId": "uuid",
  "message": {
    "id": "ui-message-id",
    "role": "user",
    "parts": [
      {
        "type": "text",
        "text": "帮我整理今天的工作重点"
      }
    ]
  }
}
```

服务端只接受最后一个用户消息包含的文本，最多 20,000 字符。

成功响应为 SSE UI Message Stream，不使用 JSON envelope。

以下错误在流开始前以 JSON 返回：

- `400 VALIDATION_ERROR`
- `404 AGENT_SESSION_NOT_FOUND`
- `409 AGENT_SESSION_BUSY`
- `503 AGENT_NOT_CONFIGURED`

### 9.5 重试失败回复

`POST /api/v1/agent/message/retry`

请求：

```json
{
  "sessionId": "uuid",
  "assistantMessageId": "uuid"
}
```

服务端只允许重试同一会话中状态为 `failed` 的 Assistant 消息。重试复用原 Assistant 消息 ID 和原用户消息，不插入新的用户消息。

成功响应为与发送消息相同的 SSE UI Message Stream。

## 10. 流式协议

### 10.1 响应头

```text
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
x-vercel-ai-ui-message-stream: v1
x-accel-buffering: no
```

### 10.2 Chunk 顺序

每个 SSE 事件为：

```text
data: <JSON.stringify(UIMessageChunk)>

```

正常回复顺序：

1. `{ "type": "start", "messageId": "<assistant-message-id>" }`
2. `{ "type": "start-step" }`
3. `{ "type": "text-start", "id": "<text-part-id>" }`
4. 多次 `{ "type": "text-delta", "id": "<text-part-id>", "delta": "..." }`
5. `{ "type": "text-end", "id": "<text-part-id>" }`
6. `{ "type": "finish-step" }`
7. `{ "type": "finish" }`
8. `data: [DONE]`

流中错误：

```text
data: {"type":"error","errorText":"模型调用失败，请重试"}

data: [DONE]
```

### 10.3 前端消费

`AgentModel.sendMessage` 使用 `DefaultChatTransport`：

- `api` 指向 `/api/v1/agent/message/send`。
- `prepareSendMessagesRequest` 只发送 `sessionId` 和最后一条用户消息。
- 通过 `sendMessages` 获取 `ReadableStream<UIMessageChunk>`。
- 通过 `readUIMessageStream` 获取增量 `UIMessage` 快照。

Effect 只把快照写入 Store，不自行拼接 SSE 字符串。

### 10.4 服务端序列化

`agentStreamView.ts` 负责：

- 写入规定响应头。
- 将 LangChain token 包装成 `text-delta`。
- 保证开始和结束 chunk 顺序。
- 在正常和错误结束时都发送 `[DONE]`。
- 对已关闭的客户端连接忽略后续写入。

服务端不能依赖 AI SDK 的 `pipeUIMessageStreamToResponse`，因为 AI SDK 只属于前端。

## 11. LangChain 运行时

### 11.1 模型

使用 `ChatOpenAI`：

```ts
new ChatOpenAI({
  model: config.model,
  apiKey: config.apiKey,
  configuration: {
    baseURL: config.baseUrl,
  },
});
```

模型必须支持 OpenAI-compatible Chat Completions API。

### 11.2 Agent

```ts
createAgent({
  model,
  tools: [],
});
```

本阶段不注册任何工具。`tools: []` 是未来工具能力的唯一扩展点。

### 11.3 消息转换

- `user` 转为 `HumanMessage`。
- `assistant` 转为 `AIMessage`。
- 只把状态为 `complete` 的非空 Assistant 历史传给模型。
- `failed` 和 `cancelled` 消息不作为模型上下文。
- 消息中的 `error` 片段只用于界面展示，不转换为模型消息。
- 可选 `AGENT_SYSTEM_PROMPT` 转为 `SystemMessage`，放在历史消息前。

### 11.4 流式调用

```ts
const stream = await agent.streamEvents(
  { messages },
  { version: "v3" },
);

for await (const message of stream.messages) {
  for await (const delta of message.text) {
    // 写出 text-delta
  }
}
```

服务端累计所有 token，流完成后写入完整 Assistant 内容。

### 11.5 上下文长度

MVP 不实现自动摘要。发送模型请求前只保留最近 40 条消息，并保证不会从 Assistant 消息中间开始截断；数据库仍保留完整历史。

## 12. 并发、失败与恢复

### 12.1 并发

同一个 `sessionId` 同时只允许一个生成请求。

服务端使用进程内 `Set<sessionId>` 记录活跃流：

- 已活跃时返回 `409 AGENT_SESSION_BUSY`。
- 流完成、失败或中断后必须移除。
- 服务端重启后不保留锁。

### 12.2 失败

模型调用失败：

- Assistant 占位消息状态改为 `failed`。
- 保存错误信息，但不保存 API Key、完整堆栈或第三方响应原文。
- 流中发送统一 `error` chunk。
- 前端显示“重试”按钮。

### 12.3 中断

客户端中断：

- Effect 调用 `AbortController.abort()`。
- 服务端停止读取 LangChain 流。
- 已累计文本写入 Assistant 消息，状态为 `cancelled`。
- 没有文本时保存空内容，状态为 `cancelled`。

### 12.4 重试

重试调用 `POST /api/v1/agent/message/retry`，只重新生成指定失败的 Assistant 消息，不重复插入用户消息。

重试前服务端读取该 Assistant 消息之前的完整会话历史，并重新创建同 ID 的流式占位状态。

## 13. 视觉规范

### 13.1 布局

桌面端：

```text
┌──────┬──────────────────┬────────────────────────────┐
│ 一级 │ Agent 二级侧栏    │ 当前会话                   │
│ 首页 │ Agent       +    │ 标题                       │
│Agent │ 今天             │ 消息记录                   │
│ 设置 │   会话 A          │                            │
│      │ 更早             │ 输入框             发送     │
│      │   会话 B          │                            │
└──────┴──────────────────┴────────────────────────────┘
```

宽度沿用现有响应式规则：

- `>=1280px`：一级 56px，二级 232px
- `768px - 1279px`：一级 52px，二级 184px
- `461px - 767px`：一级 44px，二级 104px
- `<=460px`：一级 34px，二级 88px

### 13.2 消息视觉

- 页面背景使用 `background`。
- 会话主区域使用 `surface` 或透明背景，不叠加多层卡片。
- 用户消息使用 `brand` 背景和白色文字。
- Agent 消息使用 `surfaceSubtle` 背景和 `textPrimary` 文字。
- 消息最大宽度约为内容区 78%，窄屏为 88%。
- 消息之间间距不小于 12px。
- 代码块使用 `surface` 背景、`border` 边框和横向滚动。
- 错误消息使用 `destructive` 文字，但不使用大面积红色背景。

### 13.3 输入区

- 输入区固定在主内容区底部。
- 使用现有边框、表面和焦点环 token。
- 发送按钮使用品牌色，图标按钮必须有可访问名称。
- 流式期间停止按钮使用 `Square` 图标。

### 13.4 主题

不新增颜色 token，继续使用：

- Light：`background #F2F5F8`、`surface #FFFFFF`、`surfaceSubtle #F7F8FA`、`brand #5468FF`
- Dark：`background #17191D`、`surface #1E2126`、`surfaceSubtle #25292F`、`brand #7B8AFF`

## 14. 响应式

- 390px、768px、1024px、1440px 均不能出现全局横向滚动。
- Agent 二级侧边栏不能消失。
- 390px 下新建按钮只显示图标。
- 390px 下会话项显示首字或状态图标，完整标题通过 `aria-label` 提供。
- 输入框和发送按钮不得超出主内容宽度。
- 长单词、URL、代码块不得撑破消息区域。
- 消息区域独立滚动，输入区始终可达。

## 15. 无障碍

- Agent 一级入口有 `aria-label="Agent"`。
- 新建会话按钮有 `aria-label="新建会话"`。
- 发送按钮有 `aria-label="发送消息"`。
- 停止按钮有 `aria-label="停止生成"`。
- 当前导航和当前会话使用 `aria-current="page"`。
- 消息列表使用 `role="log"` 和可访问名称。
- 流式状态使用 `aria-live="polite"`，避免每个 token 都触发完整播报。
- 输入框有可访问名称。
- 键盘焦点顺序与视觉顺序一致。
- 焦点环使用品牌色。
- 动画遵循 `prefers-reduced-motion`。

## 16. 测试

所有测试继续统一放在 `tests/e2e`，按模块拆分。

### 16.1 单元与集成测试

```text
tests/e2e/unit/agent/agentStore.test.ts
tests/e2e/unit/agent/agentModel.test.ts
tests/e2e/unit/agent/useAgentEffect.test.tsx
tests/e2e/unit/agent/agentApi.test.ts
tests/e2e/unit/agent/agentStreamProtocol.test.ts
tests/e2e/unit/agent/architectureBoundaries.test.ts
```

覆盖：

- Store 同步动作。
- Effect 创建会话、加载会话、发送、失败和中断。
- `DefaultChatTransport` 对模拟 SSE 的消费。
- `readUIMessageStream` 对文本 delta 的增量快照。
- 会话和消息 SQLite 读写。
- SSE chunk 顺序和 `[DONE]`。
- `src/` 不导入 LangChain。
- `api/` 不导入 AI SDK。

### 16.2 浏览器 E2E

```text
tests/e2e/modules/agent/navigation.e2e.ts
tests/e2e/modules/agent/sessionSidebar.e2e.ts
tests/e2e/modules/agent/chat.e2e.ts
tests/e2e/modules/agent/responsive.e2e.ts
```

覆盖：

- 一级导航进入 Agent。
- Agent 二级侧边栏出现，今日上下文侧栏不出现。
- 新建会话后 URL 为 `/#/agent/:sessionId`。
- 刷新后会话和消息仍然存在。
- 发送消息后显示流式文本。
- 多轮消息可以继续发送。
- 停止生成后保留部分文本。
- 模型错误显示重试入口。
- 390px 下两级侧边栏都保持可见。
- 390px、768px、1024px、1440px 无全局横向滚动。
- Light / Dark 在 Agent 页面正常。
- 原今日页面的 Quick Capture、完成状态和主题测试继续通过。

### 16.3 测试模型

E2E 不调用真实模型。

`api/e2eServer.ts` 向 `createApp` 注入确定性的假 `AgentRuntime`：

- 按 token 间隔输出固定文本。
- 第二轮回显上一轮用户内容，用于验证多轮上下文。
- 支持触发失败和延迟中断的测试场景。

生产环境默认使用 LangChain runtime。

## 17. 完成标准

- 一级导航包含首页、Agent、设置。
- Agent 路由显示独立会话管理二级侧栏。
- 新建会话调用服务端并进入真实会话 URL。
- 会话列表来自 SQLite，不使用示例数据。
- 消息可以流式显示并写入 SQLite。
- 刷新页面后可以恢复会话和多轮消息。
- 同一会话的上下文会传给 LangChain Agent。
- 工具列表固定为空，不执行工作台工具。
- 不支持多 Agent。
- 前端使用 Vercel AI SDK transport 和 UI Message Stream。
- 服务端使用 LangChain.js 调用 OpenAI-compatible 模型。
- `api/` 不依赖 AI SDK，`src/` 不依赖 LangChain。
- View 不直接请求 API。
- Store 不调用 Model，Effect 不保存长期业务状态。
- 所有测试位于 `tests/e2e`。
- `pnpm typecheck`、`pnpm test`、`pnpm build`、`pnpm test:e2e` 通过。
