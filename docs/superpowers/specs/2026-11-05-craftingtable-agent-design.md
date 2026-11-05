# CraftingTable Agent 设计

日期：2026-11-05
状态：已确认
实施状态：单 Agent 本地持久化与刷新恢复已实现，后续能力分阶段落地
范围：单 Agent 会话、Run、消息片段、流式协议、持久化与后续能力扩展
平台顺序：Desktop -> Web -> 手机
工程约束：[CraftingTable 工程化设计规范](./2026-11-05-craftingtable-engineering-design.md)
术语基线：[CraftingTable 术语表](../../glossary.md)

## 1. 设计目的

CraftingTable Agent 是工作台中的个人助手，不是独立聊天产品的复制品。

当前阶段只实现单 Agent 多轮会话，但领域模型、流事件和持久化结构必须为后续 Skills、工具、MCP、多 Agent 编排和评测保留稳定扩展点。

本文是 Agent 能力的长期最高规格。单 Agent、本地 SQLite 或当前供应商适配只是实施阶段，不是永久架构限制；任何临时实现都必须满足本文的契约、恢复、安全和测试不变量。

本文解决以下问题：

- 会话、消息、片段、Run 和 Step 分别是什么。
- 一次 Agent 回复如何从 Go 服务端流到 Zustand Store。
- token 增量为什么不能逐条写数据库。
- 每个消息片段如何归属到正确的消息和会话。
- 用户取消、模型失败、浏览器断线和进程重启后是什么状态。
- 吸收 Vercel AI SDK 的传输与解析分层时，如何避免状态双源。
- 后续工具、Skills、MCP 和多 Agent 如何接入而不重写前端。

## 2. Agent Runtime 的含义

`Agent Runtime` 是单次供应商调用的运行适配层，不是完整 Agent Loop。

它负责：

- 接收已经校验的 `RunInput`。
- 调用模型供应商或后续的本地模型。
- 将供应商事件转换成 CraftingTable 领域事件。
- 将增量输出交给流编码器。
- 响应取消和超时。

它不负责：

- 读写数据库。
- 修改 Zustand Store。
- 决定页面布局或 Toast。
- 决定会话标题。
- 直接渲染 Markdown。

因此，`Agent Runtime` 不是 Agent 产品本身，也不是 Agent Loop 的同义词。它是服务端领域与具体模型 SDK、模型供应商之间的单步执行边界。

三个词必须分开：

- `Run`：一次用户意图从提交到终态的完整执行。
- `Step`：Run 内一个有边界的模型、工具或 MCP 操作。
- `AgentExecutor`：负责循环、状态机、取消、限制和终态收敛的完整执行器。

首个实施阶段只有模型 Step，但 Executor 和 `RunPolicy` 必须按多 Step 语义建模，不能把“一次模型调用”写死成 Run 的全部含义。

## 3. 目标与非目标

### 3.1 当前目标

- 提供 Agent 一级入口和独立二级会话侧边栏。
- 创建、加载和切换单 Agent 会话。
- 发送用户消息并流式显示助手回复。
- 同一会话内支持多轮上下文。
- 会话、消息和已展示片段写入本地数据库。
- 刷新后恢复数据库已经提交的消息内容，并识别仍在服务端运行的 Run。
- 浏览器短暂断线或刷新后，通过 Run 游标继续订阅同一服务端 Run。
- 支持停止生成。
- 支持失败或中断后的手动重试。
- 前端吸收 Vercel AI SDK 的三层 pipe-through 思路，由项目自有 `agentTransport.ts`、`eventsource-parser`、`agentStreamProtocol.ts` 和 `apiAgentModel.ts` 处理流；生产不依赖 `ai` 或 `@ai-sdk/react`，也不自建通用 SSE 分词器。
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
- 不实现服务端进程重启后的原 Run 续传、跨进程执行或迁移。
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

### 3.4 架构演进边界

本文的完整目标始终有效，`3.2 当前非目标`只是阶段边界，不是对最终架构的拒绝。

- 当前由单进程 Run Coordinator 直接调用模型 Provider 并通过 SSE 返回，是因为本阶段只有模型 Step，没有本地工具执行。
- 引入工具、本地文件访问、命令执行、Skills 或 MCP 时，必须先建立独立的执行边界和隔离工作目录，不能继续把工具循环塞进 HTTP Controller。
- 单进程后台执行、浏览器断线重连和启动中断收敛已经实现；引入多实例、跨进程恢复或故障接管时，必须先稳定租约和事件载体语义，再决定队列与 Redis 的职责。
- SQLite、单进程锁和直接 SSE 都是当前适配器，不得泄漏成 Repository、Application 或前端契约的一部分。
- 任何阶段都不得删除 `Run` 的持久身份、事件顺序、取消和错误语义，只为缩短当前实现。

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

当前阶段只允许 Agent 工作区同时存在一条正在接收的 Live Run。切换会话、进入首页或进入设置不会取消当前 Run；当前 Run 以 `sessionId + runId` 留在应用级 AgentEffect 中继续更新 Store。只要 Live Run 尚未终止，其他会话的发送按钮保持禁用，避免前端同时维护多个流和多个活动消息。

## 5. 总体架构

```text
┌─────────────────────────────────────────────────────────────┐
│ React Renderer                                              │
│                                                             │
│ View --读取--> AgentStore                                    │
│   |                                                         │
│   +--动作--> AgentEffect --> AgentModel                     │
│                 |             -> AgentTransport             │
│                 |             -> AgentStreamProtocol        │
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
│ Agent Stream View -> UI Message Stream-compatible SSE       │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│ Local Database                                              │
│ sessions / messages / message_parts / runs / steps / events │
└─────────────────────────────────────────────────────────────┘
```

### 5.1 Agent 分层与依赖方向

分层图：[CraftingTable Agent 分层与依赖方向](../../architecture/agent-layered-architecture.png)

参考 `pi-agent-harness` 的职责分层，但不复制其 npm 包划分。CraftingTable 固定为五层：

| 层 | 归属 | 职责 |
| --- | --- | --- |
| Experience | React Renderer | View、AgentEffect、AgentModel、Zustand Store。只通过 Model 和平台 Transport 访问服务端。 |
| Access | Go `agent/access` | HTTP Controller、SSE Stream View、请求解码和传输协议编码。不承载 Agent Loop 或持久化事务。 |
| Application | Go `agent/application` | 会话用例、Application Controller、Run Coordinator、执行槽、幂等和取消入口。不实现模型调用或具体数据库。 |
| Core | Go `agent/core` | AgentExecutor、Run/Step 状态机、RunPolicy、上下文组装和终态收敛。只依赖 Core 拥有的端口。 |
| Adapters / Infrastructure | `agent/adapters` 与 `platform` | Provider Adapter、Repository 实现、SQLite/PostgreSQL、Logger、Clock、ID 和后续 Telemetry。 |

依赖方向固定为：

```text
Experience
  -> Access
  -> Application
  -> Core

Adapters / Infrastructure
  -> 实现 Application 或 Core 拥有的 Port

Bootstrap
  -> 创建具体 Adapter
  -> 注入端口实现
```

强制边界：

- Core 不得导入 HTTP Controller、SSE 编码器、SQLite、PostgreSQL、Provider SDK 或 React 类型。
- Application 不得依赖具体 Provider、数据库驱动或 Electron API。
- Access 不得直接执行 Agent Loop、写 Repository 或持有 Run 的跨请求执行上下文。
- Provider 和 Repository 通过 Port 向内依赖，不能反向要求 Core 导入具体实现。
- 具体依赖只在 `bootstrap` 或模块 composition root 中装配。
- Logger、Clock、ID 和 Telemetry 作为可替换基础设施能力注入；业务代码不直接绑定厂商 SDK。
- UI 和辅助集成不放在同一抽象层。Web UI 属于 Experience；Slack、GPU Pod 等未来集成属于独立 Adapter，不进入 Agent Core。

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

Desktop 的 Renderer 不持有 sidecar 端口和 Token。`AgentModel` 接收一个与标准 fetch 同形的平台适配器：

- Web：适配器直接使用全局 `fetch` 调用同源 `/api/v1`。
- Desktop：适配器通过窄范围 preload IPC 创建 `MessagePort`，由主进程请求已登记的 sidecar 路径。
- 两者都返回标准 `Response`；`agentTransport.ts` 只读取状态并返回 `response.body`，不能把 fetch 适配器的返回类型写成领域消息。

主进程只代理已登记的 Agent 路径和请求方法，不接受任意 URL。`AbortSignal` 不能被 IPC 或 `MessagePort` 直接结构化克隆，因此只存在于 Renderer 的 fetch 适配器内部；跨进程取消必须编码成帧。桥接协议固定为：

```ts
type AgentBridgeOpenRequest = {
  requestId: string
  path: string
  method: "GET" | "POST"
  headers: Record<string, string>
  body?: Uint8Array
}

type AgentBridgeFrame =
  | {
      type: "opened"
      requestId: string
      status: number
      headers: Record<string, string>
    }
  | {
      type: "chunk"
      requestId: string
      sequence: number
      body: Uint8Array
    }
  | { type: "ack"; requestId: string; throughSequence: number }
  | { type: "cancel"; requestId: string; reason: "renderer_aborted" }
  | { type: "ended"; requestId: string }
  | {
      type: "failed"
      requestId: string
      error: { code: CraftingTableErrorCode; message: string }
    }
```

每个请求使用独立 `MessageChannel`。主进程先发送唯一的 `opened` 帧，再按严格递增的 `sequence` 发送有界字节块。Renderer 构造 `ReadableStream<Uint8Array>` 和 `new Response(body, { status, headers })`；本地 `AbortSignal` 只触发一个 `cancel` 帧，不尝试跨进程传递 signal。

流控规则：

- 主进程最多保留 16 个未确认 chunk 或 1 MiB 未确认字节，任一上限先到即暂停读取 Go 响应。
- Renderer 只有在 chunk 进入可消费队列后才发送 `ack`；ack 单调递增，不能回退。
- 每次请求恰好一个 `ended` 或 `failed` 终止帧，取消使用 `failed` 和 `REQUEST_CANCELLED`，不能同时发送两种终止帧。
- 端口关闭、超时、页面销毁和应用退出都走同一幂等清理路径，释放 `MessageChannel` 并取消主进程到 sidecar 的 HTTP 请求。
- 主进程取消的是当前 HTTP 流订阅，不自动取消服务端 Run；用户点击“停止”时必须先调用 `run/cancel`，再关闭本地流。

桥接链路自身故障必须映射为 `Result`，不能把 `MessagePort` 异常泄漏到 View。

页面刷新、浏览器离线或 SSE 读取端关闭只结束当前展示订阅，不取消服务端 Run。只有用户执行“停止”、窗口销毁或应用退出才取消对应 Run。

### 5.2 Desktop sidecar 生命周期

首发 Desktop 基线为 Windows x64。macOS 和 Linux 的签名、路径和进程管理在 Web 版本完成后单独验收，不提前宣称跨平台完成。

启动流程：

1. Electron 主进程生成高熵短期 Token，通过环境变量传入 sidecar，不放入命令行参数。
2. Go sidecar 绑定 `127.0.0.1:0`，由操作系统分配随机端口。
3. sidecar 在 stdout 输出一次有长度上限的 JSON 握手，包含协议版本和实际端口；其他日志走 stderr 或文件。
4. 主进程在超时时间内解析握手并调用带 Token 的 `/healthz`，成功后把 transport bridge 标记为可用。
5. 握手失败、端口异常或健康检查失败时，主进程关闭子进程并显示可恢复的启动错误，不能把 Token 写进日志。

运行约束：

- 生产数据库位于 `app.getPath("userData")/craftingtable/craftingtable.db`，安装目录只读且不能保存用户数据。
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

标题不参与模型上下文。当前实施阶段使用首个用户消息去除首尾空白后的前 32 个 Unicode 字符生成标题，用户可以后续再增加重命名能力。

服务端生成的 Session、助手的 Message、Run、Step 和 Part 默认使用 UUIDv7。客户端发送的用户消息 ID 与 `clientRequestId` 也使用 UUIDv7，并由服务端校验格式。消息顺序由不可变 `seq` 决定，不能把 UUIDv7 时间片段当作业务顺序。

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
  seq: number
  role: AgentMessageRole
  status: AgentMessageStatus
  activeRunId?: string
  createdAt: string
  updatedAt: string
}
```

消息本身不保存可展示正文。正文由多个 `AgentMessagePart` 组成，避免未来工具调用、文件和图片被塞进一个字符串。

发送请求不复用持久化 `AgentMessage`，而使用明确携带正文的出站类型：

```ts
type CraftingTableSendMessage = {
  id: string
  role: "user"
  parts: Array<{
    type: "text"
    text: string
  }>
}
```

服务端先校验 `CraftingTableSendMessage`，再创建 `AgentMessage` 和对应的用户 `AgentMessagePart`。用户消息只能为 `complete`，不能带 `activeRunId`；`streaming` 只允许出现在助手消息上。

### 6.3 AgentMessagePart

`AgentMessagePart` 是消息中的一块可展示内容。

```ts
type AgentMessagePart = AgentTextPart | AgentErrorPart

type AgentMessagePartBase = {
  id: string
  messageId: string
  runId?: string
  index: number
  status: "streaming" | "complete" | "cancelled" | "failed" | "interrupted"
  createdAt: string
  updatedAt: string
}

type AgentTextPart = AgentMessagePartBase & {
  type: "text"
  content: AgentTextPartContent
}

type AgentTextPartContent = {
  type: "text"
  text: string
}

type AgentErrorPart = AgentMessagePartBase & {
  type: "error"
  content: {
    type: "error"
    error: AgentError
  }
}
```

`error` 片段只用于在失败的助手消息中展示结构化错误和重试入口，不属于模型上下文。后续可增加：

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
  requestHash: string
  policyVersion: string
  retryOfRunId?: string
  status: AgentRunStatus
  model: string
  startedAt: string
  updatedAt: string
  finishedAt?: string
  errorCode?: string
}
```

一个 Run 只对应一条助手消息的一次生成尝试。重试创建新的 Run，并通过 `retryOfRunId` 关联上一次执行。

`userMessageId` 明确记录触发本次执行的用户消息。重试不能通过“助手消息前一条”推断用户消息，否则插入历史分页、并发或未来消息编辑后会产生错误上下文。

`RunPolicy` 是随 Run 保存的不可变执行策略：

```ts
type RunPolicy = {
  version: string
  maxSteps: number
  maxDurationMs: number
  idleTimeoutMs: number
  maxConsecutiveFailures: number
  maxToolCalls?: number
}
```

首个实施阶段的策略允许一个模型 Step，但字段和持久化结构必须完整。默认基线为 `maxSteps=1`、`maxDurationMs=120000`、`idleTimeoutMs=30000`、`maxConsecutiveFailures=1`。策略变更必须提升 `version`，新 Run 读取新版本，旧 Run 仍按已保存版本解释。

### 6.5 AgentStep

`AgentStep` 表示 Run 内一个有明确边界的执行步骤。

```ts
type AgentStepKind = "model" | "tool" | "skill" | "mcp" | "subagent"

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

当前实施阶段只允许 `model` Step。后续工具、Skill、MCP 和子 Agent 调用分别成为新的 Step，不把多次调用压缩成一个无结构字符串；未启用的 Step 类型必须由 Runtime 和 Schema 明确拒绝。

### 6.6 AgentExecutor

`AgentExecutor` 持有 Run 状态机，`AgentRuntime` 只执行其中一次有界操作。Executor 的固定流程为：

```text
Executor
  -> acquire immutable RunPolicy
  -> for stepIndex in 1..RunPolicy.maxSteps:
       create Step and emit step.started
       choose the next bounded action
       execute one model/tool/skill/mcp operation
       emit step.finished
       if a final answer exists: complete the Run
       if failure count reaches maxConsecutiveFailures: fail the Run
  -> maxSteps reached without final answer:
       fail with AGENT_STEP_LIMIT_EXCEEDED
```

Executor 必须在每一步检查 `maxDuration`、`idleTimeout` 和取消信号。取消、超时、进程中断和策略超限都必须进入唯一终态，不能仅停止 goroutine。策略允许多 Step 后，Executor 负责选择下一种动作；Runtime 不得自行递归调用或偷偷扩展 Step 数量。

### 6.7 RunEvent 与 RunDelta

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

事件 payload 固定如下：

| 事件 | 必填 payload | 明确禁止 |
| --- | --- | --- |
| `run.started` | `model`、`policyVersion` | 用户正文、API Key |
| `step.started` | `kind`、`index`、`policyVersion` | 模型完整输入 |
| `message.part.started` | `partType`、`index` | 完整正文 |
| `message.part.finished` | `status`、`textLength`、`textSha256` | 完整正文、Markdown 快照 |
| `step.finished` | `status`、`errorCode?` | 供应商原始响应 |
| `run.completed` | `status`、`stepCount` | 消息正文 |
| `run.failed` | `status`、`errorCode`、`stepCount` | 堆栈和第三方原文 |
| `run.cancelled` | `status`、`reason`、`stepCount` | 消息正文 |
| `run.interrupted` | `status`、`reason`、`stepCount` | 未落库的推测正文 |

`MessagePart` 是可展示正文的唯一持久化来源。`RunEvent` 只保存语义生命周期和正文完整性摘要，`RunDelta` 只保存当前 SSE 的临时增量；三者不能互相替代。

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

- `RunEvent` 只有在进入 durable event 白名单时才持久化；白名单固定为 `run.started`、`step.started`、`message.part.started`、`message.part.finished`、`step.finished`、`run.completed`、`run.failed`、`run.cancelled`、`run.interrupted`。
- 同一个 Run 只有一个 event writer。写入者在事务内锁定 Run 的事件序号计数器，分配 `sequence` 后立即插入事件；`runId + sequence` 唯一约束是最后防线，禁止多个 goroutine 并发分配序号。
- `sequence` 从 1 开始，在同一 Run 内严格单调递增且唯一。事务失败可以留下空洞，但不能复用或回填。计数器必须在插入同一事务中通过行锁递增，不能先读最大值再在事务外写入。
- 初始事务写入 `run.started`、`step.started` 和 `message.part.started`；终止事务写入 Part 结束事件、Step/Run 终止事件。
- `text-delta` 不逐条写数据库。
- `text-delta` 由服务端在内存中累计。
- 初始事务同时插入一条空文本 `AgentMessagePart`，状态为 `streaming`；Run 结束、失败或取消时用 UPDATE 写入最终文本和终态。
- 最终 Part、消息状态、Step、Run 状态和终止 `RunEvent` 在同一事务提交；事务失败时，不能先向浏览器发送成功终止事件。
- 前端可以实时合并 token，但刷新后以数据库中的最终片段为准。
- 硬崩溃时无法执行结束分支；启动恢复会把仍为 `streaming` 的空 Part 标记为 `interrupted`。进程内尚未提交的文本允许丢失，不伪造未落库内容。
- 终态事务使用执行 context 之外的有界持久化 context 和有限重试；重试仍失败时记录稳定内部错误，并依靠启动恢复和 stalled-run watchdog 收敛。

这样既保证流式体验，也避免一条长回复产生数千次数据库写入。

## 7. 持久化模型

目标持久化边界为 Desktop SQLite、Web PostgreSQL。两者通过同一 Repository 契约实现，Repository 接口不得暴露具体数据库类型。SQLite 只支持当前单用户本地优先部署；Web 的 PostgreSQL 连接、迁移、备份和并发方案必须在 Web 实施前经过架构决策，不得假设 SQLite 行为可直接平移。

概念表：

```text
agent_sessions
agent_messages
agent_message_parts
agent_runs
agent_steps
agent_run_events
agent_idempotency_keys
agent_artifacts
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
- `agent_idempotency_keys` 使用 `(operation, client_request_id)` 唯一约束；`operation` 取 `session.create`、`message.send` 或 `message.retry`。
- `agent_idempotency_keys.request_hash` 保存规范化请求摘要，并关联已创建的 `session_id` 或 `run_id`。请求摘要由服务端对已校验的规范 JSON 计算 SHA-256，客户端不能提交或覆盖摘要。
- `agent_sessions.client_request_id` 和 `agent_runs.client_request_id` 保留为审计字段，但幂等判断只以 `agent_idempotency_keys` 为准，避免三个入口各自实现一套规则。
- `agent_runs.policy_json` 保存不可变 `RunPolicy` 快照，`policy_version` 是可查询摘要。
- `agent_messages` 使用 `(session_id, seq)` 唯一约束；`seq` 在会话内单调递增且不可修改。
- 用户消息片段使用 `(message_id, index) WHERE run_id IS NULL` 的唯一索引。
- 助手生成片段使用 `(message_id, run_id, index) WHERE run_id IS NOT NULL` 的唯一索引，保证重试产生的片段不会互相覆盖。
- `agent_run_events` 使用 `(run_id, sequence)` 唯一约束。
- `agent_runs.next_event_sequence` 是内部单调计数器；event writer 在同一事务中锁定 Run 行、分配并递增该值，再插入事件。公开 API 不暴露该计数器。
- `agent_messages.active_run_id` 指向的 Run 必须属于同一条助手消息；写入时由事务校验，并用数据库触发器或约束测试防止越界关联。
- 数据库约束必须拒绝“用户消息带 `activeRunId`”。助手消息的 `activeRunId` 表示当前展示的 Run，即使该 Run 已 `completed`、`failed`、`cancelled` 或 `interrupted` 也必须保留；它不能引用其他助手消息的 Run。
- 每个助手消息最多只有一个 `running` Run，使用部分唯一索引或等价事务约束保证。
- 会话列表按可变 `activity_seq` 倒序，但游标必须携带查询快照上界；消息分页使用不可变 `seq`，片段按 `index` 升序。

同一个助手消息发生重试时，保留旧 Run 和旧片段。`agent_messages.active_run_id` 指向当前展示的 Run，会话详情只加载该 Run 的片段。这样既保留执行历史，也不会在同一气泡中混入多次尝试。

会话列表默认每次读取 30 条，消息详情默认每次读取 40 条。会话游标允许跨页出现新活动，但同一快照内不得重复或缺失；消息游标必须只依赖不可变 `seq`。

### 7.1 大对象与 Artifact

文本消息仍保存在 `agent_message_parts`，但图片、附件、完整工具日志、Patch 和其他大对象不能直接塞入 MessagePart 或 RunEvent。它们使用独立的 `agent_artifacts` 元数据：

```ts
type AgentArtifactRef = {
  id: string
  runId?: string
  messagePartId?: string
  kind: "image" | "file" | "patch" | "tool-log" | "trace" | "other"
  mimeType?: string
  sizeBytes: number
  sha256: string
  storageKind: "local-file" | "object-storage"
  objectKey: string
  createdAt: string
}
```

规则：

- SQLite 和 PostgreSQL 只保存 Artifact 元数据、可公开预览和 `objectKey`，不保存无上限二进制正文。
- Desktop 本地内容位于应用数据目录的受控 Artifact 根目录；数据库和 API 不暴露完整本地路径。
- Web 对象存储供应商、加密、签名 URL、保留期和删除流程在 Web 架构决策门中确定。
- Artifact 写入按 `sha256 + sizeBytes` 校验完整性；缺少对象、摘要不符或超过大小上限时返回稳定错误，不能显示破损内容。
- 删除会话或清理历史时必须同时处理数据库记录和外部对象，不能留下无主文件。

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

1. 校验请求、规范化正文并计算 `requestHash`。
2. 按 `sessionId` 原子获取会话执行槽；未获得槽时立即返回 `AGENT_SESSION_BUSY`。
3. 持有执行槽后查询幂等记录并比较 `requestHash`；只有首次请求才能继续创建实体。
4. 在一个事务中创建幂等记录、用户消息、助手占位消息、空文本 Part、Run、第一个 Step 和初始 durable `RunEvent`。
5. Coordinator 注册 `runId -> cancelFunc`，创建 run-scoped `execCtx` 和 Live Event 订阅；订阅建立后才能启动 Executor。
6. 发送 SSE 的 `start`，其中 `messageMetadata` 携带 `sessionId`、`assistantMessageId`、`runId` 和 `userMessageId`。
7. Executor 按 `RunPolicy` 执行 Step，接收模型增量，累计文本并持续发送 `text-delta`。
8. 模型结束后，使用 `persistCtx = WithTimeout(WithoutCancel(execCtx), 5s)` 在一个数据库事务中更新片段、消息状态、Step、Run 和终止事件。
9. 数据库提交成功后，发送 `text-end`、`finish` 和 `[DONE]`，然后释放会话执行槽。

如果数据库提交失败，使用同一个有界持久化策略有限重试。仍然失败时发送 `error`，释放执行槽并记录 `AGENT_INTERNAL_FAILED`；启动恢复或 stalled-run watchdog 必须把遗留 `running` Run 收敛为 `interrupted`。不能用“前端已显示完整文本”代替持久化成功。

Live Event 分发不能让慢订阅者阻塞 Executor。每个订阅者使用有界队列；队列溢出时关闭该 SSE 连接并要求客户端通过会话详情重新对账，但 Run 继续执行且 durable 事件照常落库。没有订阅者时只写持久化事件，不做无限内存累积。

### 8.2 用户取消

1. 用户点击停止后，前端先调用 `run/cancel`，不要立刻中止 SSE 读取。
2. Coordinator 取消 `execCtx`，模型流停止。
3. Executor 使用独立的 `persistCtx = WithTimeout(WithoutCancel(execCtx), 5s)` 保存已累计文本并提交终止事务。
4. 助手 MessagePart、Message、Step 和 Run 标记为 `cancelled`，持久化成功后返回 cancel receipt。
5. 服务端向存活 SSE 写入 `abort`；前端收到 `abort` 或 cancel receipt 后保留已显示文本并恢复发送按钮。
6. 若 SSE 无法送达，前端以 cancel receipt 为准；若取消请求超时，则本地停止读取并通过会话详情对账，防止 UI 永久停留在 Streaming。

### 8.3 失败

- 模型错误、供应商协议错误和持久化错误都统一映射为稳定错误码。
- 流开始前失败返回 JSON 错误。
- 流开始后先发送 transient `data-agent-error`，再发送 `error` chunk，最后发送 `[DONE]`。
- `data-agent-error` 携带 `code`、`kind`、`message` 和 `retryable`；`errorText` 只放用户可读文本。
- 已累计文本在结束事务成功时保存；事务本身失败时不能宣称已经保存。
- 用户消息不删除。
- 锁释放和终态收敛使用 `defer`；任何 panic 都必须被 Executor 恢复并映射为 `AGENT_INTERNAL_FAILED`，不能留下永久 `running`。

### 8.4 中断

浏览器刷新、SSE 读取端关闭或短暂断线只结束当前订阅。只要 Go 进程仍持有 Run Coordinator，前端就通过 `runId + after` 恢复同一 Run，并合并缺失游标之后的增量。

Go 进程重启后不能续接原 Provider 流，因此不自动恢复原 Run。未来跨进程恢复能力必须复用同一 Run 状态机，不修改既有终态语义。

服务启动时：

1. 查找所有 `running` Run。
2. 在事务中将 Run、Step、助手消息和仍为 `streaming` 的 Part 标记为 `interrupted`。
3. 写入 `run.interrupted` 事件并设置结束时间。
4. 保留崩溃前已经提交的空 Part 或 checkpoint；不伪造未落库的尾部文本。

执行器同时维护最大运行时长和 stalled-run watchdog。超过阈值仍没有任何可观察进度时，必须取消 `execCtx`，使用独立持久化 context 标记 `interrupted` 并释放锁。

用户重试时创建新的 Run，并保留被中断的原 Run 和助手消息历史。目标契约中的 `retryOfRunId` 显式关联仍需后续迁移补充；当前通过同一助手消息的 `activeRunId` 确定展示尝试。

## 9. 并发

同一个 `AgentSession` 同时只允许一个运行中的 Run。

发送入口必须按以下顺序处理，不能先创建 Run 再尝试加锁：

1. 校验并规范化请求，计算 `requestHash`。
2. 以 `sessionId` 为键原子获取执行槽，记录持有者 `clientRequestId`。
3. 获取执行槽后查询 `(operation, clientRequestId)` 的幂等记录。
4. 相同 ID、相同摘要且 Run 仍在执行时返回 `409 AGENT_SESSION_BUSY` 和当前 `runId`。
5. 相同 ID、相同摘要且 Run 已终态时返回 `409 AGENT_REQUEST_ALREADY_PROCESSED`、原 `runId` 和最终状态。
6. 相同 ID、不同摘要时返回 `409 AGENT_REQUEST_ID_CONFLICT`，不得创建任何实体。
7. 首次请求在一个事务中创建幂等记录、消息、Part、Run、Step 和初始事件；事务失败时立即释放执行槽。
8. 事务成功后 Coordinator 先接管 `execCtx` 和 `runId -> cancelFunc`，Executor 才允许启动。HTTP Handler 只持有订阅句柄，不能持有执行槽所有权。

执行槽只在终态事务完成后释放，包括客户端已经断开的情况。取消接口只取消目标 `runId`，不影响已结束历史 Run；页面刷新、SSE 关闭和 `MessagePort` 关闭只释放订阅资源，不自动取消 Run。服务启动时清空进程内执行槽，并将遗留 `running` Run 收敛为 `interrupted`。

当前 Desktop 单进程实现使用进程内原子锁，API 不变量与未来多实例实现一致。Web 多实例上线前必须用数据库租约或 Redis 租约替代实现，并通过同一组并发测试；在租约方案完成前，Web 部署不得宣称支持横向扩展。租约的具体技术选型属于 Web 实施前的架构决策门。

会话创建没有现成的 `sessionId` 可加锁，使用 `(session.create, clientRequestId)` 唯一约束完成原子去重：并发同 ID、同摘要请求只有一个成功插入，其余请求读取已创建会话并返回同一结果；同 ID、不同摘要返回 `AGENT_REQUEST_ID_CONFLICT`。

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

`messageLimit` 默认 40，最大 100。`messages` 只返回当前页消息；`messageParts` 返回用户消息片段，以及助手消息当前 `activeRunId` 对应的生成片段。`activeRun` 为 `running` 时，前端刷新后显示“生成中”，并从详情返回的 `lastSequence` 继续请求 SSE。服务端回放缺失增量后继续实时推送，不创建第二个 Run。

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

创建成功后立即返回真实 `sessionId`。相同 `clientRequestId` 和相同 `requestHash` 重试时返回同一个会话；相同 ID、不同摘要返回 `409 AGENT_REQUEST_ID_CONFLICT`。前端不创建只在本地存在的临时会话。

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
- `409 AGENT_REQUEST_ID_CONFLICT`
- `503 AGENT_NOT_CONFIGURED`

成功响应为 SSE，不使用 JSON envelope。

服务端必须先将请求规范化，再计算 `requestHash`。至少统一空白、字段存在性和数组顺序；响应头、请求时间、Trace ID 等非业务字段不进入摘要。

相同 `clientRequestId` 和相同 `requestHash` 重试时，不能重复创建用户消息和 Run：

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
Run 不存在时返回 `404 AGENT_RUN_NOT_FOUND`。取消请求不携带 `clientRequestId`，因为它通过 `runId + 目标状态` 天然幂等。

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

重试的 `operation` 为 `message.retry`，成功后返回与发送消息相同的 SSE UI Message Stream。相同 `clientRequestId` 和相同摘要的重试遵循发送消息的三条幂等规则；已终态重试返回 `AGENT_REQUEST_ALREADY_PROCESSED`，非允许状态返回 `409 AGENT_MESSAGE_NOT_RETRYABLE`。

### 10.7 会话状态摘要

会话详情中的 `activeRun` 使用以下结构，不能只返回状态字符串：

```ts
type AgentActiveRun = {
  runId: string
  assistantMessageId?: string
  userMessageId?: string
  status: AgentRunStatus
  lastSequence: number
}
```

`status=running` 时，前端刷新后立即显示“生成中”，调用 `resumeRun` 从 `lastSequence` 重连 `/agent/run/stream`。可恢复网络错误使用有上限的指数退避；游标过期时重新读取 Session 详情并以持久化内容对账。恢复失败使用 `Result` 更新非阻塞错误状态，不能自动创建新 Run。

## 11. 流式协议

### 11.1 协议选择与三层 pipe-through

Go 服务端输出兼容 UI Message Stream 的 SSE。协议结构参考 Vercel AI SDK，分层方式同时参考 `mimo-chat`；前端不直接依赖 `ai` 或 `@ai-sdk/react`。

协议内部使用显式状态机和纯 Reducer，详见 [Agent 流协议状态机设计](./2026-11-05-craftingtable-agent-stream-state-machine-design.md)。该设计只改变协议层内部结构，不改变公开事件和 Store 所有权。

吸收的重点是以下三个边界：

- 请求传输与协议解析分离。
- 原始 SSE 由通用解析包清理，业务代码只处理需要的 chunk。
- 消息状态继续由 Zustand 和现有模型逻辑持有，不引入第二个消息状态源。

前端固定链路：

| 层 | 输入 | 负责方 | 输出 | CraftingTable 责任 |
| --- | --- | --- | --- | --- |
| 1. HTTP 传输 | Agent 请求 | `agentTransport.ts` | 标准 `Response` | 请求映射、非 2xx 错误 envelope、平台 fetch 注入 |
| 2. Wire 解码 | HTTP 字节流 | `agentStreamProtocol.ts` 与 `eventsource-parser` | `AsyncIterable<AgentStreamEvent>` | 清理 SSE、解析本项目需要的 chunk、稳定错误映射 |
| 3. 领域适配 | `AgentStreamEvent` | `apiAgentModel.ts` 与消息状态逻辑 | `AgentMessageSnapshot` | 文本增量、完成状态、错误片段和会话归属 |

```text
Response.body
  -> agentTransport.ts
  -> eventsource-parser
  -> agentStreamProtocol.ts
  -> AgentStreamEvent
  -> apiAgentModel.ts
  -> AgentMessage + AgentMessagePart
  -> AgentEffect
  -> Zustand
```

项目内模块的职责：

- `agentTransport.ts` 负责发送 `sessionId`、`clientRequestId`、消息关联 ID 和上下文。
- `agentStreamProtocol.ts` 负责识别 `text-delta`、`finish` 和 `error`。
- `apiAgentModel.ts` 负责把事件转换为现有消息快照，不把 `AgentStreamEvent` 写入 Store。
- Web 使用同源 fetch，Desktop 使用注入的受控 fetch 适配器。
- HTTP、协议和 Schema 错误统一转换为 `Result<T>`。

`eventsource-parser` 属于允许直接使用的通用工具包。以下内容属于项目协议和业务语义，必须在 `src` 中维护：

- UI Message Stream 中与 CraftingTable 相关的 chunk 子集。
- `start` 和 `finish` 的 `messageMetadata` 校验。
- 事件到领域消息和片段的映射。
- 取消、未知 chunk、终止事件和 EOF 的语义。

禁止在 `src` 中重复实现以下内容：

- SSE 行分块、`data:` 拼接和网络 chunk 缓冲。
- 引入完整第三方对话 Store 后继续保留 Zustand 作为另一份可修改业务状态。
- 复制外部项目的目录树、完整消息 reducer 或未使用的工具调用模块。
- 按数组位置猜测 `sessionId`、`runId` 或消息归属。

设计参考基线：

- [DefaultChatTransport](https://github.com/vercel/ai/blob/main/packages/ai/src/ui/default-chat-transport.ts)
- [HttpChatTransport](https://github.com/vercel/ai/blob/main/packages/ai/src/ui/http-chat-transport.ts)
- [readUIMessageStream](https://github.com/vercel/ai/blob/main/packages/ai/src/ui-message-stream/read-ui-message-stream.ts)
- [mimo-chat 协议分层](https://github.com/xiaomi/mimo-chat) 的传输、协议 Adapter 和状态写入分离方式。

这些仓库只作为设计和测试对照。升级协议前先更新 fixture，再运行前端 Adapter、Go 集成测试和浏览器 E2E。

Go 侧没有项目所需的官方流写入器。`server/internal/agent/access` 维护最小的 UI Message Stream 编码，负责 SSE 帧、`[DONE]`、响应头和 CraftingTable metadata；模型供应商协议仍由 `provider` 和 `core` 隔离。

早期的 `readServerEvents` 和 `parseEvent` 已由 `agentTransport.ts`、`agentStreamProtocol.ts` 和 `apiAgentModel.ts` 取代。后续协议扩展必须沿用这条项目自有链路，不回退为在 Model 内直接切分字节流。

`agentTransport.ts` 把发送参数映射为 CraftingTable 请求体：

- `sessionId`
- `clientRequestId`
- `userMessageId`
- `assistantMessageId`
- `assistantPartId`
- `runId`
- 当前上下文消息数组

服务端在 `start` 的 `messageMetadata` 中返回 `sessionId`、`runId` 和 `userMessageId`，在 `text-start` 中返回稳定的 `partId`。前端不能自行生成随后无法与数据库对应的助手消息或片段 ID。

```ts
type CraftingTableStreamMetadata = {
  sessionId?: string
  assistantMessageId?: string
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
data: {"type":"error","errorText":"模型调用失败，请重试","agentError":{"code":"AGENT_PROVIDER_FAILED","kind":"server","message":"模型调用失败，请重试","retryable":true}}

data: [DONE]
```

`agentError` 是 CraftingTable 在协议兼容子集上的扩展字段，只用于当前流传递结构化错误，不进入消息历史。`errorText` 只承担用户可读文本。结构化错误详情必须符合工程规范的错误矩阵，至少保留 `code`、`kind`、`retryable` 和已 Schema 化的 `details`。

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
- 心跳使用 SSE 注释，不进入 `AgentStreamEvent` 和 Zustand。

## 12. 前端 MVC

### 12.1 AgentModel

负责：

- 调用会话列表、详情、创建、取消和重试 API。
- 使用 `agentTransport.ts` 发起消息发送。
- 接收平台注入的 fetch 适配器；Web 使用同源请求，Desktop 使用 preload bridge。
- 把 `Response.body` 交给 `agentStreamProtocol.ts`。
- 只消费 `AgentStreamEvent`，不读取网络字节块。
- 将网络、HTTP、Schema 和协议错误统一转换为 `Result<T>`。
- 将 `AgentStreamEvent` 转换为 CraftingTable 领域消息。

不得：

- 读写 Zustand。
- 显示 Toast。
- 访问 React。
- 决定发送状态。
- 手写 SSE 解码、外部消息对象或第二份消息状态。

发送接口使用清晰的边界签名：

```ts
type AgentSendInput = {
  sessionId: string
  clientRequestId: string
  message: CraftingTableSendMessage
  signal: AbortSignal
}

type AgentSendResult = {
  assistantMessageId: string
  runId: string
  status: AgentRunStatus
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

Model 在内部消费协议事件并调用 `onSnapshot`，但不直接修改 Store。`onStarted` 只在合法 Run 建立后调用一次；`onSnapshot` 可调用零次或多次；Promise 在 `finish`、`error` 或 `abort` 终止事件完成、且服务端已返回持久化成功的终止 chunk 后解析。流开始前的 HTTP 错误、Schema 错误和流中的协议错误都转换为 `Result`；异常不穿透到 View。

用户取消对应 `status=cancelled` 的正常 `Result`，不显示错误 Toast。流在缺少终止事件时结束返回 `PROTOCOL_UNEXPECTED_EOF`。调用方不能通过检查 `onSnapshot` 的最后一次渲染推断持久化成功，必须以 Promise 的终态结果为准。

Transport 和协议 Adapter 的生命周期由 Model 持有，不能由组件临时创建。传输器关闭时释放平台 fetch 适配器和正在使用的 `MessagePort`。

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
- 将流式 `AgentMessageSnapshot` 映射到 Store。
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
  domain/
    model.go
    status.go
    errors.go
  ports/
    runtime.go
    repository.go
    clock.go
    event_publisher.go
  core/
    executor.go
    run_policy.go
    context_assembler.go
  application/
    send_message.go
    cancel_run.go
    retry_run.go
    coordinator.go
  access/
    http_controller.go
    stream_view.go
  adapters/
    provider/
      openai_compatible.go
    persistence/
      repository_sqlite.go

server/internal/platform/
  httpapi/
  store/
```

职责：

- `domain/`：Session、Message、Part、Run、Step、状态和领域错误，不导入 HTTP、数据库、Provider 或 React。
- `ports/`：Core 和 Application 需要的 Runtime、Repository、Clock、Event Publisher 抽象。
- `core/`：AgentExecutor、RunPolicy、状态机、上下文组装和 Run 终态收敛。
- `application/`：发送、取消、重试等用例，以及会话执行槽、幂等和 run-scoped context 管理。
- `access/`：HTTP 解码、校验、取消入口和领域事件到 UI Message Stream SSE 的编码。
- `adapters/provider/`：实现 RuntimePort 的具体模型供应商协议适配。
- `adapters/persistence/`：实现 RepositoryPort 的持久化适配；Desktop 使用 SQLite，Web 存储按决策门另行实现。
- `platform/`：通用 HTTP 启动、连接池、日志、时钟和基础设施装配，不实现 Agent 业务规则。

`domain`、`ports` 和 `core` 不得导入 `access` 或 `adapters`。Application 不导入具体 Provider 和数据库实现。
具体实现只在 `server/cmd/craftingtabled` 的 composition root 中创建并注入。

### 13.2 Runtime 接口

Runtime 接收已经验证的输入，并持续发送事件：

```go
type Runtime interface {
    Stream(ctx context.Context, input RunInput, emit EventSink) error
}
```

`RunInput` 至少包含：

- Run ID
- `RunPolicy`
- System Prompt
- 历史消息
- 当前用户消息
- 模型配置

Runtime 不接收数据库连接、HTTP ResponseWriter 或 Zustand 类型。

`Stream` 使用的 `ctx` 来自 Executor 的 run-scoped context，而不是 HTTP Handler 的 request context。浏览器断开只取消订阅，不能取消该 context。Executor 负责 `RunPolicy` 的总时长和空闲超时；Runtime 只负责单次供应商调用及自身更短的调用超时。SSE 每 15 秒发送一次注释心跳；心跳不写数据库，也不进入 `AgentStreamEvent`。

### 13.3 模型供应商

当前实施阶段使用 OpenAI-compatible 的 Chat Completions 流式接口。供应商原始事件只存在于 provider 适配器内部；切换到 Responses API 或其他供应商时必须新增 provider 适配器，不能改变前端领域事件。

配置基线：

- `CRAFTINGTABLE_AGENT_BASE_URL`
- `CRAFTINGTABLE_AGENT_API_KEY`
- `CRAFTINGTABLE_AGENT_MODEL`
- `CRAFTINGTABLE_AGENT_SYSTEM_PROMPT`，可选

API Key 只由 Go sidecar 从环境或系统安全存储读取。开发态允许环境变量；桌面生产态不能把 Key 写入 Renderer、preload 或前端构建产物。

转换链路：

```text
Provider SSE
  -> Provider Adapter
  -> CraftingTable RunEvent / RunDelta
  -> UI Message Stream Encoder
  -> Browser
```

前端永远不依赖供应商原始 chunk 名称。

## 14. 上下文策略

当前实施阶段不实现自动摘要，但摘要只是上下文优化，不能替代完整历史持久化和按 Run 复现所需的数据。

- 发送给模型的历史由完整轮次组成：一条用户消息加上该消息的一条 `complete` 助手回复。失败、取消或中断的轮次不进入后续上下文，除非用户针对同一用户消息创建重试。
- `failed`、`cancelled` 和 `interrupted` 的旧尝试不作为模型上下文，其保留在数据库中的 Part 只用于审计和 UI。
- 上下文组装只读取 `text` 片段；`error` 片段即使随历史消息回放，也不得进入模型消息。
- 同一条助手消息发生重试时，只读取 `activeRunId` 对应 Run 的文本片段，旧尝试的片段不得拼入上下文。
- 最多保留最近 20 个完整轮次，共不超过 40 条消息。
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
- Error：在最后一条 Agent 消息中显示可理解错误；可重试错误在同一消息内显示重试入口，页面顶部只为无法挂到消息上的错误保留提示。
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
- Electron 主进程生成短期 Token，通过环境变量或受保护握手传递，不放入命令行参数；Token 不进入前端 Store、URL、日志或异常信息。
- Agent 流经主进程窄范围 transport bridge 转发，Renderer 不能指定任意上游地址。
- 外链、文件选择和系统能力通过 preload 的最小 API 暴露。

## 17. 测试设计

产品行为、API、契约和桌面验收测试位于 `tests/e2e`。Go 中确有必要的未导出纯逻辑测试可以放在对应 package 的 `_test.go`，但不能替代这里的黑盒场景。

### 17.1 Unit

```text
tests/e2e/unit/agent/agentStore.test.ts
tests/e2e/unit/agent/agentModel.test.ts
tests/e2e/unit/agent/agentStreamAdapter.test.ts
tests/e2e/unit/agent/agentStreamProtocol.test.ts
```

覆盖：

- Store 同步 action。
- `messageId + partId` 领域合并。
- `Result<T>` 错误映射。
- 取消不显示错误 Toast。
- `AgentStreamEvent` 到领域消息的转换。
- 项目协议 Adapter 消费 Go fixture 后的快照顺序、终止和畸形事件；需要对照兼容性时可用 AI SDK 作为测试 oracle，但不进入生产依赖。

### 17.2 Integration

```text
tests/e2e/integration/agent/persistence.integration_test.go
tests/e2e/integration/agent/controller.integration_test.go
tests/e2e/integration/agent/runtime_contract.integration_test.go
```

覆盖：

- 通过真实 HTTP 应用和测试数据库观察会话、消息、片段、Run、Step 和 Event 持久化；外部测试不导入 `server/internal`。
- `messageId` 归属和片段排序。
- `clientRequestId` 在运行中、已终态和摘要冲突三种情况下的响应。
- 两个并发发送请求只有一个能获取会话执行槽；事务失败不能留下孤儿 Run 或永久锁。
- 同一 Run 的 `RunEvent.sequence` 在并发 writer 下唯一且单调。
- `RunPolicy` 的步数、总时长、空闲超时和连续失败上限进入正确终态。
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
- transport bridge 正确覆盖 `opened`、`chunk`、`ack`、`cancel`、`ended`、`failed`，并能处理窗口关闭和背压。
- 打包后的窗口能够加载并发送一条 Fake Runtime 消息。
- sidecar 崩溃后自动重启，旧 Run 被标记为 interrupted，界面提供重试。

## 18. 实施顺序

1. 建立 Agent 领域类型、数据库迁移和 Repository。
2. 实现 Fake Runtime、RunEvent、MessagePart、Run Coordinator，并按项目协议实现最小 UI Message Stream Encoder。
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
- `MessagePart` 保存所有可展示正文，`RunEvent` 不保存正文，`RunDelta` 不进入数据库。
- 用户取消和普通失败可以保留已提交的部分文本；硬中断只保证已提交片段，并明确提供重试入口。
- 同一 `clientRequestId` 不会重复创建消息和 Run。
- `AgentExecutor` 遵守不可变 `RunPolicy`，Runtime 只执行单次有界调用。
- 前端使用 `agentTransport.ts`、`eventsource-parser` 和 `agentStreamProtocol.ts` 组成三层 pipe-through，不引入第三方对话 Store。
- `src` 不包含自建通用 SSE Parser、第三方 `UIMessageChunk` Reducer 或第二份消息 Store。
- Go 输出通过项目 fixture 验证；需要兼容性对照时可运行 AI SDK oracle，但生产链路使用项目自有最小 Encoder。
- Zustand 是前端业务状态唯一事实源。
- Go Runtime 不访问数据库，Repository 不依赖 HTTP。
- `src` 不包含供应商原始事件解析，`server` 不输出未经转换的供应商协议。
- 模型 Key 不进入 Renderer、Store 或日志。
- 产品行为、API、契约和桌面验收测试位于 `tests/e2e`；Go package 白盒测试不替代这些场景。
- 390px、768px、1024px 和 1440px 的 Light / Dark 均通过验证。
- 真实 Electron 进程完成启动、离线恢复和一条 Fake Runtime 会话。
