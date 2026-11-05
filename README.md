# CraftingTable

CraftingTable 是一个单用户个人事务工作台。当前仓库使用 React、TypeScript、Vite、ShadcnUI、Zustand 和 Go 模块化单体后端；运行数据保存在本地 SQLite。

`leeat` 是当前唯一用户的标识，不作为项目名、产品名或领域模型前缀。

## 当前范围

- 今日时间流和快捷待办。
- Light / Dark 主题。
- Agent 会话工作区、独立会话侧栏和流式消息界面。
- 一级导航、二级侧边栏和响应式布局。

Agent 在开发模式通过 Go 服务端的 `/api/v1/agent/stream` 连接 OpenAI 兼容模型；浏览器不会读取模型 API Key。当前接口输出项目定义的 UI Message Stream 兼容子集，前端由 `agentTransport.ts`、`eventsource-parser`、`agentStreamProtocol.ts` 和 `apiAgentModel.ts` 组成三层 pipe-through。Vitest 和 Playwright 固定使用 `FakeAgentModel`，Go 集成测试使用 loopback Fake Provider，以保证结果可重复。

相关文档：

- [术语表](docs/glossary.md)
- [工程化设计规范](docs/superpowers/specs/2026-11-05-craftingtable-engineering-design.md)
- [Agent 设计](docs/superpowers/specs/2026-11-05-craftingtable-agent-design.md)
- [Agent 流协议状态机设计](docs/superpowers/specs/2026-11-05-craftingtable-agent-stream-state-machine-design.md)
- [Agent 架构图](docs/architecture/README.md)
- [早期个人工作台设计（已取代）](docs/superpowers/specs/2026-11-04-leeat-personal-workbench-design.md)
- [旧 Agent 工作区设计（已取代）](docs/superpowers/specs/20261104LeeatAgentWorkspaceDesign.md)

## 设计基线

两份核心设计状态为“已确认”，实施状态为“分阶段落地”。规范始终保留最高目标；阶段划分只决定交付顺序，不降低契约、安全、恢复和验收标准。

- 平台顺序：Desktop -> Web -> 手机。
- 前端：React、TypeScript、Vite、ShadcnUI、Zustand、MVC。
- 后端：Go 1.26 模块化单体，标准库 `net/http`，Desktop 使用 SQLite，Web 存储和横向扩展方案在进入 Web 实施前通过架构决策记录确定。
- Agent：单 Agent 起步，预留工具、Skills、MCP、多 Agent 和评测扩展点。
- 架构采用策略：目标架构完整采用，系统能力渐进落地；规范不因 MVP 缩水，也不提前复制外部项目的全部技术栈和进程拓扑。
- 前端 Model 边界返回 `Result<T>`，Zustand 是业务状态唯一事实源。
- 开源项目采用原则：Vercel AI SDK、`mimo-chat` 和 `aisdk-go` 只作为设计与测试参考，不进入生产依赖。吸收后的实现必须用 CraftingTable 自己的类型、错误模型、Adapter 和测试重新表达，不直接移植其对话层、业务层或完整 Agent 架构。
- 只有职责单一且不拥有业务语义的通用工具包可以直接依赖，例如 `eventsource-parser` 用于 SSE 清理、`lodash` 用于通用数据操作。项目自己维护 HTTP Transport、协议子集映射和领域消息 Adapter。
- Agent 前端由 `agentTransport.ts`、`agentStreamProtocol.ts` 和 `apiAgentModel.ts` 分层处理流；`eventsource-parser` 只负责 SSE 分帧，不承担消息状态或领域语义。
- 开发流程遵循 SDD：先更新设计与实施计划，再写失败测试和最小实现；未完成文档评审的变更不进入生产代码。
- 产品行为、API、契约和桌面验收测试统一位于 `tests/e2e`。
- Web 身份、多端同步、多实例租约、可恢复流和 Redis 职责必须在对应架构决策门完成后才能进入生产实现。

## 后续计划：Agent Run 恢复与 Redis

状态：当前实施阶段采用中断后手动重试。可恢复流、跨进程恢复和 Redis 职责必须完成方案对比后再进入生产实现。

当前实施阶段的执行边界：

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

Agent 能力评测将作为独立项目推进，项目名暂定为 `craftingtable-agent-eval`。评测平台需要支持 SWE-bench、个人 Agent 场景、Skills、MCP、安全评测和多 Agent 编排评测。

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
3. 新建 `craftingtable-agent-eval`。
4. 首先接入 SWE-bench 官方 Harness。
5. 接入 CraftingTable 个人事务 Agent 和内部回归数据集。
6. 增加 Skills、MCP、安全和多 Agent 编排评测。
7. 最后建设评测 Web UI、实验比较和排行榜。

本阶段不创建评测平台代码、数据库、页面或路由。

## 开发命令

```text
Copy-Item .env.example .env.local
# 在 .env.local 中填写模型配置
pnpm dev
pnpm typecheck
pnpm test
pnpm test:go
pnpm test:e2e
pnpm build
```

`.env.local` 已被 Git 忽略，`.env.example` 只保留非敏感默认值。开发模式通过 `go run ./server/cmd/craftingtabled -env-file=.env.local` 启动后端；需要无网络调试时，可在本地环境显式设置 `VITE_AGENT_SOURCE=fake`。

当前 Agent 会话和消息仍保存在浏览器进程内，刷新页面不会恢复。SQLite 会话持久化、Electron IPC、身份系统和生产级流恢复仍属于后续实施阶段。
