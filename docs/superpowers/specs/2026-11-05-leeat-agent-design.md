# Leeat Agent 设计

日期：2026-11-05
状态：待评审
范围：单 Agent 会话、Run、消息片段、流式协议、持久化与后续能力扩展
平台顺序：Desktop -> Web -> 手机
工程约束：[Leeat 工程化设计规范](./2026-11-05-leeat-engineering-design.md)
术语基线：[Leeat 术语表](../../glossary.md)

## 1. 设计目的

Leeat Agent 是工作台中的个人助手，不是独立聊天产品的复制品。

当前阶段只实现单 Agent 多轮会话，但领域模型、流事件和持久化结构必须为后续 Skills、工具、MCP、多 Agent 编排和评测保留稳定扩展点。

本文解决以下问题：

- 会话、消息、片段、Run 和 Step 分别是什么。
- 一次 Agent 回复如何从 Go 服务端流到 Zustand Store。
- token 增量为什么不能逐条写数据库。
- 每个消息片段如何归属到正确的消息和会话。
- 用户取消、模型失败、浏览器断线和进程重启后是什么状态。
- Vercel AI SDK 只承担传输与解析时，如何避免状态双源。
- 后续工具、Skills、MCP 和多 Agent 如何接入而不重写前端。

## 2. Agent Runtime 的含义

`Agent Runtime` 是一次 Agent 执行过程的运行适配层。

它负责：

- 接收已经校验的 `RunInput`。
- 调用模型供应商或后续的本地模型。
- 将供应商事件转换成 Leeat 领域事件。
- 在执行过程中持续推进 Step。
- 将增量输出交给流编码器。
- 响应取消和超时。

它不负责：

- 读写数据库。
- 修改 Zustand Store。
- 决定页面布局或 Toast。
- 决定会话标题。
- 直接渲染 Markdown。

因此，`Agent Runtime` 不是 Agent 产品本身，也不是 Agent Loop 的同义词。它是服务端领域与具体模型 SDK、模型供应商之间的执行边界。

## 3. 目标与非目标

### 3.1 当前目标

- 提供 Agent 一级入口和独立二级会话侧边栏。
- 创建、加载和切换单 Agent 会话。
- 发送用户消息并流式显示助手回复。
- 同一会话内支持多轮上下文。
- 会话、消息和已展示片段写入本地数据库。
- 刷新后恢复数据库已经提交的消息内容，并识别仍在服务端运行的 Run。
- 支持停止生成。
- 支持失败或中断后的手动重试。
- 前端使用 Vercel AI SDK 的 Transport 和 UI Message Stream 读取能力。
- 前端不以 `useChat` 作为业务状态源。
- 服务端使用 Go 模块化单体。
- 保持 Light / Dark、响应式布局和无障碍。

### 3.2 当前非目标

- 不实现多 Agent、子 Agent 或 Agent 间通信。
- 不实现工具注册和工具调用。
- 不实现 Skills、MCP Server 或工作台数据读写工具。
- 不实现模型切换、参数编辑和系统提示词编辑页面。
- 不实现会话删除、归档、分享和搜索。
- 不实现消息编辑、分支会话和人工评分。
- 不实现附件、图片、语音和多模态输入。
- 不实现自动断线重连和跨进程 Run 恢复。
- 不保证进程崩溃时尚未提交到数据库的尾部文本仍然存在。
- 不接入 Redis、任务队列或持久化执行引擎。

### 3.3 必须保留的扩展点

- `AgentMessagePart` 可以增加新片段类型。
- `AgentRun` 可以拥有多个 `AgentStep`。
- `AgentStep` 可以扩展为工具、Skill、MCP 和子 Agent 步骤。
- `RunEvent` 可以增加事件类型，但已有字段不能改变含义。
- Runtime 可以替换模型供应商，不改变前端协议。
- 执行状态可以增加 queued、resuming 等状态，不改变已有状态含义。
- 评测功能可以复用 Run、Step、Event 和 Artifact，但评测平台独立部署。

## 4. 信息架构

一级导航：

1. 首页 `/`
2. Agent `/agent`
3. 设置 `/settings`

Agent 路由：

- `/agent`：未选择会话时的空状态。
- `/agent/:sessionId`：当前会话。

Agent 页面使用两级侧边栏：

```text
┌────────┬────────────────────┬──────────────────────────────┐
│ 一级   │ Agent 会话侧栏      │ Agent 会话内容                │
│ 首页   │ Agent         +    │ 标题                         │
│ Agent  │ 今天               │ 消息历史                     │
│ 设置   │   会话 A            │                              │
│        │ 更早               │ 输入区                发送    │
│        │   会话 B            │                              │
└────────┴────────────────────┴──────────────────────────────┘
```

二级侧栏负责：

- 新建会话。
- 按最近更新时间列出会话。
- 切换当前会话。
- 显示 Agent 配置是否可用。

主内容区负责：

- 当前会话标题和更新时间。
- 消息历史和流式内容。
- 输入框、发送和停止按钮。
- Loading、Empty、Streaming、Error 和 NotConfigured 状态。

不增加顶部应用栏，不重复显示品牌名和当前路由标题。

MVP 只允许 Agent 工作区同时存在一条正在接收的 Live Run。切换会话、进入首页或进入设置不会取消当前 Run；当前 Run 以 `sessionId + runId` 留在应用级 AgentEffect 中继续更新 Store。只要 Live Run 尚未终止，其他会话的发送按钮保持禁用，避免前端同时维护多个流和多个活动消息。

## 5. 总体架构

```text
┌─────────────────────────────────────────────────────────────┐
│ React Renderer                                              │
│                                                             │
│ View --读取--> AgentStore                                    │
│   |                                                         │
│   +--动作--> AgentEffect --> AgentModel                     │
│                 |             -> DefaultChatTransport       │
│                 |             -> readUIMessageStream        │
│                 +--同步写入--> AgentStore                    │
└──────────────────────────────┬──────────────────────────────┘
                               │ HTTP + SSE
┌──────────────────────────────▼──────────────────────────────┐
│ Go API                                                      │
│                                                             │
│ Agent HTTP Controller                                       │
│   -> Agent Application Controller                           │
│     -> Agent Repository                                     │
│     -> Run Coordinator                                      │
│        -> Agent Executor (run-scoped context)               │
│           -> Agent Runtime                                  │
│              -> Model Provider Adapter                      │
│                                                             │
│ HTTP Handler -> Live Event Subscriber                       │
│ Agent Stream View -> AI SDK UI Message Stream SSE           │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│ Local Database                                              │
│ sessions / messages / message_parts / runs / steps / events │
└─────────────────────────────────────────────────────────────┘
```

Desktop 与 Web 共用：

- Agent 领域类型。
- HTTP 路径。
- SSE 协议。
- MVC 前端代码。
- Fake Runtime 测试场景。

Desktop 额外拥有：

- Electron 主进程和 preload。
- 本地 Go sidecar 生命周期管理。
- sidecar 端点和鉴权 Token 的主进程代理。
- 文件、外链和系统能力桥接。

Desktop 的 Renderer 不持有 sidecar 端口和 Token。`AgentModel` 接收一个平台注入的 fetch 适配器：

- Web：适配器直接调用同源 `/api/v1`。
- Desktop：适配器通过窄范围 preload IPC 和 `MessagePort` 请求主进程转发。
- 两者对 `DefaultChatTransport` 暴露相同的 fetch 行为，并返回 `ReadableStream<UIMessageChunk>`。

主进程只代理已登记的 Agent 路径和请求方法，不接受任意 URL。SSE 流关闭、窗口销毁和应用退出都必须向上游发送取消信号并释放端口。

### 5.1 Desktop sidecar 生命周期

首发 Desktop 基线为 Windows x64。macOS 和 Linux 的签名、路径和进程管理在 Web 版本完成后单独验收，不提前宣称跨平台完成。

启动流程：

1. Electron 主进程生成高熵短期 Token，通过环境变量传入 sidecar，不放入命令行参数。
2. Go sidecar 绑定 `127.0.0.1:0`，由操作系统分配随机端口。
3. sidecar 在 stdout 输出一次有长度上限的 JSON 握手，包含协议版本和实际端口；其他日志走 stderr 或文件。
4. 主进程在超时时间内解析握手并调用带 Token 的 `/healthz`，成功后把 transport bridge 标记为可用。
5. 握手失败、端口异常或健康检查失败时，主进程关闭子进程并显示可恢复的启动错误，不能把 Token 写进日志。

运行约束：

- 生产数据库位于 `app.getPath("userData")/leeat/leeat.db`，安装目录只读且不能保存用户数据。
- 开发数据库使用项目忽略目录中的独立文件，不与生产库混用。
- 主进程退出时调用受鉴权的内部 shutdown，等待 sidecar 关闭 HTTP 服务和数据库；超时后终止子进程。
- sidecar 异常退出时最多自动重启 3 次，使用递增退避；重启后由启动清理把旧 `running` Run 标记为 `interrupted`。
- UI 必须显示 sidecar 不可用状态和重试入口，不能无限显示 Loading。

## 6. 领域对象

### 6.1 AgentSession

`AgentSession` 表示一次连续对话。

```ts
type AgentSession = {
  id: string
  title: string
  createdAt: string
  updatedAt: string
}
```

标题不参与模型上下文。MVP 使用首个用户消息的前 32 个字符生成标题，用户可以后续再增加重命名能力。

服务端生成的 Session、助手的 Message、Run、Step 和 Part 默认使用 UUIDv7。客户端发送的用户消息 ID 与 `clientRequestId` 也使用 UUIDv7，并由服务端校验格式。排序仍以数据库时间和显式序号为准，不能把 ID 时间片段当作业务顺序。

### 6.2 AgentMessage

`AgentMessage` 表示会话中的一条完整用户或助手消息。

```ts
type AgentMessageRole = "user" | "assistant"

type AgentMessageStatus =
  | "streaming"
  | "complete"
  | "cancelled"
  | "failed"
  | "interrupted"

type AgentMessage = {
  id: string
  sessionId: string
  role: AgentMessageRole
  status: AgentMessageStatus
  activeRunId?: string
  createdAt: string
  updatedAt: string
}
```

消息本身不保存可展示正文。正文由多个 `AgentMessagePart` 组成，避免未来工具调用、文件和图片被塞进一个字符串。

### 6.3 AgentMessagePart

`AgentMessagePart` 是消息中的一块可展示内容。

```ts
type AgentMessagePartType = "text"

type AgentMessagePart = {
  id: string
  messageId: string
  runId?: string
  index: number
  type: AgentMessagePartType
  status: "streaming" | "complete" | "cancelled" | "failed" | "interrupted"
  content: AgentTextPartContent
  createdAt: string
  updatedAt: string
}

type AgentTextPartContent = {
  type: "text"
  text: string
}
```

当前只接受 `text`。后续可增加：

- `tool-call`
- `tool-result`
- `skill`
- `file`
- `image`
- `reasoning`

新片段类型必须新增 Schema、持久化迁移、UI 渲染和测试，不能只增加一个类型字符串。

归属关系固定为：

```text
AgentSession 1 -> N AgentMessage 1 -> N AgentMessagePart
AgentRun     1 -> N 助手消息的生成片段
```

片段通过 `messageId` 归属消息，消息通过 `sessionId` 归属会话。用户消息片段没有 `runId`；助手消息的生成片段必须带 `runId`。禁止在流事件中仅依靠数组顺序推断片段属于哪条消息。

### 6.4 AgentRun

`AgentRun` 表示一次 Agent 执行。

```ts
type AgentRunStatus =
  | "running"
  | "completed"
  | "failed"
  | "cancelled"
  | "interrupted"

type AgentRun = {
  id: string
  sessionId: string
  userMessageId: string
  assistantMessageId: string
  clientRequestId: string
  retryOfRunId?: string
  status: AgentRunStatus
  model: string
  startedAt: string
  finishedAt?: string
  errorCode?: string
}
```

一个 Run 只对应一条助手消息的一次生成尝试。重试创建新的 Run，并通过 `retryOfRunId` 关联上一次执行。

`userMessageId` 明确记录触发本次执行的用户消息。重试不能通过“助手消息前一条”推断用户消息，否则插入历史分页、并发或未来消息编辑后会产生错误上下文。

### 6.5 AgentStep

`AgentStep` 表示 Run 内一个有明确边界的执行步骤。

```ts
type AgentStepKind = "model"

type AgentStep = {
  id: string
  runId: string
  index: number
  kind: AgentStepKind
  status: "running" | "completed" | "failed" | "cancelled" | "interrupted"
  startedAt: string
  finishedAt?: string
  errorCode?: string
}
```

MVP 一次 Run 通常只有一个模型 Step。后续工具和 MCP 调用分别成为新的 Step，不把多次调用压缩成一个无结构字符串。

### 6.6 RunEvent 与 RunDelta

必须区分两类流数据：

- `RunEvent`：有业务语义的生命周期事件，必须能持久化。
- `RunDelta`：token 或 UI 增量，只存在于当前 SSE 链路。

`RunEvent` 持久化结构：

```ts
type AgentRunEventType =
  | "run.started"
  | "step.started"
  | "message.part.started"
  | "message.part.finished"
  | "step.finished"
  | "run.completed"
  | "run.failed"
  | "run.cancelled"
  | "run.interrupted"

type AgentRunEvent = {
  id: string
  runId: string
  sequence: number
  type: AgentRunEventType
  stepId?: string
  partId?: string
  payload: Record<string, unknown>
  createdAt: string
}
```

`payload` 必须由对应事件类型的 Schema 校验，不能把所有字段都塞进一个无约束对象。事件 envelope 的 `runId + sequence` 唯一，`stepId` 和 `partId` 是可选关联键；收到不属于当前 Run 的关联 ID 时，整个事件属于协议错误。

`RunEvent` 示例：

```text
run.started
step.started
message.part.started
message.part.finished
step.finished
run.completed
run.failed
run.cancelled
run.interrupted
```

`RunDelta` 示例：

```text
text-delta
```

规则：

- `RunEvent` 在业务边界写入数据库。
- `text-delta` 不逐条写数据库。
- `text-delta` 由服务端在内存中累计。
- Run 结束、失败或取消时，将最终文本片段一次性写入 `AgentMessagePart`。
- 最终片段、消息状态、Step、Run 状态和终止 `RunEvent` 在同一事务提交；事务失败时，不能先向浏览器发送成功终止事件。
- 前端可以实时合并 token，但刷新后以数据库中的最终片段为准。
- 硬崩溃时无法执行结束分支，因此只保证此前已经提交的片段；进程内尚未提交的尾部文本允许丢失。

这样既保证流式体验，也避免一条长回复产生数千次数据库写入。

## 7. 持久化模型

MVP 本地优先版本使用 SQLite，Web 版本后续可替换为 PostgreSQL。Repository 接口不得暴露具体数据库类型。

概念表：

```text
agent_sessions
agent_messages
agent_message_parts
agent_runs
agent_steps
agent_run_events
```

关键约束：

- `agent_message_parts.message_id` 外键指向 `agent_messages.id`。
- `agent_message_parts.run_id` 为可空外键并指向 `agent_runs.id`；用户消息片段为空，助手生成片段必填。
- `agent_messages.session_id` 外键指向 `agent_sessions.id`。
- `agent_runs.user_message_id` 外键指向 `agent_messages.id`。
- `agent_runs.assistant_message_id` 外键指向 `agent_messages.id`。
- 两个消息 ID 必须与 Run 属于同一会话，角色分别为 `user` 和 `assistant`；该约束由写入事务校验并有集成测试覆盖。
- `agent_steps.run_id` 外键指向 `agent_runs.id`。
- `agent_run_events.run_id` 外键指向 `agent_runs.id`。
- `agent_sessions.client_request_id` 全局唯一，用于创建会话幂等。
- `agent_runs.client_request_id` 在会话内唯一，用于发送幂等。
- 用户消息片段使用 `(message_id, index) WHERE run_id IS NULL` 的唯一索引。
- 助手生成片段使用 `(message_id, run_id, index) WHERE run_id IS NOT NULL` 的唯一索引，保证重试产生的片段不会互相覆盖。
- `agent_run_events` 使用 `(run_id, sequence)` 唯一约束。
- `agent_messages.active_run_id` 指向的 Run 必须属于同一条助手消息；写入时由事务校验，并用数据库触发器或约束测试防止越界关联。
- 会话列表按 `updated_at` 倒序，消息分页按 `created_at` 和 `id` 稳定排序，片段按 `index` 升序。

同一个助手消息发生重试时，保留旧 Run 和旧片段。`agent_messages.active_run_id` 指向当前展示的 Run，会话详情只加载该 Run 的片段。这样既保留执行历史，也不会在同一气泡中混入多次尝试。

会话列表默认每次读取 30 条，消息详情默认每次读取 40 条。游标必须包含稳定排序键，不能用不断变化的 `updated_at` 单独分页。

## 8. Run 生命周期

```text
创建 Run
  -> running
    -> completed
    -> failed
    -> cancelled
    -> interrupted
```

状态含义：

- `running`：服务端已接管本次执行。
- `completed`：模型正常结束，最终片段和消息已提交。
- `failed`：模型、协议或持久化失败；已经进入结束事务的部分文本会保留。
- `cancelled`：用户主动停止；已经进入结束事务的部分文本会保留。
- `interrupted`：服务端进程重启或执行丢失；只保留崩溃前已经提交到数据库的片段，未提交尾部不保证存在。

状态转换只能由后端执行。前端不得直接把数据库中的 Run 改成 `completed`。

### 8.1 正常执行

```text
Client                 HTTP Handler          Coordinator / Runtime       DB
  | POST message/send     |                         |                     |
  |---------------------->|                         |                     |
  |                       | validate + create run   |                     |
  |                       |------------------------>|                     |
  |                       |                         |-------------------->|
  |                       | start stream            | run-scoped context  |
  |<----------------------| start                   |                     |
  |                       |------------------------>| model request       |
  |                       |<------------------------| delta               |
  |<----------------------| text-delta              |                     |
  |                       |                         |                     |
  |                       |                         | finish              |
  |                       |                         | persist part/run    |
  |                       |                         |-------------------->|
  |<----------------------| text-end / finish       |                     |
  |<----------------------| [DONE]                  |                     |
```

关键顺序：

1. 校验请求和会话。
2. 在一个事务中创建用户消息、助手占位消息、Run 和第一个 Step，并提交初始 `RunEvent`。
3. Run Coordinator 使用独立 run-scoped context 启动执行；HTTP Handler 只订阅事件。
4. 发送 SSE 的 `start`，其中 `messageMetadata` 携带 `sessionId`、`runId` 和 `userMessageId`。
5. 接收模型增量，累计文本并持续发送 `text-delta`。
6. 模型结束后，在一个数据库事务中提交片段、消息状态、Step、Run 和终止事件。
7. 数据库提交成功后，发送 `text-end`、`finish` 和 `[DONE]`。

如果数据库提交失败，发送 `error`，Run 标记为 `failed`，不能用“前端已显示完整文本”代替持久化成功。

### 8.2 用户取消

1. 前端调用 `run/cancel` 并同时中止当前 HTTP 流。
2. Coordinator 取消该 Run 的 run-scoped context，模型流停止。
3. 保存已累计的部分文本并提交终止事务。
4. 助手 MessagePart、Message、Step 和 Run 标记为 `cancelled`。
5. 前端收到 `abort` 后保留已显示文本，发送按钮恢复可用。

### 8.3 失败

- 模型错误、供应商协议错误和持久化错误都统一映射为稳定错误码。
- 流开始前失败返回 JSON 错误。
- 流开始后先发送 transient `data-agent-error`，再发送 `error` chunk，最后发送 `[DONE]`。
- `data-agent-error` 携带 `code`、`kind`、`message` 和 `retryable`；`errorText` 只放用户可读文本。
- 已累计文本在结束事务成功时保存；事务本身失败时不能宣称已经保存。
- 用户消息不删除。

### 8.4 中断

MVP 不自动恢复原 Run。

服务启动时：

1. 查找所有 `running` Run。
2. 在事务中将 Run、Step、助手消息和仍为 `streaming` 的 Part 标记为 `interrupted`。
3. 写入 `run.interrupted` 事件并设置结束时间。
4. 保留崩溃前已经提交的片段；不伪造未落库的尾部文本。

用户重试时创建新的 Run，并通过 `retryOfRunId` 关联被中断的 Run。

## 9. 并发

同一个 `AgentSession` 同时只允许一个运行中的 Run。

MVP 使用服务端进程内锁：

- 当前进程已在执行该 `sessionId` 时返回 `409 AGENT_SESSION_BUSY`。
- Run 结束、失败、取消或中断后释放锁。
- 服务端重启后锁自然丢失。
- Coordinator 维护 `runId -> cancelFunc`，取消接口只取消目标 Run，不影响同一会话已结束的历史 Run。
- HTTP 连接关闭只取消 SSE handler 的写入订阅，不自动取消 Run；用户离开页面后执行仍可继续。
- 前端同时只允许一条 Live Run。切换会话不取消它，但其他会话暂时不能发送新消息。

该方案不适用于多实例 Web 横向扩展。后续接入 Redis 或数据库租约时，必须保持 API 返回的 `AGENT_SESSION_BUSY` 语义不变。

## 10. API

API 前缀为 `/api/v1`，只使用 `GET` 和 `POST`。

```text
GET  /api/v1/agent/sessions?limit=30&cursor=...
GET  /api/v1/agent/session?sessionId=...&messageLimit=40&beforeMessageId=...
POST /api/v1/agent/session/create
POST /api/v1/agent/message/send
POST /api/v1/agent/run/cancel
POST /api/v1/agent/message/retry
```

### 10.1 会话列表

`limit` 默认 30，最大 50；`cursor` 是包含 `updatedAt` 和 `id` 的不透明游标。

```json
{
  "data": {
    "sessions": [],
    "nextCursor": null,
    "configured": true
  }
}
```

`configured` 只表示服务端具备模型配置，不表示模型供应商当前一定可用。

### 10.2 会话详情

```json
{
  "data": {
    "session": {},
    "messages": [],
    "messageParts": [],
    "activeRun": null,
    "nextMessageCursor": null
  }
}
```

`messageLimit` 默认 40，最大 100。`messages` 只返回当前页消息；`messageParts` 返回用户消息片段，以及助手消息当前 `activeRunId` 对应的生成片段。`activeRun` 为 `running` 时，前端刷新后显示“生成中”，轮询该详情接口直到 Run 结束；MVP 不重放丢失的 token。

会话不存在时返回 `404 AGENT_SESSION_NOT_FOUND`。

### 10.3 创建会话

请求：

```json
{
  "clientRequestId": "client-request-id"
}
```

响应：

```json
{
  "data": {
    "session": {}
  }
}
```

创建成功后立即返回真实 `sessionId`。相同 `clientRequestId` 重试时返回同一个会话。前端不创建只在本地存在的临时会话。

### 10.4 发送消息

请求：

```json
{
  "sessionId": "session-id",
  "clientRequestId": "client-request-id",
  "message": {
    "id": "user-message-id",
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

流开始前的错误以 JSON 返回：

- `400 VALIDATION_ERROR`
- `404 AGENT_SESSION_NOT_FOUND`
- `409 AGENT_SESSION_BUSY`
- `503 AGENT_NOT_CONFIGURED`

成功响应为 SSE，不使用 JSON envelope。

相同 `clientRequestId` 和相同请求内容重试时，不能重复创建用户消息和 Run：

- Run 正在执行时返回 `409 AGENT_SESSION_BUSY`，并带上当前 `runId`。
- Run 已经结束时返回 `409 AGENT_REQUEST_ALREADY_PROCESSED`，并带上 `runId` 和最终状态；前端随后加载会话详情。
- 相同 `clientRequestId` 携带不同 `message.id` 或正文时返回 `409 AGENT_REQUEST_ID_CONFLICT`。

### 10.5 取消

请求：

```json
{
  "sessionId": "session-id",
  "runId": "run-id"
}
```

取消是幂等操作。Run 已经结束时返回当前状态，不视为异常。

响应：

```json
{
  "data": {
    "runId": "run-id",
    "status": "cancelled"
  }
}
```

### 10.6 重试

请求：

```json
{
  "sessionId": "session-id",
  "assistantMessageId": "assistant-message-id",
  "clientRequestId": "client-request-id"
}
```

只允许对 `failed`、`cancelled` 或 `interrupted` 的助手消息重试。服务端复用助手消息 ID，创建新 Run，不重复创建用户消息。
服务端从该消息的 `activeRunId` 读取原始 `userMessageId`，不接受前端通过数组位置推断来源。

## 11. 流式协议

### 11.1 协议选择

Go 服务端输出兼容 Vercel AI SDK UI Message Stream 的 SSE。

前端使用：

- `DefaultChatTransport` 发起请求并读取 `ReadableStream<UIMessageChunk>`。
- `readUIMessageStream` 将 chunk 流转换成连续的 `UIMessage` 快照。
- Leeat 自己的适配器把 `UIMessage` 映射为 `AgentMessage` 和 `AgentMessagePart`。

前端不使用 `useChat`，避免 AI SDK 内部消息状态与 Zustand 形成双源。

`DefaultChatTransport` 通过 `prepareSendMessagesRequest` 把 AI SDK 的发送参数映射为 Leeat 请求体：

- `sessionId`
- `clientRequestId`
- 最后一条用户 `UIMessage`，其中包含稳定的用户消息 `id`

服务端在 `start` 的 `messageMetadata` 中返回 `sessionId`、`runId` 和 `userMessageId`，在 `text-start` 中返回稳定的 `partId`。前端不能自行生成随后无法与数据库对应的助手消息或片段 ID。

```ts
type LeeatUIMessageMetadata = {
  sessionId?: string
  runId: string
  userMessageId?: string
  status?: AgentRunStatus
}
```

`messageMetadata` 由 Zod Schema 校验。`start` 必须包含 `sessionId`、`runId` 和 `userMessageId`，`finish` 必须包含 `runId` 和最终 `status`。缺少当前事件要求的字段时，整条流进入 `PROTOCOL_INVALID_METADATA`，不能把 `runId` 留空后继续写入 Zustand。

### 11.2 响应头

```text
Content-Type: text/event-stream
Cache-Control: no-cache, no-transform
Connection: keep-alive
x-vercel-ai-ui-message-stream: v1
x-accel-buffering: no
```

### 11.3 正常事件

```text
data: {"type":"start","messageId":"assistant-message-id","messageMetadata":{"sessionId":"session-id","runId":"run-id","userMessageId":"user-message-id"}}

data: {"type":"start-step"}

data: {"type":"text-start","id":"part-id"}

data: {"type":"text-delta","id":"part-id","delta":"你"}

data: {"type":"text-delta","id":"part-id","delta":"好"}

data: {"type":"text-end","id":"part-id"}

data: {"type":"finish-step"}

data: {"type":"finish","finishReason":"stop","messageMetadata":{"runId":"run-id","status":"completed"}}

data: [DONE]
```

### 11.4 错误事件

```text
data: {"type":"data-agent-error","id":"run-id","transient":true,"data":{"code":"AGENT_PROVIDER_FAILED","kind":"server","message":"模型调用失败，请重试","retryable":true}}

data: {"type":"error","errorText":"模型调用失败，请重试"}

data: [DONE]
```

`data-agent-error` 属于 AI SDK 的 transient custom data chunk，不进入消息历史；`errorText` 只承担用户可读文本。适配器必须先缓存结构化错误，再处理随后的 `error` chunk。

### 11.5 取消事件

```text
data: {"type":"abort","reason":"user_cancelled"}

data: [DONE]
```

取消不是错误，不显示错误 Toast。服务端完成部分文本和状态提交后，前端将消息标记为 `cancelled`。

### 11.6 协议不变量

- 每一条助手消息的流必须以 `start` 开始。
- `text-start`、`text-delta`、`text-end` 使用相同 `part-id`。
- 一次流只能出现一个终止事件：`finish`、`error` 或 `abort`。
- `[DONE]` 是传输结束哨兵，不是消息内容。
- 正常 EOF 前没有终止事件视为 `PROTOCOL_UNEXPECTED_EOF`。
- 重复终止事件视为协议错误。
- 已知结构错误必须进入失败状态，不能静默忽略。
- 未来新增的未知非终止 chunk 可以忽略并记录 debug 日志。
- 心跳使用 SSE 注释，不进入 `UIMessage` 和 Zustand。

## 12. 前端 MVC

### 12.1 AgentModel

负责：

- 调用会话列表、详情、创建、取消和重试 API。
- 通过 `DefaultChatTransport` 发起消息发送。
- 接收平台注入的 fetch 适配器；Web 使用同源请求，Desktop 使用 preload bridge。
- 通过 `readUIMessageStream` 读取快照。
- 将网络、HTTP、Schema 和协议错误统一转换为 `Result<T>`。
- 将 `UIMessage` 转换为 Leeat 领域消息。

不得：

- 读写 Zustand。
- 显示 Toast。
- 访问 React。
- 决定发送状态。

发送接口使用清晰的边界签名：

```ts
type AgentSendInput = {
  sessionId: string
  clientRequestId: string
  message: AgentMessage
  signal: AbortSignal
}

type AgentSendResult = {
  assistantMessageId: string
  runId: string
}

type AgentMessageSnapshot = {
  message: AgentMessage
  parts: AgentMessagePart[]
}

type AgentSendHandlers = {
  onStarted: (started: AgentSendResult) => void
  onSnapshot: (snapshot: AgentMessageSnapshot) => void
}

type SendMessage = (
  input: AgentSendInput,
  handlers: AgentSendHandlers,
) => Promise<Result<AgentSendResult>>
```

Model 在内部消费 AI SDK 流并调用 `onSnapshot`，但不直接修改 Store。流开始前的 HTTP 错误、Schema 错误和流中的协议错误都转换为 `Result`；异常不穿透到 View。

`DefaultChatTransport` 的构造参数由 Model 持有，不能由组件临时 new 一份。传输器关闭时释放平台 fetch 适配器和正在使用的 `MessagePort`。

### 12.2 AgentStore

保存：

- `sessions`
- `activeSession`
- `messagesBySessionId`
- `partsByMessageId`
- `agentConfigured`
- `listPhase`
- `sessionPhase`
- `sendPhase`
- `activeRunBySessionId`
- `liveRun`
- `nextSessionCursor`
- `nextMessageCursor`
- `error`

Store action 只做同步状态变更：

- `setSessions`
- `upsertSession`
- `setActiveSession`
- `setSessionMessages`
- `appendSessionMessage`
- `upsertSessionMessage`
- `upsertMessagePartByMessageId`
- `setActiveRunForSession`
- `setLiveRun`
- `setSendPhase`
- `setPagination`
- `setError`
- `resetAgent`

消息片段合并键固定为：

```text
messageId + partId
```

禁止以“当前最后一条消息”或数组下标作为身份。

### 12.3 AgentEffect

负责：

- 加载会话列表和详情。
- 创建会话。
- 发送消息。
- 将流式 `UIMessage` 快照映射到 Store。
- 在应用级维护 `runId -> AbortController`，页面切换不立即中止当前 Live Run。
- 调用取消和重试。
- 处理 Toast。
- 在 Run 结束后刷新当前会话元数据。

Effect 不保存长期会话内容。刷新恢复时必须重新从 Model 加载。它只保存短期资源：当前 Live Run、取消控制器和流订阅。Store 通过 `sessionId` 保存各会话已加载消息，因此切换到首页或其他会话不会丢失已经收到的内容。

### 12.4 View

负责：

- 会话侧栏。
- 消息气泡。
- Markdown 渲染。
- 输入框。
- 发送、停止和重试按钮。
- Loading、Empty、Error 和 NotConfigured 状态。

不得：

- 直接导入 Model。
- 执行 `fetch`。
- 拼接 SSE。
- 根据 token 数或数据库结构生成业务状态。

## 13. Go 服务端

### 13.1 模块边界

```text
server/internal/agent/
  controller.go
  model.go
  coordinator.go
  executor.go
  repository.go
  runtime.go
  stream_view.go
  contracts.go

server/internal/agent/provider/
  openai_compatible.go

server/internal/platform/
  httpapi/
  store/
```

职责：

- `controller.go`：HTTP 解码、校验、调用应用流程、取消和响应。
- `model.go`：Agent 领域对象和状态转换。
- `coordinator.go`：管理活动 Run、session 锁、run-scoped context 和取消入口。
- `executor.go`：脱离 HTTP 连接执行 Run、累计增量、提交结果并发布 Live Event。
- `repository.go`：会话、消息、片段、Run、Step 和 Event 的存储接口。
- `runtime.go`：Runtime 接口和 RunInput/RunEvent。
- `stream_view.go`：领域事件到 UI Message Stream SSE 的编码。
- `provider/`：具体模型供应商协议适配。

### 13.2 Runtime 接口

Runtime 接收已经验证的输入，并持续发送事件：

```go
type Runtime interface {
    Stream(ctx context.Context, input RunInput, emit EventSink) error
}
```

`RunInput` 至少包含：

- Run ID
- System Prompt
- 历史消息
- 当前用户消息
- 模型配置

Runtime 不接收数据库连接、HTTP ResponseWriter 或 Zustand 类型。

`Stream` 使用的 `ctx` 来自 Executor 的 run-scoped context，而不是 HTTP Handler 的 request context。模型调用默认超时为 120 秒，SSE 每 15 秒发送一次注释心跳；心跳不写数据库，也不进入 `UIMessage`。

### 13.3 模型供应商

MVP 使用 OpenAI-compatible 的 Chat Completions 流式接口。供应商原始事件只存在于 provider 适配器内部。

配置基线：

- `LEEAT_AGENT_BASE_URL`
- `LEEAT_AGENT_API_KEY`
- `LEEAT_AGENT_MODEL`
- `LEEAT_AGENT_SYSTEM_PROMPT`，可选

API Key 只由 Go sidecar 从环境或系统安全存储读取。开发态允许环境变量；桌面生产态不能把 Key 写入 Renderer、preload 或前端构建产物。

转换链路：

```text
Provider SSE
  -> Provider Adapter
  -> Leeat RunEvent / RunDelta
  -> UI Message Stream Encoder
  -> Browser
```

前端永远不依赖供应商原始 chunk 名称。

## 14. 上下文策略

MVP 不实现自动摘要。

- 发送给模型的历史只包含 `complete` 的助手消息。
- `failed`、`cancelled` 和 `interrupted` 的旧尝试不作为模型上下文。
- 最多保留最近 40 条完整消息。
- 输入正文总量默认上限为 20,000 个 Unicode 字符；超限时从最旧的完整对话轮次开始截断，并至少保留当前用户消息。
- 截断时不能从一条助手消息的中间开始。
- 数据库保留完整会话历史，不因模型上下文截断而删除数据。

后续增加摘要或检索时，摘要结果作为独立 Artifact 或系统上下文，不覆盖原始消息。

## 15. UI 与交互

### 15.1 消息

- 用户消息右对齐，助手消息左对齐。
- 助手消息支持 GitHub Flavored Markdown。
- 禁止渲染原始 HTML。
- 代码块支持横向滚动。
- 消息最大宽度约 78%，窄屏约 88%。
- 长 URL、长单词和代码不能撑破主内容区。
- 用户消息使用品牌色，助手消息使用次级表面。

### 15.2 输入

- `Enter` 发送。
- `Shift + Enter` 换行。
- 判断 IME composing，中文选词期间不发送。
- 空白内容不能发送。
- 输入框最多显示约 6 行。
- 流式期间发送按钮变为停止按钮。
- 请求进行中禁止重复提交。

### 15.3 状态

- Loading：侧栏和会话内容使用 Skeleton。
- Empty：无会话或无消息时显示空状态。
- Streaming：显示已接收文本和停止按钮。
- Error：显示可理解错误和重试入口。
- NotConfigured：显示模型未配置提示，不显示假回复。
- Interrupted：显示上次生成中断和重试入口。

### 15.4 响应式

- 390px、768px、1024px 和 1440px 不能出现全局横向滚动。
- Agent 二级侧边栏不能消失。
- 窄屏新建按钮只显示图标。
- 窄屏会话项可以只显示首字或图标，但完整标题通过 `aria-label` 提供。
- 输入区始终可达，消息区独立滚动。

### 15.5 无障碍

- 当前导航使用 `aria-current="page"`。
- 新建、发送、停止等图标按钮必须有可访问名称。
- 消息列表使用 `role="log"`。
- 流式状态使用节制的 `aria-live="polite"`，不逐 token 播报。
- 焦点顺序、Dialog 和快捷键符合现有工作台规范。
- 颜色不是状态的唯一表达方式。

## 16. 安全与隐私

- 模型 API Key 只存在于服务端环境或系统安全存储。
- Renderer、preload、Zustand 和日志中都不能出现 API Key。
- 日志不记录用户消息正文、模型完整响应、文件内容和完整本地路径。
- 错误信息可以包含稳定错误码和用户提示，不包含供应商原始报错。
- Desktop sidecar 只监听 `127.0.0.1`。
- Renderer 不直接控制 sidecar 端口和鉴权 Token。
- Electron 主进程生成短期 Token，通过 sidecar 启动参数或受保护握手传递；Token 不进入前端 Store、URL、日志或异常信息。
- Agent 流经主进程窄范围 transport bridge 转发，Renderer 不能指定任意上游地址。
- 外链、文件选择和系统能力通过 preload 的最小 API 暴露。

## 17. 测试设计

产品行为、API、契约和桌面验收测试位于 `tests/e2e`。Go 中确有必要的未导出纯逻辑测试可以放在对应 package 的 `_test.go`，但不能替代这里的黑盒场景。

### 17.1 Unit

```text
tests/e2e/unit/agent/agentStore.test.ts
tests/e2e/unit/agent/agentModel.test.ts
tests/e2e/unit/agent/agentMessageReducer.test.ts
tests/e2e/unit/agent/agentStreamProtocol.test.ts
```

覆盖：

- Store 同步 action。
- `messageId + partId` 合并。
- `Result<T>` 错误映射。
- 取消不显示错误 Toast。
- `UIMessage` 快照到领域消息的转换。
- SSE 顺序、终止和畸形事件。

### 17.2 Integration

```text
tests/e2e/integration/agent/repository.integration_test.go
tests/e2e/integration/agent/controller.integration_test.go
tests/e2e/integration/agent/runtime_contract.integration_test.go
```

覆盖：

- 会话、消息、片段、Run、Step 和 Event 持久化。
- `messageId` 归属和片段排序。
- `clientRequestId` 幂等。
- 同一会话并发限制。
- 浏览器断开后 Run 继续完成，以及取消接口只取消目标 Run。
- 流开始前和流开始后的错误格式。
- 进程启动时将 running Run、Step、Message 和 streaming Part 标记为 interrupted。
- 会话列表和消息详情遵守默认分页上限。

### 17.3 Browser

```text
tests/e2e/modules/agent/navigation.e2e.ts
tests/e2e/modules/agent/sessionSidebar.e2e.ts
tests/e2e/modules/agent/chat.e2e.ts
tests/e2e/modules/agent/streamFailure.e2e.ts
tests/e2e/modules/agent/responsive.e2e.ts
```

覆盖：

- 一级导航进入 Agent。
- 二级会话侧栏始终存在。
- 新建会话后进入真实 URL。
- 刷新后恢复消息。
- 生成期间刷新时识别 `activeRun`，且不会创建第二条 Run。
- 切换会话不取消当前 Live Run，其他会话发送按钮暂时禁用。
- 多轮消息上下文。
- 停止生成保留部分文本。
- 失败、重试和中断状态。
- 中文输入法组合期间不误发送。
- 390px 和 1440px 的 Light / Dark。

### 17.4 Fake Runtime

E2E 不调用真实模型。Fake Runtime 通过场景脚本控制：

- 固定文本增量。
- 延迟输出。
- 多轮回声。
- 模型错误。
- 用户取消。
- 连接断开。
- 畸形事件。
- 乱序事件。
- 重复终止事件。
- EOF 前没有终止事件。

### 17.5 Desktop

```text
tests/e2e/desktop/agentStartup.desktop.ts
tests/e2e/desktop/agentOffline.desktop.ts
tests/e2e/desktop/agentCrashRecovery.desktop.ts
```

覆盖：

- Electron 启动并拉起本地 sidecar。
- 应用离线启动和本地会话恢复。
- Renderer 不持有 API Key。
- preload 只暴露允许的 IPC。
- 打包后的窗口能够加载并发送一条 Fake Runtime 消息。
- sidecar 崩溃后自动重启，旧 Run 被标记为 interrupted，界面提供重试。

## 18. 实施顺序

1. 建立 Agent 领域类型、数据库迁移和 Repository。
2. 实现 Fake Runtime、RunEvent、MessagePart、Run Coordinator 和 SSE Encoder。
3. 实现 Go Controller、幂等发送、取消、断线脱离和中断清理。
4. 接入 Electron sidecar 启动、主进程 transport bridge 和本地数据库路径。
5. 实现 `AgentModel`、`AgentStore` 和 `AgentEffect`。
6. 实现 Agent 两级侧边栏、消息区、输入区和状态页面。
7. 接入 OpenAI-compatible Chat Completions provider。
8. 完成 browser、integration 和 desktop 测试。
9. 最后接入真实模型进行人工验收。

前端可先使用可编排 Fake Runtime 完成 UI 和状态验证，但 Fake Runtime 不能成为生产默认数据源。

## 19. 后续扩展

### 19.1 工具

- 新增 `AgentTool` 定义和 Tool Registry。
- 新增 `tool-call`、`tool-result` MessagePart。
- 每次工具调用成为新的 `AgentStep`。
- 工具输入输出需要超时、权限和日志脱敏。

### 19.2 Skills

- Skill 是可版本化的系统指令和资源集合。
- Skill 注入 Runtime 上下文，不直接变成用户消息。
- Run 保存使用的 Skill 版本，便于评测复现。

### 19.3 MCP

- MCP Client 位于 Go Runtime 侧。
- MCP Server 配置和凭据按权限隔离。
- MCP 调用复用工具 Step 和 Artifact 语义。

### 19.4 多 Agent

- 通过父子 Run 或 `parentStepId` 关联。
- 子 Agent 的最终输出作为父 Run 的 ToolResult 或 Artifact，不共享可随意修改的 Store 状态。
- 当前协议预留 Run 关系字段，不提前实现编排器。

### 19.5 恢复与 Redis

当前只支持用户手动重试。后续依次比较：

1. Stalled Run 检测和从头重试。
2. Step 检查点恢复。
3. Redis 流缓存、租约和任务队列。
4. Temporal、Inngest 类持久化执行引擎。

在方案确定前，不为 Redis 预设唯一职责。

## 20. 完成标准

- 一级导航包含首页、Agent 和设置。
- Agent 页面有独立二级会话侧栏。
- 会话和消息来自真实 Store 持久化，不使用示例数据。
- 刷新后恢复已提交的会话内容；正在运行的 Run 能被识别，完成后重新加载最终片段。
- 同一会话多轮上下文可继续发送。
- 流式 token 实时显示，但不逐条写数据库。
- 用户取消和普通失败可以保留已提交的部分文本；硬中断只保证已提交片段，并明确提供重试入口。
- 同一 `clientRequestId` 不会重复创建消息和 Run。
- 前端使用 Vercel AI SDK Transport 和 UI Message Stream，不使用 `useChat`。
- Zustand 是前端业务状态唯一事实源。
- Go Runtime 不访问数据库，Repository 不依赖 HTTP。
- `src` 不包含供应商原始事件解析，`server` 不输出未经转换的供应商协议。
- 模型 Key 不进入 Renderer、Store 或日志。
- 产品行为、API、契约和桌面验收测试位于 `tests/e2e`；Go package 白盒测试不替代这些场景。
- 390px、768px、1024px 和 1440px 的 Light / Dark 均通过验证。
- 真实 Electron 进程完成启动、离线恢复和一条 Fake Runtime 会话。
