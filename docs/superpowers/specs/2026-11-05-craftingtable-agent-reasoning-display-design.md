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
  -> Go StreamChunk
  -> reasoning-start / reasoning-delta / reasoning-end
  -> 前端 reasoning MessagePart
  -> 最终正文上方的可折叠面板
```

关键决定：

- reasoning 与最终正文分别持久化。
- 页面刷新和重新打开会话后，reasoning 仍可查看。
- reasoning 进行中默认展开，正文开始后自动折叠。
- 用户手动切换后，不再被自动折叠规则覆盖。
- 页面刷新或重新打开会话后默认折叠。
- 面板标题固定为 `Reasoning`。
- 不显示 Brain 图标，不显示计时器、耗时、token 或 usage。
- reasoning 默认不进入下一轮模型上下文。
- 本次只识别当前上游的 `reasoning_content`，不增加多供应商兼容层。
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

当前 MiMo 模型会返回真实 reasoning 内容，因此问题不在模型，而在完整的接入管线缺失。

## 3. 上游事实

当前配置使用 `xiaomi/mimo-v2.6-pro`。针对现有网关的验证结果：

- 非流式响应存在 `choices[0].message.reasoning_content`。
- 流式响应存在 `choices[0].delta.reasoning_content`。
- `reasoning_content` 与 `content` 是独立字段。
- 在验证请求中，reasoning 增量先于正文增量到达。

本次不承诺 reasoning 是模型的完整隐藏思维链。界面展示的是模型服务实际返回、允许客户端读取的内容。

## 4. 目标

- 实时接收并展示 MiMo 返回的 reasoning。
- reasoning 与最终正文在协议、存储和 UI 中完全分离。
- 支持刷新、重连和重新打开会话后的 reasoning 恢复。
- 提供可访问、可折叠且不会打断阅读的 reasoning 面板。
- 无 reasoning 时优雅降级，不显示空面板。
- 保持现有 Run 终态、取消和恢复语义。
- 保持前端只依赖 CraftingTable 内部协议。

## 5. 非目标

- 不实现 OpenAI、Anthropic、Gemini、DeepSeek 等多供应商 reasoning 适配。
- 不兼容 `delta.reasoning` 或其他供应商字段别名。
- 不解析、不持久化、不展示 usage、reasoning token、耗时或成本。
- 不显示 reasoning 面板计时器。
- 不保存或处理 provider reasoning signature、encrypted content 或其他元数据。
- 不增加多个 reasoning block。
- 不把 reasoning 注入后续模型上下文。
- 不在本次实现 reasoning Markdown 渲染。
- 不改变 Provider 请求参数，不增加 thinking 开关。
- 不改变现有 Provider Parser 对 `finish_reason` 和 `[DONE]` 的处理方式。
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
      |
      v
Provider Stream Parser
  "reasoning"
  "delta"
      |
      v
Run Coordinator
  StreamReasoningStart
  StreamReasoningDelta
  StreamReasoningEnd
  StreamTextDelta
      |
      v
Event Journal
  Reasoning string
  Text string
      |
      v
UI Message Stream
  reasoning-start
  reasoning-delta
  reasoning-end
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
本阶段不新增 metadata 字段。

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

`AssistantPartID` 保持现有 API 字段名，避免破坏调用方。`ReasoningPartID`
新增为可选字段。

若 Run 结束时 reasoning 内容为空，删除为空创建的 reasoning Part。正文和错误
Part 保留。

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
```

Finalize 在现有事务中：

1. 更新正文内容和状态。
2. 更新 reasoning 内容和状态。
3. 处理 error Part。
4. 如果 reasoning 为空，删除 reasoning Part。
5. 更新 Message 和 Run 终态。

Reasoning 和正文共享同一个终态与事务，不分别提交。

`InterruptRunRecord` 同样增加 `ReasoningPartID` 和 `ReasoningText`。取消、失败、
中断、Finalize 重试和启动失败回退都必须同步 settling 已存在的 reasoning Part。

### 8.5 Context Messages

构造模型上下文时只读取 `PartText`。`PartReasoning` 不进入
`ProviderMessage`。

## 9. SQLite 迁移

新增迁移：

```text
server/internal/platform/store/migrations/003_agent_reasoning.sql
```

不修改已经发布的 `002_agent.sql`。

SQLite 需要重建 `agent_message_parts`：

1. 创建带新类型约束的临时表。
2. 复制现有 `text` 和 `error` 数据。
3. 删除旧表。
4. 重命名新表。
5. 重建用户 Part 和助手 Run Part 的唯一索引。

迁移必须验证：

- 旧 `text` Part 内容完整。
- 旧 `error_json` 完整。
- Part ID、Message ID、Run ID、索引、状态和时间字段不变。
- 新 `reasoning` 类型可以写入。
- 迁移可以应用于已经执行过 `001` 和 `002` 的数据库。

## 10. 流协议

### 10.1 Provider Parser

本次只增加：

```json
{
  "choices": [
    {
      "delta": {
        "reasoning_content": "..."
      }
    }
  ]
}
```

Parser 产生内部事件：

```text
reasoning
```

空字符串不产生事件。不增加 `reasoning` 字段别名。

本次不读取 usage，也不改变 `finish_reason` 收到后结束解析的现有行为。

### 10.2 StreamChunk

新增：

```text
StreamReasoningStart
StreamReasoningDelta
StreamReasoningEnd
```

Reasoning Chunk 使用 `ReasoningPartID` 作为 `PartID`。

### 10.3 Event Journal

`JournalSnapshot` 增加独立字段：

```text
Reasoning string
Text      string
```

只有 `StreamReasoningDelta` 追加到 `Reasoning`，只有 `StreamTextDelta` 追加到
`Text`。

### 10.4 UI Message Stream

新增片段：

```text
reasoning-start
reasoning-delta
reasoning-end
```

JSON 结构：

```json
{
  "type": "reasoning-start",
  "id": "reasoning-part-id"
}
```

```json
{
  "type": "reasoning-delta",
  "id": "reasoning-part-id",
  "delta": "text"
}
```

```json
{
  "type": "reasoning-end",
  "id": "reasoning-part-id"
}
```

旧前端会把未知类型安全忽略，新前端将其解析为 reasoning 事件。

### 10.5 事件顺序

正常顺序：

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
text-end
finish-step
finish
```

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
- 第一个非空正文增量前发送 `reasoning-end`，前提是 reasoning 已开始。
- 第一个非空正文增量前发送 `text-start`。
- 终态前补齐尚未关闭的 reasoning 或 text。
- 不发送空的 reasoning-start/end 对。
- 同一 Run 的任何 reasoning 事件都使用同一个 `ReasoningPartID`。

## 11. 前端协议与 Materializer

### 11.1 Contracts

新增：

```ts
type AgentReasoningPartContent = {
  type: "reasoning";
  text: string;
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

新增三个 discriminated union 片段：

```text
reasoning-start
reasoning-delta
reasoning-end
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

### 11.3 Materializer

`RunMaterializer` 增加：

```text
reasoningStart()
reasoningDelta(text, cursor)
```

Materializer 独立维护 reasoning Part 和 text Part：

- 只有 `reasoningStart` 才创建 reasoning Part，避免空面板。
- `reasoningDelta` 只追加 reasoning 内容。
- `delta` 只追加正文内容。
- `finish` 同时更新两个 Part 的状态。
- `error` 保持现有行为，并同步更新已经存在的 reasoning Part 状态。

创建 Materializer 时支持：

```text
reasoningPartId
initialSnapshot
```

从 Session 详情恢复时，分别查找 reasoning 和 text Part，不依赖数组位置。

## 12. 恢复行为

### 12.1 运行中刷新

`GetAgentSession` 在 Active Run 存在时读取 Journal：

- `PartText` 使用 `snapshot.Text` 回填。
- `PartReasoning` 使用 `snapshot.Reasoning` 回填。

前端重新订阅 SSE 后继续接收缺失增量。

### 12.2 完成后刷新

Reasoning 和正文从 SQLite 读取。Reasoning Part 按普通 MessagePart 合并到
Zustand。

### 12.3 取消、失败和中断

已经收到的 reasoning 与正文一起保留。Reasoning Part 使用与 Message 相同的终态：

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

标题固定为：

```text
Reasoning
```

不使用 Brain 或其他装饰图标，不显示计时器、耗时或 token。标题右侧只显示折叠
箭头：

- 折叠时箭头向右。
- 展开时箭头向下。

整个标题行是可点击按钮。

### 13.3 状态规则

- 首次出现真实 reasoning 内容时显示面板。
- reasoning 流式阶段默认展开。
- 第一个正文片段出现时自动折叠。
- 折叠状态实现为 `auto | open | closed`。用户点击后将状态改为 `open` 或
  `closed`，自动规则不再覆盖。
- 完成、失败、取消或中断后仍可展开已收到内容。
- 页面刷新或重新打开会话后默认折叠。
- 无 reasoning 内容时不显示空面板。
- 不使用“正在思考…”等合成内容替代真实 reasoning。

### 13.4 内容样式

- 使用现有 `text-textSecondary`、`border-border` 等主题 token。
- 同时支持亮色和暗色主题。
- 内容区使用左侧细竖线表达层级，不使用内嵌卡片。
- 使用较小字号、次要颜色和 `whitespace-pre-wrap`。
- 不做 Markdown 渲染。
- 最大高度约为 `40vh`，超出后独立滚动。
- 流式增长时，仅当用户位于内容底部才自动跟随。

### 13.5 可访问性

- 标题按钮设置 `aria-expanded` 和 `aria-controls`。
- 折叠箭头不作为唯一状态表达。
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
| 取消 | 已收到 reasoning 和正文落库 |
| 失败 | 错误 Part 与 reasoning Part 共存 |
| 完成 | reasoning 与正文内容完整 |
| 空 reasoning | 完成后 reasoning Part 被删除 |
| 运行中 Session | Journal 回填 reasoning |
| 迁移 | 旧数据和索引完整 |
| 上下文 | reasoning Part 不进入 Provider 消息 |

### 15.2 前端单元

- Stream Schema 接受三个 reasoning 片段。
- Reducer 产生正确的 reasoning 事件和游标。
- Materializer 分别追加 reasoning 与正文。
- 从 Snapshot 恢复 reasoning Part。
- Part 状态在终态同步更新。
- 标题固定为 `Reasoning`，不依赖时间或 token 数据。
- 折叠状态遵守 `auto | open | closed`，手动选择后不被自动规则覆盖。
- 消息列表和 reasoning 内容区都只在接近底部时自动跟随。

### 15.3 浏览器

桌面和移动端覆盖：

1. 收到 reasoning 后显示固定标题 `Reasoning`。
2. 推理阶段默认展开。
3. 正文出现后自动折叠。
4. 点击标题重新展开。
5. 用户展开或关闭后不被再次自动折叠。
6. 刷新后 reasoning 保留且默认折叠。
7. 重新打开旧会话可展开 reasoning。
8. 无 reasoning 时不显示面板。
9. 用户上翻消息列表或 reasoning 内容区时不被强制拉回底部。

自动化测试使用 mock SSE，不依赖真实 API Key。实现完成后使用当前 MiMo 做一次人工联调。

## 16. 验收标准

- 当前 MiMo 返回的 `reasoning_content` 能在正文上方显示。
- Reasoning 和正文在数据库中是两个独立 Part。
- 生成中刷新后可以继续看到 reasoning。
- 完成后刷新仍可查看 reasoning。
- 无 reasoning 的 Run 不出现空折叠面板。
- reasoning 面板标题始终为 `Reasoning`。
- 不显示计时器、耗时、token、usage 或 Brain 图标。
- 不新增 reasoning metadata 字段或 metadata 流事件。
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
TODO: reasoning 耗时和 usage token 展示后续重新设计。本次不解析
usage、不持久化这些指标，也不增加面板计时器。
```
