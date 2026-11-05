# 今日服务端实施计划（非 Agent）

日期：2026-11-05
对应设计：[今日服务端设计](../specs/2026-11-05-craftingtable-today-server-design.md)（已评审）
范围：`A-01` 日期与时区查询、`A-02` 删除与恢复、`A-03` 时间语义、`A-06` 错误码
不涉及：前端改动、Agent 模块、分类增删改、并发控制、写请求幂等

**目标**：在不改变 Agent 任何契约的前提下，让今日模块具备可指定日期的查询、固定宽度 UTC 时间、软删除墓碑与恢复能力。

**方法**：严格 TDD。每个任务先写失败测试，确认失败原因正确，再写最小实现，最后跑定向命令。

**通用约束**：

- 不修改 `001_initial.sql`、`002_agent.sql` 的任何字节，否则迁移账本校验会拒绝启动。
- 不修改 `server/internal/agent/**`、`tests/e2e/integration/agent_*`。
- 所有时间写入与返回统一为 `YYYY-MM-DDTHH:MM:SS.sssZ`。
- 保持 `GET` / `POST` 与 `{ data }` / `{ error }` 封套。
- 每个任务结束时工作区可编译、可跑测试。

## 任务清单

### T1 时间契约纯函数

**产出**：`server/internal/today/timeframe.go`、`server/internal/today/timeframe_test.go`

新增纯函数：

| 函数 | 职责 |
| --- | --- |
| `FormatTimestamp(time.Time) string` | 统一输出 3 位小数 UTC |
| `ResolveDayRange(date, tz string, now time.Time) (DayRange, string)` | 解析 `date`/`tz` 为 UTC 区间，返回面向用户的错误文案 |
| `NormalizeTimestamp(*string) (*string, string)` | 校验并规范化单个时间；空值返回 `nil` |
| `NormalizeOptionalTimestamp(*string) (*string, string)` | 区分 `nil`（不修改）与 `""`（清空） |
| `TimeRangeProblem(startAt, endAt *string) string` | `end <= start`、只有 `end` 等非法组合 |

- [ ] 先写 `timeframe_test.go`：缺省日期、缺省时区、非法 `date`/`tz`、`America/New_York` 2026-03-08 夏令时、`+08:00` 偏移规范化、纳秒输入、空值、清空语义、时间区间倒序与缺开始时间。
- [ ] 运行 `go test ./server/internal/today/ -run 'Timeframe|ResolveDayRange|Normalize|TimeRange'`，确认编译失败或断言失败。
- [ ] 实现 `timeframe.go` 至测试通过。
- [ ] 验收：`go test ./server/internal/today/`。

**注意**：`DayRange` 的 `Start`/`End` 必须是固定宽度字符串，保证与数据库列同格式比较。

### T2 领域类型与端口扩展

**产出**：`server/internal/today/item.go`

- [ ] 新增 `ValidationError{ Message string }`，实现 `error`，供 store 把校验失败映射为 `400`。
- [ ] 新增 `DeletedItem{ ID, DeletedAt }` 作为删除响应体。
- [ ] `Repository` 增加 `DeleteItem(ctx, id) (DeletedItem, error)` 与 `RestoreItem(ctx, id) (Item, error)`。
- [ ] 编译期确认 `store.SQLiteStore` 未实现前会报错（这是预期的失败信号）。
- [ ] 验收：`go build ./server/...` 失败信息指向未实现的方法。

### T3 迁移 003

**产出**：`server/internal/platform/store/migrations/003_item_deletion_and_time.sql`、迁移测试

内容按设计 4.1：新增 `deleted_at`、规范化 5 个时间列、新增索引。

- [ ] 先写测试 `tests/e2e/integration/migrations.integration_test.go` 追加用例：
  - 迁移后 `items` 存在 `deleted_at` 列；
  - 所有时间列匹配 `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`；
  - 迁移前后 `items` 行数一致；
  - `agent_*` 表的列集合不变。
- [ ] 运行 `go test ./tests/e2e/integration/ -run Migration`，确认失败。
- [ ] 添加迁移文件。
- [ ] 增加迁移后校验：`strftime` 结果为 `NULL` 的行数必须为 0，否则回滚并返回错误（在 `applyMigration` 之后、`OpenSQLite` 返回前执行）。
- [ ] 验收：`go test ./tests/e2e/integration/ -run Migration`。

**注意**：已有开发库首次启动会自动应用 003；如需干净库，使用 `-reset-database`。

### T4 存储层：时间规范化、软删除、恢复

**产出**：`server/internal/platform/store/sqlite_store.go`

- [ ] `ListToday` 增加 `AND deleted_at IS NULL`，其余排序不变。
- [ ] `CreateItem`：写入前用 `NormalizeTimestamp` 规范化，`TimeRangeProblem` 不通过时返回 `ValidationError`。
- [ ] `UpdateItem`：合并字段后规范化；空字符串表示清空；未完成条目按 `startAt` 重算 `status`，`done` 不覆盖；校验失败返回 `ValidationError`。
- [ ] `CompleteItem` / `UncompleteItem`：`getItem` 已排除删除项（`WHERE id = ? AND deleted_at IS NULL`），删除后自然返回 `ErrItemNotFound`。
- [ ] `DeleteItem`：`UPDATE items SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`；`RowsAffected == 0` 时查询该行是否已删除并返回首次墓碑，否则返回 `ErrItemNotFound`。
- [ ] `RestoreItem`：`UPDATE items SET deleted_at = NULL, updated_at = ? WHERE id = ? AND deleted_at IS NOT NULL`；未删除时直接返回当前条目；不存在返回 `ErrItemNotFound`。
- [ ] `nullableTimestamp`：空字符串写入 SQL `NULL`。
- [ ] 验收：`go test ./tests/e2e/integration/ -run Today`。

**注意**：`DeleteItem` 的幂等依赖"先查后写"，需要避免并发下重复更新 `deleted_at`；使用 `AND deleted_at IS NULL` 条件保证首次时间戳不被覆盖。

### T5 HTTP 层：查询参数与删除/恢复端点

**产出**：`server/internal/today/controller.go`、`server/cmd/craftingtabled/main.go`

- [ ] `listToday` 读取 `date`、`tz`，调用 `ResolveDayRange`，参数非法返回 `VALIDATION_ERROR`。
- [ ] 注册 `POST /api/v1/item/delete` 与 `POST /api/v1/item/restore`。
- [ ] `createItem` / `updateItem` 改用 `NormalizeTimestamp` / `NormalizeOptionalTimestamp`，并调用 `TimeRangeProblem`。
- [ ] `writeRepositoryError` 识别 `ValidationError`，把 `Message` 原样作为 `VALIDATION_ERROR.message`。
- [ ] `main.go` 增加 `_ "time/tzdata"`，保证 Windows 与精简镜像下 `LoadLocation` 可用。
- [ ] 验收：`go build ./server/... && go vet ./server/...`。

### T6 集成测试

**产出**：`tests/e2e/integration/today.integration_test.go`（追加）、必要时新增 `today_time.integration_test.go`

- [ ] 先写下列用例并确认失败：
  1. `date=2026-11-05&tz=Asia/Shanghai` 只返回该本地日条目；
  2. 不传 `tz` 与旧行为一致；
  3. 非法 `date`、非法 `tz` 返回 `400 VALIDATION_ERROR`；
  4. `America/New_York` 2026-03-08 区间为 23 小时（通过跨日条目归属验证）；
  5. `endAt <= startAt` 与"只有 endAt"返回 `400`；
  6. `+08:00` 偏移入参被规范化为 `Z` 且为 3 位小数；
  7. 删除后列表不含该条；重复删除返回同一 `deletedAt`；删除不存在返回 `404`；
  8. 删除后 `update` / `complete` 返回 `404`；`restore` 后条目恢复原 `status` 与 `categoryId`；
  9. `restore` 未删除条目幂等返回当前条目。
- [ ] 实现至全部通过。
- [ ] 验收：`go test ./tests/e2e/integration/ -run Today -count=1`。

### T7 文档与全量校验

**产出**：`README.md`（测试与契约说明）、设计文档状态

- [ ] README「当前范围」补充：今日查询支持 `date`/`tz`，删除为软删除并可恢复，时间为固定 3 位小数 UTC。
- [ ] 确认设计文档第 9 节与实现一致；不一致时先改文档再改代码。
- [ ] 全量校验：
  - `go build ./server/...`
  - `go vet ./server/...`
  - `go test ./server/...`
  - `go test ./tests/e2e/integration/ -count=1`
  - `pnpm typecheck`（确认前端未受影响）
- [ ] 确认 `git diff` 未触碰 `001_initial.sql`、`002_agent.sql`、`server/internal/agent/**`。

## 执行顺序与检查点

```text
T1 ──> T2 ──> T4 ──> T5 ──> T6 ──> T7
                │
T3 ─────────────┘
```

| 检查点 | 命令 | 期望 |
| --- | --- | --- |
| CP1（T1–T2） | `go test ./server/internal/today/` | 纯函数测试通过；`go build` 仍因 store 未实现而失败 |
| CP2（T3） | `go test ./tests/e2e/integration/ -run Migration` | 迁移与校验用例通过 |
| CP3（T4–T5） | `go build ./server/... && go vet ./server/...` | 无编译与静态检查问题 |
| CP4（T6） | `go test ./tests/e2e/integration/ -run Today` | 9 条新用例通过，旧用例不回归 |
| CP5（T7） | 全量命令 | 全部通过，diff 未触碰 Agent 与旧迁移 |

## 风险与应对

| 风险 | 触发信号 | 应对 |
| --- | --- | --- |
| 迁移把时间列写成 NULL | 校验行数 > 0 | 事务回滚，保留旧库，修正迁移后重试 |
| 旧集成测试依赖"服务端时区当天" | `TodayAPIRoundTrip` 失败 | 用例显式传 `tz`，或在测试内按本地时区推算期望 |
| 软删除后计数变化 | 前端侧栏数量不一致 | 前端需求 `T-08` 同步处理；本轮只保证列表不含删除项 |
| 并发重复删除 | 两次 `deletedAt` 不同 | 条件更新 + 首次时间戳优先，集成测试覆盖 |
| Windows 缺少 zoneinfo | `LoadLocation` 报错 | 引入 `time/tzdata` |

## 交付物

- 代码：`timeframe.go`、`item.go`、`sqlite_store.go`、`controller.go`、`main.go`、`003_*.sql`
- 测试：`timeframe_test.go`、迁移用例、今日集成用例
- 文档：设计文档状态更新、README 更新
- 提交粒度建议：T1+T2 一次、T3 一次、T4+T5 一次、T6 一次、T7 一次
