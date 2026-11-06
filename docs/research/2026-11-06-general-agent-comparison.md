# CraftingTable 通用 Agent 调研与当前实现对比

日期：2026-11-06
分支：`leeatMvp`
状态：调研结论，不是已批准的实施设计
范围：通用 Agent 运行时、执行边界、工具循环、权限、恢复、上下文、观测和生态能力

## 1. 结论

CraftingTable 当前不是完整的通用 Agent，而是一个可靠性较好的单 Agent 对话运行时：

- 已经支持多轮对话、流式输出、Reasoning 展示、持久化、刷新恢复、取消、超时、重试和幂等。
- 还没有形成“模型决策 -> 工具调用 -> 环境反馈 -> 继续推理 -> 完成判断”的执行闭环。
- 已经有一个较强的 Run 控制面，但缺少真正的行动面：工具、隔离工作区、审批、工具结果和 Artifact。

一句话定位：

> CraftingTable 已有 Agent Run 的控制面，但还没有通用 Agent 的行动面。

当前最值得补的不是多 Agent，也不是 MCP，而是单 Agent 的工具循环。引入真实写操作或本地命令前，必须先补齐执行边界、权限裁决和隔离工作区。

## 2. 调研口径

本报告参考以下官方资料，访问日期均为 2026-11-06：

| 资料 | 主要参考价值 |
| --- | --- |
| [Anthropic: Building effective agents](https://www.anthropic.com/research/building-effective-agents) | Workflow 与 Agent 的区别、最小 Agent 循环、停止条件、沙箱和评测 |
| [OpenAI Agents SDK](https://openai.github.io/openai-agents-python/) | Agent Loop、tools、sessions、handoffs、guardrails、MCP、tracing |
| [OpenAI Agents SDK: Human-in-the-loop](https://openai.github.io/openai-agents-python/human_in_the_loop/) | `interruptions`、可序列化 `RunState`、审批恢复、幂等副作用 |
| [OpenAI Agents SDK: Sandbox Agents](https://openai.github.io/openai-agents-python/sandbox_agents/) | 持久工作区、文件系统、Shell、Skills、快照和恢复 |
| [Claude Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) | Agent Loop、内置工具、Sessions、Hooks、Subagents、MCP、Skills |
| [Claude Agent SDK: Permissions](https://code.claude.com/docs/en/agent-sdk/permissions) | Hooks、deny/ask/allow 规则、`canUseTool`、权限与系统沙箱的区别 |
| [Google Agent Development Kit](https://adk.dev/) | Multi-Agent、Graph Workflow、Evaluation、Deployment、Tracing |
| [OpenHands SDK Architecture](https://docs.openhands.dev/sdk/arch/overview) | 软件 Agent、Bash/文件工具、Docker/Remote Workspace、Context Condenser |
| [LangGraph](https://docs.langchain.com/oss/python/langgraph/overview) | 有状态编排、durable execution、checkpoint、HITL、Memory |
| [Microsoft Agent Framework](https://learn.microsoft.com/en-us/agent-framework/overview/agent-framework-overview) | Graph Workflow、长任务、状态管理、上下文压缩、可观测性 |
| [Model Context Protocol](https://modelcontextprotocol.io/specification/2025-11-25) | Tools、Resources、Prompts 等外部能力接入协议 |

## 3. 通用 Agent 的核心闭环

Anthropic 明确区分两类 agentic system：

- Workflow：由预先定义的代码路径编排模型和工具。
- Agent：由模型动态决定步骤和工具使用方式。

通用 Agent 的基础单位是增强模型：模型加检索、工具和记忆。实际执行通常是一个循环：

```text
用户目标
  -> 组装上下文
  -> 模型推理与决策
  -> 选择工具和参数
  -> 权限裁决 / 人工审批
  -> 在受控环境执行
  -> 获取真实环境结果
  -> 将结果写回上下文
  -> 继续推理
  -> 满足停止条件或请求人工介入
  -> 提交最终回答和 Artifact
```

一个可用的通用 Agent 至少要回答五个问题：

| 问题 | 对应能力 |
| --- | --- |
| 模型能做什么 | Tool Registry、工具 Schema、Skills、MCP |
| 模型在哪里做 | Workspace、沙箱、文件系统、Shell、网络边界 |
| 什么操作可以自动做 | Allow / Ask / Deny、审批中断、审计、幂等 |
| 执行失败后怎么办 | Run/Step 状态机、Checkpoint、恢复、补偿 |
| 如何证明它做对了 | Trace、Artifact、Eval、回归数据集 |

不是所有任务都值得做成自主 Agent。Anthropic 的建议是先用最简单可行的调用或 Workflow；只有任务确实需要动态步骤、环境反馈和迭代时，再升级到 Agent。

## 4. 主流方案对比

### 4.1 能力矩阵

“部分”表示项目具备相关基础，但不是当前核心能力或仍需应用自行组装。

| 方案 | Agent Loop | 工具与生态 | 隔离工作区 | 恢复与 HITL | 上下文与记忆 | 观测与评测 |
| --- | --- | --- | --- | --- | --- | --- |
| CraftingTable | 无动态循环，当前只有一次模型 Step | 无工具、Skills、MCP | 无 | 强：Run 持久化、游标恢复、取消、终态裁决；无 Step 检查点和审批 | 部分：最近轮次和字符截断，无摘要、检索和长期记忆 | 部分：durable RunEvent；无 Trace 平台和 Eval |
| OpenAI Agents SDK | 内建 Runner 循环，直到模型判定结束 | Tools、MCP、Handoffs、Guardrails | Sandbox Agents，支持工作区、快照和恢复 | `interruptions`、可序列化 `RunState`、暂停审批后恢复 | Sessions 保存对话；沙箱 Memory 和 compaction | 内建 Tracing；Eval 需应用或外部平台 |
| Claude Agent SDK | 复用 Claude Code 的 Agent Loop | 内置工具、自定义工具、MCP、Skills、Subagents | 权限控制加 sandboxed Bash | Sessions resume/fork；Hooks；审批回调 | 上下文管理、项目 Skills、项目记忆 | Hooks、OpenTelemetry、checkpoint、cost/usage |
| Google ADK | 单 Agent 与 Multi-Agent 编排 | Tools、MCP、Action confirmation、Skills | Agent Runtime 和部署环境 | Session/State、Resume/Cancel；Graph Workflow | Session state；需要按应用补充长期记忆 | Evaluation、Traces、Metrics、User/Environment Simulation |
| OpenHands | 面向软件任务的多步推理执行 | Bash、文件编辑、Web、MCP、Task Tool | 本地、Docker、Remote/Kubernetes Workspace | 持久化、状态事件、远程 Agent Server | Context Condenser、Persistent Memory | Metrics、Observability、Tracing |
| LangGraph | 低层 Graph 运行时，模型步骤与确定性步骤混排 | Tool 由 LangChain/应用提供 | 本身不提供系统沙箱 | Durable execution、Checkpoint、Interrupt、Time travel | 短期线程状态、长期 Store、Memory | Streaming 和 LangSmith 观测 |
| Microsoft Agent Framework | Agent 与 Function 的 Graph/Workflow | 多模型、工具、上下文提供器、中间件 | 主要交给部署环境 | 长任务状态、HITL、Don't-ask-again 审批 | Context compaction、File access、Memory | Telemetry、Evaluation service 集成 |

### 4.2 关键参考结论

#### OpenAI Agents SDK

- 把 Agent Loop 作为默认执行模型，应用不需要自己写“模型最多循环几次”的框架壳。
- HITL 不是普通确认弹窗，而是 Run 级中断：待审批工具调用进入 `interruptions`，运行状态可以序列化，之后恢复原 Run。
- 审批只表示“允许执行”，不保证 exactly-once。有外部副作用的工具必须使用稳定幂等键，并在无法确认结果时先对账。
- Sandbox Agents 把工作区、文件、Shell、Skills、快照和会话恢复放在同一执行边界内。

#### Claude Agent SDK

- 重点不是只暴露模型 API，而是复用完整 Agent Loop、上下文管理和内置工具。
- Session 可以继续、恢复和 fork；Hooks 可以拦截工具前后行为。
- 权限评分是分层策略：Hooks、deny、ask、permission mode、allow、`canUseTool`。
- 权限规则不等于操作系统隔离。文档明确要求命令执行还需要 sandboxed Bash；脚本仍可能绕过只针对工具调用的路径规则。

#### OpenHands

- 通用软件 Agent 的核心不是聊天，而是真实 Workspace、可执行环境和观察结果。
- 同一 Agent 代码可以切换 Local、Docker 和 Remote Workspace，说明 Workspace 应是独立 Port，而不是塞进 Executor 的分支。
- Context Condenser、Persistent Memory、事件和 Tracing 都是长任务运行时的组成部分。

#### LangGraph

- 它提供低层有状态编排，不替应用规定 Agent 架构。
- Durable execution、Checkpoint、Interrupt 和 Time travel 说明“运行状态”不只是数据库里的一条 Run 记录，而是可恢复的 Step 级状态。
- 短期线程状态与长期 Memory 要分开建模。

#### Google ADK 与 Microsoft Agent Framework

- 两者都把单 Agent 之后的演进方向放在 Graph Workflow 和 Multi-Agent 编排上。
- 它们同时强调 Evaluation、Tracing、Deployment 和长任务状态，说明多 Agent 不是孤立能力，必须建立在观测和状态管理之上。

#### MCP

- MCP 解决模型与外部 Tools、Resources、Prompts 的互操作问题。
- MCP 不是执行沙箱，也不是审批系统，更不能替代内部 ToolCall、ToolResult、权限和幂等契约。

## 5. CraftingTable 当前能力盘点

### 5.1 已实现且可复用的基础

#### Run 控制面

- Session、Message、Part、Run、Step、RunEvent 和幂等记录保存在 SQLite。
- `RunCoordinator` 持有独立 Context；浏览器断开不会取消 Run。
- 支持取消、最长运行时间、Provider 空闲超时、终态有限重试和 Finalize 失败降级。
- 服务启动时把遗留 `running` Run 收敛为 `interrupted`。

#### 流与恢复

- Provider 增量先进入有界 Event Journal，再转换为 SSE。
- SSE 使用单调游标，前端通过 `runId + lastSequence` 恢复。
- 浏览器刷新、短暂断线和 Session 重新加载已覆盖。
- 慢订阅者不会阻塞 Provider；Journal 溢出后以前端重新对账作为兜底。

#### 幂等与终态

- 创建 Session、发送和重试都有持久化 `clientRequestId`。
- 同 Session 只允许一个运行中的 Run。
- 完成、取消、超时和 Finalize 失败经过统一终态裁决。

#### Reasoning

- 上游 `reasoning_content` 被转换为独立 Reasoning Part。
- Reasoning 流式展示、折叠、持久化和刷新恢复已实现。
- Reasoning 不进入下一轮模型上下文，避免把展示内容误当对话事实。

### 5.2 只有结构或设计预留

| 能力 | 当前事实 |
| --- | --- |
| Step | `agent_steps` 表存在，但只保存序号、状态和时间；当前业务固定创建一条模型 Step |
| Step 事件 | SSE 有 `start-step` / `finish-step`，但一次 Run 只有一段模型执行 |
| AgentExecutor | 设计中有完整 Executor；当前实现主要由 `RunCoordinator` 直接调用 Provider |
| RunEvent | 可记录生命周期，但还不是可检索的完整 Trace |
| 上下文管理 | 有轮次和字符上限，没有摘要、检索、压缩或 Step 级上下文快照 |
| Artifact | 文档和架构图有扩展位，当前没有持久化模型和执行路径 |

### 5.3 尚未实现

- Tool Registry、工具 Schema、ToolCall、ToolResult 和工具执行器。
- 模型与工具之间的多 Step 循环。
- 工具审批、权限策略、用户确认和暂停后恢复。
- 文件系统、Shell、Workspace、Docker 或远程沙箱。
- Skills、MCP Client、Subagent、Handoff 和 Multi-Agent。
- Step Checkpoint、跨进程接管和持久化执行引擎。
- Trace 平台、Artifact 清单和正式 Eval。
- 服务端多供应商归一化；该能力仍按既定计划后置。

## 6. 当前实现与通用 Agent 的主要差距

### P0：Agent Loop 与类型化工具协议

当前 Provider 接口只接收角色和文本消息：

```text
StreamProvider.Stream(ctx, []ProviderMessage) -> io.ReadCloser
```

它没有工具定义、工具选择、工具调用和工具结果语义。Coordinator 收到文本增量后直接结束 Run，因此无法让模型看到执行结果并继续下一轮。

需要补齐的不是一个 `tools` 请求字段，而是一组稳定领域契约：

```text
AgentExecutor
  -> ModelStep
  -> ToolCallStep
  -> ToolResultObservation
  -> ModelStep
  -> FinalAnswer
```

工具调用和结果必须成为有身份的 Part 或 Artifact 引用，不能压成一条普通文本消息。

### P0：执行边界与副作用治理

工具的输入首先不可信：

- 模型可能生成错误参数。
- 工具参数可能包含路径穿越、命令注入或越权资源。
- 外部请求可能在返回前已经产生副作用。

因此，工具执行至少需要：

1. Tool Schema 校验。
2. 应用级权限裁决。
3. 需要时创建审批中断。
4. 在受控 Workspace 中执行。
5. 记录结构化 Observation。
6. 对副作用工具使用稳定幂等键。

对于只读工作台数据工具，可以先没有 Docker，但仍需要独立 Tool Runtime、超时、取消和审计。对于文件、Shell 或外部写操作，系统沙箱不能省略。

### P1：Workspace 与 Artifact

通用 Agent 的“真实反馈”通常来自环境：

- 文件内容。
- 命令退出码和输出。
- 测试结果。
- 网页或外部系统响应。
- 生成的 Patch、报告、图片和日志。

当前只有消息文本，没有 Workspace 和 Artifact。若直接接入文件或 Shell，会把临时输出、长日志和最终答案混在 MessagePart 中，后续恢复、展示和评测都会失控。

### P1：Step 检查点、审批恢复与上下文工程

当前 Run 恢复解决“浏览器断线”，但不是“执行中断后继续完成”。

通用 Agent 需要至少保存：

- 已完成模型 Step 的输入摘要和输出引用。
- 待执行和已执行的 ToolCall。
- ToolResult 或失败状态。
- 待审批项和审批决定。
- 当前 Workspace / Sandbox Snapshot。
- 下一 Step 可安全恢复的位置。

上下文也需要从“截断最近消息”升级为 ContextManager，按模型 Step 组装：

- System Prompt。
- Skill 指令。
- 当前任务状态。
- 相关对话轮次。
- 最近工具 Observation。
- 检索或记忆。
- 预算和压缩策略。

### P1：Trace、Artifact 和 Eval

当前 `RunEvent` 能证明 Run 生命周期，但不能回答：

- 模型为什么调用这个工具。
- 工具实际输入和输出是什么。
- 哪一步开始偏离目标。
- 哪个模型、Prompt、Skill、Tool Schema 版本产生了该结果。
- 同一任务在不同版本下是否变好。

Trace 和 Eval 不应等到多 Agent 之后再做。单 Agent 工具循环一出现，失败模式就会明显增加。

### P2：Skills、MCP 和 Multi-Agent

- Skills 应先建立在稳定 ContextManager 和版本记录上。
- MCP 应复用内部 ToolCall、ToolResult、权限和超时语义。
- Subagent 和 Multi-Agent 应建立在父子 Run、Step、Artifact、Trace 和预算关系上。

在这些底层语义稳定前接 Multi-Agent，只会放大状态、授权和恢复问题。

## 7. 建议的目标结构

在保留现有工作台和恢复能力的前提下，建议把执行结构演进为：

```text
HTTP / SSE Access
  -> Application Controller
    -> Run Coordinator
      -> AgentExecutor
        -> ContextManager
        -> ModelRuntime
        -> PermissionPolicy
        -> ToolRuntime
          -> Tool Registry
          -> Workspace Port
            -> Local Workspace
            -> Sandbox Workspace
        -> Observation / Artifact Writer
        -> Run/Step State Store
        -> Trace / Event Sink
```

关键边界：

- `RunCoordinator` 管执行所有权、取消、并发和终态，不实现工具分支。
- `AgentExecutor` 管模型与工具的多 Step 循环和停止条件。
- `ModelRuntime` 只负责单次模型调用及其供应商协议转换。
- `ToolRuntime` 管工具查找、参数校验、超时、执行和结构化结果。
- `PermissionPolicy` 在工具执行前裁决 allow / ask / deny。
- `Workspace` 隔离文件与命令；本机、Docker 和远程实现通过 Port 替换。
- `ContextManager` 负责 Step 级上下文组装、压缩和记忆注入。
- Trace 与 Artifact 从第一天进入工具循环，而不是事后补日志。

## 8. 演进路线

### 路线 A：工具闭环优先

先做无副作用工具：

1. 抽出 `AgentExecutor`、`ToolRuntime` 和类型化 ToolCall / ToolResult。
2. 支持模型多 Step 循环和 `maxSteps` / `maxToolCalls` 停止条件。
3. 先接只读工作台工具，例如查询 Today、会话摘要和只读统计。
4. 同步补工具状态 UI、Trace、Artifact 和 Fake Tool Provider 测试。

优点：最快验证 Agent Loop、协议和 UI。
限制：不能先接 Shell、写文件或外部写接口。

### 路线 B：安全执行平台优先

先做 Workspace、Sandbox、权限和审批，再接工具。

优点：从第一天按真实副作用设计，适合文件、代码和命令型 Agent。
缺点：距离首个可感知 Agent 能力更远，需要先承担容器和生命周期复杂度。

### 路线 C：Trace / Eval 优先

先把 RunEvent 升级成 Trace、Artifact 和版本化 Eval，再做 Loop。

优点：后续每次能力扩展都有可比较证据。
缺点：用户暂时得不到新的行动能力，研究成本较高。

### 推荐：A 起步，B 作为写操作门槛

最合理的顺序是：

1. 先建立 AgentExecutor、工具协议和只读工具闭环。
2. 在第一个写工具之前完成 PermissionPolicy、审批中断、幂等和 Workspace / Sandbox。
3. 再引入文件、Shell、外部写操作和 Artifact。
4. 工具语义稳定后接 Trace/Eval。
5. Trace/Eval 稳定后接 Skills 和 MCP。
6. 单 Agent 和评测稳定后再考虑 Subagent / Multi-Agent。

这条路线既不会把安全无限后置，也不会为了尚未出现的 Shell 能力提前建设完整容器平台。

## 9. 对现有设计的直接影响

- 现有 Session、Run、Event Journal、幂等和终态裁决继续保留。
- `agent_steps` 需要增加 `kind`、输入摘要、输出引用、错误、尝试次数和时间信息。
- 新增 ToolCall / ToolResult / Approval / Artifact 的稳定 Schema 和迁移。
- `StreamChunk` 需要增加工具生命周期事件；未知事件仍遵循现有协议兼容规则。
- 前端需要显示模型思考中、工具执行中、等待审批、工具失败和继续生成等状态。
- 多供应商字段仍由服务端归一化；前端只消费统一 ToolCall / ToolResult 契约。
- Reasoning 继续作为独立展示 Part，不与工具 Observation 和普通正文混合。

## 10. 明确不做或继续后置

- 本轮不改多供应商归一化。
- 本轮不引入 Multi-Agent。
- MCP 不先于内部工具协议。
- Reasoning Markdown 渲染继续作为独立 TODO。
- Reasoning 计时器和 usage 展示继续后置。
- 不把 Vercel AI SDK、OpenAI Agents SDK 或其他框架直接作为生产运行时依赖。
- 不把工具循环写入 HTTP Controller。
- 不把完整 Chain-of-Thought 当作产品能力；只展示上游明确提供的 Reasoning 或摘要。

## 11. 待确认的产品选择

第一条真实工具链决定 Workspace、审批和 Eval 的优先级。需要先确定 CraftingTable 最想先擅长哪类任务：

1. 工作台事务：Today、计划、个人数据。
2. 文件与代码：读写仓库、运行测试、生成 Patch。
3. 外部服务：日历、邮件、文档或其他 MCP 服务。

如果是工作台事务，路线 A 可以最快产生价值。
如果是文件与代码，路线 A 之后必须很快进入路线 B，不能只做进程内工具伪装成通用 Agent。
