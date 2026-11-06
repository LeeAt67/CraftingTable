# SQLite 全面迁移到 PostgreSQL 的条件调研

日期：2026-11-06
状态：调研结论（未进入设计）
范围：CraftingTable 全量数据存储从本地 SQLite 迁移到 PostgreSQL 的触发条件、量化门槛、前置条件与迁移路径
排除：Redis、消息队列、多实例租约的具体实现选型；这些属于同一批架构决策但不因数据库迁移而自动成立
关联需求：[D-01](../requirements.md)
依据：[工程化设计规范](../superpowers/specs/2026-11-05-craftingtable-engineering-design.md)、[Agent 设计](../superpowers/specs/2026-11-05-craftingtable-agent-design.md)、[今日服务端设计](../superpowers/specs/2026-11-05-craftingtable-today-server-design.md)、当前代码

> 本文只回答"什么时候该迁、迁之前必须先满足什么"。不回答"怎么迁完"，也不改变任何已确认的设计。

## 1. 结论

**现在不迁移，但先把判定条件写死。**

当前部署是单用户、单进程、单机本地文件，这个形态正是 SQLite 官方定位的场景（<https://www.sqlite.org/whentouse.html>）。在没有实测数据支撑的情况下换数据库，收益是想象的，代价是确定的：运维面、部署面、测试矩阵和故障面同时扩大。

真正需要警惕的是另一个方向：**等到被迫迁移时才发现代码里到处是 SQLite 方言**。所以现在的正确动作不是换库，而是把触发条件量化、把方言收敛，让将来的迁移是一次受控切换，而不是一次重写。

推荐采用四条硬触发线（§4），任一成立即启动迁移评估；同时先做完 §6 的六项前置条件，成本远低于迁移本身。

## 2. 现状事实

先记录当前实现的可验证状态，避免把"以为"当成"现状"。

| 项目 | 当前事实 | 证据 |
| --- | --- | --- |
| 驱动 | `modernc.org/sqlite`（纯 Go，无 CGO） | `go.mod` |
| 连接池 | `SetMaxOpenConns(1)`、`SetMaxIdleConns(1)` | `sqlite_store.go` |
| 日志模式 | 未设置 `journal_mode`，即默认 rollback journal，**未启用 WAL** | `sqlite_store.go` 只设 `foreign_keys` 与 `busy_timeout` |
| 并发模型 | 连接池 1 + 未开 WAL = **所有数据库访问全局串行** | 同上 |
| 表数量 | 10 张（今日 2 张、Agent 8 张） | `001`~`004` 迁移 |
| 迁移机制 | 文件名 + SHA-256 账本，内容变更即拒绝启动 | `schema_migrations` |
| 时间存储 | 固定 3 位小数 UTC 文本，可直映 `timestamptz` | `timeframe.go`、`004` 迁移 |
| 主键 | TEXT，应用层生成 UUID 风格 ID | `newID()` |
| 软删除 | `deleted_at` 墓碑，为跨端同步预留 | `004` 迁移 |
| SQLite 专有 SQL | `INSERT OR IGNORE`、`strftime()`、`PRAGMA`、测试里的 `pragma_table_info` 与 `GLOB` | 迁移文件与 `sqlite_store.go` |

两条推论值得单独说明：

**推论一：当前不是"SQLite 撑不住"，而是"还没让它撑"。** 连接池被限制为 1，即使库本身支持多读并发，应用层也已经把并发压成串行。**任何"SQLite 慢"的判断在改这个设置之前都不成立。**

**推论二：未开 WAL 是有意的保守选择。** WAL 让读写不互相阻塞，但也引入 `-wal`、`-shm` 文件、checkpoint 和"所有进程必须同主机"的硬约束（<https://www.sqlite.org/wal.html>）。在单连接前提下它几乎不带来收益，所以当前不启用是合理的；一旦要提高并发，第一件事是开 WAL + 放宽连接池，而不是换数据库。

## 3. SQLite 的真实边界（有据可查的部分）

| 边界 | 事实 | 出处 |
| --- | --- | --- |
| 流量 | 官方说法：日请求低于 10 万次基本没问题，10 倍也验证过；这是保守估计而非硬上限 | sqlite.org/whentouse.html |
| 写入并发 | 同一时刻只有一个写者，写事务期间持有库级写锁 | sqlite.org/lockingv3.html |
| 多主机 | **WAL 明确不支持网络文件系统**，所有进程必须在同一台主机 | sqlite.org/wal.html |
| 部署形态 | 适合嵌入、桌面、边缘、单机服务端；不适合"数据放在中心、客户端通过网络访问" | sqlite.org/whentouse.html |
| 运维能力 | 没有内置 PITR、只读副本、在线主备；备份是文件级快照 | 同上 |

对应的工程含义：

- **单机 + 单写者** 是 SQLite 的设计前提，不是缺陷。绕过它的办法（分库、分片、每用户一个文件）都会把复杂度推给业务代码。
- **"多个应用实例共享一个库文件"** 在 SQLite 里做不到，这不是调优问题，是架构不匹配。当前 `SetMaxOpenConns(1)` 反而把这个前提守住了。
- 数据库文件放在网络盘（NFS/SMB）上会破坏锁语义，属于静默数据损坏风险，必须避免。

## 4. 触发条件

四条触发线，任一成立就启动迁移评估。全部可观测、可复核。

### T1 访问形态改变（硬条件，不可绕）

- 需要**多个进程或实例同时写同一个库**：Web 多实例、桌面与 Web 同时在线、跨端同步服务。
- 需要**把数据库文件放在共享存储**上供多机访问。
- 部署环境**不提供持久本地盘**：无状态容器、Serverless、多副本滚动发布。

任一条成立，SQLite 在架构上就不成立，与数据量无关。此时迁移不是优化，是前提。

### T2 并发与延迟实测恶化

先按 §7 的方法测量，满足任一条件再谈迁移：

| 指标 | 阈值 | 说明 |
| --- | --- | --- |
| 同时写请求 | 持续 > 1 且需要并行完成 | 当前全局串行，排队必然发生 |
| 写 p95 延迟 | 持续 > 200 ms | 排除测量前的连接池限制影响 |
| `SQLITE_BUSY` 或写超时 | 日均 > 0 次且非人为干扰 | `busy_timeout` 已设 5 s，超时说明真的争用 |
| 读请求被写阻塞 | 可观测的读 p95 抬升 | 未开 WAL 时写会阻塞读 |

关键限定：**必须在开启 WAL 并放宽连接池之后重新测量**。否则测到的是配置瓶颈，不是数据库上限。

### T3 数据规模与运维目标

| 指标 | 阈值 | 原因 |
| --- | --- | --- |
| 库文件大小 | > 2 GB | 文件级备份、恢复、校验的窗口开始不可接受 |
| 单表行数 | > 500 万 | 索引重建与迁移耗时可感知 |
| 备份窗口 | > 5 分钟 | 备份期间需要停写或依赖快照 |
| RPO | < 5 分钟 | SQLite 只能靠文件快照，做不到持续归档 |
| RTO | < 1 分钟 | 无热备，恢复等于拷文件 |
| 审计/合规 | 需要保留期与不可篡改 | 需要专门的归档与权限模型 |

这些阈值是可商量的，关键是**先有目标再选数据库**，而不是反过来。

### T4 能力缺口

以下能力在 SQLite 上要么没有、要么需要自己实现：

- 行级安全（RLS）与多租户隔离。
- 在线 `CREATE INDEX CONCURRENTLY` 一类不阻塞写入的 DDL。
- 逻辑复制、只读副本地读扩展。
- `SERIALIZABLE` 隔离级别的显式选择与冲突检测（SQLite 的事务是串行化的，但没有"隔离级别"这个概念可供调参）。
- `JSONB`、`GIN` 索引、窗口函数的高级形态、物化视图刷新策略。
- 跨进程的 advisory lock（SQLite 只有文件锁）。

注意：这些缺口**只有在需求真的出现时才算触发器**。为"以后可能要用"而迁移，属于典型过早优化。

### T5 组织与交付形态

- 需要有独立 DBA 或运维角色接手备份、升级、容量管理。
- 需要多环境（开发/预发/生产）数据库实例而非各自一个文件。
- 需要把数据库作为托管服务采购。

## 5. 不构成迁移理由

这些经常被当成理由，但不成立，写下来是为了将来不被反复提起：

| 说法 | 为什么不成立 |
| --- | --- |
| "PostgreSQL 更正式/更专业" | 选型依据是约束匹配，不是印象。SQLite 官方定位就是"竞争对象是 `fopen()`"，不是 Postgres |
| "数据量会变大" | 增长到 §4 T3 的阈值之前，SQLite 更好维护。先测，再判断 |
| "以后要多用户" | 多用户属于 T1，等它真的成为需求时再迁移；现在迁移只是提前承担成本 |
| "SQLite 性能差" | 当前连接池限 1、未开 WAL，任何性能结论都需要先排除配置因素 |
| "想要事务/外键/复杂查询" | SQLite 都支持，且已启用 `foreign_keys` |
| "怕数据损坏" | 单机文件 + 正确备份在实践中可靠；真正的风险是放网络盘和多写者，那属于 T1 |

## 6. 迁移前置条件（技术上必须先做完）

这些不是迁移步骤，而是**让将来的迁移可行**的准备。与数据库无关，本身都值得做。

| 编号 | 前置条件 | 当前差距 |
| --- | --- | --- |
| P-01 | 所有 SQL 收敛到 Repository 实现内，业务层不出现方言 | 基本满足：`today` 与 `agent` 都通过 port 访问；但 `store` 包同时实现两个领域 |
| P-02 | 方言集中：`strftime`、`INSERT OR IGNORE`、`PRAGMA`、`GLOB` 只在 SQLite Adapter 内 | 未满足：`004` 迁移与 `sqlite_store.go` 内联了 `strftime` |
| P-03 | 时间规范化下沉到领域层，迁移脚本不依赖数据库函数 | 部分满足：`timeframe.go` 已有 `FormatTimestamp`，但 `004` 仍用 `strftime` |
| P-04 | 迁移账本与驱动解耦，支持第二套实现 | 未满足：账本逻辑与 SQLite 绑定 |
| P-05 | 事务与隔离语义显式化，不依赖"单连接所以天然串行" | 未满足：当前依赖 `MaxOpenConns(1)` |
| P-06 | 双后端测试矩阵：集成测试可切换 SQLite / PostgreSQL | 未满足：`tests/e2e/integration` 只跑 SQLite |

P-01 至 P-04 是纯重构，可以在没有迁移计划时单独做；P-05、P-06 建议在 T1 触发后再投入，避免为不存在的需求维护两套测试。

## 7. 测量方法（迁移判断的前提）

没有数据就没有判断。建议先补这几项观测，成本低、对现状无侵入：

1. **写路径计数**：按表统计每秒写入与并发度，用现有 `request_id` 日志关联。
2. **延迟分位**：HTTP 层已有 `duration_ms`，按路由分 p50/p95/p99 统计。
3. **锁等待**：`busy_timeout` 触发次数（需要显式捕获 `SQLITE_BUSY` 并计数）。
4. **库体积增长**：定时记录文件大小与行数，形成趋势线。
5. **备份演练**：真实测一次"停写 + 拷贝 + 校验 + 恢复"的耗时，得到 RTO 基线。

这五项数据齐了，§4 的 T2、T3 就能直接判定，不需要争论。

## 8. 迁移路径（触发后）

只在 §4 任一条件成立时才启动。核心原则：**先可回退，再切换**。

| 阶段 | 动作 | 回退方式 |
| --- | --- | --- |
| 0 | 补齐 §6 的 P-01 至 P-04 | 无行为变化 |
| 1 | 引入 PostgreSQL Adapter，双后端可启动 | 配置切回 SQLite |
| 2 | 数据迁移脚本：TEXT 时间 → `timestamptz`，TEXT 主键 → `uuid` 或保留 TEXT | 保留原库不动 |
| 3 | 影子写：SQLite 为主，PostgreSQL 同步写，比对差异 | 关闭影子写 |
| 4 | 影子读校验：抽样比对两端查询结果 | 关闭影子读 |
| 5 | 切换：PostgreSQL 为主，SQLite 转只读保留一个版本周期 | 切回 SQLite（需要反向同步窗口） |
| 6 | 移除 SQLite 路径 | 从备份恢复 |

阶段 3、4 的价值在于：**在真正切换前就暴露方言差异、时区处理差异和事务语义差异**，而不是切换当天才发现。

## 9. 待决策问题

| 编号 | 问题 | 影响 |
| --- | --- | --- |
| Q-01 | 是否先开 WAL 并放宽连接池，再谈迁移 | 决定 T2 的测量基线是否有效 |
| Q-02 | 主键保持 TEXT 还是迁移到 `uuid` 类型 | 影响迁移脚本与索引大小 |
| Q-03 | 时间列迁移到 `timestamptz` 还是一次性转换文本 | 影响 §8 阶段 2 的复杂度 |
| Q-04 | PostgreSQL 部署形态：自建 / 托管 / 容器内 | 影响运维成本与 T5 判定 |
| Q-05 | 是否需要多租户；若需要，RLS 还是应用层隔离 | 影响是否触发 T4 |
| Q-06 | 迁移期间是否允许停写，最长多久 | 决定能否跳过影子写阶段 |

## 10. 与其他文档的关系

- [工程化设计规范](../superpowers/specs/2026-11-05-craftingtable-engineering-design.md) 的 P4 阶段把 PostgreSQL 与租约、任务队列绑定在"Web 多实例或故障接管成为真实需求"这一触发器上。本文把这个触发器拆成 T1 至 T5 五类可观测条件，结论一致：**由需求驱动，不由技术偏好驱动**。
- [Agent 设计](../superpowers/specs/2026-11-05-craftingtable-agent-design.md) 要求 Repository 契约不暴露具体数据库类型，本文 P-01 是它的落地检查项。
- [今日服务端设计](../superpowers/specs/2026-11-05-craftingtable-today-server-design.md) 已按 PostgreSQL 预留：固定宽度 UTC 文本可直映 `timestamptz`，`deleted_at` 同样可空时间列。

## 11. 参考

- SQLite：When To Use（适用场景与流量经验值）<https://www.sqlite.org/whentouse.html>
- SQLite：Write-Ahead Logging（同主机约束、checkpoint）<https://www.sqlite.org/wal.html>
- SQLite：File Locking And Concurrency（pager 锁与并发模型）<https://www.sqlite.org/lockingv3.html>
- PostgreSQL：Transaction Isolation（隔离级别与异常现象矩阵）<https://www.postgresql.org/docs/current/transaction-iso.html>
- PostgreSQL：Explicit Locking（表锁、行锁、advisory lock）<https://www.postgresql.org/docs/current/explicit-locking.html>
