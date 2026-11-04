# Leeat

Leeat 是一个单用户个人事务工作台。当前仓库保留 React、TypeScript、Vite 原型和临时 Fastify/SQLite 实现；目标架构是 Desktop 优先、本地优先、Go 模块化单体后端和共享 React 前端。

## 当前范围

- 今日时间流和快捷待办。
- Light / Dark 主题。
- Agent 会话工作区的设计与后续实现。
- 一级导航、二级侧边栏和响应式布局。

相关文档：

- [术语表](docs/glossary.md)
- [工程化设计规范](docs/superpowers/specs/2026-11-05-leeat-engineering-design.md)
- [Agent 设计](docs/superpowers/specs/2026-11-05-leeat-agent-design.md)
- [个人工作台 MVP 设计规范](docs/superpowers/specs/2026-11-04-leeat-personal-workbench-design.md)
- [旧 Agent 工作区设计（已取代）](docs/superpowers/specs/20261104LeeatAgentWorkspaceDesign.md)

## 拟议设计基线

两份核心设计目前状态为“待评审”，确认后再作为实施基线。

- 平台顺序：Desktop -> Web -> 手机。
- 前端：React、TypeScript、Vite、ShadcnUI、Zustand、MVC。
- 后端：Go 模块化单体，本地版本使用 SQLite，Web 存储适配后续决定。
- Agent：单 Agent 起步，预留工具、Skills、MCP、多 Agent 和评测扩展点。
- 前端 Model 边界返回 `Result<T>`，Zustand 是业务状态唯一事实源。
- 产品行为、API、契约和桌面验收测试统一位于 `tests/e2e`。

## 后续计划：Agent Run 恢复与 Redis

状态：当前 MVP 采用中断后手动重试，长期恢复策略待决策。

当前 MVP 的边界：

- 浏览器断开不会自动终止服务端正在执行的 Run。
- 服务端进程重启后，遗留的 `running` 或 `queued` Run 标记为 `interrupted`。
- 不自动恢复原 Run；用户重试时创建新的 Run，并通过 `retryOfRunId` 关联原 Run。
- 已保存的 Message、MessagePart、Step 和 RunEvent 保留，不删除执行历史。

后续需要重新比较以下四种恢复层级，再决定是否升级：

1. 标记为 `interrupted`，由用户手动重试并创建新 Run。
2. 使用队列检测 stalled Run，从头重新执行任务。
3. 按 Step 保存检查点，从最后一个可靠边界继续原 Run。
4. 接入 Temporal、Inngest 等持久化执行引擎，通过事件历史或检查点恢复。

Redis 是后续确定要接入的基础设施，但不在此刻预设职责和方案。流式事件缓存、可恢复流、任务队列、租约、限流和缓存等候选用途，需要在接入前单独调研、对比并决策。

## 后续计划：独立 Agent Evaluation Platform

状态：已规划，暂不实施。

Agent 能力评测将作为独立项目推进，项目名暂定为 `leeat-agent-eval`。评测平台需要支持 SWE-bench、个人 Agent 场景、Skills、MCP、安全评测和多 Agent 编排评测。

评测平台独立立项的原因：

- SWE-bench 依赖 Python、Docker、镜像缓存和大体积日志。
- 评测会执行不可信仓库代码，需要独立的资源限制和沙箱。
- 评测任务运行时间长，不应与工作台 API 共用进程和数据库。
- 评测平台的发布节奏、依赖和运行环境与工作台不同。

### 平台边界

评测平台采用独立代码库和独立部署，内部仍是模块化单体。核心边界如下：

- `BenchmarkAdapter`：把 SWE-bench、个人事务、Skills 和 MCP 场景转换为统一评测任务。
- `CandidateAdapter`：通过 HTTP、CLI 或容器启动被测 Agent。
- `Grader`：执行官方测试、状态断言、轨迹检查、安全断言、LLM Judge 或人工评分。
- `Trace` 和 `Artifact`：作为平台长期保存的核心资产。
- `Experiment` 和 `Baseline`：用于版本比较和回归门禁。

平台上不复制工作台业务状态，也不共用工作台数据库。

### 工作台后续预留

开始建设评测平台前，工作台只需要逐步补齐：

- 无头 Agent Run 入口，可启动、查询、取消并获取结果。
- 统一 `runId`、`traceId` 和 Attempt 关联关系。
- `AgentRun`、`AgentStep`、工具调用、MCP 调用和子 Agent 调用记录。
- Patch、文件、测试日志和最终输出等 Artifact 清单。
- 模型、Prompt、Skill、Tool Schema 和 MCP Server 版本信息。
- Trace 数据脱敏和密钥隔离。

评测数据不进入前端 Zustand。Zustand 只保存工作台会话和 UI 状态。

### 实施顺序

1. 完成并稳定当前工作台和单 Agent 会话能力。
2. 工作台增加无头 Agent Run、Trace 和 Artifact 契约。
3. 新建 `leeat-agent-eval`。
4. 首先接入 SWE-bench 官方 Harness。
5. 接入 Leeat 个人事务 Agent 和内部回归数据集。
6. 增加 Skills、MCP、安全和多 Agent 编排评测。
7. 最后建设评测 Web UI、实验比较和排行榜。

本阶段不创建评测平台代码、数据库、页面或路由。

## 开发命令

```text
pnpm dev
pnpm typecheck
pnpm test:e2e
```
