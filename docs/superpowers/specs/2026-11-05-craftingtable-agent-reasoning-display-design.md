# CraftingTable Agent Reasoning Display Design

- **Status:** Draft for review
- **Date:** 2026-11-05
- **Scope:** MiMo reasoning 内容的流式接收、持久化、恢复和折叠展示
- **Depends on:** [Agent 设计](./2026-11-05-craftingtable-agent-design.md)、[持久化与恢复设计](./2026-11-05-craftingtable-agent-persistence-recovery-design.md)

## 1. 决策摘要

本次把模型返回的 reasoning 内容定义为一等 `MessagePart`，与最终正文分离：

```text
assistant message
  part 0: reasoning
  part 1: text
  part 2: error, 仅在失败时产生
```

采用以下链路：

```text
MiMo delta.reasoning_content
MiMo reasoning_tokens
  -> Go StreamChunk
  -> reasoning-start / reasoning-delta / reasoning-end / reasoning-metadata
  -> 前端 reasoning MessagePart
  -> 最终正文上方的可折叠面板
```

关键决定：

- reasoning 与最终正文分别持久化。
- 页面刷新和重新打开会话后，reasoning 仍可查看。
- 推理进行中默认展开，正文开始后自动折叠。
- 用户手动切换后，不再被自动折叠规则覆盖。
- 重新加载页面后默认折叠。
- reasoning 面板标题使用 `Reasoning (3s)`，不使用 Brain 图标，也不显示“思考中...”。
- 流式阶段显示实时计时器，reasoning 结束后冻结为服务端记录的真实耗时。
- 只有上游返回真实 usage 时才保存 reasoning token 数，并仅在展示开关允许时显示。
- 时间、token 等指标保存在通用 Part metadata JSON 中，展示开关与真实值分离。
- reasoning 默认不进入下一轮模型上下文。
- 本次只识别当前上游的 `reasoning_content`，不在服务端增加多供应商兼容层。
- 本次以纯文本展示 reasoning，Markdown 渲染留作后续 TODO。

## 2. 当前问题

当前 Agent 链路只能显示最终正文：

1. Provider 请求只发送 `model`、`messages` 和 `stream`。
2. OpenAI-compatible SSE Parser 只读取 `choices[].delta.content`。
3. Coordinator 只产生 `StreamTextDelta`。
4. UI Message Stream 只定义 `text` 和 `error` 片段。
5. SQLite 的 `agent_message_parts.type` 只允许 `text` 和 `error`。
6. 前端 Materializer 只维护正文 Part。
7. 当前“正在思考…”只是没有可见 Part 时的占位文案，不是真实 reasoning。
8. Provider Parser 丢弃 usage，前端拿不到 MiMo 返回的真实 reasoning token 数。
9. Parser 在收到 `finish_reason` 时立即结束，可能错过随后到达的 usage chunk。
10. MessagePart 没有可扩展 metadata，无法在不增加专用数据库列的情况下保存耗时和 token。

当前 MiMo 模型会返回真实 reasoning 内容，因此问题不在模型，而在完整的接入管线缺失。

## 3. 上游事实

当前配置使用 `xiaomi/mimo-v2.6-pro`。针对现有网关的验证结果：

- 非流式响应存在 `choices[0].message.reasoning_content`。
- 流式响应存在 `choices[0].delta.reasoning_content`。
- `reasoning_content` 与 `content` 是独立字段。
- 在验证请求中，reasoning 增量先于正文增量到达。
- 流式响应最终包含顶层 `usage`，其中
  `usage.completion_tokens_details.reasoning_tokens` 是 MiMo 返回的真实
  reasoning token 数。
- usage chunk 可能在带 `finish_reason` 的 chunk 之后到达，因此不能在第一次看到
  `finish_reason` 时立即关闭流解析。

本次不承诺 reasoning 是模型的完整隐藏思维链。界面展示的是模型服务实际返回、允许客户端读取的内容。

## 4. 目标

- 实时接收并展示 MiMo 返回的 reasoning。
- reasoning 与最终正文在协议、存储和 UI 中完全分离。
- 支持刷新、重连和重新打开会话后的 reasoning 恢复。
- 提供可访问、可折叠且不会打断阅读的 reasoning 面板。
- 无 reasoning 时优雅降级，不显示空面板。
- 显示真实耗时，并允许通过持久化字段控制是否展示耗时。
- 仅在上游返回真实 usage 时保存 reasoning token 数，并允许通过持久化字段控制是否展示。
- 保持现有 Run 终态、取消和恢复语义。
- 保持前端只依赖 CraftingTable 内部协议。

## 5. 非目标

- 不实现 OpenAI、Anthropic、Gemini、DeepSeek 等多供应商 reasoning 适配。
- 不兼容 `delta.reasoning` 或其他供应商字段别名。
- 不保存或处理 provider reasoning signature、encrypted content 或其他元数据。
- 不展示成本、推理预算或估算 token 数。
- 不根据字符数推算 token，也不在缺少 usage 时显示占位 token。
- 本次不增加用户可见的展示设置页面；`display` 字段先作为协议和持久化能力提供给 UI。
- 不把 reasoning 注入后续模型上下文。
- 不在本次实现 reasoning Markdown 渲染。
- 不改变 Provider 请求参数，不增加 thinking 开关。
- 不承诺服务端进程重启后恢复未提交的 reasoning，边界与正文一致。

## 6. 方案对比

### 6.1 方案 A：一等 Reasoning MessagePart

新增独立 `reasoning` Part，维护独立流事件，并在 Run 结束时与正文一起持久化。

优点：

- 刷新和重新打开会话后仍可查看。
- reasoning 与正文职责清晰。
- 不会污染最终回答或下一轮模型上下文。
- 可以复用现有 Journal、Run 恢复和 Part Materializer。
- 为后续统一多供应商 reasoning 协议保留自然扩展点。

代价：

- 需要新增数据库迁移。
- 需要扩展 StreamChunk、UI Stream、前端 Schema 和 Materializer。
- 需要同时维护 reasoning 与 text 的流开始和结束顺序。

### 6.2 方案 B：仅通过 SSE 临时展示

Reasoning 只在当前连接中显示，不进入 MessagePart 或数据库。

优点：

- 后端和数据库改动较少。

代价：

- 刷新后丢失。
- 与当前持久化恢复模型不一致。
- 无法查看旧会话的 reasoning。

### 6.3 方案 C：把 reasoning 拼进正文

以特殊分隔符把 reasoning 和正文写入同一个 text Part。

优点：

- 数据库和协议改动最少。

代价：

- reasoning 与最终答案语义混淆。
- 容易被错误发送回模型上下文。
- UI 需要解析特殊标记。
- 无法可靠维护独立状态。

### 6.4 结论

采用方案 A。

## 7. 数据流

```text
Provider SSE
  choices[0].delta.reasoning_content
  choices[0].delta.content
  usage.completion_tokens_details.reasoning_tokens
      |
      v
Provider Stream Parser
  "reasoning"
  "delta"
  "usage"
      |
      v
Run Coordinator
  StreamReasoningStart
  StreamReasoningDelta
  StreamReasoningEnd
  StreamTextDelta
  StreamReasoningMetadata
      |
      v
Event Journal
  Reasoning string
  Text string
  ReasoningMetadata
      |
      v
UI Message Stream
  reasoning-start
  reasoning-delta
  reasoning-end
  reasoning-metadata
  text-start
  text-delta
  text-end
      |
      v
Frontend Protocol + Materializer
  reasoning Part
  text Part
```

## 8. Domain 与存储模型

### 8.1 Part 类型

新增：

```go
PartReasoning PartType = "reasoning"
```

`agent_message_parts` 的类型约束扩展为：

```text
text
reasoning
error
```

Reasoning 内容复用 `text_content` 字段保存。`error_json` 只用于 error Part。
新增通用 `metadata_json` 保存结构化指标与展示开关，不为 duration 或 token
分别增加数据库列。

### 8.2 Run 创建

新 Run 创建时同时生成：

- `ReasoningPartID`
- `AssistantPartID`，继续表示最终正文 Part ID

新 Run 的 Part 索引：

```text
reasoning: 0
text:      1
error:     后续可用索引
```

`AssistantPartID` 保持现有 API 字段名，避免破坏调用方。`ReasoningPartID` 新增为可选字段。

若 Run 结束时 reasoning 内容为空，删除为空创建的 reasoning Part。正文和错误 Part 保留。

### 8.3 Run 执行与 Handle

扩展：

```text
RunExecution.ReasoningPartID
RunHandle.ReasoningPartID
```

`AssistantPartID` 仍表示正文 Part。

### 8.4 Finalize

`FinalizeRunRecord` 增加：

```text
ReasoningPartID
ReasoningText
ReasoningMetadata
```

Finalize 在现有事务中：

1. 更新正文内容和状态。
2. 更新 reasoning 内容和状态。
3. 处理 error Part。
4. 如果 reasoning 为空，删除 reasoning Part。
5. 更新 Message 和 Run 终态。

Reasoning 和正文共享同一个终态与事务，不分别提交。

`InterruptRunRecord` 同样增加 `ReasoningPartID`、`ReasoningText` 和
`ReasoningMetadata`。取消、失败、中断、Finalize 重试和启动失败回退都必须
同步 settling 已存在的 reasoning Part。

### 8.5 Metadata 与展示开关

数据库使用一个可扩展字段：

```sql
metadata_json TEXT NOT NULL DEFAULT '{}'
```

本阶段 reasoning Part 的 metadata 约定为：

```json
{
  "durationMs": 3000,
  "tokenCount": 12,
  "display": {
    "showDuration": true,
    "showTokenCount": true
  }
}
```

字段语义：

- `durationMs` 是服务端测得的真实整数毫秒数；没有有效测值时省略。
- `tokenCount` 只来自 `usage.completion_tokens_details.reasoning_tokens`；
  缺失时省略，绝不由字符数估算。
- `display.showDuration` 控制标题是否拼接耗时。
- `display.showTokenCount` 控制标题是否拼接 token 数。
- 当前新建 reasoning Part 时两个展示开关都默认为 `true`。
- 展示开关只影响渲染，不影响真实值采集和持久化；后续关闭展示不需要迁移数据库。
- `text` 和 `error` Part 使用空对象 `{}`，不要为未知 JSON 字段建立业务逻辑。

流式事件、Session 响应和最终持久化记录使用同一套 metadata 语义。前端对缺失
的真实值不做伪造，对关闭的指标不显示，也不保留空括号。

### 8.6 计时与 Usage 语义

- 计时从第一个非空 `reasoning_content` delta 到达服务端时开始。
- 计时在第一个非空正文 delta 到达时结束；若没有正文，则在 reasoning 段结束或
  Run 进入终态时结束。
- 服务端使用单调时钟测量，持久化整数 `durationMs`，避免使用消息创建时间估算。
- 流式 UI 使用服务端给出的 `startedAt` 计算当前秒数，并使用服务端终值覆盖本地估算。
- reasoning token 数只在收到真实 usage 后产生；usage 缺失不是错误。
- 取消、失败或中断时，已经产生的耗时和 token metadata 与 reasoning 内容一起保留。

### 8.7 Context Messages

构造模型上下文时只读取 `PartText`。`PartReasoning` 不进入 `ProviderMessage`。

## 9. SQLite 迁移

新增迁移：

```text
server/internal/platform/store/migrations/003_agent_reasoning.sql
```

不修改已经发布的 `002_agent.sql`。

SQLite 需要重建 `agent_message_parts`：

1. 创建带新类型约束和 `metadata_json TEXT NOT NULL DEFAULT '{}'` 的临时表。
2. 复制现有 `text` 和 `error` 数据。
3. 删除旧表。
4. 重命名新表。
5. 重建用户 Part 和助手 Run Part 的唯一索引。

迁移必须验证：

- 旧 `text` Part 内容完整。
- 旧 `error_json` 完整。
- 旧 Part 的 `metadata_json` 自动得到 `{}`。
- Part ID、Message ID、Run ID、索引、状态和时间字段不变。
- 新 `reasoning` 类型可以写入。
- reasoning metadata 可以往返 JSON 编码和解码。
- 迁移可以应用于已经执行过 `001` 和 `002` 的数据库。

## 10. 流协议

### 10.1 Provider Parser

本次读取三类上游数据：

```json
{
  "choices": [
    {
      "delta": {
        "reasoning_content": "..."
      }
    }
  ],
  "usage": {
    "completion_tokens_details": {
      "reasoning_tokens": 12
    }
  }
}
```

Parser 产生内部事件：

```text
reasoning
delta
usage
done
```

规则：

- 非空 `delta.reasoning_content` 产生 `reasoning` 事件。
- 非空 `delta.content` 产生 `delta` 事件。
- 只要出现 `usage.completion_tokens_details.reasoning_tokens`，就产生 `usage`
  事件，包括真实的 `0`。
- 空字符串不产生 reasoning 或 text 事件。
- 不增加 `reasoning` 字段别名。
- `finish_reason` 只表示 Provider 已结束生成，不立即产生 `done`，也不关闭
  Parser。
- Parser 继续读到 `[DONE]`；若 EOF 前已观察到 `finish_reason`，可把该 EOF 当作
  正常结束。未观察 `finish_reason` 的 EOF 仍是 `ErrUnexpectedEOF`。
- 这样可以接收位于 `finish_reason` chunk 之后、且可能没有 choices 的 usage
  chunk。

### 10.2 StreamChunk

新增：

```text
StreamReasoningStart
StreamReasoningDelta
StreamReasoningEnd
StreamReasoningMetadata
```

Reasoning Chunk 使用 `ReasoningPartID` 作为 `PartID`。相关 Chunk 需要携带：

```text
StartedAt
DurationMS
ReasoningMetadata
```

### 10.3 Event Journal

`JournalSnapshot` 增加独立字段：

```text
Reasoning          string
Text               string
ReasoningMetadata  structured metadata
```

只有 `StreamReasoningDelta` 追加到 `Reasoning`，只有 `StreamTextDelta` 追加到
`Text`。Journal 同时保存 reasoning 的开始时间和最新真实 metadata，使运行中
Session 刷新可以拿到当前耗时与已经到达的 token 数。

### 10.4 UI Message Stream

新增片段：

```text
reasoning-start
reasoning-delta
reasoning-end
reasoning-metadata
```

`reasoning-start`：

```json
{
  "type": "reasoning-start",
  "id": "reasoning-part-id",
  "startedAt": "2026-11-05T10:00:00.000Z",
  "display": {
    "showDuration": true,
    "showTokenCount": true
  }
}
```

`reasoning-delta`：

```json
{
  "type": "reasoning-delta",
  "id": "reasoning-part-id",
  "delta": "text"
}
```

`reasoning-end`：

```json
{
  "type": "reasoning-end",
  "id": "reasoning-part-id",
  "durationMs": 3000
}
```

`reasoning-metadata` 是只更新现有 reasoning Part 的增量 metadata，不创建新的
reasoning block，也不推定缺失值：

```json
{
  "type": "reasoning-metadata",
  "id": "reasoning-part-id",
  "metadata": {
    "tokenCount": 12
  }
}
```

`reasoning-metadata` 在真实 usage 到达后才发送。没有 usage 时不发送，Session
响应中也不出现 `tokenCount`。旧前端会把未知类型安全忽略，新前端将其解析为
reasoning 事件。

### 10.5 事件顺序

有 reasoning 和正文时：

```text
start
start-step
reasoning-start
reasoning-delta
reasoning-delta
reasoning-end
text-start
text-delta
text-delta
reasoning-metadata
text-end
finish-step
finish
```

`reasoning-end` 在 reasoning 段结束时发送，`reasoning-metadata` 在 Provider
流读完并取得 usage 后发送。两者顺序可以相隔正文增量；前端不得因为 metadata
晚到而重复创建 reasoning Part。

没有 reasoning 时：

```text
start
start-step
text-start
text-delta
text-end
finish-step
finish
```

实现要求：

- 第一个非空 reasoning 增量前发送 `reasoning-start`。
- 第一个非空正文增量前发送 `reasoning-end` 和当前真实 `durationMs`，前提是
  reasoning 已开始。
- 第一个非空正文增量前发送 `text-start`。
- Provider 流结束后如有真实 usage，发送一次 `reasoning-metadata`。
- 终态前补齐尚未关闭的 reasoning 或 text。
- 不发送空的 reasoning-start/end 对。
- 同一 Run 的任何 reasoning 事件都使用同一个 `ReasoningPartID`。

## 11. 前端协议与 Materializer

### 11.1 Contracts

新增：

```ts
type AgentPartDisplay = {
  showDuration: boolean;
  showTokenCount: boolean;
};

type AgentReasoningMetadata = {
  durationMs?: number;
  tokenCount?: number;
  display: AgentPartDisplay;
};

type AgentReasoningPartContent = {
  type: "reasoning";
  text: string;
  metadata: AgentReasoningMetadata;
};

type AgentReasoningPart = AgentMessagePartBase & {
  type: "reasoning";
  content: AgentReasoningPartContent;
};
```

`AgentMessagePart` 扩展为：

```ts
AgentTextPart | AgentReasoningPart | AgentErrorPart
```

`AgentRunHandle` 增加可选字段：

```ts
reasoningPartId?: string;
```

现有 `assistantPartId` 继续表示正文 Part。

### 11.2 Stream Schema

新增四个 discriminated union 片段：

```text
reasoning-start
reasoning-delta
reasoning-end
reasoning-metadata
```

Reducer 将 `reasoning-delta` 转换为：

```ts
{
  type: "reasoning-delta";
  id: string;
  delta: string;
  cursor: number;
}
```

Reducer 为其余三类片段产生对应事件。`reasoning-metadata` 执行浅合并，不覆盖已经
存在的 `durationMs` 或 `display`。

### 11.3 Materializer

`RunMaterializer` 增加：

```text
reasoningStart(startedAt, display)
reasoningDelta(text, cursor)
reasoningEnd(durationMs)
reasoningMetadata(metadata)
```

Materializer 独立维护 reasoning Part 和 text Part：

- 只有 `reasoningStart` 才创建 reasoning Part，避免空面板。
- `reasoningDelta` 只追加 reasoning 内容。
- `reasoningEnd` 写入服务端终值 `durationMs`。
- `reasoningMetadata` 合并真实 token 数。
- `delta` 只追加正文内容。
- `finish` 同时更新两个 Part 的状态。
- `error` 保持现有行为，并同步更新已经存在的 reasoning Part 状态。

创建 Materializer 时支持：

```text
reasoningPartId
initialSnapshot
```

从 Session 详情恢复时，分别查找 reasoning 和 text Part，不依赖数组位置。
运行中的 Session metadata 需要包含截至快照时刻的真实 `durationMs`，使刷新后的
计时器从服务端基线继续，而不是从 0 重新开始。

## 12. 恢复行为

### 12.1 运行中刷新

`GetAgentSession` 在 Active Run 存在时读取 Journal：

- `PartText` 使用 `snapshot.Text` 回填。
- `PartReasoning` 使用 `snapshot.Reasoning` 回填。
- `PartReasoning.metadata.durationMs` 使用快照时刻的当前耗时回填。
- 已经收到 usage 时，`PartReasoning.metadata.tokenCount` 使用真实值回填。

前端重新订阅 SSE 后继续接收缺失增量。

### 12.2 完成后刷新

Reasoning 和正文从 SQLite 读取。Reasoning Part 按普通 MessagePart 合并到 Zustand。

### 12.3 取消、失败和中断

已经收到的 reasoning、真实 metadata 与正文一起保留。Reasoning Part 使用与
Message 相同的终态：

```text
complete
cancelled
failed
interrupted
```

### 12.4 服务端进程重启

Reasoning 不提供强于正文的恢复能力。未在 Finalize 事务中提交的 reasoning 不保证恢复。

### 12.5 空 Reasoning

没有 reasoning 的模型或 Run：

- 不发送 reasoning 流事件。
- 不显示 UI 面板。
- Run 完成后删除空的 reasoning Part。

## 13. UI 设计

### 13.1 位置

Reasoning 面板位于最终正文上方，与正文属于同一条 assistant Message。

### 13.2 标题

标题由固定名称、耗时和 token 数组成：

```text
Reasoning (3s)
Reasoning (3s · 12 tokens)
Reasoning (12 tokens)
Reasoning
```

实际标题由 metadata 和展示开关决定：

- `showDuration=true` 且存在 `durationMs` 时显示整秒耗时。
- `showTokenCount=true` 且存在 `tokenCount` 时显示 token 数。
- 两个真实值都存在且都展示时使用中点分隔：
  `Reasoning (3s · 12 tokens)`。
- 字段缺失或对应开关为 `false` 时省略该段；所有段都省略时只显示
  `Reasoning`。
- 流式阶段耗时会持续增长，例如 `Reasoning (1s)`、`Reasoning (2s)`；
  reasoning 结束后使用服务端终值，不在前端继续累加。
- 耗时使用 `floor(durationMs / 1000)` 得到整秒，最小为 `0s`。
- token 为 `1` 时使用 `1 token`，其余使用 `N tokens`。

不使用 Brain、灯泡或其他装饰图标。标题右侧只显示折叠箭头：

- 折叠时箭头向右。
- 展开时箭头向下。

整个标题行是可点击按钮。

### 13.3 状态规则

- 首次出现真实 reasoning 内容时显示面板。
- reasoning 流式阶段默认展开。
- 第一个正文片段开始时自动折叠。
- 折叠状态实现为 `auto | open | closed`。用户点击后将状态改为 `open` 或
  `closed`，自动规则不再覆盖。
- 完成、失败、取消或中断后仍可展开已收到内容。
- 页面刷新或重新打开会话后默认折叠。
- 无 reasoning 内容时不显示空面板。
- 不使用“正在思考…”等合成内容替代真实 reasoning。
- 计时器和 token 数都不得导致空括号；没有可展示段时标题仅为 `Reasoning`。

### 13.4 内容样式

- 使用现有 `text-textSecondary`、`border-border` 等主题 token。
- 同时支持亮色和暗色主题。
- 内容区使用左侧细竖线表达层级，不使用内嵌卡片。
- 使用较小字号、次要颜色和 `whitespace-pre-wrap`。
- 不做 Markdown 渲染。
- 最大高度约为 `40vh`，超出后独立滚动。
- 流式增长时，仅当用户位于内容底部才自动跟随。
- 计时器每秒边界更新一次，组件卸载时清理 interval。
- 动效使用 `motion-reduce` 兼容策略；计时文字本身不做逐秒闪烁。

### 13.5 可访问性

- 标题按钮设置 `aria-expanded` 和 `aria-controls`。
- 折叠箭头不作为唯一状态表达。
- 标题的可访问名称包含当前可见的耗时和 token 数，例如
  `Reasoning, 3 seconds, 12 tokens`。
- 计时器更新不进入 live region，避免每秒播报。
- reasoning 增量不逐字进入读屏播报。
- 无正文时读屏状态为“Agent 正在思考”。
- 正文出现后读屏状态为“Agent 正在回复”。

### 13.6 滚动行为

- 外层消息列表只在更新前用户位于接近底部时调用 `scrollIntoView`。
- 用户主动向上翻阅后，新的 reasoning 或正文增量不得把视图强行拉回底部。
- reasoning 内容区维护独立滚动位置，仅在用户位于其底部时跟随最新内容。
- 用户主动上滑后暂停 reasoning 内部自动跟随；用户回到底部后恢复。
- 折叠或展开不得抢夺输入框焦点，也不得重置外层会话滚动位置。

## 14. 错误处理与隐私

- 不记录 reasoning 正文日志。
- 日志可以记录 Part ID、事件类型、字符数和错误类型。
- Provider 流异常沿用现有 Run 失败流程。
- Reasoning 缺失不是错误，按无 reasoning 处理。
- Reasoning 持久化失败与正文一样进入现有 Finalize 失败和回退流程。
- 用户已确认 reasoning 保存在本地 SQLite，因此 UI 和文档不得暗示内容不会被持久化。

## 15. 测试矩阵

### 15.1 Go 单元与集成

| 场景 | 验证 |
| --- | --- |
| 只有 reasoning | 产生 reasoning 事件，无空白正文事件 |
| reasoning 后正文 | 事件顺序正确 |
| 只有正文 | 不产生 reasoning 事件 |
| 空 reasoning_content | 忽略 |
| `finish_reason` 后还有 usage chunk | Parser 不提前结束，usage 被读取 |
| 只有 `finish_reason` 后 EOF | 作为正常完成，不报意外 EOF |
| usage 缺失 | 不生成 tokenCount，Run 正常完成 |
| usage token 为 0 | 保留并持久化真实 0 |
| 同一流多次产生 usage | 使用最后一个有效 reasoning token 值 |
| 取消 | 已收到 reasoning 和正文落库 |
| 失败 | 错误 Part 与 reasoning Part 共存 |
| 完成 | reasoning 与正文内容完整 |
| 完成 | durationMs 使用服务端测量终值 |
| metadata 展示开关为 false | 真实值仍落库，开关值保持不变 |
| 空 reasoning | 完成后 reasoning Part 被删除 |
| 运行中 Session | Journal 回填 reasoning、当前耗时和 token |
| 乱序/晚到 metadata | 只更新原 reasoning Part，不创建第二个 Part |
| 迁移 | 旧数据、索引和默认 metadata 完整 |
| 上下文 | reasoning Part 不进入 Provider 消息 |

### 15.2 前端单元

- Stream Schema 接受四个 reasoning 片段。
- Reducer 产生正确的 reasoning 事件和游标。
- 晚到的 `reasoning-metadata` 只合并 metadata。
- Materializer 分别追加 reasoning 与正文。
- 从 Snapshot 恢复 reasoning Part。
- Part 状态在终态同步更新。
- 假计时器下标题从 `Reasoning (0s)` 更新到 `Reasoning (1s)`。
- `reasoning-end` 后计时器停止并采用服务端终值。
- 标题覆盖耗时、token、两者和两者都关闭四种组合。
- 缺少真实值或展示开关关闭时不出现空括号。
- 折叠状态遵守 `auto | open | closed`，手动选择后不被自动规则覆盖。
- 消息列表和 reasoning 内容区都只在接近底部时自动跟随。

### 15.3 浏览器

桌面和移动端覆盖：

1. 收到 reasoning 后显示 `Reasoning (0s)` 或 `Reasoning (1s)`，不显示 Brain 图标。
2. 推理阶段默认展开。
3. 正文出现后自动折叠。
4. 点击标题重新展开。
5. 用户展开或关闭后不被再次自动折叠。
6. usage 晚到时标题更新为 `Reasoning (3s · 12 tokens)`。
7. 将 `showDuration` 或 `showTokenCount` 设为 `false` 时对应段消失。
8. 刷新后 reasoning 保留且默认折叠。
9. 运行中刷新后计时从服务端当前值继续，不从 0 重启。
10. 重新打开旧会话可展开 reasoning。
11. 无 reasoning 时不显示面板。
12. 用户上翻消息列表或 reasoning 内容区时不被强制拉回底部。

自动化测试使用 mock SSE，不依赖真实 API Key。实现完成后使用当前 MiMo 做一次人工联调。

## 16. 验收标准

- 当前 MiMo 返回的 `reasoning_content` 能在正文上方显示。
- Reasoning 和正文在数据库中是两个独立 Part。
- 生成中刷新后可以继续看到 reasoning。
- 完成后刷新仍可查看 reasoning。
- 无 reasoning 的 Run 不出现空折叠面板。
- reasoning 面板标题使用 `Reasoning (Ns)`，结束后按开关展示真实耗时和 token。
- 计时器在 reasoning 结束后冻结，刷新后从服务端基线继续或读取最终值。
- token 只来自 MiMo 的真实 reasoning usage，不出现估算值。
- `display.showDuration` 和 `display.showTokenCount` 能独立控制对应段。
- duration、token 和展示开关通过通用 metadata JSON 持久化，没有专用指标列。
- `finish_reason` 之后到达的 usage 不会丢失。
- reasoning 不进入后续模型上下文。
- 折叠交互支持键盘和读屏。
- 主动上翻的用户不会被流式更新强制拉回底部。
- 旧会话、旧数据和现有恢复测试不回归。
- `typecheck`、前端测试、Go 测试、迁移测试、构建和相关 E2E 全部通过。

## 17. 后续 TODO

```text
TODO: 下一阶段由服务端多供应商适配层统一 OpenAI、Anthropic、
Gemini、DeepSeek 等上游的 reasoning 字段和元数据。前端只接收
CraftingTable 统一 reasoning 协议，不接触供应商原始字段。
```

```text
TODO: reasoning 内容后续支持 Markdown 渲染，包括标题、列表、
代码块等格式。本次按纯文本展示并保留换行。
```

```text
TODO: 若产品需要用户自行切换耗时和 token 展示，再增加设置入口修改
display.showDuration 和 display.showTokenCount；本次只提供字段、持久化和
UI 渲染能力。
```
