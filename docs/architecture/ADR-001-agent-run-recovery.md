# ADR-001: Agent Run Recovery

- **Status:** Accepted
- **Implementation:** Implemented and verified on 2026-11-05
- **Date:** 2026-11-05
- **Decision owner:** leeat
- **Design:** [CraftingTable Agent Persistence And Recovery Design](../superpowers/specs/2026-11-05-craftingtable-agent-persistence-recovery-design.md)

## Context

实施前，CraftingTable 把 Agent Session 和 Message 保存在浏览器内存，Go 服务只把模型流绑定到一个 HTTP 请求。这会导致刷新后会话丢失，也会在浏览器断线时取消模型请求。

目标架构要求：

- 会话事实可以恢复。
- Run 生命周期由服务端拥有。
- 浏览器断线可以继续执行和重连。
- 服务进程重启不伪造未持久化内容。
- 未来可以迁移到 PostgreSQL 和 Redis，而不改变前端业务契约。

## Decision

采用以下组合：

1. SQLite 持久化 Session、Message、Part、Run、Step 和生命周期 RunEvent。
2. Run Coordinator 使用独立 Context 执行模型请求。
3. `text-delta` 聚合在内存，并对当前 Run 写入有界实时事件日志。
4. SSE 使用单调 `id`，客户端通过 `after` 游标重连。
5. 正常结束、失败和取消都在同一个数据库事务中提交最终文本和终态。
6. 服务启动时把遗留 `running` Run 标记为 `interrupted`。
7. Session 创建、消息发送和消息重试使用持久化幂等键。

## Consequences

正向：

- 页面刷新和网络短暂断开不再丢会话。
- 慢客户端或断开客户端不会阻塞 Provider。
- token 不逐条写数据库，最终文本仍只有一个持久化事实来源。
- 游标和 Run 状态语义可以在未来切换到 Redis Streams。

代价：

- 服务端进程崩溃后不能继续同一个模型流。
- 实时事件日志需要内存上限、慢消费者保护和过期清理。
- 实现需要新增数据库事务、后台执行所有权和前端重连编排。

## Rejected Alternatives

### 每个 text delta 写入数据库

拒绝原因：

- 长回答产生过多小写入。
- 把临时 token 增量变成长期事实。
- 仍不能解决 Provider 流在进程重启后的续接问题。

### 直接使用 Redis Streams

拒绝原因：

- 当前只有单进程 SQLite，提前引入分布式基础设施。
- 需要先稳定游标、事件和 Run 语义，否则只是把问题移到 Redis。

### 只保存最终消息

拒绝原因：

- 无法恢复生成中的部分内容。
- 无法准确识别遗留 Run。
- 不符合完整恢复目标。

## Invariants

- HTTP 请求断开不能取消 Run。
- SSE 游标只在单个 Run 内有意义。
- 客户端游标不能后退。
- token 增量不是长期数据库事实。
- `running` Run 只能有一个事件 writer。
- 同一 Session 不能同时运行多个 Run。
- 终止数据库事务成功后，才能发布成功终止流事件。
- 进程重启只允许把遗留 Run 收敛为 `interrupted`。
- API Key 不得进入数据库、日志或客户端。

## Implementation Gate

实现已确认：

- 现有 Agent 页面交互和错误展示保持兼容。
- 新接口继续使用统一 JSON envelope 和稳定错误码。
- 迁移按文件名顺序执行，并可在已有 SQLite 文件上重复运行。
- 现有 Today 表和数据不受影响。
- 全部测试继续位于 `tests/e2e`。

## Implementation Record

2026-11-05 已完成单进程 SQLite 持久化和游标恢复：

- Session、Message、Part、Run、Step、RunEvent 和幂等键使用 SQLite 持久化。
- Run Coordinator 使用独立 Context，浏览器刷新或 HTTP 断线不会取消当前 Run。
- 前端从 Session 详情中的 `activeRun.lastSequence` 恢复 SSE 订阅。
- Go 进程重启会把遗留 `running` Run 收敛为 `interrupted`，不恢复原 Provider 流。
- 测试证据位于 `tests/e2e/integration`、`tests/e2e/unit/agent` 和 `tests/e2e/modules/agent`。

## Follow-Up Decisions

以下事项不在本 ADR 内决定：

- Web 多实例的事件转发与租约实现。
- Redis Streams、队列或 Pub/Sub 的具体职责。
- Artifact 大对象存储。
- Tool、Skill、MCP 和多 Agent 事件。
- 服务端进程重启后重新发起模型的策略。
