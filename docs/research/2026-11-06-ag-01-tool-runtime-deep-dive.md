# AG-01 工具运行时深度调研

日期：2026-11-06
分支：`leeatMvp`
需求：[AG-01 Tool Registry、工具 Schema、ToolCall / ToolResult 契约与工具执行器](../requirements.md)
状态：调研与推荐方案，不是已批准的实施设计
关联调研：[通用 Agent 调研与当前实现对比](./2026-11-06-general-agent-comparison.md)

本文出现的 `ToolRegistry`、`ToolCall`、`ToolResult`、`ToolInvocation` 等名称是调研期工作术语。设计批准后再写入术语表，不能仅凭本文就认为这些名称已成为已确认契约。

## 1. 结论

AG-01 不是一个“调用函数的接口”，而是 Agent 的行动面基础：

```text
工具定义
  -> 模型可见的 Tool Schema
  -> 模型发起 ToolCall
  -> 服务端校验参数
  -> Tool Executor 执行
  -> 生成 ToolResult
  -> 持久化并供下一次模型 Step 使用
```

推荐结论：

1. 使用 JSON Schema 作为工具输入契约的唯一事实源。
2. Tool Registry 在 composition root 静态装配，工具名称、描述、Schema 和版本在运行中不可变。
3. ToolCall 和 ToolResult 使用 CraftingTable 自有领域类型；供应商原始字段只存在于 Provider Adapter。
4. 每次 ToolCall 对应一个新的 Tool Step。执行状态以 Step 为准，工具参数和结果保存在独立 Tool Invocation 数据中，不塞进普通文本 Part。
5. AG-01 先支持只读、进程内、领域专用工具；不接 Shell、写文件、外部写接口，也不做审批和沙箱，后者属于 AG-03 / AG-04。
6. AG-01 不单独形成用户可见闭环。它应与 AG-02 配合验证；在 AG-02 完成前，生产环境默认不启用真实工具调用。
7. 第一阶段只支持串行工具调用，不自动重试副作用工具，不支持动态工具注册和任意 MCP 工具注入。

一句话定位：

> AG-01 是“定义、校验、执行和记录一次工具调用”的基础设施，不负责 Agent 的多 Step 决策循环。

## 2. 需求边界

### 2.1 AG-01 负责

- Tool Definition 的稳定结构和元数据。
- Tool Registry 的注册、查找、版本和确定性排序。
- 输入输出 JSON Schema 的验证。
- ToolCall、ToolResult、ToolError 的领域契约。
- Tool Executor 的超时、取消、错误映射和结果限制。
- OpenAI-compatible Provider Adapter 对工具定义和工具调用的协议映射。
- Tool Step 与 Tool Invocation 的持久化基础。
- Tool 生命周期的服务端事件和测试。

### 2.2 AG-01 不负责

- AG-02：模型与工具之间的多 Step 循环、停止条件和失败预算。
- AG-03：`allow / ask / deny` 权限裁决、人工审批和暂停后恢复。
- AG-04：文件系统、Shell、Docker、远程 Workspace 和系统沙箱。
- AG-06：Skills、MCP Client、Subagent 和 Multi-Agent。
- AG-07：完整 Artifact 存储、Trace 平台和 Eval。
- AG-08：多供应商归一化。
- UI 中的完整工具卡片、参数展开和审批交互；AG-01 只定义稳定数据与最小事件。

### 2.3 与 AG-02 的依赖关系

没有 AG-02 时，系统可以做到：

```text
模型请求 ToolCall
  -> Executor 执行工具
  -> 保存 ToolResult
```

但不能做到：

```text
ToolResult
  -> 重新调用模型
  -> 模型基于结果继续回答
```

因此 AG-01 的验收重点应是内部契约、持久化和执行正确性，而不是“用户已经可以完成带工具的任务”。真实工具调用应通过 Feature Flag 默认关闭，由 Fake Provider 和只读工具完成回归。

这里的 Fake Provider 只属于自动化测试和 loopback 集成测试，不能变成生产环境的假回复或模型降级路径。功能启用后，真实用户请求仍直接调用真实模型 API；单测和可重复集成测试使用可控替身。

### 2.4 相邻需求的职责矩阵

| 问题 | AG-01 | AG-02 | AG-03 | AG-04 |
| --- | --- | --- | --- | --- |
| 工具如何声明、查找和校验 | 负责 | 消费 | 消费 | 不负责 |
| 一次 ToolCall 如何执行和终结 | 负责 | 触发 | 执行前裁决 | 提供隔离环境 |
| ToolResult 何时回送模型 | 提供领域对象 | 负责 | 不负责 | 不负责 |
| 循环何时停止 | 不负责 | 负责 | 不负责 | 不负责 |
| ask / deny 与暂停恢复 | 只保留 Authorizer Port | 处理暂停后的循环 | 负责 | 不负责 |
| Shell、文件和网络隔离 | 明确不提供 | 不负责 | 不负责 | 负责 |

这个矩阵用于防止实现时把“循环控制”误塞进 Tool Executor，或把“权限判断”写进 Handler。

## 3. 当前实现事实

### 3.1 Provider 只支持文本请求

当前 `StreamProvider` 只接收 `[]ProviderMessage`，消息只有 `role` 和 `content`：

```go
type ProviderMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type StreamProvider interface {
    Stream(context.Context, []ProviderMessage) (io.ReadCloser, error)
}
```

它不能传入工具列表，也不能表达 assistant tool call 和 tool result 消息。

### 3.2 流解析器忽略 tool_calls

当前 `relayProviderStream` 只解析：

- `delta.reasoning_content`
- `delta.content`
- `finish_reason`

它没有累计 `delta.tool_calls`，而且把任意非空 `finish_reason` 都当作正常完成。引入工具后，`finish_reason=tool_calls` 只能结束当前 Model Step，不能结束整个 Run。

### 3.3 Step 只有通用壳

`agent_steps` 目前只有：

- `id`
- `run_id`
- `step_index`
- `status`
- `created_at`
- `finished_at`

业务代码固定创建 `step_index=0`，Finalize 也固定找第 0 个 Step。即使保存了更多 Step，当前执行器也不会读取和恢复。

### 3.4 MessagePart 没有工具类型

当前 Part 只允许 `text`、`reasoning`、`error`，并且只保存 `text_content` 和 `error_json`。工具参数和结构化结果没有稳定存储位置。

### 3.5 前端状态机是单模型 Step 模型

当前前端 reducer 认识 `start-step` / `finish-step`，但 Materializer 只维护一个 reasoning Part 和一个 text Part。它还不能表达：

- 多个工具 Step。
- reasoning、tool call、tool result、下一段 text 的时间顺序。
- 一个 Run 内多个 Model Step。

### 3.6 与现有设计基线的关系

现有 Agent 设计已经预留了多 Step、`kind: "tool"` 和工具 MessagePart，也没有要求把完整工具参数塞进文本 Part。因此 AG-01 与长期方向不冲突。

需要区分两种“非目标”：

- Agent 设计 §3.2 的“不实现工具”是当前实施阶段边界，不是对目标架构的否定。
- AG-01 调研稿的“不实现完整工具时间线”是本需求的交付边界；多 Step UI 应和 AG-02 一起设计。

真正的设计修订点只有一个：现有文档没有明确 ToolCall、参数、ToolResult 和 ToolError 的持久化事实源。推荐在批准 AG-01 设计时补充“AgentStep 管生命周期，ToolInvocation 管执行载荷”，并同步术语表和持久化设计，而不是先改代码再补契约。

## 4. 外部方案研究

### 4.1 OpenAI Agents SDK

OpenAI Agents SDK 的 Function Tool 支持两种定义方式：

- 从函数签名和 docstring 自动生成 JSON Schema。
- 直接提供 name、description、JSON Schema 和 invoke handler。

其运行管线统一处理 Schema 验证、上下文注入、guardrails、超时、失败格式化和 tracing。直接调用原函数会绕过这些边界。

可借鉴点：

- Tool Definition 与 Handler 必须分开。
- 输入必须在 Handler 前验证。
- Tool 超时是执行契约的一部分。
- Tool 错误分为“返回给模型让其恢复”和“硬失败终止运行”两类。
- 自定义 Tool 应显式提供完整 JSON Schema，而不是依赖隐式反射。

### 4.2 Claude Agent SDK

Claude Agent SDK 的工具定义包含：

- name
- description
- input schema
- handler
- 结构化结果 content blocks

权限、hooks 和工具执行是独立层。工具定义不自动获得执行权限。

可借鉴点：

- Tool Schema 与权限策略解耦。
- Tool Result 可以是多块结构化内容，而不是裸字符串。
- 执行器和权限系统不能合并成一个 Handler。

### 4.3 Google ADK

ADK 的 Function Tool 支持从类型和 docstring 生成声明，也允许显式 JSON Schema。`ToolContext` 由框架注入，模型不可见。确认操作和工具执行是相邻但独立的机制。

可借鉴点：

- Run/Session 上下文由执行器注入，不能出现在模型输入 Schema 中。
- 工具声明与运行上下文分离。
- 需要为长任务工具和中间状态预留协议，但 AG-01 不实现。

### 4.4 MCP

MCP Tools 定义了清晰的协议语义：

- `tools/list` 返回 name、description、inputSchema、可选 outputSchema、annotations。
- `tools/call` 传入 name 和 arguments。
- 结果包含 content blocks 和 `isError`。
- annotations 在不可信 Server 上只能当提示，不能当安全事实。

可借鉴点：

- 内部 Tool Contract 应天然支持未来的 MCP Adapter。
- JSON Schema 默认以 Draft 2020-12 为准。
- `isError` 表示工具级业务失败，不应复用传输层错误。
- Tool Annotation 不等于权限。

### 4.5 Anthropic 工具设计建议

Anthropic 的实践强调：

- 按服务或资源命名空间组织工具。
- 不要为每个低层 API 暴露一个工具。
- 返回高信号、上下文相关的结果，而不是所有底层字段。
- 输出要控制 token，截断时给出可执行提示。
- 错误消息应告诉模型如何修正，而不是暴露 traceback。
- 工具质量要用 Eval 衡量。

可借鉴点：

- Tool Registry 的价值不只是注册，还包括限制工具面和避免错误工具选择。
- Tool Result 是模型上下文的一部分，必须有信息密度预算。
- Tool 名称、描述和 Schema 需要测试，而不是只测试 Handler。

### 4.6 OWASP LLM06 Excessive Agency

OWASP 对 Agent 过度代理风险的建议包括：

- 最小化扩展功能。
- 最小化扩展权限。
- 在用户上下文中执行。
- 高危操作要求人工批准。
- 限制资源范围和副作用。

AG-01 虽然不实现审批和沙箱，但必须让后续策略能插入执行前边界，不能把权限判断写进工具 Handler。

### 4.7 工具结果也是不可信输入

即使工具来自可信 Registry，工具结果仍可能包含由用户、外部系统或历史数据写入的指令性文本。模型上下文中的 ToolResult 必须被标记为数据，而不是新的系统指令：

- Tool Schema 和 Registry 元数据来自可信代码，不能由 ToolResult 覆盖。
- ToolResult 中的“忽略之前指令”“调用其他工具”等内容只作为普通业务数据展示。
- 后续 AG-03 的工具选择策略必须基于当前用户意图和权限，不受 ToolResult 文本自行扩权。
- Test fixture 应包含工具结果提示注入样例，确保模型回复不会把数据误当系统策略。

### 4.8 外部方案到本需求的决策映射

| 来源 | 可验证事实 | AG-01 决策 |
| --- | --- | --- |
| OpenAI Agents SDK | Function Tool 管线统一处理 Schema 验证、超时、失败格式化和 tracing | Handler 前强制校验，Executor 统一映射超时和错误 |
| Claude Agent SDK | 工具定义、Handler、权限与 hooks 是不同层 | Tool Runtime 与 Authorizer Port 分离 |
| Google ADK | 运行上下文由框架注入，确认机制独立于普通参数 | Run/Step 上下文不出现在模型 Schema，审批留给 AG-03 |
| MCP | `tools/list` / `tools/call`、JSON Schema、content blocks、`isError` | 内部契约兼容 list/call 语义，业务失败不混同传输失败 |
| Anthropic 工具实践 | 工具面应少而任务导向，结果应高信号且 token-efficient | 首版只接一个 Today 工具，并限制返回字段和字节数 |
| OWASP LLM06 | 最小扩展面、最小权限、高危操作审批 | 首版只读、无 Shell、无写操作，执行前保留策略边界 |

这个映射是选择方案的证据链，不代表 AG-01 已获得实施批准。

## 5. 方案取舍

### 5.1 不直接嵌入外部 Agent 框架

不推荐把 OpenAI Agents SDK、Claude Agent SDK 或 LangGraph 直接引入 Go 后端：

- 现有 Run、SSE、SQLite、幂等和终态语义会形成双状态源。
- 前端协议和恢复机制会被迫跟随外部框架。
- 多供应商归一化仍会推迟，但架构会先被单一供应商绑定。
- 当前需求只需要最小 Tool Runtime，不需要完整框架。

采用原则仍是吸收语义和测试方法，不直接移植运行时。

### 5.2 不把每个底层 API 都暴露成 Tool

不推荐：

- `get_item_by_id`
- `list_items_by_date`
- `list_items_by_category`
- `read_database`

更推荐围绕任务设计：

- `today_search`
- `today_get_item`
- `today_summary`

第一版工具数量应少而稳定。工具过多会增加 Schema token、错误选择和权限面。

与当前 Today 代码对照后，这三个工具的成本并不相同：

| 工具 | 当前能力 | AG-01 需要补的内容 |
| --- | --- | --- |
| `today_search` | 仓库只有 `ListToday(ctx, startAt, endAt)`；没有通用搜索或单条读取端口 | 在领域服务内按标题、状态、分类过滤，返回精简列表；不把 SQL 暴露给 Handler |
| `today_get_item` | 没有 `GetItem` Repository 方法 | 新增窄端口和软删除语义测试，不能通过列表全量扫描掩盖契约缺失 |
| `today_summary` | 可由条目列表和分类聚合计算 | 新增只读统计适配器，明确完成度、任务数、专注时长口径 |

因此推荐先只实现 `today_search`。它足以验证 Schema、参数校验、超时、结果截断、持久化和模型工具选择，同时把 PR 控制在现有 Today 能力可以支撑的范围内。`today_get_item` 和 `today_summary` 应在首个工具闭环稳定后再追加。

### 5.3 JSON Schema 是唯一事实源

三种可选方案：

| 方案 | 优点 | 风险 |
| --- | --- | --- |
| Go 类型反射生成 Schema | 开发快 | 输出难控制，描述和 provider subset 不稳定 |
| 每个工具手写验证代码 | 无额外依赖 | 无法统一生成 Provider Schema，容易漂移 |
| 手写 JSON Schema 作为源，Handler 再做强类型解码 | 契约稳定，可同时用于验证和 Provider | 需要 Schema 编译和一致性测试 |

推荐第三种。Handler 接收通过 Schema 验证的 `json.RawMessage`，再解码到具体 Go 输入类型；不能只依赖 Go 解码认为输入合法。

### 5.4 工具调用表示方式

三种持久化方案：

| 方案 | 优点 | 风险 |
| --- | --- | --- |
| 只存普通 MessagePart 文本 | 改动小 | 参数、结果、状态和重试都无结构 |
| 把全部字段塞进 agent_steps JSON | 表少 | 类型漂移，难以查询和约束 |
| Step 生命周期 + 独立 Tool Invocation 表 | 边界清楚，可扩展审批和 Artifact | 需要新表与 join |

推荐第三种：

- `AgentStep` 是执行生命周期事实源。
- Tool Invocation 保存 ToolCall、ToolResult 和 ToolError。
- `tool-call` / `tool-result` MessagePart 后续只保存展示摘要或引用，不复制执行状态。

这比把所有信息塞进 `agent_steps` 更利于未来接入 Skills、MCP 和 Subagent。

### 5.5 不在 AG-01 引入完整 Agent 框架

继续观察 OpenAI Agents SDK、Claude Agent SDK、Google ADK、LangGraph 和 Temporal 后，当前仍不建议直接引入完整运行时。

原因不是“框架不好”，而是当前系统的稳定边界已经由现有代码定义：

- Run 的持久身份、终态裁决、取消和浏览器刷新恢复由 CraftingTable 自己维护。
- SSE 事件游标和 MessagePart 对账由现有 Access/Core 层控制。
- SQLite 事务、`clientRequestId` 幂等和服务重启收敛已经落地。
- 多供应商归一化明确后置，先接外部框架会把新的状态源提前带入核心。

只吸收可验证的工程约束：Schema 验证先于 Handler、上下文由执行器注入、错误可恢复边界、超时/取消、工具轨迹和 Fake Provider 测试。

## 6. 推荐领域契约

### 6.1 ToolDefinition

```ts
type ToolDefinition = {
  name: string
  namespace: string
  version: string
  title: string
  description: string
  inputSchema: unknown
  outputSchema?: unknown
  annotations: {
    readOnlyHint: boolean
    destructiveHint: boolean
    idempotentHint: boolean
    openWorldHint: boolean
  }
  timeoutMs: number
  maxInputBytes: number
  maxResultBytes: number
}
```

约束：

- `name` 在 Registry 内唯一，首版限制为 1 到 64 个 ASCII 字母、数字、下划线或连字符。
- `namespace` 只用于组织和过滤，不拼进模型 Tool Name。
- `version` 是稳定语义版本或递增整数，执行时写入 Step。
- `description` 描述“什么时候应该调用”，不能复制实现细节。
- `inputSchema` 顶层必须是 object。
- 首版至少要求 `additionalProperties: false` 和显式 `required`。
- 不允许远程 `$ref`、任意代码执行型扩展字段和包含秘密的默认值。
- `outputSchema` 可先作为可选字段；项目内建工具应尽量提供。
- `readOnlyHint` 等 annotation 必须来自可信的代码注册，不来自模型或远端元数据。
- `timeoutMs` 必须落在运行策略允许的上下界内。
- Annotation 只用于策略提示，不能单独授权执行；`readOnlyHint=true` 也不能证明 Handler 没有副作用。

### 6.2 ToolCall

```ts
type ToolCallStatus =
  | "received"
  | "running"
  | "completed"
  | "failed"
  | "cancelled"
  | "interrupted"

type ToolCall = {
  id: string
  providerCallId: string
  runId: string
  stepId: string
  name: string
  version: string
  arguments: unknown
  argumentsHash: string
  status: ToolCallStatus
  createdAt: string
  startedAt?: string
  finishedAt?: string
}
```

`providerCallId` 只用于重建同一次模型请求的消息链。领域身份始终使用 CraftingTable 自己的 `id`，避免供应商 ID 成为长期主键。

`providerCallId` 是 Provider Adapter 重建消息链所需的关联键，不是长期业务主键。首版要求 Provider 必须给出非空 id；缺失时视为协议错误，不由服务端伪造后再假装与上游一致。

未来 AG-03 增加审批时，可新增 `awaiting_approval` 和 `rejected`，但当前 Schema 不提前暴露未实现状态。

### 6.3 ToolResult

```ts
type ToolResultContent =
  | { type: "text"; text: string }
  | { type: "json"; value: unknown }

type ToolResult = {
  callId: string
  content: ToolResultContent[]
  isError: boolean
  error?: ToolError
  truncated: boolean
  encodedBytes: number
  createdAt: string
}
```

约定：

- 成功结果和可恢复的 Tool Error 都必须产生 ToolResult。
- 模型可见内容由结果内容生成；完整内部异常不能直接进入模型上下文。
- 首版只支持 text 和 json，不引入图片、文件和 Artifact。
- 超过 `maxResultBytes` 时由 Executor 截断，并附带模型可理解的截断说明。
- `isError=true` 不等于 Run 失败；是否继续由 AG-02 的政策决定。

### 6.4 ToolError

```ts
type ToolError = {
  code: string
  kind: "validation" | "not_found" | "conflict" | "timeout" |
        "cancelled" | "policy" | "upstream" | "internal"
  message: string
  retryable: boolean
  hint?: string
}
```

错误分层：

- 参数不符合 Schema：`validation`，Handler 不执行。
- 业务资源不存在：`not_found`，可返回给模型调整参数。
- Handler 返回可预期失败：`isError=true`，不输出堆栈。
- 超时或取消：由 Executor 终结 Tool Step。
- Panic、编码失败或内部不变量破坏：`internal`，默认不可重试。
- 上游错误：保存稳定 code，不把供应商原始响应直接给模型。

## 7. Tool Registry

### 7.1 静态注册

首版在 composition root 注册工具：

```text
bootstrap
  -> create Today service
  -> create Today read-only Tool adapters
  -> register in ToolRegistry
  -> inject Registry and Executor into Agent runtime
```

不实现动态插件、运行时上传工具或模型自行注册工具。

### 7.2 Registry 不变量

- 启动时拒绝重复 name。
- 启动时编译并验证全部 JSON Schema。
- 启动时拒绝不支持的 JSON Schema 关键字。
- Tool 列表按 name 确定性排序。
- 生成 Catalog Revision 或 Hash，并写入 Run 或 Trace。
- 运行中不增删工具；Registry 实例不可变。
- Tool Handler 只能通过构造注入的窄接口访问业务服务，不能拿 SQLiteStore 执行任意 SQL。

Registry 还应提供两个只读视图：

- `Catalog()`：按名称稳定排序，用于生成 Provider 工具列表和 Catalog Hash。
- `Resolve(name, version)`：执行时解析精确版本，拒绝隐式升级到“最新版本”。

### 7.3 Provider Name 映射

内部名称和未来 MCP 名称可能不完全兼容所有供应商。首版直接使用 provider-safe name；AG-06 接 MCP 时增加稳定 alias 表，不改变领域 ID。

### 7.4 Schema 校验库候选

调研时项目使用 Go 1.26.5。候选对比如下：

| 库 | 最新版本与状态 | 适配判断 |
| --- | --- | --- |
| `github.com/santhosh-tekuri/jsonschema/v6` | `v6.0.3`，2026-06 发布，支持 Draft 4/6/7、2019-09、2020-12，Apache-2.0，要求 Go 1.21 | 推荐作为 Phase 1 spike 首选 |
| `github.com/kaptinlin/jsonschema` | `v0.9.10`，2026-09 发布，但模块要求 Go 1.27 | 当前工具链不满足，不采用 |
| `github.com/qri-io/jsonschema` | `v0.2.1`，最后发布于 2021 | 版本过旧，不作为新契约依赖 |

推荐不等于直接合并依赖。Phase 1 spike 必须验证：

- 能编译项目实际使用的 Draft 2020-12 Schema。
- 可以关闭或拒绝远程 `$ref` 和网络加载。
- 错误能映射为稳定字段路径和 `validation` ToolError，不把库内部字符串直接暴露给模型。
- 对重复 key、非法数字和大输入的行为由 CraftingTable 自己的解码边界决定。
- 依赖只在 Agent Core 的 Schema 适配层出现，不进入领域对象 JSON tag 或业务 Handler。

## 8. Tool Executor

### 8.1 执行流程

```text
ToolCall received
  -> resolve ToolDefinition by name and version
  -> enforce input byte limit
  -> validate input with JSON Schema
  -> create or claim Tool Step
  -> authorizer hook
  -> derive timeout from tool + run deadline
  -> invoke handler with run-scoped context
  -> recover panic into internal error
  -> validate optional output schema
  -> bound and serialize result
  -> persist ToolResult and terminal Step
  -> emit tool lifecycle events
```

最重要的执行顺序约束是：Schema 校验失败时 Handler 绝不能启动；Tool Step 进入 `running` 前必须完成条件更新，防止同一调用被并发执行。

### 8.2 超时与取消

- Tool Timeout 不能超过 Run 剩余时间。
- Run 取消、Journal 关闭和 Service 关闭都必须传播到 Tool Context。
- Handler 必须响应 `context.Context`；同步阻塞调用不能直接塞进 Executor。
- 超时后不自动重试副作用工具。
- AG-01 不启动无归属后台 goroutine。

### 8.3 并发

首版串行执行同一模型响应中的多个 ToolCall，顺序以 Provider 给出的 call index 为准。理由：

- 串行更容易保证状态、事件和 SQLite 写入顺序。
- 大多数只读工具没有并行收益。
- 并行会让结果顺序、失败聚合和取消语义变复杂。

未来可在 Tool Definition 上增加 `concurrencySafe`，但仍由 AG-02 统一控制。

### 8.4 重试

AG-01 默认不自动重试任何工具。

- 只读且显式 `retryable` 的工具可由后续策略有限重试。
- 写工具必须提供稳定幂等键并得到目标系统幂等保证。
- 未确认副作用结果时进入未知状态，不能盲目重放。

### 8.5 Authorizer Hook

AG-01 只提供执行前 Hook，默认实现为 allow：

```go
type ToolAuthorizer interface {
    Authorize(context.Context, ToolInvocation) error
}
```

AG-03 再把 Hook 升级为 allow / ask / deny 并支持暂停恢复。首版不要把权限判断写进工具 Handler，也不要让 Handler 自己决定是否等待人工输入。

## 9. Provider Adapter

### 9.1 请求契约应扩展

Provider 接口不能继续只接收文本消息。建议抽象为：

```go
type ModelRequest struct {
    Messages []ProviderMessage
    Tools    []ProviderToolDefinition
    ToolChoice string
}
```

这里的 `ModelRequest` 是 Provider Adapter 的输入，不应直接成为前端或持久化协议。内部 Registry 保存完整规范，Adapter 只接收经过 provider subset 规范化后的工具列表。

首版 `ToolChoice` 只支持 `auto`。不支持强制指定工具、并行工具开关和供应商专有参数；这些能力需要独立需求和 Eval，不能通过透传任意 JSON 获得。

`ProviderMessage` 需要支持：

- user text
- assistant text / reasoning / tool calls
- tool result with call id

### 9.2 OpenAI-compatible 映射

当前唯一 Adapter 仍只支持 Chat Completions：

- Tool Definition -> `tools[].function`
- Model ToolCall -> assistant `tool_calls`
- ToolResult -> role `tool` + `tool_call_id`
- `finish_reason=tool_calls` -> 当前 Model Step 结束，存在待执行工具

供应商原始 `tool_calls`、`function`、`arguments` 和 finish reason 不得进入前端或 Core 之外的领域对象。

### 9.3 流式 ToolCall 聚合

OpenAI-compatible 流式工具调用通常按 index 分片：

- 第一片可能包含 id、type、function.name。
- 后续片主要包含 function.arguments 字符串增量。
- 同一响应可以包含多个 call index。

Parser 必须在内存中按 index 聚合，直到当前 Model Step 结束：

- 缺少 id 或 name 视为协议错误。
- arguments 必须能解析为 JSON。
- 重复 index 必须合并到同一 call。
- index 越界或同一字段冲突视为协议错误。
- 未完成的 ToolCall 不得交给 Executor。

工具参数分片可能跨多个网络 chunk，甚至包含转义字符的半个 JSON 字符串。聚合器必须按“字节串拼接后再解析”测试，不能对每片单独做 JSON 解析。

如果同一模型响应同时包含可见文本和 ToolCall，文本属于当前 Model Step 的中间输出。AG-01 应保存它并保持调用顺序，但不得把该文本误判为最终答案；是否继续由 AG-02 决定。

### 9.4 终止语义必须修正

当前 `[DONE]` 和 `finish_reason` 被当成 Run 完成。引入工具后必须区分：

- Model Step completed with final answer。
- Model Step completed with tool calls。
- Run completed。

否则工具调用会被错误地终结为无答案 Run。

### 9.5 Schema 规范与 Provider 子集

内部 Schema 统一使用 JSON Schema Draft 2020-12，但 Provider 往往只接受其中的一个子集。实现分两层：

1. Registry 编译并验证完整规范，拒绝远程 `$ref`、动态代码和非对象根。
2. Provider Adapter 生成明确支持的子集；遇到无法表达的字段应在启动或测试期失败，不能静默删除约束。

建议首版只允许：

- `type`、`properties`、`required`。
- `additionalProperties: false`。
- `enum`、`const`。
- 基础 `items`。
- `minLength`、`maxLength`、`minimum`、`maximum`。
- 有明确 provider 映射的 `description`。

这一限制是协议安全边界，不是 JSON Schema 能力上限。后续扩展关键字时同时增加 Adapter 映射和契约测试。

`argumentsHash` 必须基于确定性 JSON 序列化计算，不能直接对原始流式字节或 Go map 序列化结果求哈希。首版可使用按 key 排序的规范编码，并用测试固定字符串、数组、数字和 Unicode 行为。

### 9.6 工具结果回填模型上下文

AG-01 应提供可序列化为下一次模型请求的领域对象，但不在 Executor 内自行发起第二次模型调用。OpenAI-compatible Adapter 需要能表达：

```text
assistant message with tool_calls
tool message with tool_call_id and result content
assistant final message
```

规则：

- ToolResult 回填时使用同一 `providerCallId` 关联 assistant tool call。
- 模型可见结果只包含结果内容和稳定错误摘要，不包含数据库 ID、堆栈、表名或内部路径。
- 多个 ToolResult 的上下文顺序与 Tool Step 顺序一致。
- 不把 Reasoning Part 回填给模型；Reasoning 仍是展示内容，不是对话上下文。
- 上下文预算不足时应截断低价值结果并保留明确提示，不能把某个 ToolResult 静默省略成“工具没运行”。

这条序列化边界可以放在 Provider Adapter，但消息顺序和哪些内容属于模型上下文的规则不能由 Adapter 临时决定。

## 10. 持久化建议

### 10.1 Step

扩展 `agent_steps`：

- `kind`：首版 `model` / `tool`
- `parent_step_id`：为父子 Step 预留，首版可为空
- `error_code`：仅保存稳定错误摘要，完整错误留在 Tool Invocation

`step_index` 保持 Run 内单调递增。

Step 的职责是生命周期和排序，不保存完整 arguments/result。工具名称、版本和 Provider Call ID 只存一份，放在 Tool Invocation 中，避免两个表出现不一致。

### 10.2 Tool Invocation

新增 `agent_tool_invocations`，与 Tool Step 一对一：

- `id`
- `step_id`
- `run_id`
- `provider_call_id`
- `tool_name`
- `tool_version`
- `arguments_json`
- `arguments_hash`
- `result_json`
- `result_is_error`
- `result_truncated`
- `encoded_bytes`
- `created_at`
- `updated_at`

Tool Call 与 Tool Result 都存在这一张表中，避免为 1:1 生命周期拆出两张表。

唯一约束和并发规则：

- `step_id` 唯一，一条 Tool Step 只能有一个 Invocation。
- `(run_id, provider_call_id)` 唯一，防止同一次上游调用被重复创建。
- `arguments_hash` 用于审计和幂等辅助，不能替代业务幂等键。
- Invocation 创建与 Tool Step 创建在同一事务中完成。
- 状态从 `received` 到 `running` 使用条件更新；竞争失败的一方读取已有终态，不得再次调用 Handler。

完整 ToolResult 由 `result_json` 保存，错误也属于结果的一部分。若后续需要按错误码查询，可增加 `result_error_code` 索引列，但该列只能由同一写入事务生成。

### 10.3 MessagePart

首版不把完整参数和结果存进普通 MessagePart。

- `AgentStep` 和 Tool Invocation 是执行事实源。
- 后续 UI 需要时增加 `tool-call` / `tool-result` Part，只保存展示摘要和 stepId。
- 若 UI 需要完整参数，读取 Tool Invocation，不复制第二份 JSON。

现有 Agent 设计 §19.1 只提出增加工具 Part，没有规定 Part 是否存完整载荷；§7.1 又明确完整工具日志不应进入 MessagePart。批准 AG-01 设计时应把这条边界写清楚，避免实现者把执行事实复制到 Part。

### 10.4 恢复

进程重启后仍按现有策略把 active Tool Step 标记为 `interrupted`：

- 不自动重放 Handler。
- 不伪造成 completed。
- 保留 arguments 和已有结果供审计。
- 用户重试创建新 Run，不复用旧 call id。

恢复流程必须先读取 Tool Step 和 Tool Invocation，再决定是否允许新一轮模型调用。只要存在未终结的 Tool Step，原来的 Run 就不能自动继续；AG-05 实现检查点前没有安全机制证明副作用未发生。

## 11. 事件与前端最小契约

AG-01 可以定义但默认不展示以下 SSE 事件：

- `tool-call`
- `tool-execution-start`
- `tool-result`
- `tool-error`

事件只包含：

- stepId
- callId
- toolName
- 状态
- 可安全展示的摘要

不向前端推送：

- Provider 原始 tool_calls。
- Handler 堆栈。
- 完整内部日志。
- 敏感参数原值。

前端完整时间线、多个 text/reasoning 段和工具卡片留给 AG-02 一起设计，避免在单 step UI 上临时拼接。

事件与事实分层：

- Tool Step 和 Tool Invocation 是数据库事实源。
- SSE 工具事件只服务当前订阅者，不承担长期历史。
- Journal 过期或事件丢失时，前端通过 Session 详情重建最终工具时间线。
- 事件重放必须有稳定 `callId` 和 `stepId`，不能用数组位置推断归属。

## 12. 安全与资源限制

### 12.1 资源预算

建议首版默认值：

| 项目 | 建议上限 |
| --- | --- |
| 单 Run 可见工具数 | 20 |
| 单个 Schema 大小 | 32 KiB |
| Tool Catalog 总大小 | 128 KiB |
| 单次 arguments | 32 KiB |
| 单次模型可见结果 | 64 KiB |
| Registry 名称长度 | 64 字符 |
| JSON 嵌套深度 | 32 层 |
| 默认 Tool Timeout | 10 秒 |

这些是防御性起点，不是永久产品常量，最终由配置和 Eval 调整。

### 12.2 输入安全

- JSON 必须拒绝重复 key 或采用明确的最后值策略并有测试。
- 拒绝 `NaN`、`Infinity` 和非标准 JSON。
- Schema 不允许远程引用。
- Handler 不能信任 `name`、`namespace` 或 description 做权限判断。
- 所有路径、SQL、URL 和命令参数必须由领域 Tool 再校验。

### 12.3 日志

允许记录：

- callId
- stepId
- toolName / version
- argumentsHash
- duration
- result bytes
- error code

默认不记录：

- 完整 arguments
- 完整 result
- 用户消息
- 文件内容
- 密钥和凭据

## 13. 测试与验收

### 13.1 Registry 与 Schema

- 重复工具名导致启动失败。
- 非法或不支持的 JSON Schema 导致启动失败。
- 工具列表顺序和 Catalog Hash 稳定。
- 输入缺字段、类型错误、多字段和边界值被正确拒绝。
- Handler 在 Schema 验证前不会被调用。
- Provider 子集规范化有 golden test；不支持的约束会让启动或测试失败。
- `argumentsHash` 对相同 JSON 的不同 key 顺序产生相同结果，对语义不同的数字和 Unicode 产生明确且稳定的结果。
- 工具结果提示注入 fixture 不会改变 Registry、权限策略或系统指令。

### 13.2 Executor

- 成功执行产生 completed Tool Step 和 ToolResult。
- 业务错误产生 `isError=true`，不泄露内部错误。
- 超时、取消和进程中断进入唯一终态。
- Handler panic 被转为稳定的 internal error。
- 超限结果被截断并标记。
- 同一个 ToolCall 不能被并发执行两次。

### 13.3 Provider

- 分片 tool call arguments 正确聚合。
- 多个 tool calls 保持稳定顺序。
- `finish_reason=tool_calls` 不终结 Run。
- 缺少 id/name、非法 JSON、未知 finish reason 有明确错误。
- 同一响应同时包含文本和 ToolCall 时，两者都被保存且不会把中间文本当最终答案。
- 现有纯文本和 Reasoning 流不回归。

### 13.4 持久化与恢复

- Tool Step、Tool Invocation 和结果在同一事务边界内可恢复。
- 服务重启把运行中的 Tool Step 标记为 interrupted。
- Session 重新加载能重建 ToolCall 和 ToolResult。
- 旧 Session 没有 Tool Step 时保持兼容。
- 重复的 `providerCallId` 不会生成两个 Tool Step 或执行 Handler 两次。

### 13.5 默认测试策略

- Go 单元测试覆盖 Registry、Schema、Executor、Parser。
- Go 集成测试使用 loopback Fake Provider 和 Fake Tool。
- Vitest 只测试前端可消费的稳定事件和 Schema。
- 默认 Playwright 不调用真实模型。
- 真实模型工具调用作为显式 opt-in，不作为默认回归。
- Fake Provider 只存在于测试装配，不进入生产 Composition Root，也不向用户返回伪造模型内容。

### 13.6 AG-01 完成定义

AG-01 进入 `已实现` 至少要同时满足：

1. Registry 启动时完成 Schema 编译，重复名称和 provider 不支持的 Schema 会阻止启动。
2. 模型 ToolCall 参数经 Schema 校验后才可能执行 Handler。
3. 同一 ToolCall 在并发和重试场景下最多执行一次。
4. Tool Step、Tool Invocation、ToolResult 有唯一终态且可在 Session 重载后恢复。
5. 超时、取消、panic、非法 JSON、超大结果和未知工具都有稳定错误映射。
6. `finish_reason=tool_calls` 只结束 Model Step，不会把 Run 误标为 completed。
7. 纯文本和 Reasoning 的现有测试全部不回归。
8. Fake Provider + `today_search` 能完成一次可重放的工具执行链路。
9. 真实工具调用仍由 Feature Flag 控制，默认测试不访问真实模型。

第 8 条只证明 AG-01 的执行链路；用户看到“模型调用工具并继续回答”的闭环，仍需要 AG-02。

## 14. 建议实施拆分

### Phase 1：领域契约与 Registry

- ToolDefinition
- ToolCall / ToolResult / ToolError
- Registry 静态注册
- JSON Schema 编译与验证
- 不接 Provider、不写数据库

### Phase 2：Executor 与 Fake Tool

- ToolAuthorizer allow hook
- 超时、取消、panic、错误映射
- 大小限制和结果截断
- 纯内存测试

### Phase 3：Provider ToolCall 协议

- ModelRequest 工具列表
- 流式 tool_calls 聚合
- Model Step 终止语义
- Fake Provider 多片段测试

### Phase 4：持久化与恢复

- Step kind 与 tool invocation 迁移
- 创建、执行、终结事务
- Session 读取与 interrupted 恢复

### Phase 5：首个只读工具

推荐只接 `today_search`。它不产生外部副作用，足以验证 Schema、结果质量和模型工具选择。

不在首批同时接 `today_get_item` 和 `today_summary`：

- `today_get_item` 需要新增仓储端口和软删除测试。
- `today_summary` 需要先确认完成度和专注时长口径；T-07、T-08 仍是待实现需求。
- 一个工具就能验证 AG-01；三个工具会增加 Schema token 和错误选择面，却不会增加对运行时的覆盖。

### Phase 6：与 AG-02 联调

- 模型调用工具。
- 工具结果回填模型上下文。
- 模型继续生成最终回答。
- 多个 ToolCall 和 false-positive 工具选择进入 Eval。

## 15. 工作量与风险

粗略估计：单人 8 到 12 个工程日，不含 AG-02、UI 完整工具卡片、审批和沙箱。

| 阶段 | 粗略工作量 |
| --- | --- |
| 领域契约与 Registry | 1.5 到 2 天 |
| Executor 与 Fake Tool | 2 到 3 天 |
| Provider ToolCall 协议 | 2 到 3 天 |
| SQLite 持久化与恢复 | 2 到 3 天 |
| `today_search` 与集成测试 | 1 到 2 天 |

估算不包含真实模型质量调优。工具描述、Schema 和结果摘要通常需要多轮 Eval，不能把编译通过当成“工具设计完成”。

主要风险：

- Provider tool call 分片聚合和终止语义容易产生协议 bug。
- `agent_steps` 和 MessagePart 的关系若设计错误，后续 AG-02 会返工。
- Tool 结果直接成为模型上下文，必须控制 token、错误质量和提示注入面。
- 没有 AG-02 时无法做真实用户闭环验收。
- 首个工具过宽会让模型误用；应先使用少量只读工具和固定 Eval。

## 16. 推荐决策

| 决策 | 推荐 |
| --- | --- |
| Schema 来源 | 手写 JSON Schema，Handler 再做强类型解码 |
| Registry 生命周期 | 启动时静态注册，运行中不可变 |
| ToolCall 身份 | 服务端 UUID + Provider Call ID |
| 持久化 | AgentStep + 独立 Tool Invocation |
| Tool 结果 | 结构化 content + `isError`，限制大小 |
| 并行 | 首版串行 |
| 自动重试 | 首版禁用 |
| 权限 | 仅保留 Authorizer Port，AG-03 实现 |
| 沙箱 | AG-04 实现 |
| UI | AG-02 统一定义多 Step 时间线 |
| 首个工具 | 只做 `today_search` |
| Provider 范围 | 只接当前 OpenAI-compatible Adapter，不做多供应商归一化 |
| Schema 边界 | 内部 Draft 2020-12，Adapter 只下发明确支持的子集 |

## 17. 待确认问题

需要确认的只有首个工具范围。当前推荐：

- 首版只做 `today_search`，参数为日期、可选标题关键字、可选状态和可选分类。
- 返回精简列表，包含 `id`、标题、状态、分类、开始/结束时间和是否完成。
- 不返回数据库模型全部字段，不返回无界描述，不在首版增加 `today_get_item` 和 `today_summary`。

设计批准后，下一步产物应是正式设计文档和实施计划，而不是直接写业务代码。

## 18. 参考资料

- [OpenAI Agents SDK: Tools](https://openai.github.io/openai-agents-python/tools/)
- [OpenAI Agents SDK: Human-in-the-loop](https://openai.github.io/openai-agents-python/human_in_the_loop/)
- [Claude Agent SDK: Custom tools](https://code.claude.com/docs/en/agent-sdk/custom-tools)
- [Claude Agent SDK: Permissions](https://code.claude.com/docs/en/agent-sdk/permissions)
- [Google ADK: Function tools](https://adk.dev/tools-custom/function-tools/)
- [Google ADK: Action confirmations](https://adk.dev/tools-custom/confirmation/)
- [MCP: Tools](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)
- [Anthropic: Writing effective tools for agents](https://www.anthropic.com/engineering/writing-tools-for-agents)
- [OWASP LLM06: Excessive Agency](https://genai.owasp.org/llmrisk/llm062025-excessive-agency/)
- [santhosh-tekuri/jsonschema v6](https://pkg.go.dev/github.com/santhosh-tekuri/jsonschema/v6)
- [kaptinlin/jsonschema](https://pkg.go.dev/github.com/kaptinlin/jsonschema)
- [qri-io/jsonschema](https://pkg.go.dev/github.com/qri-io/jsonschema)
