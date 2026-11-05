# CraftingTable Agent 持久化与恢复交接文档

日期：2026-11-05
分支：`leeatMvp`
实现提交：`299ff85 feat(agent): finalize persistent run recovery`
上一基线：`d425c70 docs: record agent persistence implementation`
状态：已完成实现、验证和本地提交；未 push，未 merge

## 1. 交接结论

单 Agent 工作台的本地持久化、浏览器刷新恢复和单进程 Run 恢复已经完成。

当前可以做到：

- 创建 Session 后刷新页面，会话仍然存在。
- 用户消息和已完成的助手消息从 SQLite 恢复。
- 生成过程中刷新页面，恢复当前有效 Run，并继续接收剩余 SSE 增量。
- 浏览器短暂断开不会取消 Go 进程内的 Run。
- 完成、取消、失败和超时都收敛到明确终态。
- 幂等发送和重试不会重复创建消息或 Run。
- 旧 Run 恢复不会误读同一助手消息后续尝试的内容。
- Go 进程重启后，遗留 `running` Run 收敛为 `interrupted`。

当前不能做到：

- Go 进程重启后继续原 Provider 流。
- 多实例部署下的 Run 租约、事件转发和跨进程接管。
- Redis、工具、Skills、MCP、多 Agent 编排和评测平台。
- Electron 生产壳和移动端。

## 2. 关键架构事实

### 2.1 执行所有权

- Agent Run 由 Go 单进程 `RunCoordinator` 持有，执行 Context 与 HTTP 请求 Context 分离。
- 浏览器关闭 SSE、刷新页面或短暂断线只结束订阅，不取消 Run。
- Coordinator 在终态提交成功后才释放 Session 执行槽。
- 同一 Session 同时只允许一个运行中的 Run。

### 2.2 持久化

SQLite 保存：

- `agent_sessions`
- `agent_messages`
- `agent_message_parts`
- `agent_runs`
- `agent_steps`
- `agent_run_events`
- `agent_idempotency_keys`
- `schema_migrations`

数据库迁移按文件名顺序执行，并在 `schema_migrations` 中保存 SHA-256。已应用迁移内容变化时，服务拒绝启动。

### 2.3 两类序号不能混用

- durable `RunEvent.sequence` 是数据库生命周期事件序号。
- SSE `id` 来自进程内 Event Journal，是当前 Run 的 UI chunk 游标。
- `activeRun.lastSequence` 只使用 Event Journal 游标。
- Journal 不存在时 `lastSequence` 为 `0`，不回退到 durable 事件序号。

### 2.4 恢复边界

- Session 详情在一个只读事务中读取一致快照。
- 前端恢复必须校验请求的 `runId`。
- 发送响应丢失时只按用户消息 ID 恢复，不猜测其他活动 Run。
- 重试恢复携带 `previousRunId`，不恢复重试前的旧 Run。
- Event Journal 上限为 8 MiB；溢出后关闭，前端重新读取 Session 详情并按数据库终态对账。
- Go 进程重启不会伪造 token，也不会自动重启原模型请求。

### 2.5 终态裁决

- 完成、取消和超时竞争经过同一个终态裁决点。
- 只有一个路径可以提交最终状态。
- 已完成终态声明后，取消请求等待最终结果。
- Finalize 失败时回退为 `interrupted`。
- 终态持久化使用脱离 HTTP 请求的 Context、有限超时和有限重试。
- Coordinator 启动失败后的清理同样使用脱离请求的 Context。

## 3. 运行参数

默认值：

- Run 最长执行时间：120 秒。
- Provider 连续空闲超时：30 秒。
- SSE 注释心跳：15 秒。
- Event Journal 上限：8 MiB。
- Session 列表默认：30 条。
- Session 详情消息默认：40 条。
- Finalize 默认重试：3 次。

相关环境变量：

```text
API_PORT
CRAFTINGTABLE_DATABASE_PATH
CRAFTINGTABLE_AGENT_BASE_URL
CRAFTINGTABLE_AGENT_API_KEY
CRAFTINGTABLE_AGENT_MODEL
CRAFTINGTABLE_AGENT_SYSTEM_PROMPT
CRAFTINGTABLE_AGENT_STREAM_HEARTBEAT
```

密钥只允许存在于 Go 服务端环境或本地 `.env.local`。不得写入源码、SQLite、日志、浏览器 Store 或测试 fixture。

## 4. API 与幂等契约

Agent API 前缀：`/api/v1`。

主要入口：

```text
GET  /api/v1/agent/sessions
GET  /api/v1/agent/session
POST /api/v1/agent/session/create
POST /api/v1/agent/message/send
POST /api/v1/agent/message/retry
GET  /api/v1/agent/run/stream
POST /api/v1/agent/run/cancel
```

创建 Session、发送和重试使用持久化 `clientRequestId` 与服务端计算的 `requestHash`。

- 相同 ID、相同摘要且 Run 运行中：`409 AGENT_SESSION_BUSY`，返回原 `runId`。
- 相同 ID、相同摘要且 Run 已终态：`409 AGENT_REQUEST_ALREADY_PROCESSED`，返回原 `runId` 和状态。
- 相同 ID、不同摘要：`409 AGENT_REQUEST_ID_CONFLICT`。
- 幂等重放不会重新启动或终结原 Run。

前端业务状态仍由 Zustand 持有。SSE 事件只是协议输入，不形成第二份持久业务状态。

## 5. 前端恢复链路

```text
Session 详情
  -> activeRun
  -> 校验 requestedRunId
  -> 从 lastSequence 订阅 run/stream
  -> eventsource-parser 清理 SSE
  -> agentStreamProtocol 映射事件
  -> apiAgentModel 合并消息与 Part
  -> Zustand Store
  -> Agent View
```

前端不依赖 Vercel AI SDK 的 `useChat` 或第三方 Chat Store。项目只吸收其传输分层思路，并由本项目维护 Transport、协议 Adapter 和领域映射。

## 6. 关键文件

服务端：

- `server/cmd/craftingtabled/main.go`
- `server/internal/agent/core/coordinator.go`
- `server/internal/agent/core/service.go`
- `server/internal/agent/core/event_journal.go`
- `server/internal/agent/core/run_persistence.go`
- `server/internal/agent/core/runtime_config.go`
- `server/internal/agent/access/session_controller.go`
- `server/internal/agent/access/run_controller.go`
- `server/internal/agent/access/ui_stream.go`
- `server/internal/platform/store/agent_store.go`
- `server/internal/platform/store/sqlite_store.go`

前端：

- `src/models/agentRunClient.ts`
- `src/models/apiAgentModel.ts`
- `src/models/agentContracts.ts`
- `src/controllers/effects/useAgentEffect.ts`
- `src/models/agentTransport.ts`
- `src/models/agentStreamProtocol.ts`
- `src/models/agentStreamReducer.ts`

文档：

- `docs/superpowers/specs/2026-11-05-craftingtable-agent-persistence-recovery-design.md`
- `docs/superpowers/specs/2026-11-05-craftingtable-agent-design.md`
- `docs/superpowers/specs/2026-11-05-craftingtable-engineering-design.md`
- `docs/architecture/ADR-001-agent-run-recovery.md`
- `docs/superpowers/plans/2026-11-05-craftingtable-agent-persistence-recovery.md`

## 7. 测试位置

所有项目测试继续位于 `tests/e2e`。

服务端集成：

- `tests/e2e/integration/agent_persistence.integration_test.go`
- `tests/e2e/integration/agent_recovery.integration_test.go`
- `tests/e2e/integration/agent_idempotency_api.integration_test.go`
- `tests/e2e/integration/agent_stream_heartbeat.integration_test.go`
- `tests/e2e/integration/migrations.integration_test.go`

前端与协议：

- `tests/e2e/unit/agent/agentRunClient.test.ts`
- `tests/e2e/unit/agent/apiAgentModel.test.ts`
- `tests/e2e/unit/agent/agentEffect.test.ts`
- `tests/e2e/unit/agent/event_journal_test.go`
- `tests/e2e/unit/agent/agentStreamProtocol.test.ts`
- `tests/e2e/unit/agent/agentStreamReducer.test.ts`

浏览器：

- `tests/e2e/modules/agent/chat.e2e.ts`
- `tests/e2e/modules/agent/navigation.e2e.ts`
- `tests/e2e/modules/agent/responsive.e2e.ts`

## 8. 验证结果

2026-11-05 已验证：

```powershell
pnpm typecheck
pnpm test
go test ./server/... ./tests/e2e/integration/... ./tests/e2e/unit/agent -count=1
go test -race ./server/... ./tests/e2e/integration/... ./tests/e2e/unit/agent -count=1
go vet ./server/...
gofmt -d
pnpm build
pnpm test:e2e
git diff --check
```

结果：

- Vitest：17 个测试文件，80 个测试通过。
- Playwright：桌面和移动项目共 28 个测试通过。
- Go 全量测试通过。
- Go race detector 通过。
- `go vet`、`gofmt -d`、`git diff --check` 通过。
- 生产构建通过。
- 人工浏览器验证了创建会话、流式回复、生成中刷新恢复、完成后 URL 刷新恢复，以及页面无真实 console error。
- 停止、取消、重试和第二次发送禁用由 Playwright E2E 覆盖。

构建警告：

- Vite 报告主 chunk 约 519 kB，超过默认 500 kB 提示线。
- 这是非阻塞性能待办，可在后续做路由级代码分割。

## 9. 本地运行

```powershell
Copy-Item .env.example .env.local
pnpm dev
```

在 `.env.local` 中填写真实模型配置，不要把真实 Key 提交到仓库。

只跑自动化测试：

```powershell
pnpm typecheck
pnpm test
pnpm test:e2e
go test ./server/... ./tests/e2e/integration/... ./tests/e2e/unit/agent -count=1
```

## 10. 已知风险与下一步

当前风险：

- 刷新恢复依赖 Go 进程仍持有 Coordinator；进程重启后只能显示 `interrupted`。
- Event Journal 为进程内存，不能跨进程共享。
- SQLite 和进程内执行锁只适合当前单用户本地场景。
- Web 多实例、Redis 和跨进程恢复尚未实现，不得在部署文档中宣称支持。
- `go.mod` 当前声明 Go 1.26；如果目标机器使用不同版本，需要按实际工具链生成二进制。
- 前端主 chunk 超过 500 kB 提示线。

建议后续顺序：

1. 稳定工作台今日视图和 Agent 基础体验。
2. 设计 Electron 主进程、sidecar、preload 和本地数据库路径。
3. 再评估 Web 身份、PostgreSQL、多实例租约和 Redis 职责。
4. 工具、Skills、MCP 和多 Agent 前先补齐 Step、Artifact、Trace 与权限模型。
5. 独立建设 `craftingtable-agent-eval`，不把评测依赖塞入工作台进程。

## 11. 交接检查

- [x] 实现已提交到 `leeatMvp`。
- [x] 工作区无未提交业务改动。
- [x] 设计文档、ADR、工程规范和实施计划已同步。
- [x] 自动化测试全部通过。
- [x] 人工页面刷新恢复已验证。
- [x] 已知限制和后续决策门已记录。
- [x] 未 push，未 merge。
