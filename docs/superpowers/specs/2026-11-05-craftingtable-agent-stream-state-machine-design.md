# CraftingTable Agent Stream State Machine Design

- **Status:** Implemented and verified on 2026-11-05
- **Date:** 2026-11-05
- **Scope:** Agent 前端流协议内部结构
**Depends on:** [工程化设计规范](./2026-11-05-craftingtable-engineering-design.md)、[Agent 设计](./2026-11-05-craftingtable-agent-design.md)

## 1. 决策摘要

使用“Schema 判别联合 + 显式协议状态机 + 纯 Reducer”重构 `agentStreamProtocol.ts`。

`eventsource-parser` 继续只负责 SSE 分帧。`agentStreamProtocol.ts` 负责 JSON、Schema、生命周期和协议错误。`apiAgentModel.ts` 只把已经验证的领域事件映射为文本增量，不再自行判断流是否正常结束。

该设计吸收 Reducer、State 和 Strategy 的思想，但不在只有三种生命周期状态的协议上引入 Handler 类、动态注册表或 Chain of Responsibility。模式服务于复杂度，不能替代复杂度。

## 2. 背景

当前 `agentStreamProtocol.ts` 同时承担：

- SSE 读取。
- JSON 解析。
- Zod Schema 校验。
- `started` 和 `terminalSeen` 生命周期判断。
- 未知事件过滤。
- 领域事件映射。
- 协议错误生成。

`apiAgentModel.ts` 还保存一个 `finished` 标记，用于判断是否收到终止事件。生命周期状态因此分散在两个模块中：

```text
agentStreamProtocol.ts: started + terminalSeen
apiAgentModel.ts:        finished
```

协议继续扩展时，新增事件、EOF 规则或终止语义容易产生遗漏。状态应当集中在协议边界，而不是由调用方猜测。

## 3. 目标

- 用一个显式状态机表达所有协议生命周期。
- 让 Schema、状态转换和 I/O 分离。
- 让 `apiAgentModel.ts` 只消费已验证事件。
- 把 EOF 与 `[DONE]` 的完整性判断收回协议层。
- 保持 `readAgentStream` 的公开签名不变。
- 保持 `AgentStreamEvent`、模型边界和 Zustand 状态所有权不变。
- 所有新增行为先写失败测试，再写最小实现。

## 4. 非目标

- 不修改 Go SSE 输出格式。
- 不修改 HTTP 路由或请求体。
- 不引入 Vercel AI SDK、`aisdk-go` 或其他完整流框架。
- 不把协议事件写入 Zustand。
- 不为 Tool、Skills、MCP 或多 Agent 预留空 Handler。
- 不重构 `agentTransport.ts` 的请求和错误 envelope。
- 不在本设计中改变 UI 展示或消息持久化。

## 5. 方案对比

### 5.1 方案 A：显式状态机与纯 Reducer

保留一个 Zod 判别联合作为协议 Schema Registry。把事件生命周期收敛到 `reduceAgentStream(state, part)`，返回下一状态和本次产生的领域事件。

优点：

- 生命周期只有一份事实源。
- Reducer 是纯函数，可以直接覆盖状态转换矩阵。
- 新增事件只需同时更新 Schema、Reducer 和 fixture。
- 不增加类层级、接口数量或运行时注册流程。

代价：

- Reducer 内仍有一个按事件类型分支的 `switch`。
- 状态增加时必须补状态转换测试。

### 5.2 方案 B：Handler Registry 与 Strategy

为每种 chunk 注册一个 Handler，例如 `start`、`text-delta`、`finish` 和 `error` 各自处理。

优点：

- 单类事件逻辑局部化。
- 大量事件类型时便于扩展。

代价：

- TypeScript 对判别联合的 Handler 泛型约束会明显增加。
- 当前只有八种协议 chunk，注册表不会减少实际代码量。
- 状态校验仍需要一个共享上下文，容易变成隐式状态。

### 5.3 方案 C：Chain of Responsibility

每一层判断一种事件或错误，不匹配时交给下一层。

优点：

- 单条规则可独立加入。

代价：

- 事件顺序和终止约束难以静态检查。
- 当前规则具有互斥的判别类型，不需要责任链。
- 调试时只能观察链式副作用，状态转换不直观。

### 5.4 结论

采用方案 A。

原因不是“模式更简单”，而是当前问题本身就是一个有限状态机。显式状态转换比工具型 Handler 更接近业务语义。

## 6. 目标架构

```text
ReadableStream<Uint8Array>
  -> eventsource-parser
  -> JSON.parse
  -> streamPartSchema
  -> parseAgentStreamPart
  -> reduceAgentStream
  -> AgentStreamEvent
  -> apiAgentModel
  -> AgentMessageSnapshot
  -> AgentEffect
  -> Zustand
```

内部职责：

| 组件 | 职责 | 不负责 |
| --- | --- | --- |
| `eventsource-parser` | SSE 分帧和跨网络 chunk 缓冲 | JSON、业务语义、状态 |
| `streamPartSchema` | 已知 chunk 的结构校验 | 生命周期顺序 |
| `parseAgentStreamPart` | 区分已知、未知和畸形 chunk | 维护跨事件状态 |
| `reduceAgentStream` | 状态转换、事件产出和顺序校验 | SSE、I/O、Store |
| `readAgentStream` | 组合流读取、解析、Reducer 和 EOF 检查 | 领域消息快照 |
| `apiAgentModel` | 领域事件到文本流的映射 | 判断协议是否完整结束 |

## 7. 状态机

状态集合：

```ts
type AgentStreamPhase =
  | "awaiting-start"
  | "streaming"
  | "terminal";
```

状态只描述协议生命周期，不保存消息正文、模型配置、AbortSignal 或 Store 数据。

状态对象：

```ts
type AgentStreamState = {
  phase: AgentStreamPhase;
};
```

状态转换：

| 当前状态 | 事件 | 下一状态 | 输出 | 失败 |
| --- | --- | --- | --- | --- |
| `awaiting-start` | `start` | `streaming` | 无 | 元数据无效 |
| `awaiting-start` | 其他已知 chunk | `awaiting-start` | 无 | `PROTOCOL_INVALID_METADATA` |
| `streaming` | `start-step` | `streaming` | 无 | 无 |
| `streaming` | `text-start` | `streaming` | 无 | 无 |
| `streaming` | `text-delta` | `streaming` | `text-delta` | 无 |
| `streaming` | `text-end` | `streaming` | 无 | 无 |
| `streaming` | `finish-step` | `streaming` | 无 | 无 |
| `streaming` | `finish` | `terminal` | `finish` | 无 |
| `streaming` | `error` | `terminal` | `error` | 无 |
| `terminal` | 任意已知语义 chunk | `terminal` | 无 | `PROTOCOL_INVALID_METADATA` |
| 任意状态 | 未知非终止 chunk | 保持不变 | 无 | 无 |
| 任意状态 | `[DONE]` | 保持不变 | 无 | 非 `terminal` 时为 `PROTOCOL_UNEXPECTED_EOF` |
| 任意状态 | 网络 EOF | 保持不变 | 无 | 非 `terminal` 时为 `PROTOCOL_UNEXPECTED_EOF` |

说明：

- `start` 只能出现一次。
- `text-delta` 和所有控制 chunk 只能在 `streaming` 状态出现。
- `finish` 和 `error` 都是终止事件，一次流只能成功产生其中一个。
- `warning` 等未来非终止事件必须先加入 Schema 和状态转换矩阵，不能直接穿透为领域事件。
- 未知 `type` 可以忽略；已知 `type` 的结构错误不能忽略。

## 8. 内部接口

接口用于固定模块边界，具体命名可在实施计划中微调，但职责不能合并回调用方。

```ts
type ParsedAgentStreamPart =
  | { kind: "known"; part: AgentStreamPart }
  | { kind: "unknown" };

type AgentStreamTransition = {
  state: AgentStreamState;
  events: AgentStreamEvent[];
};

function parseAgentStreamPart(value: unknown): ParsedAgentStreamPart;

function reduceAgentStream(
  state: AgentStreamState,
  part: AgentStreamPart,
): AgentStreamTransition;

function assertAgentStreamComplete(state: AgentStreamState): void;
```

约束：

- `parseAgentStreamPart` 不访问网络和 Store。
- `reduceAgentStream` 不访问网络、流或 Store；协议违规通过稳定错误边界返回或抛出。
- `assertAgentStreamComplete` 在 EOF 和 `[DONE]` 时调用。
- 所有协议错误通过现有 `agentRequestError` 边界转换为稳定 `AgentError`。

## 9. 错误处理

| 场景 | 错误码 | 可重试 |
| --- | --- | --- |
| SSE data 不是合法 JSON | `PROTOCOL_INVALID_METADATA` | 否 |
| 已知 chunk 字段结构错误 | `PROTOCOL_INVALID_METADATA` | 否 |
| `start` 缺失或重复 | `PROTOCOL_INVALID_METADATA` | 否 |
| 文本事件出现在 `start` 前 | `PROTOCOL_INVALID_METADATA` | 否 |
| 终止事件重复或位于错误阶段 | `PROTOCOL_INVALID_METADATA` | 否 |
| EOF 或 `[DONE]` 到达时没有终止事件 | `PROTOCOL_UNEXPECTED_EOF` | 是 |
| 错误 chunk 没有 `agentError` | `AGENT_PROVIDER_FAILED` 回退值 | 是 |

未知非终止 chunk 保持兼容，不产生错误，也不能影响当前状态。

## 10. 数据与状态所有权

- 状态机只保存当前协议阶段。
- `AgentStreamEvent` 是协议层输出，不是业务 Store。
- `apiAgentModel` 负责把 `text-delta` 转为文本流。
- `error` 在 Model 边界转为 `Result<T>` 的错误值。
- `finish` 只表示流完整结束，不直接修改 Zustand。
- Zustand 继续保存会话、消息、片段和发送状态。

## 11. TDD 与测试设计

全部测试位于 `tests/e2e/unit/agent/`，遵守项目统一测试目录规则。

测试顺序：

1. 先补状态矩阵测试，并观察失败。
2. 再提取 Reducer 和完成性检查。
3. 最后删除 `apiAgentModel` 中的 `finished` 标记。
4. 每次重构保持现有测试通过。

必须覆盖：

- `start` 前出现 `text-delta`。
- `start` 重复。
- `start` 元数据缺失。
- `text-delta` 正常归属。
- `finish` 正常终止。
- `finish` 元数据缺失。
- `error` 正常映射。
- `error` 后继续出现已知协议 chunk。
- 未知非终止 chunk 被忽略。
- 已知畸形 chunk 被拒绝。
- JSON 畸形被拒绝。
- `[DONE]` 前没有终止事件。
- EOF 前没有终止事件。
- `[DONE]` 前已有终止事件时正常结束。

纯 Reducer 测试与 SSE Adapter 测试并存：

- Reducer 测试验证状态转换。
- Adapter 测试验证 `eventsource-parser` 输入到 `AgentStreamEvent` 的真实链路。
- `apiAgentModel` 测试只验证领域映射和错误值，不重复验证状态机全集。

## 12. 兼容性与迁移

- `readAgentStream` 的输入输出保持不变。
- `AgentStreamEvent` 的当前三种类型保持不变。
- `agentTransport` 和 Go 服务端不需要修改。
- `apiAgentModel` 删除 `finished` 标记后，异常结束仍由 `readAgentStream` 抛出 `PROTOCOL_UNEXPECTED_EOF`。
- 本次不新增第三方依赖。
- 若状态机行为与现有 fixture 冲突，以 Agent 设计中的协议不变量为准，不允许通过放宽测试修复。

## 13. 实施顺序

1. 评审并批准本设计。
2. 使用 `writing-plans` 生成可执行实施计划。
3. 先写状态矩阵失败测试。
4. 提取内部 Schema 类型和纯 Reducer。
5. 把 EOF 完整性判断移入协议层。
6. 简化 `apiAgentModel`。
7. 运行前端测试、构建、Go 测试和协议 fixture。
8. 更新 Agent 设计的实施状态。

## 14. 验收标准

- 协议生命周期只有一个事实源。
- `agentStreamProtocol.ts` 的公开接口不变。
- `apiAgentModel.ts` 不再维护 `finished`。
- 所有已知 chunk 的 Schema 错误都进入稳定协议错误。
- 所有状态转换都有测试。
- EOF 和 `[DONE]` 的完整性规则在协议层验证。
- `pnpm test`、`pnpm build`、`pnpm test:go`、`go vet` 和 `gofmt -d` 通过。
- 文档、实施计划和测试能互相追溯。

## 15. SDD 门禁

本设计遵循以下顺序：

```text
Design Spec
  -> Self Review
  -> User Review
  -> Implementation Plan
  -> Failing Tests
  -> Minimal Implementation
  -> Verification
  -> Status Update
```

未通过用户评审前，不创建生产实现。实施中若发现设计不成立，先更新本设计或新增 ADR，再继续编码。

## 16. Implementation Record

- Schema Parser: `src/models/agentStreamSchema.ts`.
- Pure Reducer: `src/models/agentStreamReducer.ts`.
- SSE Adapter: `src/models/agentStreamProtocol.ts`.
- Domain adapter: `src/models/apiAgentModel.ts`.
- Protocol tests: `tests/e2e/unit/agent/agentStreamSchema.test.ts`, `agentStreamReducer.test.ts`, `agentStreamProtocol.test.ts`.
- Full verification passed with `pnpm test`, `pnpm build`, `pnpm test:go`, `go vet ./server/...`, `gofmt -d`, and `git diff --check`.
