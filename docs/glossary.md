# CraftingTable 术语表

日期：2026-11-05
状态：已确认
适用范围：产品文档、设计规范、数据模型、接口和前端代码

本文统一 CraftingTable 的业务词、流式协议词和 MVC 架构词。后续文档与本表冲突时，先修订本表，再同步其他文档和代码。

## 1. 使用规则

- 产品界面可以用中文，代码类型和字段统一使用英文名。
- 项目名和产品名统一使用 `CraftingTable`；`leeat` 只表示当前用户身份，不用作类型、环境变量或领域对象前缀。
- `Agent 会话`统一使用 `AgentSession`，不使用单独的 `Session`。
- 用户登录会话统一使用 `AuthSession`，不使用单独的 `Session`。
- `Thread` 不作为本项目领域词；对话统一使用 `AgentSession`。
- MVC 的 `Model` 与语言模型分别写作 `MVC Model` 和 `LLM Model`。
- 交流中不使用含义不清的 `Service`；应明确为 MVC Model、Effect、API 或具体外部服务。
- 协议事件名必须带协议归属。CraftingTable 自定义事件不能被称为 OpenAI 事件或 AI SDK 事件。

## 2. 业务对象

| 中文 | 代码名 | 定义 |
| --- | --- | --- |
| Agent | `Agent` | CraftingTable 中的单个个人工作助手。当前阶段只有一个 Agent，不包含多 Agent。 |
| Agent 会话 | `AgentSession` | 一次连续对话，包含标题、创建时间、消息列表和会话状态。 |
| 身份会话 | `AuthSession` | 用户登录状态。当前实施阶段未实现账号，但保留这个词用于区分 Agent 会话。 |
| 当前会话 | `activeSession` | 当前在 Agent 工作区展示的 `AgentSession`。 |
| 会话列表 | `sessionList` | 二级侧边栏展示的 `AgentSession` 集合。 |
| 消息 | `AgentMessage` | Agent 会话中的一条完整消息，例如一条用户消息或一条助手消息。 |
| 用户消息 | `userMessage` | `role` 为 `user` 的 `AgentMessage`。 |
| 助手消息 | `assistantMessage` | `role` 为 `assistant` 的 `AgentMessage`。 |
| 消息角色 | `AgentMessageRole` | 消息发送方，取值为 `user` 或 `assistant`。 |
| 消息状态 | `AgentMessageStatus` | 消息状态，取值为 `streaming`、`complete`、`cancelled`、`failed` 或 `interrupted`。 |
| 内容片段 | `AgentMessagePart` | 消息中可展示内容的持久化来源。用户消息片段没有 `runId`，助手生成片段必须带 `runId`；当前包含文本和错误片段。 |
| 片段状态 | `AgentMessagePartStatus` | 片段状态，取值为 `streaming`、`complete`、`cancelled`、`failed` 或 `interrupted`。 |
| 文本片段 | `AgentTextPart` | `type` 为 `text` 的 `AgentMessagePart`。 |
| 错误片段 | `AgentErrorPart` | `type` 为 `error` 的 `AgentMessagePart`，用于在失败的助手消息中展示结构化错误和重试入口，不参与模型上下文。 |
| Agent 执行 | `AgentRun` | 一条助手消息的一次生成尝试，包含状态、模型和重试关系。 |
| 触发用户消息 | `userMessageId` | 触发一次 `AgentRun` 的用户消息标识；重试不能靠数组位置推断来源。 |
| Agent 步骤 | `AgentStep` | Run 内一个有边界的执行步骤。当前只有模型步骤，后续可扩展工具和 MCP。 |
| Run 策略 | `RunPolicy` | 约束 Run 的步数、时长、空闲超时、连续失败次数和未来工具调用上限；策略版本随 Run 保存。 |
| 生命周期事件 | `RunEvent` | 有业务语义并需要持久化的 Run 事件。 |
| 流增量 | `RunDelta` | 只用于当前流链路展示的文本或 UI 增量，不逐条持久化。 |
| 当前 Run | `activeRunId` | 助手消息当前展示的 Run。重试时保留旧 Run，但只显示新 Run 的片段。 |
| 运行中会话 | `activeRun` | 会话详情中正在执行或刚结束、需要恢复状态的 Run 摘要。 |
| 前端活动流 | `liveRun` | 当前前端正在接收的 Run；当前阶段同时最多一个，以 `sessionId + runId` 关联。 |
| 客户端请求标识 | `clientRequestId` | 写请求用于幂等和重试去重的客户端生成标识。 |
| 持久化数据 | `persistedData` | 保存在服务端数据库中的会话和消息。 |
| 运行时状态 | `runtimeState` | 当前页面内存中的状态，例如正在生成、已取消或发生错误。 |
| 状态回填 | `hydrate` | 从服务端加载持久化数据并写入前端 Store。 |
| 业务状态源 | `sourceOfTruth` | 页面渲染真正信任的唯一状态来源。 |
| 双源 | `dualSource` | 两个位置同时保存并且都能修改同一份状态，可能导致状态不一致。 |

## 3. Agent 会话状态

| 中文 | 代码名 | 定义 |
| --- | --- | --- |
| 空闲 | `idle` | 当前会话没有提交请求，也没有进行中的流。 |
| 提交中 | `submitting` | 请求已经发出，但尚未收到第一个有效流事件。 |
| 流式生成中 | `streaming` | 正在接收并应用助手消息的流事件。 |
| 已完成 | `complete` | 本次生成正常结束，消息已经完整。 |
| 已取消 | `cancelled` | 用户主动停止生成，已接收内容可以保留。 |
| 失败 | `error` | 本次生成失败，已接收的部分内容仍可以保留。 |

`sendPhase` 只描述发送流程，建议取值为 `idle`、`submitting`、`streaming` 和 `error`。单条消息的最终状态使用 `AgentMessageStatus`，取值为 `streaming`、`complete`、`cancelled`、`failed` 和 `interrupted`。`complete` 是消息完成状态，Run 和 Step 的对应成功状态是 `completed`，两者不能混用。

## 4. 流式数据

| 中文 | 代码名 | 定义 |
| --- | --- | --- |
| 流 | `stream` | 数据分多次持续到达，而不是一次性返回完整结果。 |
| 流式响应 | `streamingResponse` | 服务端在模型生成过程中持续发送增量数据。 |
| 语言模型 Token | `llmToken` | 语言模型输出的基本文本单位。它不是网络 chunk，也不是事件。 |
| 网络 Chunk | `networkChunk` | `reader.read()`每次从 HTTP 连接读取到的字节块。一个网络 chunk 可能包含半个 SSE 事件或多个 SSE 事件。 |
| 消息 Chunk | `messageChunk` | 协议层面的数据单元，例如`text-delta`。它不等于网络字节块。 |
| 增量 | `delta` | 相对上一次状态新增的部分，通常是一小段文本。 |
| 消息快照 | `messageSnapshot` | 某一时刻一条消息的完整状态，不是新增片段。 |
| 三层 pipe-through | `threeLayerPipeThrough` | 前端固定链路：`agentTransport.ts` 获取 HTTP 字节流，`eventsource-parser` 清理 SSE，`agentStreamProtocol.ts` 映射项目协议，`apiAgentModel.ts` 生成领域消息快照。 |
| 协议吸收 | `protocolAbsorption` | 理解外部协议及其分层取舍后，用本项目类型、错误模型、Adapter 和测试重新实现最小必要子集，不直接引入其业务层。 |
| 直接依赖 | `directDependency` | 职责单一、边界清晰且不拥有业务语义的通用包，例如 `eventsource-parser`、`lodash`。 |
| 流事件 | `streamEvent` | 带明确语义的流数据，例如消息开始、文本追加、生成完成或失败。 |
| SSE | `SSE` | Server-Sent Events，基于 HTTP 的服务端事件流。 |
| SSE 事件 | `sseEvent` | SSE 格式中的事件，通常由`event`、`data`和空行组成。 |
| 心跳 | `heartbeat` | 保持连接活跃的信号，不进入业务消息和 Zustand。 |
| 结束哨兵 | `doneSentinel` | `[DONE]`一类传输结束标记，不是助手消息。 |
| 结束原因 | `finishReason` | 本次生成的结束原因，例如正常完成、长度限制或用户取消。 |
| 断线重连 | `reconnect` | 连接中断后重新建立连接。 |
| 恢复流 | `resume` | 从已有消息或指定位置继续接收流。 |

## 5. 协议与处理

| 中文 | 代码名 | 定义 |
| --- | --- | --- |
| 协议 | `protocol` | 前后端约定的数据格式、字段含义、事件顺序和结束规则。 |
| 网络格式 | `wireFormat` | 数据真正通过网络传输时的格式，例如 SSE 文本。 |
| Schema | `schema` | 协议和数据的结构定义；前端可使用 TypeScript 类型和 Zod，后端使用对应语言类型与校验。 |
| 生命周期事件 `RunEvent` | `runEvent` | CraftingTable 服务端定义、需要持久化的 Agent 执行事件。 |
| 流增量定义 | `RunDelta` | CraftingTable 服务端产生、只用于当前流链路的临时增量。 |
| 流式传输 | `agentTransport.ts` | CraftingTable 自有 HTTP Transport，负责请求映射、非 2xx 错误 envelope 和平台 fetch 注入。 |
| SSE 清理 | `eventsource-parser` | 直接依赖的通用包，只负责把 SSE 字节流清理为事件，不解释 Agent 业务语义。 |
| 协议子集 Adapter | `agentStreamProtocol.ts` | 把项目需要的 UI Message Stream 子集映射为 `AgentStreamEvent`，负责终止、未知事件和协议错误语义。 |
| 领域事件 | `AgentStreamEvent` | CraftingTable 前端使用的临时领域事件，当前包括文本增量、完成和错误；它不是 Zustand 业务状态。 |
| 领域消息适配 | `apiAgentModel.ts` | 消费 `AgentStreamEvent`，生成现有 Agent 模型需要的 `AgentMessageSnapshot`，不把第三方消息对象写入 Store。 |
| 协议适配器 | `AgentStreamAdapter` | 在需要拆分时，把协议事件转换为 CraftingTable 领域消息和片段。 |
| 流元数据 | `CraftingTableStreamMetadata` | `start`、`finish` 等 chunk 中的 CraftingTable 关联数据，包含 `sessionId`、`runId`、`userMessageId` 和终态。 |
| 瞬态数据块 | `transient data chunk` | `transient: true` 的 `data-*` 块，只用于当前流传递结构化状态，不加入消息历史。 |
| 桌面传输桥 | `AgentTransportBridge` | preload 暴露的窄范围 Agent 请求桥；主进程持有 sidecar 地址和 Token。 |
| 适配器 | `adapter` | 把第三方协议转换为 CraftingTable 领域结构的组件。 |
| 事件顺序 | `eventOrder` | 协议规定的事件先后关系，例如开始、增量、结束。 |
| 终止事件 | `terminalEvent` | 表示本次生成结束的事件，正常结束和失败都只能出现一个终止事件。 |

职责边界：

- `agentTransport.ts` 只负责请求、响应状态和错误 envelope。
- `eventsource-parser` 只负责 SSE 分帧；项目不重复实现通用 SSE 清理器。
- `agentStreamProtocol.ts` 只识别项目需要的协议子集并产出 `AgentStreamEvent`。
- `apiAgentModel.ts` 负责领域消息和片段映射，不保存第二份长期状态。
- Store 负责保存业务状态。
- View 负责展示状态。

## 6. 当前流协议

当前 `/api/v1/agent/stream` 使用项目定义的 UI Message Stream 兼容子集，由 Go 服务端编码为 SSE。

协议结构参考 Vercel AI SDK UI Message Stream，但编码、错误扩展、事件子集和测试由 CraftingTable 维护；不直接依赖 `ai`、`@ai-sdk/react` 或完整 Go SDK。

正常顺序为：

```text
start
start-step
text-start
text-delta
text-end
finish-step
finish
[DONE]
```

失败时先发送 transient `data-agent-error` 结构化错误，再使用`error`代替`finish`，然后发送`[DONE]`。用户取消使用`abort`。一次生成只能出现一个终止事件。

`start` 的 `messageMetadata` 必须携带 `sessionId`、`runId` 和 `userMessageId`；`text-start` 返回稳定的 `partId`。这些 ID 是领域对象与流事件的关联键，不由前端猜测。

`message-start` 和 `message-finish` 是早期学习方案中的名称，不再作为正式协议事件。正式领域生命周期使用 `RunEvent`；前端将协议 chunk 映射为 `AgentStreamEvent`，不把第三方协议对象写入 Store。

## 7. 协议归属

| 名称 | 归属 | 用途 |
| --- | --- | --- |
| `choices[].delta.content` | OpenAI Chat Completions | 供应商原始模型流。 |
| `response.output_text.delta` | OpenAI Responses API | Responses API的文本增量事件。 |
| UI Message Stream chunk | 参考 Vercel AI SDK | 前端 UI 消息流协议单元；CraftingTable 只实现当前需要的子集。 |
| `RunEvent` | CraftingTable | 服务端持久化的 Agent 生命周期事件。 |
| `RunDelta` | CraftingTable | 仅存在于当前流链路的临时增量。 |

数据链路应保持为：

```text
LLM Token
  -> LLM Model 原始流
  -> Go Provider Adapter
  -> CraftingTable RunEvent / RunDelta
  -> Go UI Message Stream Encoder
  -> HTTP + SSE
  -> eventsource-parser
  -> agentStreamProtocol.ts
  -> apiAgentModel.ts
  -> Zustand
  -> View
```

## 8. 外部项目与依赖名词

| 中文 | 名称 | 定义 |
| --- | --- | --- |
| AI SDK | `AI SDK` | Vercel 提供的 TypeScript AI 工具包；CraftingTable 只参考其传输、协议和测试设计。 |
| UI 消息流协议 | `UI Message Stream` | Vercel AI SDK 定义的前端消息流协议，不是 OpenAI 官方协议；CraftingTable 输出兼容子集。 |
| 默认聊天传输器 | `DefaultChatTransport` | AI SDK 的 HTTP Transport 实现，只作为 `agentTransport.ts` 的设计参考，不进入生产依赖。 |
| UI 消息流读取器 | `readUIMessageStream` | AI SDK 把消息块归并为 UI 消息快照的函数，只作为协议研究参考，不进入生产依赖。 |
| 产品链路参考 | `mimo-chat` | 参考其传输、协议 Adapter 和状态写入分离方式；不复制其目录、业务层或 Agent 编排。 |
| Go 流实现参考 | `aisdk-go` | Go 社区对 AI SDK 流的实现，只作为协议对照；不进入生产依赖。 |
| SSE 清理依赖 | `eventsource-parser` | 允许直接依赖的通用 SSE 解析包，由项目 Adapter 调用。 |
| 通用工具依赖 | `lodash` | 允许直接依赖的通用数据工具库，不承载业务语义。 |

## 9. MVC 与前端架构

| 中文 | 代码名 | 定义 |
| --- | --- | --- |
| MVC Model | `Model` | 负责领域类型、接口契约、数据读取和写入封装，不读写 Store。 |
| LLM Model | `LLMModel` | 提供推理能力的具体语言模型，例如 OpenAI-compatible 模型。 |
| Controller | `Controller` | 把用户动作转换为 Effect 调用和 Store 更新。 |
| Effect | `Effect` | 处理异步和副作用，例如请求、流消费、取消、错误处理和 Toast。 |
| Store | `Store` | 保存业务状态并提供同步 action。 |
| State | `State` | Store 中的数据。 |
| View | `View` | React 页面和组件，只负责渲染和触发用户动作。 |
| Action | `action` | Store 提供的同步状态修改函数。 |
| Selector | `selector` | 从 Store 中选择派生状态的函数。 |
| 取消控制器 | `AbortController` | 浏览器取消机制，用于停止请求和流读取。 |
| Agent 运行时 | `AgentRuntime` | 单次供应商调用的执行适配层，接收已验证的 RunInput 并输出领域事件；它不是完整 Agent Loop，也不访问数据库。 |
| Agent 执行器 | `AgentExecutor` | 使用 run-scoped context 执行 Run、累计增量、提交结果并发布 Live Event 的组件。 |
| Run 策略 | `RunPolicy` | 定义 `maxSteps`、`maxDuration`、`idleTimeout`、`maxConsecutiveFailures` 和未来 `maxToolCalls` 的可版本化限制。 |
| Run 协调器 | `RunCoordinator` | 管理活动 Run、会话锁、取消函数和 run-scoped context 的组件。 |
| 体验层 | `Experience Layer` | React Renderer 中的 View、Effect、Model 和 Store，只通过接入层访问服务端能力。 |
| 接入层 | `Access Layer` | Go 的 HTTP Controller、SSE Stream View 和请求协议适配，不实现 Agent Loop 或持久化事务。 |
| Agent 应用层 | `Agent Application Layer` | 会话发送、取消、重试、执行槽、幂等和 Run 协调等用例边界。 |
| Agent 核心层 | `Agent Core Layer` | AgentExecutor、Run/Step 状态机、RunPolicy、上下文组装和执行终态收敛。 |
| 适配器与基础设施层 | `Adapters / Infrastructure Layer` | 实现 Runtime、Repository、Clock、日志和 Telemetry 端口的具体技术适配。 |
| 端口 | `Port` | 由 Application 或 Core 需要的一方定义的抽象，具体 Adapter 向内实现，不反向依赖业务核心。 |
| 组合根 | `composition root` | 仅在进程启动时创建具体 Adapter 并注入 Port 的位置，默认位于 `server/cmd/craftingtabled`。 |

命名建议：

- `AgentModel`：Agent API 和领域状态边界。
- `AgentStore`：Agent 会话、消息和状态。
- `AgentEffect`：Agent 异步流程。
- `AgentTransport`：HTTP 请求和平台 fetch 适配。
- `AgentStreamProtocol`：UI Message Stream 项目子集到 `AgentStreamEvent` 的转换。
- `AgentStreamAdapter`：协议事件到 CraftingTable 领域消息的转换。
- `AgentRuntime`：Go 服务端执行边界。

## 10. 歧义检查

以下表达不要单独使用：

| 避免写法 | 推荐写法 |
| --- | --- |
| Session | `AgentSession`或`AuthSession` |
| Thread | `AgentSession` |
| Model | `MVC Model`或`LLM Model` |
| Chunk | `networkChunk`、`sseEvent`或`AgentStreamEvent` |
| Event | `RunEvent`、`sseEvent`或`AgentStreamEvent` |
| Stream | `SSE`、`ReadableStream`或具体协议流 |
| AI 事件 | 明确写 OpenAI 事件、UI Message Stream chunk 或 CraftingTable 事件 |
| Layer | 明确写 `Experience Layer`、`Access Layer`、`Agent Application Layer`、`Agent Core Layer`或`Adapters / Infrastructure Layer` |
| Adapter | 明确写 `Provider Adapter`、`Repository Adapter`或具体外部集成 Adapter |
