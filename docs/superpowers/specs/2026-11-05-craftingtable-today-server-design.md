# CraftingTable 今日服务端设计（非 Agent）

日期：2026-11-05
状态：已评审（2026-11-05 决议），进入实施计划
范围：`A-01` 日期与时区查询、`A-02` 删除、`A-03` 时间语义、`A-06` 错误码
关联：[非 Agent 工作台需求调研](../../research/2026-11-05-non-agent-workbench-requirement-research.md)、[工程化设计规范](./2026-11-05-craftingtable-engineering-design.md)、[术语表](../../glossary.md)
排除：Agent 会话、`AgentRun`、SSE 协议、Agent 持久化与恢复

> 本文件只定义契约与取舍。评审通过后才编写实施计划，再写失败测试，最后实现。

## 1. 背景

今日模块当前的服务端行为存在三个会在后续阶段放大成本的问题：

1. **日界由服务端进程时区决定**。`listToday` 用 `time.Now()` 和 `now.Location()` 计算当天区间，部署机时区变化会改变同一份数据的"今天"。
2. **时间文本宽度不固定**。写入使用 `time.RFC3339Nano`，小数位长度可变；SQLite 的 TEXT 比较是字节字典序，实测 `'2026-11-05T09:00:00.5Z' < '2026-11-05T09:00:00Z'` 成立，排序和范围查询都会出错。
3. **缺少删除能力**，且未来需要跨端同步，因此删除必须留下可同步的痕迹。

## 2. 决策记录

### DD-1 日界时区：客户端显式提供，服务端回退本地时区

- `GET /api/v1/today` 接受 `date=YYYY-MM-DD` 与 `tz=IANA`。
- `tz` 缺省时使用服务端本地时区，并在响应头与日志标记为 deprecated；后续版本可改为必填。
- `date` 缺省时取 `tz` 下的当天。

依据：

- RFC 3339 要求时间必须带偏移量，不存在"无时区"的合法表示（<https://www.rfc-editor.org/rfc/rfc3339>）。
- RFC 5545 §3.3.5 的 DATE-TIME 允许多种形式，但日历互操作必须由数据本身或查询参数携带时区（<https://www.rfc-editor.org/rfc/rfc5545#section-3.3.5>）。
- Google Calendar `events.list` 要求 `timeMin`/`timeMax` 为**带强制时区偏移**的 RFC3339，`timeZone` 只影响响应时区，缺省取日历时区（<https://developers.google.com/workspace/calendar/api/v3/reference/events/list>）。
- CalDAV `time-range` 同样由客户端给出区间（<https://www.rfc-editor.org/rfc/rfc4791>）。

代价：前端必须始终传 `tz`；服务端需内置 IANA 数据库（`time/tzdata`），避免依赖宿主机 zoneinfo。

### DD-2 删除：软删除墓碑，为跨端同步预留

- `items` 增加 `deleted_at`；删除写入时间戳，列表查询默认 `deleted_at IS NULL`。
- 删除幂等：条目存在但已删除，返回同一墓碑（`200`）；条目从未存在，返回 `NOT_FOUND`。
- 提供恢复入口，撤销与将来的同步冲突处理共用同一机制。

依据：

- CalDAV 的 `DELETE` 是硬删除，重复删除返回 404，因为它不承担同步职责（<https://www.rfc-editor.org/rfc/rfc4791>）。
- Google Calendar 把删除表达为 `status: "cancelled"`，默认不返回，`showDeleted` 才返回；增量同步依赖这一表示（同 `events.list` 文档）。
- Microsoft Graph delta 用 `@removed` 标记表示"已删除"，同样服务于"告诉客户端这条没了"（<https://learn.microsoft.com/en-us/graph/delta-query-overview>）。
- Stripe 删除返回 `{ id, object, deleted: true }`，保留 ID 以便引用方判断失效（<https://docs.stripe.com/api>）。

结论：既然确定要做跨端同步，就必须付墓碑成本；硬删除会让同步端无法区分"删除"和"从未存在"。墓碑只在同步接入后才有对外查询需求，本次不新增同步端点，但表结构先落地，避免二次迁移。

### DD-3 时间格式：固定 3 位小数 UTC 文本

- 线上格式：`YYYY-MM-DDTHH:MM:SS.sssZ`，例如 `2026-11-05T01:30:00.000Z`。
- 存储格式与线上格式一致，SQLite 用 TEXT，迁移到 PostgreSQL 时对应 `timestamptz`。
- 入参允许任意合法 RFC 3339 偏移量，服务端统一转换为上述格式。

依据：

- SQLite 没有日期类型，官方给出 TEXT ISO-8601、REAL Julian day、INTEGER Unix 三种表示（<https://www.sqlite.org/datatype3.html>）。
- SQLite 的 TEXT 比较使用 BINARY collation，即字节字典序，因此同列内宽度必须一致（<https://www.sqlite.org/datatype3.html>）。
- SQLite 自身的时间函数只接受最多 3 位小数，9 位小数会让 `strftime` 等函数失去可解析性（<https://www.sqlite.org/lang_datefunc.html>）。
- 实测 `strftime('%Y-%m-%dT%H:%M:%fZ', value)` 可把 `RFC3339Nano`、无小数、1 位小数统一为 3 位，迁移可纯 SQL 完成。
- 固定宽度文本在 PostgreSQL 中可直接 `::timestamptz`，迁移不需要改变线上契约。

被否决的方案：`INTEGER` Unix 毫秒（可排序但不可读，且与现有数据、日志、调试都不兼容）、`REAL` Julian day（浮点精度与可读性都差）、固定 9 位小数（SQLite 原生时间函数不可用，且无实际精度收益）。

## 3. API 契约

所有响应沿用现有封套：成功 `{ "data": ... }`，失败 `{ "error": { code, kind, message, retryable, requestId } }`。

### 3.1 `GET /api/v1/today`

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `date` | 否 | `YYYY-MM-DD`；缺省为 `tz` 下的今天 |
| `tz` | 否 | IANA 时区名，如 `Asia/Shanghai`；缺省为服务端本地时区（deprecated） |

- 成功：`200`，`data` 为 `Item[]`，按 `start_at IS NULL, start_at, created_at` 升序，即"已安排在前按时间升序，未安排在后按创建时间升序"。
- `date` 或 `tz` 非法：`400 VALIDATION_ERROR`，`message` 为 `date 必须是 YYYY-MM-DD` 或 `tz 必须是 IANA 时区名称`。
- 已删除条目不出现在结果中。
- 查询区间为 `[本地日 00:00, 次日 00:00)`，按 `tz` 换算成 UTC；夏令时切换日按真实长度计算。

### 3.2 `POST /api/v1/item/create`

请求：`{ title, categoryId?, startAt?, endAt? }`

- `startAt` 为空 → `status: "unplanned"`；否则 `status: "planned"`。
- `endAt` 存在但 `startAt` 为空 → `400`，`message` 为 `填写结束时间时必须填写开始时间`。
- `endAt <= startAt` → `400`，`message` 为 `结束时间必须晚于开始时间`。
- 成功：`200`，`data` 为完整 `Item`。

### 3.3 `POST /api/v1/item/update`

请求：`{ id, title?, description?, categoryId?, startAt?, endAt? }`

- 字段缺省表示不修改；`startAt: ""` 或 `endAt: ""` 表示清空该时间。
- 校验规则同 create；清空 `startAt` 后，未完成条目回到 `unplanned`，已完成条目保持 `done`。
- 时间变化会重算未完成条目的 `status`，不覆盖 `done`。
- 已删除条目视为不存在，返回 `404 NOT_FOUND`。
- 成功：`200`，`data` 为完整 `Item`。

### 3.4 `POST /api/v1/item/complete` / `POST /api/v1/item/uncomplete`

- 请求：`{ id }`。
- 已删除条目返回 `404`。
- `uncomplete` 依据是否存在 `startAt` 恢复为 `planned` 或 `unplanned`。

### 3.5 `POST /api/v1/item/delete`

请求：`{ id }`

| 场景 | 状态 | 响应 |
| --- | --- | --- |
| 删除成功 | `200` | `{ "data": { "id": "...", "deletedAt": "..." } }` |
| 已删除，重复删除 | `200` | 返回首次的 `deletedAt`，不修改原值 |
| 从未存在 | `404` | `NOT_FOUND` |

同类条目被删除后，`complete` / `uncomplete` / `update` 一律返回 `404`，避免"已删除还能修改"的歧义。

### 3.6 `POST /api/v1/item/restore`

请求：`{ id }`

| 场景 | 状态 | 响应 |
| --- | --- | --- |
| 恢复成功 | `200` | 完整 `Item`，`status` 依据 `startAt` 与 `completedAt` 恢复 |
| 未删除 | `200` | 返回当前条目，幂等 |
| 从未存在 | `404` | `NOT_FOUND` |

`restore` 同时服务前端撤销与将来的同步回滚，但本轮前端可以先只用延迟提交，不强制调用。

### 3.7 未纳入本次范围

- 增量同步端点（`since` / `syncToken` / `includeDeleted`）。
- 分类的增删改。
- 并发控制（`If-Match` / 版本号）与写请求幂等（`clientRequestId`）。

## 4. 数据模型与迁移

### 4.1 迁移 `004_item_deletion_and_time.sql`

```sql
ALTER TABLE items ADD COLUMN deleted_at TEXT;

UPDATE items
SET start_at = strftime('%Y-%m-%dT%H:%M:%fZ', start_at)
WHERE start_at IS NOT NULL AND start_at <> '';

UPDATE items
SET end_at = strftime('%Y-%m-%dT%H:%M:%fZ', end_at)
WHERE end_at IS NOT NULL AND end_at <> '';

UPDATE items
SET completed_at = strftime('%Y-%m-%dT%H:%M:%fZ', completed_at)
WHERE completed_at IS NOT NULL AND completed_at <> '';

UPDATE items
SET created_at = strftime('%Y-%m-%dT%H:%M:%fZ', created_at)
WHERE created_at IS NOT NULL AND created_at <> '';

UPDATE items
SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', updated_at)
WHERE updated_at IS NOT NULL AND updated_at <> '';

CREATE INDEX IF NOT EXISTS idx_items_deleted_at ON items(deleted_at);
```

约束：

- 只新增列与规范化数据，不改变已有列语义；`001_initial.sql`、`002_agent.sql` 保持字节不变，否则迁移账本的 SHA-256 校验会拒绝启动。
- 迁移在单事务内执行；`strftime` 对合法 ISO-8601 返回规范化文本，对无法解析的值返回 NULL，因此迁移后必须校验 `NULL` 数量为 0，否则回滚并中止启动。
- 迁移完成后，`items` 中所有时间列都满足 `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`。
- 新增迁移文件后，已有数据库首次启动自动应用；开发库需要重新生成时使用 `-reset-database`。

### 4.2 面向 PostgreSQL 的预留

- 时间列在 PostgreSQL 中使用 `timestamptz`，线上文本可直接解析，无需修改契约。
- 迁移 SQL 不使用 SQLite 专有语法（除本次一次性规范化），查询条件保持 ANSI。
- 主键仍为 TEXT，便于直接映射 `uuid`；不引入自增列。
- `deleted_at` 在 PostgreSQL 中同样为可空 `timestamptz`，索引策略保持一致。

## 5. 错误码

| code | HTTP | kind | retryable | 默认提示 |
| --- | --- | --- | --- | --- |
| `VALIDATION_ERROR` | 400 | `validation` | false | 参数校验失败，使用具体 `message` |
| `NOT_FOUND` | 404 | `not_found` | false | 事务条目不存在 |
| `INTERNAL_ERROR` | 500 | `unknown` | true | 服务暂时不可用，请稍后重试 |

- 前端按 `code` 映射，不直接展示后端 `message` 以外的内容；`message` 仅用于校验类可读提示。
- `CONFLICT`、`UNAVAILABLE` 属于后续阶段（并发控制、服务不可用重试），本次不实现，避免出现无人处理的错误码。

## 6. 与 Agent 的隔离

- 只修改 `items` 表与今日模块，不触碰 `agent_*` 表、路由和迁移文件。
- 复用 `httpapi` 的封套、`requestID` 和日志中间件，不共享业务存储。
- 今日模块的测试不依赖 Fake Agent Provider。

## 7. 验收测试（实现阶段之前先写失败用例）

集成测试（`tests/e2e/integration/today.integration_test.go` 或新增文件）：

1. `date=2026-11-05&tz=Asia/Shanghai` 只返回该本地日条目，跨日条目按本地日归属。
2. 不传 `tz` 时行为与旧版本一致，结果不变。
3. 非法 `date`、非法 `tz` 返回 `400`，`code=VALIDATION_ERROR`。
4. 夏令时切换日（`America/New_York`，2026-03-08）查询区间为 23 小时。
5. 创建时 `endAt <= startAt` 返回 `400`；`endAt` 无 `startAt` 返回 `400`。
6. 入参 `+08:00` 偏移被规范化为 `Z`，响应文本为 3 位小数。
7. 删除后列表不再返回；重复删除返回同一 `deletedAt`；删除不存在的 `id` 返回 `404`。
8. 删除后 `complete` / `update` 返回 `404`；`restore` 后条目恢复原状态。
9. 迁移后所有时间列匹配固定宽度正则；迁移前后条目数量一致。

单元测试（`server/internal/today`）：

1. `ResolveDayRange` 覆盖缺省日期、缺省时区、非法参数、夏令时。
2. `NormalizeTimestamp` 覆盖 `Z`、正负偏移、无小数、纳秒、空值、非法值。
3. `TimeRangeProblem` 覆盖合法区间、相等、倒序、缺开始时间。

## 8. 风险与回滚

| 风险 | 影响 | 处理 |
| --- | --- | --- |
| 迁移 `strftime` 返回 NULL | 时间列被清空 | 迁移事务内校验 `NULL` 计数，非 0 则回滚并拒绝启动 |
| 旧客户端不传 `tz` | 行为与之前一致，不会报错 | 保留回退路径并标记 deprecated |
| 前端仍按本地时区格式化 | 展示与查询口径不一致 | 由前端需求 `T-01` 同步处理，本轮只提供契约 |
| 软删除被误当作"数据已清" | 用户以为彻底删除 | 文档与设置页说明；后续可加清理策略 |
| 迁移与 Agent 迁移顺序冲突 | 启动失败 | 文件名递增，互不改动既有文件 |

回滚：SQLite 使用迁移前备份文件恢复；由于迁移只新增列并规范化文本，代码回滚后旧代码仍可读取 `deleted_at` 以外的数据（旧代码不识别该列，会忽略已删除条目，需注意这一差异）。

## 9. 评审决议

2026-11-05 评审通过，决议如下：

| 编号 | 决议 |
| --- | --- |
| R-1 | 已删除条目被 `update` / `complete` / `uncomplete` 访问时返回 `404 NOT_FOUND`，与"不存在"表现一致 |
| R-2 | `restore` 本轮实现：清空 `deleted_at`，`id`、分类、完成状态不变；未删除时幂等返回当前条目 |
| R-3 | `tz` 缺省保留回退到服务端本地时区，标注 deprecated，不改为必填 |
| R-4 | 列表排序保持 `start_at IS NULL, start_at, created_at` 升序，前端排序改造不在本轮 |

实施顺序见 [今日服务端实施计划](../plans/2026-11-05-craftingtable-today-server.md)。
