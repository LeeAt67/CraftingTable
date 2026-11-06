# CraftingTable 需求索引

日期：2026-11-06
状态：持续维护
用途：全仓库唯一的需求清单。任何"要做什么"都先在这里有一行，再链接到详细设计或调研。
术语基线：[术语表](./glossary.md)

## 1. 这份文档解决什么

需求散落在 specs、plans、research、handoffs 里，时间一长没人知道还有多少没做、某个想法有没有被记录过。这份文档只做一件事：**给出全量需求的一句话索引和当前状态**。

它不复制设计和调研内容。详情永远在链接的文档里，这里只保留能够判断"要不要做、做没做"的最少信息。

## 2. 维护规则

### 2.1 什么时候必须新增一行

出现下列任一情况，就先在这里登记，再动手：

- 用户明确提出一个新需求、改进点或缺陷修复。
- 用户在对话或文档里留下 TODO、以后要做、先记一下之类的意图。
- 设计或调研文档里产出了新的待办项。
- 代码里发现必须处理但本次不做的技术债。

反过来，纯提问、代码走读、排查结论不需要登记。

### 2.2 新增步骤

1. 选一个未被占用的编号（见 3.1 前缀表），追加到对应分组。
2. 填四项：需求一句话、优先级、状态、调研与设计文档链接。
3. 调研或设计文档还没写时，链接列写 `待补`，不要为了填满而临时编一份文档。
4. 这个需求来自哪份调研或设计，就把该文档路径和章节名写进链接列；同时在 §4 的反向索引里登记一行，让"一份文档支撑了哪些需求"也能查到。
5. 需求实现后，只改状态列，不要删除行。历史记录本身有价值。

### 2.3 状态取值

| 状态 | 含义 |
| --- | --- |
| `已实现` | 功能可用，有对应测试 |
| `部分实现` | 后端或某一端已就绪，另一端或边界情况未完成，必须在备注里写清缺什么 |
| `待实现` | 设计与计划已确认，可以直接开工 |
| `提案` | 已记录但未评审，动手前需要先补设计 |
| `已取代` | 被其他需求或文档替代，保留行并注明替代者 |

### 2.4 优先级

沿用 [非 Agent 工作台需求调研 §3.1](./research/2026-11-05-non-agent-workbench-requirement-research.md) 的定义：`P0` 阻塞日常闭环、`P1` 影响可靠性或数据安全、`P2` 体验增强。

## 3. 需求列表

### 3.1 编号前缀

| 前缀 | 领域 |
| --- | --- |
| `T` | 今日工作台 |
| `S` | 设置 |
| `N` | 导航与布局 |
| `A` | 今日 API 与数据 |
| `Q` | 质量与非功能 |
| `AG` | Agent 能力 |

编号一经使用不复用。删除需求时保留编号，状态改 `已取代`。

### 3.2 今日工作台（T）

来源：[非 Agent 需求调研 § EPIC T](./research/2026-11-05-non-agent-workbench-requirement-research.md)

| 编号 | 需求 | 优先级 | 状态 | 调研 / 设计文档 |
| --- | --- | --- | --- | --- |
| T-01 | 日期导航：昨天 / 今天 / 明天，日期写入 URL | P0 | 部分实现 | [验收标准 T-01](./research/2026-11-05-non-agent-workbench-requirement-research.md)；后端 `?date=&tz=` 已就绪，见 [今日服务端设计 §3.1](./superpowers/specs/2026-11-05-craftingtable-today-server-design.md)；前端未接 |
| T-02 | 侧栏筛选真实生效，条件写入 URL | P0 | 待实现 | [验收标准 T-02](./research/2026-11-05-non-agent-workbench-requirement-research.md) |
| T-03 | 条目编辑：标题、描述、分类、起止时间 | P0 | 部分实现 | [验收标准 T-03](./research/2026-11-05-non-agent-workbench-requirement-research.md)；后端 `update` 支持清空时间与校验，见 [今日服务端设计 §3.3](./superpowers/specs/2026-11-05-craftingtable-today-server-design.md)；前端无入口 |
| T-04 | 删除条目与撤销 | P1 | 部分实现 | [验收标准 T-04](./research/2026-11-05-non-agent-workbench-requirement-research.md)；后端 `delete` / `restore` 已实现（软删墓碑），见 [今日服务端设计 §3.5 §3.6](./superpowers/specs/2026-11-05-craftingtable-today-server-design.md)；前端未接 |
| T-05 | 安排与调整时间 | P0 | 部分实现 | [验收标准 T-05](./research/2026-11-05-non-agent-workbench-requirement-research.md)；后端按 `startAt` 重算状态且不覆盖 `done`；前端未接 |
| T-06 | 时间流排序与分区 | P1 | 待实现 | [验收标准 T-06](./research/2026-11-05-non-agent-workbench-requirement-research.md) |
| T-07 | 专注时长口径 | P1 | 待实现 | [验收标准 T-07](./research/2026-11-05-non-agent-workbench-requirement-research.md)；当前按 event 区间求和，未处理跨天与脏数据 |
| T-08 | 完成度口径统一 | P1 | 待实现 | [验收标准 T-08](./research/2026-11-05-non-agent-workbench-requirement-research.md) |
| T-09 | Quick Capture 增强：分类、时间、连续录入 | P2 | 待实现 | [验收标准 T-09](./research/2026-11-05-non-agent-workbench-requirement-research.md) |
| T-10 | 加载、空与错误状态 | P0 | 部分实现 | [验收标准 T-10](./research/2026-11-05-non-agent-workbench-requirement-research.md)；已有骨架屏、错误重试、Toast；缺行内错误与防重复提交 |
| T-11 | 键盘与快捷操作 | P2 | 待实现 | [验收标准 T-11](./research/2026-11-05-non-agent-workbench-requirement-research.md) |

### 3.3 设置（S）

来源：[非 Agent 需求调研 § EPIC S](./research/2026-11-05-non-agent-workbench-requirement-research.md)

| 编号 | 需求 | 优先级 | 状态 | 调研 / 设计文档 |
| --- | --- | --- | --- | --- |
| S-01 | 设置页信息架构：外观、数据、关于 | P0 | 待实现 | [验收标准 S-01](./research/2026-11-05-non-agent-workbench-requirement-research.md)；当前为静态占位页 |
| S-02 | 外观设置：浅色 / 深色 / 跟随系统 | P0 | 待实现 | [验收标准 S-02](./research/2026-11-05-non-agent-workbench-requirement-research.md)；当前仅手动两态，持久化键为 `leeat-theme` |
| S-03 | 偏好设置：每周起始日、12/24 小时制、默认分类 | P2 | 待实现 | [验收标准 S-03](./research/2026-11-05-non-agent-workbench-requirement-research.md) |
| S-04 | 数据管理：导出、导入、备份 | P1 | 待实现 | [验收标准 S-04](./research/2026-11-05-non-agent-workbench-requirement-research.md) |
| S-05 | 关于与诊断 | P2 | 待实现 | [验收标准 S-05](./research/2026-11-05-non-agent-workbench-requirement-research.md) |
| S-06 | 设置持久化方案决策 | P0 | 提案 | [验收标准 S-06](./research/2026-11-05-non-agent-workbench-requirement-research.md)；需先出决策记录 |

### 3.4 导航与布局（N）

来源：[非 Agent 需求调研 § EPIC N](./research/2026-11-05-non-agent-workbench-requirement-research.md)

| 编号 | 需求 | 优先级 | 状态 | 调研 / 设计文档 |
| --- | --- | --- | --- | --- |
| N-01 | 导航激活状态与页面标题一致 | P1 | 部分实现 | [验收标准 N-01](./research/2026-11-05-non-agent-workbench-requirement-research.md)；`NavLink` 已有激活态与 `aria-label`，页面标题映射未做 |
| N-02 | 深链与浏览器历史 | P1 | 待实现 | [验收标准 N-02](./research/2026-11-05-non-agent-workbench-requirement-research.md)；依赖 T-01、T-02 |
| N-03 | 窄屏侧栏可展开 | P2 | 部分实现 | [验收标准 N-03](./research/2026-11-05-non-agent-workbench-requirement-research.md)；当前窄屏只压缩为窄栏，没有抽屉 |
| N-04 | 跳过导航与焦点管理 | P1 | 待实现 | [验收标准 N-04](./research/2026-11-05-non-agent-workbench-requirement-research.md) |

### 3.5 今日 API 与数据（A）

来源：[非 Agent 需求调研 § EPIC A](./research/2026-11-05-non-agent-workbench-requirement-research.md)、[今日服务端设计](./superpowers/specs/2026-11-05-craftingtable-today-server-design.md)

| 编号 | 需求 | 优先级 | 状态 | 调研 / 设计文档 |
| --- | --- | --- | --- | --- |
| A-01 | 按日期与时区查询今日条目 | P0 | 已实现 | [设计 §3.1](./superpowers/specs/2026-11-05-craftingtable-today-server-design.md)、[计划 T5](./superpowers/plans/2026-11-05-craftingtable-today-server.md)；`GET /api/v1/today?date=&tz=` |
| A-02 | 删除条目 | P1 | 已实现 | [设计 §3.5 §3.6](./superpowers/specs/2026-11-05-craftingtable-today-server-design.md)；软删墓碑 + 幂等 + `restore`，迁移 `004_item_deletion_and_time.sql` |
| A-03 | 时间语义与时区契约 | P0 | 已实现 | [设计 §2 DD-3](./superpowers/specs/2026-11-05-craftingtable-today-server-design.md)；固定 3 位小数 UTC，`endAt` 必须晚于 `startAt` |
| A-04 | 更新并发控制 | P2 | 提案 | [验收标准 A-04](./research/2026-11-05-non-agent-workbench-requirement-research.md) |
| A-05 | 分类管理：增删改 | P2 | 提案 | [验收标准 A-05](./research/2026-11-05-non-agent-workbench-requirement-research.md) |
| A-06 | 错误码表统一 | P1 | 部分实现 | [验收标准 A-06](./research/2026-11-05-non-agent-workbench-requirement-research.md)；后端已有 `VALIDATION_ERROR`、`NOT_FOUND`、`INTERNAL_ERROR`，前端按 `code` 映射未做 |
| A-07 | 写请求幂等 | P2 | 提案 | [验收标准 A-07](./research/2026-11-05-non-agent-workbench-requirement-research.md) |

### 3.6 质量与非功能（Q）

来源：[非 Agent 需求调研 § EPIC Q](./research/2026-11-05-non-agent-workbench-requirement-research.md)

| 编号 | 需求 | 优先级 | 状态 | 调研 / 设计文档 |
| --- | --- | --- | --- | --- |
| Q-01 | 可访问性基线 WCAG AA | P0 | 部分实现 | [验收标准 Q-01](./research/2026-11-05-non-agent-workbench-requirement-research.md)；图标按钮有 `aria-label`，缺 `aria-live` 与跳过导航 |
| Q-02 | 响应式与触摸目标 | P1 | 部分实现 | [验收标准 Q-02](./research/2026-11-05-non-agent-workbench-requirement-research.md)；390/768/1024/1440 无横向滚动已有 E2E，触摸目标尺寸未校验 |
| Q-03 | 性能预算 | P2 | 待实现 | [验收标准 Q-03](./research/2026-11-05-non-agent-workbench-requirement-research.md) |
| Q-04 | 观测与隐私日志 | P1 | 部分实现 | [验收标准 Q-04](./research/2026-11-05-non-agent-workbench-requirement-research.md)；已有 `requestId` 与请求日志，前端错误上报与脱敏检查未做 |
| Q-05 | 测试基线 | P0 | 部分实现 | [验收标准 Q-05](./research/2026-11-05-non-agent-workbench-requirement-research.md)；今日后端有单元与集成测试，前端编辑/删除/筛选用例未补 |

### 3.7 Agent 能力（AG）

来源：[通用 Agent 调研与当前实现对比](./research/2026-11-06-general-agent-comparison.md)。该文档是调研结论，不是已批准的设计；下列需求进入实现前必须先出设计与计划。

| 编号 | 需求 | 优先级 | 状态 | 调研 / 设计文档 |
| --- | --- | --- | --- | --- |
| AG-01 | Agent Loop 与类型化工具协议 | P0 | 提案 | [调研 §6 P0](./research/2026-11-06-general-agent-comparison.md) |
| AG-02 | 执行边界与副作用治理：权限、审批、沙箱 | P0 | 提案 | [调研 §6 P0](./research/2026-11-06-general-agent-comparison.md) |
| AG-03 | Workspace 与 Artifact | P1 | 提案 | [调研 §6 P1](./research/2026-11-06-general-agent-comparison.md) |
| AG-04 | Step 检查点、审批恢复与上下文工程 | P1 | 提案 | [调研 §6 P1](./research/2026-11-06-general-agent-comparison.md)、[Run 恢复 ADR](./architecture/ADR-001-agent-run-recovery.md) |
| AG-05 | Trace、Artifact 与 Eval | P1 | 提案 | [调研 §6 P1](./research/2026-11-06-general-agent-comparison.md) |
| AG-06 | Skills、MCP 与 Multi-Agent | P2 | 提案 | [调研 §6 P2](./research/2026-11-06-general-agent-comparison.md) |

## 4. 调研与设计文档绑定

正向索引在 §3 的表格里：每条需求指向支撑它的文档。这一节是反向索引：每份文档支撑了哪些需求。新增或更换调研文档时，两处都要改。

| 文档 | 类型 | 覆盖需求 | 状态 |
| --- | --- | --- | --- |
| [非 Agent 工作台需求调研](./research/2026-11-05-non-agent-workbench-requirement-research.md) | 调研 | T-01 至 T-11、S-01 至 S-06、N-01 至 N-04、A-01 至 A-07、Q-01 至 Q-05 | 已产出，需求待实现 |
| [今日服务端设计](./superpowers/specs/2026-11-05-craftingtable-today-server-design.md) | 设计 | A-01、A-02、A-03 | 已实现 |
| [今日服务端实施计划](./superpowers/plans/2026-11-05-craftingtable-today-server.md) | 计划 | A-01、A-02、A-03 | 已交付 |
| [通用 Agent 调研与当前实现对比](./research/2026-11-06-general-agent-comparison.md) | 调研 | AG-01 至 AG-06 | 调研结论，未进入设计 |
| [Agent 设计](./superpowers/specs/2026-11-05-craftingtable-agent-design.md) | 设计 | AG 系列的现有基线 | 已确认 |
| [Run 恢复 ADR](./architecture/ADR-001-agent-run-recovery.md) | 决策记录 | AG-04 | 已确认 |
| [工程化设计规范](./superpowers/specs/2026-11-05-craftingtable-engineering-design.md) | 规范 | Q-01、Q-05 及全部需求的流程约束 | 已确认 |

一份文档覆盖多个需求时，在单元格里用编号区间表示。需求被拆到新文档时，旧文档保留在表里并标注"已部分迁移"，方便追溯来源。

## 5. 已交付的里程碑

| 日期 | 里程碑 | 涉及需求 | 文档 |
| --- | --- | --- | --- |
| 2026-11-05 | 今日服务端契约：日期查询、固定宽度时间、软删与恢复 | A-01、A-02、A-03 | [设计](./superpowers/specs/2026-11-05-craftingtable-today-server-design.md)、[计划](./superpowers/plans/2026-11-05-craftingtable-today-server.md) |

## 6. 相关文档

- [非 Agent 工作台需求调研](./research/2026-11-05-non-agent-workbench-requirement-research.md)：T / S / N / A / Q 五个域的现状盘点与完整验收标准
- [今日服务端设计](./superpowers/specs/2026-11-05-craftingtable-today-server-design.md)：A-01 至 A-03 的契约与取舍
- [通用 Agent 调研](./research/2026-11-06-general-agent-comparison.md)：AG 需求的来源与差距分析
- [Agent 设计](./superpowers/specs/2026-11-05-craftingtable-agent-design.md)：Agent 现有契约
- [工程化设计规范](./superpowers/specs/2026-11-05-craftingtable-engineering-design.md)：SDD 流程与边界规则
- [Agent 架构图](./architecture/README.md)
- [术语表](./glossary.md)
