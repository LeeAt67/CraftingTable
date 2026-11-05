# CraftingTable Agent Reasoning 交接文档

日期：2026-11-05
分支：`leeatMvp`
状态：reasoning 流式接收、持久化、恢复和折叠展示已完成

## 1. 结论

- MiMo 的 `delta.reasoning_content` 与 `delta.content` 现在分别写入两个 MessagePart。
- Part 顺序固定为 `reasoning: 0`、`text: 1`、`error` 从 2 开始。
- 推理阶段默认展开，首个正文增量到达后自动折叠。
- 用户手动切换后，自动规则不再覆盖。
- 刷新或重新打开会话后，reasoning 保留但默认折叠。
- 没有 reasoning 的 Run 在 Finalize 时删除空 reasoning Part，不显示空面板。
- reasoning 不进入下一轮模型上下文。
- 新建会话后立即发送时，旧的会话加载响应不会覆盖本地消息或中断新 Run。

## 2. 协议

```text
MiMo delta.reasoning_content
  -> Go "reasoning" provider event
  -> StreamReasoningStart / StreamReasoningDelta / StreamReasoningEnd
  -> UI Message Stream reasoning-start / reasoning-delta / reasoning-end
  -> 前端 reasoning MessagePart
```

事件顺序为 `start`、`start-step`、`reasoning-*`、`text-*`、`finish-step`、`finish`；
没有 reasoning 时跳过 `reasoning-*`。

## 3. 存储

- `003_agent_reasoning.sql` 重建 `agent_message_parts`，类型约束扩展为 `text`、`reasoning`、`error`。
- Finalize 在同一个事务中写入 reasoning 与正文，空 reasoning 会被删除。
- Interrupt 与启动恢复会把 reasoning 状态同步为 `interrupted`。
- 未持久化的 reasoning 不提供强于正文的恢复能力。

## 4. 前端

- `agentStreamSchema` 识别三个 reasoning 片段。
- `agentRunMaterializer` 独立维护 reasoning Part 与 text Part。
- `agentReasoningPanel` 负责折叠、滚动态和可访问性。
- 标题固定为 `Reasoning`，纯文本展示，不显示计时器、耗时或 token，也不做 Markdown 渲染。
- 发送和重试会使进行中的会话加载代次失效，避免旧响应覆盖新 Run。

## 5. 测试位置

- `server/internal/agent/access/session_controller_test.go`
- `tests/e2e/integration/agent_reasoning.integration_test.go`
- `tests/e2e/integration/agent_reasoning_stream.integration_test.go`
- `tests/e2e/integration/agent_reasoning_recovery.integration_test.go`
- `tests/e2e/unit/agent/agentRunMaterializer.test.ts`
- `tests/e2e/unit/agent/agentReasoningPanel.test.tsx`
- `tests/e2e/unit/agent/agentEffect.test.ts`
- `tests/e2e/unit/agent/agentPage.test.tsx`
- `tests/e2e/modules/agent/reasoning.e2e.ts`

默认回归不访问真实模型。Reasoning 浏览器验收使用 `.env.local` 中的真实 MiMo API：

```text
pnpm test:e2e:real
```

## 6. 已知限制

- 只识别 `reasoning_content`，未在本次实现多供应商适配。
- reasoning 不渲染 Markdown。
- reasoning 不解析 usage，不展示计时器、耗时或 token。
- 旧的 `/api/v1/agent/stream` 端点仍忽略 reasoning。
- 服务端进程重启不会继续原 Provider 流。

## 7. 后续 TODO

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
