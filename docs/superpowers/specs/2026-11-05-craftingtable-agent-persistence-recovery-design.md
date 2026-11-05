# CraftingTable Agent Persistence And Recovery Design

- **Status:** Implemented and verified on 2026-11-05
- **Date:** 2026-11-05
- **Scope:** Agent Session、Message、Part、Run 的 SQLite 持久化，以及浏览器断线和页面刷新后的流恢复
- **Depends on:** [工程化设计规范](./2026-11-05-craftingtable-engineering-design.md)、[Agent 设计](./2026-11-05-craftingtable-agent-design.md)、[Agent Stream State Machine](./2026-11-05-craftingtable-agent-stream-state-machine-design.md)
- **Decision Record:** [ADR-001 Agent Run Recovery](../../architecture/ADR-001-agent-run-recovery.md)

## 1. 决策摘要

采用方案 A：

```text
SQLite 保存会话事实
  + 单进程 Run Coordinator 持有模型执行
  + 内存事件日志支持游标回放
  + 前端 Zustand 继续作为业务状态唯一事实源
```

恢复边界：

- 页面刷新后恢复 Session、Message、Part、Run 和已提交的最终内容。
- 浏览器断线或关闭页面时，服务端继续执行当前 Run。
- 客户端携带 SSE 游标重连，服务端回放缺失增量并继续实时推送。
- 服务端进程重启时，数据库中的 `running` Run 统一收敛为 `interrupted`。
- 不支持在服务端进程重启后继续同一模型流，也不伪造已经丢失的 token。
- 当前单实例实现不引入 Redis；未来多实例部署时通过相同游标契约替换实时事件载体。

本设计把持久化事实与实时流分开：

- `agent_sessions`、`agent_messages`、`agent_message_parts`、`agent_runs`、`agent_steps` 和 `agent_run_events` 保存业务事实及生命周期事件。
- `text-delta` 不逐条写数据库。当前 Run 的完整实时事件序列保存在受控内存日志中，用于断线回放。
- Run 结束时，聚合后的最终文本和终态在同一个数据库事务中提交。

## 2. 当前问题

当前 `createApiAgentModel()` 复用 `createFakeAgentModel()` 的浏览器内存 `Map` 保存会话，而 Go 的 `/agent/stream` 是无状态转发接口。

因此存在以下问题：

1. 刷新 `/agent/:id` 后浏览器内存被清空，会话显示不存在。
2. 浏览器断开时 `r.Context()` 被取消，服务端模型请求随之终止。
3. 没有稳定游标，断线后无法知道漏掉了哪些增量。
4. 当前 Run 没有持久化状态，服务重启后无法识别遗留执行。
5. 发送请求没有持久化幂等，POST 响应丢失时可能重复创建消息。

## 3. 目标

- Agent 会话和历史消息在 SQLite 中持久化。
- 页面刷新后能够恢复完整会话和当前 Run 的可见内容。
- Run 的执行生命周期独立于任一 HTTP 连接。
- SSE 使用单调游标，客户端可以从中断位置继续。
- Run 终止、取消、失败和进程重启后的状态可区分。
- 发送和重试具备持久化幂等能力。
- 保持 API 语义可迁移到未来的 PostgreSQL 和 Redis。
- 保持前端 MVC 边界和 Zustand 单一业务状态源。

## 4. 非目标

- 不实现服务端进程重启后的模型流续传。
- 不实现多实例共享实时事件。
- 不实现 Artifact、图片、附件、工具日志和大对象存储。
- 不实现 Tool、Skill、MCP、多 Agent 或评测平台。
- 不改变 Agent 页面现有视觉设计。
- 不引入 Vercel AI SDK、LangChain 或其他完整 Agent 框架作为业务状态源。
- 不把 Zustand 中的派生 UI 状态写入数据库。

## 5. 方案对比

### 5.1 方案 A：持久化事实、内存流日志和游标回放

Session、Message、Part、Run 和生命周期事件进入 SQLite。模型执行由服务端 Run Coordinator 持有。当前 Run 的实时 UI chunks 写入内存事件日志，通过单调游标回放。

优点：

- 页面刷新和浏览器断线可恢复。
- token 不逐条写数据库，写入压力可控。
- 与当前单实例 SQLite 架构匹配。
- 游标和 Run 语义可以平滑迁移到 Redis Streams 或 PostgreSQL。
- 不改变最终文本的数据库事实来源。

代价：

- 服务端进程重启时只能恢复已经提交的最终文本，不能继续原模型流。
- 实时事件日志需要内存上限和慢消费者保护。

### 5.2 方案 B：每个 SSE chunk 都持久化

把 `text-delta` 也写入 `agent_run_events`，客户端直接按数据库游标回放。

优点：

- 服务端重启后能恢复到最后一个已提交 token。
- 实现概念直观。

代价：

- 长回答会产生大量小事务或批量写入。
- token 增量进入长期事件事实，偏离现有 RunEvent 白名单。
- 仍需解决模型流本身不可恢复的问题，不能真正续接 provider。

### 5.3 方案 C：直接引入 Redis Streams

Run 事件写入 Redis，多个服务实例订阅相同流。

优点：

- 为多实例和跨进程恢复预留能力。
- 天然支持消费组和游标。

代价：

- 当前单用户、单进程部署没有必要。
- 需要同时设计 Redis 持久化、过期、故障恢复和 SQLite 一致性。
- 违反当前阶段不提前引入基础设施的原则。

### 5.4 结论

采用方案 A。现在解决真实问题，同时保留未来替换事件载体的契约。

## 6. 总体架构

```text
React View
  -> AgentEffect
    -> AgentStore
    -> AgentModel
      -> Session HTTP Adapter
      -> Run Stream Adapter

Go Agent Access
  -> Agent Service
    -> Repository Port
      -> SQLite Repository
    -> Run Coordinator
      -> Event Journal
      -> Provider Port
        -> OpenAI-compatible Provider
```

职责边界：

| 组件 | 职责 | 不负责 |
| --- | --- | --- |
| React View | 展示 Store，触发 Effect | 网络、数据库、协议合并 |
| Zustand Store | 会话、消息、Part、liveRun 和 UI 状态 | 持久化、SSE 分帧 |
| AgentEffect | 用例编排、加载和恢复触发 | DOM、SQL、模型协议 |
| ApiAgentModel | HTTP 与流协议适配、增量快照 | 业务事实的长期存储 |
| Access Controller | HTTP 校验、JSON/SSE 编码 | 模型执行所有权 |
| Agent Service | Session/Message/Run 用例和事务边界 | HTTP、Provider SDK |
| Run Coordinator | 后台执行、取消、事件日志、终止收敛 | HTTP 连接生命周期 |
| Repository Port | 持久化契约 | SQLite 细节 |
| SQLite Repository | 事务、查询和迁移 | SSE、Provider |

## 7. 数据模型

新增迁移：

```text
server/internal/platform/store/migrations/002_agent.sql
```

### 7.1 `agent_sessions`

| 字段 | 类型 | 约束 |
| --- | --- | --- |
| `id` | TEXT | PRIMARY KEY |
| `title` | TEXT | NOT NULL |
| `client_request_id` | TEXT | NOT NULL |
| `activity_seq` | INTEGER | NOT NULL |
| `created_at` | TEXT | NOT NULL |
| `updated_at` | TEXT | NOT NULL |

约束：

- `client_request_id` 唯一。
- 会话列表按 `activity_seq DESC`，其次按 `updated_at DESC`。

### 7.2 `agent_messages`

| 字段 | 类型 | 约束 |
| --- | --- | --- |
| `id` | TEXT | PRIMARY KEY |
| `session_id` | TEXT | NOT NULL, FOREIGN KEY |
| `seq` | INTEGER | NOT NULL |
| `role` | TEXT | `user` 或 `assistant` |
| `status` | TEXT | 消息状态 |
| `active_run_id` | TEXT | NULL, 当前展示 Run |
| `user_message_id` | TEXT | NULL, 助手消息所属用户消息 |
| `created_at` | TEXT | NOT NULL |
| `updated_at` | TEXT | NOT NULL |

约束：

- `UNIQUE(session_id, seq)`。
- 用户消息的 `active_run_id` 必须为空。
- 助手消息的 `active_run_id` 必须指向同一助手消息的 Run。
- 同一助手消息最多有一个 `running` Run。

### 7.3 `agent_message_parts`

| 字段 | 类型 | 约束 |
| --- | --- | --- |
| `id` | TEXT | PRIMARY KEY |
| `message_id` | TEXT | NOT NULL, FOREIGN KEY |
| `run_id` | TEXT | NULL, FOREIGN KEY |
| `part_index` | INTEGER | NOT NULL |
| `type` | TEXT | 当前为 `text` 或 `error` |
| `status` | TEXT | Part 状态 |
| `text_content` | TEXT | 文本正文 |
| `error_json` | TEXT | NULL, 结构化错误 |
| `created_at` | TEXT | NOT NULL |
| `updated_at` | TEXT | NOT NULL |

约束：

- 用户 Part 的 `run_id` 为空。
- 助手 Part 的 `run_id` 必填。
- 同一 message/run/index 组合唯一。

### 7.4 `agent_runs`

| 字段 | 类型 | 约束 |
| --- | --- | --- |
| `id` | TEXT | PRIMARY KEY |
| `session_id` | TEXT | NOT NULL, FOREIGN KEY |
| `user_message_id` | TEXT | NOT NULL, FOREIGN KEY |
| `assistant_message_id` | TEXT | NOT NULL, FOREIGN KEY |
| `status` | TEXT | Run 状态 |
| `client_request_id` | TEXT | NOT NULL |
| `error_json` | TEXT | NULL |
| `next_event_sequence` | INTEGER | NOT NULL, 初始为 1 |
| `started_at` | TEXT | NOT NULL |
| `finished_at` | TEXT | NULL |
| `created_at` | TEXT | NOT NULL |
| `updated_at` | TEXT | NOT NULL |

约束：

- 两个消息 ID 必须属于同一 Session，角色分别为 `user` 和 `assistant`。
- 同一 Session 最多一个 `running` Run。
- `client_request_id` 受幂等表保护。
- `next_event_sequence` 只用于 durable RunEvent 分配序号，在同一事务中原子递增。

### 7.5 `agent_steps`

| 字段 | 类型 | 约束 |
| --- | --- | --- |
| `id` | TEXT | PRIMARY KEY |
| `run_id` | TEXT | NOT NULL, FOREIGN KEY |
| `step_index` | INTEGER | NOT NULL |
| `status` | TEXT | Step 状态 |
| `created_at` | TEXT | NOT NULL |
| `finished_at` | TEXT | NULL |

当前单 Agent 只有 Step 0，但保留独立状态，避免后续工具调用把 Step 状态塞入 Run 或 Message。

约束：

- `UNIQUE(run_id, step_index)`。

### 7.6 `agent_run_events`

| 字段 | 类型 | 约束 |
| --- | --- | --- |
| `id` | TEXT | PRIMARY KEY |
| `run_id` | TEXT | NOT NULL, FOREIGN KEY |
| `sequence` | INTEGER | NOT NULL |
| `event_type` | TEXT | 生命周期事件类型 |
| `payload_json` | TEXT | NOT NULL |
| `created_at` | TEXT | NOT NULL |

约束：

- `UNIQUE(run_id, sequence)`。
- 持久化白名单为 `run.started`、`step.started`、`message.part.started`、`message.part.finished`、`step.finished`、`run.completed`、`run.failed`、`run.cancelled`、`run.interrupted`。
- `text-delta` 不进入该表。

### 7.7 `agent_idempotency_keys`

| 字段 | 类型 | 约束 |
| --- | --- | --- |
| `operation` | TEXT | `session.create`、`message.send`、`message.retry` |
| `client_request_id` | TEXT | NOT NULL |
| `request_hash` | TEXT | NOT NULL |
| `session_id` | TEXT | NULL |
| `run_id` | TEXT | NULL |
| `created_at` | TEXT | NOT NULL |

约束：

- `PRIMARY KEY(operation, client_request_id)`。
- 相同 key 和相同 hash 且 Run 仍在执行时返回 `409 AGENT_SESSION_BUSY`，并携带原 `runId` 与当前状态。
- 相同 key 和相同 hash 且 Run 已终态时返回 `409 AGENT_REQUEST_ALREADY_PROCESSED`，并携带原 `runId` 与最终状态。
- 相同 key 和不同 hash 返回 `409 AGENT_REQUEST_ID_CONFLICT`。

## 8. Run 生命周期

```text
running
  -> completed
  -> failed
  -> cancelled
  -> interrupted
```

状态规则：

- 创建事务直接写入 `running`。事务提交后，Coordinator 必须立即接管该 Run。
- 如果提交后 Coordinator 无法启动，服务端必须把该 Run 明确收敛为 `failed`。
- `completed` 表示最终文本和所有终态已提交。
- `failed` 保存稳定错误和已经生成的部分文本。
- `cancelled` 保存用户取消前的部分文本。
- `interrupted` 表示执行进程已经丢失。
- 启动恢复只把 `running` 更新为 `interrupted`，不处理正常终态。

## 9. 执行与事务边界

### 9.1 创建发送

发送事务按以下顺序执行：

1. 校验 Session、消息角色、Part 和请求标识。
2. 读取或创建幂等键。
3. 插入用户消息和用户文本 Part。
4. 插入助手空消息。
5. 插入 `running` Run。
6. 插入 Step 0。
7. 插入助手空文本 Part。
8. 更新助手消息 `active_run_id`。
9. 提交事务。

提交后，Agent Service 将 Run 交给 Run Coordinator。HTTP 发送接口返回 RunHandle，不等待模型完成。首个用户消息同时更新 Session 标题和活动时间。

### 9.2 运行

Run Coordinator 使用独立 Run Context，不继承 HTTP 请求 Context。

执行过程：

1. 注册 Run 所有权并确认当前 Session 没有其他执行者。
2. 记录 `run.started`、`step.started` 和 `message.part.started`。
3. 调用 Provider Stream。
4. 每个 token 追加到内存文本缓冲。
5. 每个 UI chunk 分配单调递增的流游标并写入实时事件日志。
6. 通知当前订阅者。慢订阅者被断开，不能阻塞 Provider。

### 9.3 终止事务

正常完成、失败或取消时：

1. 在一个并发安全的终态裁决点确定 completed、failed 或 cancelled；终态一旦被声明，取消请求等待最终结果，不再覆盖已提交方向。
2. 离开 HTTP/Pipe 的短生命周期，创建有限时长的持久化 Context。
3. 在一个事务中更新 Part 最终文本和状态。
4. 更新消息状态和 Run 终态。
5. 完成 Step。
6. 写入对应终止 RunEvent 和下一个生命周期序号。
7. 更新 Session 活动时间。
8. 提交事务。
9. 向事件日志发布终止 chunk。

如果终止事务失败：

- 不向浏览器发送虚假的成功终止事件。
- 使用脱离请求 Context 的有限时长 Context 和有限重试。
- 首次发送、重试在 Coordinator 启动失败时也使用同一持久化策略；Coordinator 尚未注册不能成为遗留 `running` 的理由。
- 终态写入重试耗尽后执行 `InterruptAgentRun`，把 Run、Step、Message 和 Part 收敛为 `interrupted`。
- 如果 Finalize 和 Interrupt 都失败，记录内部错误，并在临时 Journal TTL 到期后释放内存所有权；客户端重新读取 Session 详情，进程重启时由启动恢复继续收敛。

## 10. 实时事件日志与游标

每个 Run 有独立的内存事件日志：

```ts
type RunStreamEvent = {
  sequence: number
  part: AgentStreamPart
}
```

规则：

- 游标从 1 开始，同一 Run 内严格递增。
- SSE 使用 `id: <sequence>`。
- `start`、`start-step`、`text-start`、`text-delta`、`text-end`、`finish-step`、`finish`、`error` 和 `[DONE]` 均属于实时流。
- 生命周期 RunEvent 的数据库序号与实时 UI chunk 游标是两个不同概念，不共享编号。
- 客户端以收到的 SSE `id` 作为恢复游标。
- 断线重连时，服务端总是先发送可重建协议状态的 `start`。
- `start` 使用原游标，客户端游标只允许取最大值，不能后退。
- `after` 小于可回放窗口起点时，客户端必须重新获取 Session 详情，再从详情中的当前 Part 快照继续。
- 实时日志在 Run 终止后保留 5 分钟，支持终止边界重连；过期后以数据库终态为准。
- 单个 Run 的实时日志设置 8 MiB 上限。超过上限时永久关闭 Journal，现有订阅结束，后续订阅返回 `AGENT_STREAM_CURSOR_EXPIRED`；Run 仍完成持久化收敛，客户端通过 Session 详情对账。
- 活跃 SSE 每 15 秒写入一次 `: keep-alive` 注释并立即 flush，避免中间代理把无 token 的连接视为空闲。

内存日志是实时恢复缓存，不是长期事实来源。Session 详情始终以数据库记录为基础，并在 Run 仍活跃时合并 Coordinator 的当前文本和游标。

## 11. HTTP API

所有 JSON 成功响应继续使用 `httpapi.OK` envelope，失败使用 `httpapi.Fail`。

### 11.1 创建 Session

```text
POST /api/v1/agent/session/create
```

请求：

```json
{
  "clientRequestId": "uuid"
}
```

响应：

```json
{
  "session": {
    "id": "uuid",
    "title": "新会话",
    "createdAt": "ISO-8601",
    "updatedAt": "ISO-8601"
  }
}
```

### 11.2 会话列表

```text
GET /api/v1/agent/session/list
```

返回 `AgentSession[]`，按最近活动时间倒序。

### 11.3 会话详情

```text
GET /api/v1/agent/session/get?sessionId=<uuid>
```

返回：

```json
{
  "session": {},
  "messages": [],
  "messageParts": [],
  "activeRun": {
    "runId": "uuid",
    "status": "running",
    "lastSequence": 42
  },
  "nextMessageCursor": null
}
```

当 Run 活跃时：

- 助手 Part 返回 Coordinator 中的当前聚合文本。
- `lastSequence` 返回可订阅的当前位置。
- 当 Run 已终止时，只返回数据库终态。

### 11.4 发送消息

```text
POST /api/v1/agent/message/send
```

请求：

```json
{
  "sessionId": "uuid",
  "clientRequestId": "uuid",
  "message": {
    "id": "uuid",
    "role": "user",
    "parts": [
      {
        "type": "text",
        "text": "你好"
      }
    ]
  }
}
```

响应：

```json
{
  "runId": "uuid",
  "sessionId": "uuid",
  "userMessageId": "uuid",
  "assistantMessageId": "uuid",
  "assistantPartId": "uuid",
  "status": "running"
}
```

相同 `clientRequestId` 和相同请求摘要不能重新启动原 Run：

- Run 正在执行时返回 `409 AGENT_SESSION_BUSY`，`details` 包含原 `runId` 和 `running`。
- Run 已终态时返回 `409 AGENT_REQUEST_ALREADY_PROCESSED`，`details` 包含原 `runId` 和最终状态。
- 相同标识对应不同请求摘要时返回 `409 AGENT_REQUEST_ID_CONFLICT`，`details` 包含 `clientRequestId`。

### 11.5 重试

```text
POST /api/v1/agent/message/retry
```

请求：

```json
{
  "sessionId": "uuid",
  "assistantMessageId": "uuid",
  "clientRequestId": "uuid"
}
```

重试保留旧 Run 和旧 Part，创建新 Run 与新 Part，并把助手消息的 `activeRunId` 指向新 Run。

### 11.6 订阅 Run

```text
GET /api/v1/agent/run/stream?runId=<uuid>&after=<sequence>
```

响应：

```text
Content-Type: text/event-stream
x-vercel-ai-ui-message-stream: v1

id: 1
data: {"type":"start",...}

id: 18
data: {"type":"text-delta",...}

data: [DONE]
```

接口要求：

- HTTP 连接只负责订阅，不拥有 Run 生命周期。
- 连接断开只移除订阅者。
- 实时日志仍在保留期时，服务端可以返回完整终止序列。
- 实时日志已过期且 Run 已终态时，返回 `409 AGENT_STREAM_CURSOR_EXPIRED`。客户端随后调用 Session 详情读取最终事实，不在 SSE 中伪造历史终止事件。
- 未知 Run 返回 `AGENT_RUN_NOT_FOUND`。

### 11.7 取消 Run

```text
POST /api/v1/agent/run/cancel
```

请求：

```json
{
  "sessionId": "uuid",
  "runId": "uuid"
}
```

取消请求成功后，Coordinator 取消 Provider Context，并执行取消终止事务。

## 12. 前端契约变更

### 12.1 `AgentModel`

新增恢复方法：

```ts
resumeRun(
  input: {
    sessionId: string
    runId: string
    afterSequence: number
    signal: AbortSignal
  },
  handlers: AgentSendHandlers,
): Promise<Result<AgentSendResult>>
```

`sendMessage()` 和 `retryMessage()` 不再直接 POST 到旧 `/agent/stream`：

1. 调用 JSON 接口创建 Run。
2. 收到 RunHandle。
3. 调用 `/agent/run/stream` 订阅。
4. 将协议事件物化为 AgentMessageSnapshot。
5. 返回最终 Run 状态。

POST 响应丢失时，前端重新读取 Session 详情，并且只恢复与当前请求绑定的 Run。发送请求按用户消息 ID 绑定；重试请求同时校验新 `activeRunId` 不等于重试前的 Run，避免旧页面恢复成后续尝试。

### 12.2 页面刷新

`loadSession()` 的恢复流程：

```text
GET session/get
  -> 替换当前 Store 历史
  -> activeRun.status === running
    -> setLiveRun
    -> setSendPhase(streaming)
    -> model.resumeRun(afterSequence)
      -> 后续文本更新 Store
```

### 12.3 断线重连

ApiAgentModel 持有当前 Run 的 `lastSequence`：

- SSE 帧成功后更新游标。
- 可恢复网络错误、EOF 和非终态断流触发重连。
- 使用 `after=<lastSequence>`。
- 指数退避，次数和总时长有上限。
- 重连期间 Store 保留已有内容，不显示临时错误横幅。
- 超过重试上限或 Event Journal 已关闭时，重新读取 Session 详情；持久化终态存在则完成对账，不存在则返回 `AGENT_RUN_INTERRUPTED`，不能停留在只有心跳的无限订阅中。
- 旧页面的 `runId` 已被同一助手消息的新 Run 取代时，不读取新 Run 的文本或状态，也不订阅旧 Run。
- Effect 对同一 Run 只允许一个活动订阅，避免页面刷新、Sidebar 和 Route 重新渲染触发重复流。

### 12.4 停止

Composer 的停止操作：

1. 调用 `/agent/run/cancel`。
2. 取消本地订阅。
3. 等待 Session 详情或取消流写入终态。

本地 AbortSignal 只取消 HTTP 订阅，不能单独作为服务端 Run 取消。

## 13. 幂等与并发

- `session.create`、`message.send` 和 `message.retry` 都使用 `(operation, client_request_id)`。
- 请求摘要由服务端规范化 JSON 后计算 SHA-256。
- Session 创建在相同 key 和 hash 下返回原 Session。
- 发送和重试在相同 key 和 hash 下不重放执行：运行中返回 `AGENT_SESSION_BUSY`，终态返回 `AGENT_REQUEST_ALREADY_PROCESSED`。
- 相同 key 和不同 hash 返回 `409 AGENT_REQUEST_ID_CONFLICT`。
- 发送前事务检查 Session 是否已有 `running` Run。
- SQLite 部分唯一索引提供最后约束。
- Coordinator 注册 Run 时使用内存锁再次防止同 Session 双执行。

## 14. 错误模型

| 场景 | Code | HTTP | Retryable |
| --- | --- | --- | --- |
| Session 不存在 | `AGENT_SESSION_NOT_FOUND` | 404 | false |
| Run 不存在 | `AGENT_RUN_NOT_FOUND` | 404 | false |
| Session 已有运行中 Run | `AGENT_SESSION_BUSY` | 409 | false |
| 幂等请求仍在执行 | `AGENT_SESSION_BUSY` | 409 | false |
| 幂等请求已经完成 | `AGENT_REQUEST_ALREADY_PROCESSED` | 409 | false |
| 幂等键内容冲突 | `AGENT_REQUEST_ID_CONFLICT` | 409 | false |
| 消息不可重试 | `AGENT_MESSAGE_NOT_RETRYABLE` | 409 | false |
| 可回放窗口丢失 | `AGENT_STREAM_CURSOR_EXPIRED` | 409 | true |
| 数据库写入失败 | `AGENT_PERSISTENCE_FAILED` | 500 | true |
| Provider 请求失败 | `AGENT_PROVIDER_FAILED` | 模型流 error | true |
| 协议无效 | `PROTOCOL_INVALID_METADATA` | 模型流 error | false |
| 协议意外结束 | `PROTOCOL_UNEXPECTED_EOF` | 模型流 error | true |

数据库错误不直接暴露 SQL、文件路径或底层驱动信息。

## 15. 安全与资源限制

沿用现有限制：

- 模型上下文最多保留最近 20 个完整轮次、40 条消息。
- 上下文正文总量最多 20,000 个 Unicode 字符，超限时从最旧完整轮次开始截断，并始终保留当前用户消息。
- 单条消息最多 100,000 Unicode 字符。
- 标识符最多 200 字符。

新增限制：

- 一个 Session 同时最多一个 running Run。
- 单个 Run 最长执行 120 秒；Provider 连续 30 秒没有可观察输出时判定空闲超时。
- 实时事件日志每 Run 最多 8 MiB。
- Event Journal 超过上限后永久关闭，新订阅返回 `AGENT_STREAM_CURSOR_EXPIRED`，不能退化为只发送心跳。
- SSE 每 15 秒发送一次 `: keep-alive` 注释心跳，心跳不进入领域事件或 Zustand。
- Session 详情首期一次返回全部消息；分页契约保留 `nextMessageCursor`，当前为 `null`。
- 慢 SSE 订阅者使用有界发送队列，队列满时断开并由客户端重连。
- Session 详情只加载助手消息当前 `active_run_id` 对应的 Part；旧重试历史保留但不混入当前气泡。
- API Key 只存在于服务端，不进入请求、日志或数据库。
- 数据库保存用户文本，不接受客户端提交 SQL、表名或任意错误 JSON。

## 16. 代码修改清单

### 16.1 Go 服务端新增

| 文件 | 内容 |
| --- | --- |
| `server/internal/agent/domain/session.go` | Session、Message、Part 的领域类型和状态常量 |
| `server/internal/agent/domain/run.go` | Run、RunHandle、RunEvent 和状态常量 |
| `server/internal/agent/core/repository.go` | Repository Port 和事务输入输出 |
| `server/internal/agent/core/service.go` | Session CRUD、发送、重试、取消和详情组合 |
| `server/internal/agent/core/coordinator.go` | 后台 Run 执行、终止事务和启动恢复 |
| `server/internal/agent/core/event_journal.go` | 有界实时事件日志、订阅者和游标回放 |
| `server/internal/agent/core/runtime_config.go` | Run 总时长、空闲超时、Finalize 重试和 Journal TTL 默认值 |
| `server/internal/agent/core/run_persistence.go` | 脱离请求 Context 的 Finalize / Interrupt 有限重试 |
| `server/internal/agent/access/session_controller.go` | Session 与消息 JSON 路由 |
| `server/internal/agent/access/run_controller.go` | Run 订阅和取消路由 |
| `server/internal/platform/store/agent_store.go` | SQLite Repository、事务和启动恢复查询 |
| `server/internal/platform/store/migrations/002_agent.sql` | Agent 表和索引 |

### 16.2 Go 服务端修改

| 文件 | 修改 |
| --- | --- |
| `server/internal/platform/store/sqlite_store.go` | 嵌入并按文件名顺序执行迁移；用 `schema_migrations` 保存文件名、SHA-256 和完成时间；暴露同一 `*sql.DB` 给 Agent Repository |
| `server/cmd/craftingtabled/main.go` | 组装 Agent Repository、Service、Coordinator 和 Controllers；启动恢复 |
| `server/internal/agent/access/stream_controller.go` | 删除无状态 Provider 转发职责，或拆分后仅保留 UI Message Stream 编码器 |
| `server/internal/agent/provider/openai_compatible.go` | 保持 Provider Port；确认 Context 由 Run Coordinator 传入 |
| `server/internal/agent/core/stream_parser.go` | 保持 Provider SSE 解析，不拥有 Session 或 HTTP 生命周期 |
| `go.mod`、`go.sum` | 仅在实现确需新增依赖时变化；当前设计不需要新依赖 |

### 16.3 前端新增

| 文件 | 内容 |
| --- | --- |
| `src/models/agentRunClient.ts` | 创建 Run、订阅、游标重连和 RunHandle 校验 |
| `src/models/agentRunMaterializer.ts` | 将协议事件转换为 Message/Part Snapshot，避免 ApiAgentModel 膨胀 |

### 16.4 前端修改

| 文件 | 修改 |
| --- | --- |
| `src/models/agentContracts.ts` | 增加 RunHandle、`activeRun.lastSequence` 和恢复输入 |
| `src/models/agentModel.ts` | 增加 `resumeRun`；生产环境不再通过假模型保存 Session |
| `src/models/apiAgentModel.ts` | 改为真实 Session CRUD、创建 Run、订阅和恢复 |
| `src/models/agentTransport.ts` | 增加 Session API、发送、重试、取消和带 `after` 的流请求 |
| `src/models/agentStreamSchema.ts` | 将 SSE `id` 纳入已校验游标，保持现有 chunk Schema |
| `src/models/agentStreamProtocol.ts` | 读取并校验 SSE id，输出游标；不把游标写入 Zustand |
| `src/models/agentStreamReducer.ts` | 领域事件携带不可回退的游标 |
| `src/controllers/effects/useAgentEffect.ts` | 加载 activeRun 后调用 `resumeRun`；发送/重试走新模型流程 |
| `src/controllers/stores/agentStore.ts` | 仅在需要时增加 liveRun 游标或恢复动作；继续作为业务状态唯一事实源 |
| `src/models/fakeAgentModel.ts` | 保留为测试和 Fake Runtime；移除生产路径依赖 |
| `src/pages/agent/index.tsx` | 预期不改变视觉与主要结构，仅在 Effect 契约变化时少量调整 |

### 16.5 测试新增或修改

全部测试继续集中在 `tests/e2e`。

| 文件 | 覆盖 |
| --- | --- |
| `tests/e2e/integration/agent_persistence.integration_test.go` | Session/Message/Part 持久化、重启读取、遗留 Run 收敛 |
| `tests/e2e/integration/agent_recovery.integration_test.go` | 客户端断开后服务端继续、游标回放、取消 |
| `tests/e2e/integration/agent_idempotency_api.integration_test.go` | 请求 ID 冲突、运行中重放和终态重放 |
| `tests/e2e/integration/agent_stream_heartbeat.integration_test.go` | SSE 注释心跳与 flush |
| `tests/e2e/integration/migrations.integration_test.go` | 迁移账本、checksum 篡改和旧库升级 |
| `tests/e2e/unit/agent/apiAgentModel.test.ts` | API 编排、游标跟踪、断线重连 |
| `tests/e2e/unit/agent/agentRunClient.test.ts` | RunHandle 校验、退避、游标过期 |
| `tests/e2e/unit/agent/agentStreamProtocol.test.ts` | SSE id、重复回放、游标不可后退 |
| `tests/e2e/unit/agent/agentEffect.test.ts` | 刷新后恢复 activeRun |
| `tests/e2e/modules/agent/navigation.e2e.ts` | 刷新 URL 后历史和会话恢复 |
| `tests/e2e/modules/agent/chat.e2e.ts` | 正常发送、停止和重试仍可用 |

## 17. 测试矩阵

必须覆盖：

1. 创建 Session 后重启服务，Session 仍存在。
2. 用户消息和已完成助手消息重启后完整恢复。
3. 页面刷新后直接访问 `/agent/:id` 不再显示不存在。
4. Run 运行中刷新，Session 详情含当前文本和 activeRun。
5. 浏览器断开后 Provider Context 不被取消。
6. 客户端从游标 N 重连，只收到 N 之后的增量。
7. 重连时协议状态可由新的 `start` 重建。
8. 终止游标前断线，重连后能收敛到 completed。
9. 服务启动时遗留 running Run 变为 interrupted。
10. 相同发送请求重复提交只产生一条用户消息和一个 Run。
11. 相同 clientRequestId、不同请求内容返回冲突。
12. Session Busy 时不启动第二个 Run。
13. 取消持久化部分文本和 cancelled 状态。
14. Provider 失败保留部分文本和结构化错误。
15. 慢订阅者不会阻塞 Run Coordinator。
16. 游标过期返回稳定错误并可通过 Session 详情恢复。
17. Zustand 不直接持有 SSE chunk 或数据库对象。
18. UI 在恢复过程中不出现重复消息或重复 token。
19. Session 详情在同一只读事务中读取，不能混合 running 与 terminal 状态。
20. 旧 Run 的恢复不能读取同一助手消息的新 Run 内容。
21. Durable RunEvent 序号不能作为 SSE 恢复游标。
22. 完成、取消和超时在最终提交前经过同一次终态裁决。
23. Coordinator 启动失败时，Finalize 使用脱离请求的 Context 和有限重试。

## 18. 迁移和兼容

- 现有 `items` 与 `categories` 表不修改。
- 迁移按文件名顺序执行，并创建 `schema_migrations(name, checksum, applied_at)` 账本。
- 已应用迁移只比较 SHA-256，不重复执行；checksum 不一致时拒绝启动，防止已有数据库悄悄偏离代码。
- 尚无账本的旧数据库执行带 `IF NOT EXISTS` / `INSERT OR IGNORE` 的迁移后写入账本，不丢失已有 Today 数据。
- 旧浏览器内存 Session 不迁移；当前尚未进入生产，无法保证其数据存在。
- 现有 `/api/v1/agent/stream` 不再作为前端主路径。
- 为降低一次变更范围，可在迁移期临时保留旧路由，但它不得继续被视为持久化接口。
- API 错误继续使用现有 `httpapi.ErrorBody`。
- 前端 `AgentStreamPart` 的外部 wire shape 不改变，只增加 SSE `id`。

## 19. 实施顺序

1. 评审本设计并批准 ADR。
2. 使用 `writing-plans` 生成可执行计划。
3. 先写 SQLite Repository 和恢复集成测试，观察失败。
4. 实现迁移与 Repository。
5. 写 Session/Message 用例失败测试并实现 Service。
6. 写 Run Coordinator 和事件日志失败测试并实现。
7. 写 HTTP 契约失败测试并实现 Controllers。
8. 更新前端协议游标测试。
9. 实现 ApiAgentModel 与恢复链路。
10. 运行 Go、Vitest、Playwright、Build 和真实页面刷新验证。
11. 更新本文状态、Agent 设计实施记录和 README 当前事实。

## 20. 验收标准

- 刷新 Agent 页面后可以读取所有已持久化历史。
- 生成中刷新后，会话显示当前部分内容，并继续接收后续内容。
- 浏览器断线不会终止服务端模型请求。
- 断线重连不重复用户消息、助手消息或已经合并的文本。
- 服务端重启后遗留 Run 显示为 interrupted。
- 同一幂等请求不会重复创建 Session、Message 或 Run。
- Provider Key 不出现在数据库、日志、浏览器请求或响应。
- 所有测试继续位于 `tests/e2e` 的对应模块。
- `pnpm typecheck`、`pnpm test`、`go test ./server/... ./tests/e2e/integration/... ./tests/e2e/unit/agent`、`go vet ./server/...`、`gofmt -d`、`pnpm test:e2e` 和 `pnpm build` 通过。

## 21. SDD 门禁

```text
Design Spec
  -> Self Review
  -> User Review
  -> Implementation Plan
  -> Failing Tests
  -> Minimal Implementation
  -> Integration Verification
  -> Status And README Update
```

本设计和 ADR 已通过实施计划、失败测试、最小实现和浏览器验收，当前状态为已实现。

## 22. 实施记录

实现日期：2026-11-05。

已落地的执行边界：

- SQLite 保存 Session、Message、Part、Run、Step、生命周期 RunEvent 和幂等键。
- 单进程 Run Coordinator 持有 Provider Context，HTTP 订阅断开不取消 Run。
- 内存 Event Journal 保存当前 Run 的 UI chunks，SSE `id` 提供单调游标。
- 页面刷新后读取 Session 详情，并从 `activeRun.lastSequence` 恢复订阅。
- Session 详情在一个只读事务内读取；`activeRun.lastSequence` 只由 Coordinator Journal 覆盖，不再复用 durable RunEvent 序号。
- 发送和重试的幂等重放不会重新启动原 Run；运行中与终态分别返回 `AGENT_SESSION_BUSY` 和 `AGENT_REQUEST_ALREADY_PROCESSED`。
- Run 最长 120 秒、空闲 30 秒；SSE 每 15 秒发送注释心跳。
- 终态提交使用独立 Context、3 次默认 Finalize 重试和 Interrupt 兜底；取消与完成通过同一裁决点决定。
- Event Journal 上限 8 MiB，溢出后永久关闭并通过 Session 详情恢复。
- SQLite 迁移使用文件名和 checksum 账本，旧数据库升级保留 Today 数据。
- 前端恢复强制校验 `runId`，旧页面不会读取同一助手消息后续尝试的内容。
- Go 进程重启时只把遗留 `running` Run 收敛为 `interrupted`，不续接原 Provider 流，也不伪造 token。

主要代码：

- `server/internal/platform/store/migrations/002_agent.sql`
- `server/internal/platform/store/agent_store.go`
- `server/internal/agent/core/coordinator.go`
- `server/internal/agent/core/event_journal.go`
- `server/internal/agent/core/runtime_config.go`
- `server/internal/agent/core/run_persistence.go`
- `server/internal/agent/access/session_controller.go`
- `server/internal/agent/access/run_controller.go`
- `src/models/agentTransport.ts`
- `src/models/agentRunClient.ts`
- `src/models/apiAgentModel.ts`
- `src/controllers/effects/useAgentEffect.ts`

回归测试：

- `tests/e2e/integration/agent_persistence.integration_test.go`
- `tests/e2e/integration/agent_recovery.integration_test.go`
- `tests/e2e/integration/agent_idempotency_api.integration_test.go`
- `tests/e2e/integration/agent_stream_heartbeat.integration_test.go`
- `tests/e2e/integration/migrations.integration_test.go`
- `tests/e2e/unit/agent/agentRunClient.test.ts`
- `tests/e2e/unit/agent/apiAgentModel.test.ts`
- `tests/e2e/unit/agent/agentEffect.test.ts`
- `tests/e2e/unit/agent/event_journal_test.go`
- `tests/e2e/modules/agent/navigation.e2e.ts`
- `tests/e2e/modules/agent/chat.e2e.ts`

验证命令：

```powershell
pnpm typecheck
pnpm test
go test ./server/... ./tests/e2e/integration/... ./tests/e2e/unit/agent
go vet ./server/...
pnpm build
pnpm test:e2e
git diff --check
```
