# CraftingTable 工程化设计规范

日期：2026-11-05
状态：已确认
实施状态：分阶段落地
范围：CraftingTable 的代码组织、架构边界、错误处理、契约、测试、安全与文档治理
适用范围：Desktop、Web，以及后续移动端共用的前端代码
关联设计：[CraftingTable Agent 设计](./2026-11-05-craftingtable-agent-design.md)
术语基线：[CraftingTable 术语表](../../glossary.md)

## 1. 目的

本文定义 CraftingTable 在进入实现和持续迭代后必须遵守的工程规则。

规范的目标不是复制大型团队流程，而是让以下问题在提交代码前就能被发现：

- View、Store、Effect 和 Model 的依赖方向被打乱。
- API、流事件和前端类型逐渐不一致。
- 网络错误、取消和协议错误混进组件。
- 主题值、日志和 Electron 安全边界被绕过。
- 文档继续描述已经废弃的技术方案。
- 测试数量增加，但关键失败路径仍没有覆盖。

本文只保留能够降低 CraftingTable 返工成本的规则。每条规则都应能通过代码结构、类型、测试、脚本或评审清单验证。

本文是长期最高规格，不因产品所处生命周期或当前交付范围而删减要求。产品范围和实施顺序可以分阶段，规范目标不能降级。每项能力必须标记为 `required`、`bootstrap` 或 `enforcing`，并明确验收命令和转为阻塞门禁的时点。

## 2. 核心原则

1. **边界优先于抽象数量**：只有承担真实职责的模块才创建接口和目录，不为形式完整增加空层。
2. **单一事实源**：业务状态、错误码、协议、术语和设计 Token 都只能有一个权威来源。
3. **错误是数据**：I/O 和协议失败通过 `Result<T>` 返回；程序缺陷仍使用异常，由错误边界处理。
4. **Parse，don't cast**：外部数据先校验再使用，禁止把 `unknown`、JSON 或第三方响应直接断言成领域类型。
5. **完整目标，分阶段强制**：规范保留全部最终门禁；开发期按改动范围增量验证，但不得把未启用描述成不需要。
6. **桌面端不是浏览器**：Electron 的 IPC、文件、外链、快捷键、离线启动和打包行为必须在真实桌面进程中验证。
7. **文档描述当前事实或明确标注状态**：过时方案不继续伪装成当前设计。
8. **规范高于当前实现**：现有原型不满足规范时，结论是迁移实现，不是降低规范。
9. **开源引进吸收再创新**：开源项目作为设计与实现样本，先理解约束、取舍和失败模式，再用本项目自己的领域语言重新设计；不直接拿来，不整包照搬，也不复制未经理解的实现。
10. **SDD 先于实现**：功能、行为或架构发生变化时，先写或更新设计文档，再形成实施计划，最后用失败测试驱动最小实现。未评审的设计不能直接进入生产代码。

### 2.1 架构采用策略

CraftingTable 采用“**目标架构完整采用，系统能力渐进落地**”的策略。

- “完整采用”指最终契约、模块边界、状态所有权、Run 生命周期、Port/Adapter 方向和验收标准一次性定稿，不随 MVP 缩水。
- “渐进落地”指具体组件按真实约束引入，不因为目标架构中存在某个组件就提前复制其技术栈、目录数量或运行复杂度。
- 目标架构只吸收能够降低长期返工的结构原则，不照搬外部项目的全部业务模块、基础设施或部署方式。
- 每项延迟实现都必须在本文标记触发条件和进入生产前必须满足的门槛，不能用“以后再说”代替契约设计。
- 不创建没有真实职责的空层、空接口或占位模块。接口在边界需要稳定或存在真实替换需求时建立。
- 阶段实现可以暂时使用 SQLite、单进程执行和直接 SSE，但不得改变未来 PostgreSQL、Daemon、Redis 和多实例机制需要遵守的领域语义。

### 2.2 开源项目采用规则

成熟开源项目用于验证方向、发现边界条件和比较实现，不作为直接复制业务架构的理由。引入一个项目时，先回答以下问题：

1. 它解决的具体问题是什么？
2. 哪些约束与 CraftingTable 相同，哪些不同？
3. 哪些实现值得吸收，哪些只适用于原项目的产品形态？
4. 本项目应保留什么更简单的边界？
5. 如何用当前项目的测试证明吸收后的行为正确？

吸收结果必须用 CraftingTable 的目录、类型、错误模型和测试重新表达。禁止直接拿来外部项目的整个业务层、状态机、目录树或未使用模块。

通用工具包可以直接依赖，但必须满足以下条件：

- 包的职责单一，例如 `eventsource-parser` 只处理 Server-Sent Events（SSE）解析。
- 包不拥有 CraftingTable 的业务状态、领域协议或持久化语义。
- 包可以锁定版本，许可证清晰，并能在 Adapter 边界替换。
- 升级时可以用项目测试验证行为，不依赖人工观察。

例如 `eventsource-parser` 和 `lodash` 可以直接使用。Vercel AI SDK、`mimo-chat` 等包含完整产品链路或 Agent 协议设计的项目，应作为参考基线，通过研究记录、设计对比和项目内 Adapter 吸收，不直接粘贴其业务实现。这里的“直接使用”只适用于不会成为本项目业务状态、领域协议或 Agent 架构事实源的工具包。

## 3. 目标与非目标

### 3.1 目标

- 前端保持 React、TypeScript、Vite、ShadcnUI、Zustand 和 MVC。
- 后端保持 Go 模块化单体，当前阶段不拆分微服务。
- Desktop 采用本地优先架构，Web 复用同一套 API 和领域契约。
- MVC 的 Model、Effect、Store、View 有明确且可检查的依赖方向。
- Agent 流式协议能够独立测试，不依赖真实模型。
- 产品行为、API、契约和桌面验收测试统一放在 `tests/e2e`。
- 设计系统、日志安全和文档链接有自动检查入口。
- 首个可交付版本是 Windows Desktop，Go sidecar 同时承载今日和 Agent；Fastify 原型只作为迁移期间的历史对照实现。

### 3.2 阶段边界

以下只是当前实施阶段的非目标，不改变长期规范：

- 当前阶段不复制外部项目的全部进程拓扑、基础设施和业务模块；只采用模块化单体、显式组装、Port/Adapter 和清晰状态所有权等目标原则。
- 当前阶段不引入 Monorepo、Turborepo、Feature Flag、MR Gate、多环境分支或生产从库。
- 不引入 TanStack Query 代替 Zustand 业务状态。
- 不强制所有函数都返回 `Result<T>`。
- 不为个人项目复制大型团队的组织流程和发布审批。
- 不在此阶段建设独立 Agent Evaluation Platform。

### 3.3 反目标

以下结果即使功能可用，也视为工程失败：

- 页面可以运行，但 View 直接请求 API 或导入 Model。
- Store 保存一份状态，第三方 Chat Hook 或 SDK 内置 Store 又保存另一份可修改状态。
- SSE 已发送成功，但数据库中的消息和前端显示无法对应。
- 每次 token 都写数据库，导致消息存储快速膨胀。
- API 新增字段后，前端静默显示空白或崩溃。
- 日志中包含 API Key、Token、完整路径或用户消息正文。
- Chrome 中通过，但 Electron 安装包无法启动或无法执行核心操作。

## 4. 目标目录结构

目录以职责命名，不使用含义不明的缩写目录。

```text
src/                          React View、Controller 和前端 Model
  components/
  controllers/
    effects/
    stores/
  layouts/
  models/
  pages/
  router/
  styles/

electron/                     Electron 主进程与 preload
  main/
  preload/

server/                       Go 模块化单体
  cmd/craftingtabled/
  internal/
    agent/
      domain/
      ports/
      core/
      application/
      access/
      adapters/
        provider/
        persistence/
    today/
    platform/
      httpapi/
      store/

contracts/                    跨端类型、Schema 和协议样例
tests/e2e/                    验收、契约和产品行为测试
  unit/
  integration/                API 黑盒、契约和 Go 外部测试
  modules/
  desktop/
  support/

docs/
  architecture/
  harness/
  superpowers/
    specs/
    plans/
scripts/                      检查脚本和开发工具
```

`contracts` 在接口尚未稳定时可以先保存手写类型和样例。接口稳定后，应以 Go 服务端或独立 JSON Schema 为源生成 TypeScript 类型，并增加漂移检查。

仓库根目录保留一个 `go.mod`，`server/` 属于该模块。这样既维持 `server` 的代码边界，又能让根级 `tests/e2e/integration` 中的 Go 黑盒测试被明确纳入测试命令。Go 命令固定为：

```text
go test ./server/... ./tests/e2e/integration/...
```

## 5. MVC 与依赖边界

### 5.1 前端依赖方向

```text
View
  ├─ 读取 -> Store
  └─ 动作 -> Effect
               ├─ 调用 -> Agent/Today Model -> Transport / ApiModel -> Go API
               └─ 同步写入 -> Store
```

允许的依赖：

- View 可以读取 Store，并调用 Effect 暴露的动作。
- Effect 可以调用 Model，并调用 Store 的同步 action。
- Store 只能依赖类型、纯函数和同步工具。
- Model 可以依赖契约、Zod Schema、Transport 和 API 地址。

禁止的依赖：

- Store 导入 Model、Effect、React 组件或执行 `fetch`。
- Model 导入 Store、Effect、页面或 React 组件。
- View 直接导入 Model、执行 `fetch`、访问数据库或拼装 SSE。
- Effect 保存可跨页面恢复的长期业务状态。
- 页面通过深路径修改 Store 内部字段。

### 5.2 Controller 分工

Controller 固定拆为 `effects` 和 `stores`。

Store 负责：

- 保存页面业务状态。
- 提供同步 action。
- 提供派生 selector。
- 保证同一业务事实只有一份可修改状态。

Effect 负责：

- 调用 Model。
- 处理请求生命周期和流消费。
- 维护 `AbortController` 等临时资源。
- 将结果写入 Store。
- 将错误转换成用户可理解的 Toast 或页面状态。

Effect 的闭包或 `ref` 可以保存 `AbortController`、当前订阅等短期资源，但不能保存刷新后仍需恢复的业务数据。

### 5.3 后端依赖方向

Go 后端采用模块化单体：

```text
Experience
  -> Access
  -> Application
  -> Core

Adapters / Infrastructure
  -> 实现 Application 或 Core 拥有的 Port

Bootstrap composition root
  -> 创建具体 Adapter
  -> 注入 Port 实现
```

| 层 | 包边界 | 允许依赖 | 禁止依赖 |
| --- | --- | --- | --- |
| Experience | `src`、`electron` | Access 暴露的 HTTP/SSE 与受控桥接接口 | Go Domain、Repository、Provider SDK |
| Access | `server/internal/agent/access`、`platform/httpapi` | Application、Domain 的公开契约 | Core 内部执行状态、具体 Repository |
| Application | `server/internal/agent/application` | Core、Application/Core 定义的 Port、Domain | HTTP Controller、SSE 编码器、Provider SDK、具体数据库 |
| Core | `server/internal/agent/core`、`agent/domain`、`agent/ports` | Domain、Port 和标准库 | Access、Adapters、SQLite、PostgreSQL、Electron、React |
| Adapters / Infrastructure | `agent/adapters`、`platform` | Port、Domain、具体 SDK 或数据库驱动 | UI、HTTP Handler、Application 用例内部实现 |

端口所有权规则：

- `RuntimePort` 和 `RepositoryPort` 等接口由 Application/Core 需要的一方定义，不按具体供应商或数据库设计。
- Provider Adapter 实现 `RuntimePort`；Persistence Adapter 实现 `RepositoryPort`。
- Core 只持有接口和领域对象，不知道当前注入的是 Fake Runtime、OpenAI-compatible Provider、SQLite 还是 PostgreSQL。
- 具体实现只在 `server/cmd/craftingtabled` 的 composition root 中创建和注入。
- Logger、Clock、ID 和 Telemetry 通过基础设施接口注入，业务代码不直接导入厂商 SDK。

HTTP Controller 只负责解码、校验、调用 Application 用例和序列化响应。Repository 不依赖 HTTP，Agent Runtime 不直接访问数据库，数据库事务内不调用模型或其他外部 I/O。

Agent 的执行生命周期不能绑定到一次 HTTP 连接：

- HTTP 请求 context 只控制响应写入和 SSE 订阅。
- 创建 Run 时由 Run Coordinator 建立独立的 run-scoped context。
- 浏览器断开后，Run 可以继续执行；用户取消、进程退出或服务关闭通过 Coordinator 取消 run context。
- HTTP Handler 不得把 `request.Context()` 直接传给模型调用。

## 6. TypeScript 规范

- 开启 TypeScript `strict`，不关闭 `noImplicitAny` 等严格检查。
- 外部输入默认使用 `unknown`，通过 Zod 或显式类型守卫收窄。
- 禁止用 `as SomeDomainType` 跳过运行时校验；类型断言只允许用于已经验证或语言本身无法表达的场景。
- 自建函数统一使用箭头函数；React 组件仍按 shadcn 现有写法处理。
- 文件命名沿用现有 lowerCamelCase，例如 `agentStore.ts`；React 组件名使用 PascalCase。
- 公共 API、Model/Effect 边界、流、事务、重试和安全逻辑必须使用完整中文 TSDoc，包含职责、参数、返回值、副作用和失败语义。
- 简单内部函数由类型、命名和测试表达职责；只有非显然行为、竞态、性能和约束需要注释。禁止生成重复代码字面含义的机械 TSDoc。
- 注释解释原因、约束、竞态和业务规则，不复述代码字面含义。
- 注释与实现不一致视为缺陷。

### 6.1 流协议与第三方依赖

Agent 前端吸收 Vercel AI SDK 和 `mimo-chat` 的分层方法。Vercel AI SDK、`mimo-chat` 和 `aisdk-go` 不进入生产依赖，也不直接移植其对话层或业务层。完整协议和职责表以 [Agent 设计 11.1](./2026-11-05-craftingtable-agent-design.md#111-协议选择与三层-pipe-through) 为准。

固定链路：

1. `agentTransport.ts` 发起请求并取得标准 `Response`。
2. `agentStreamProtocol.ts` 使用通用 `eventsource-parser` 清理 SSE，再把本项目需要的 chunk 转换为 `AgentStreamEvent`。
3. `apiAgentModel.ts` 消费领域事件，由现有消息状态逻辑生成 `AgentMessageSnapshot`。

协议层只暴露 `text-delta`、`finish` 和 `error` 等业务需要的语义。未知的非终止 chunk 可以忽略，已知结构错误必须进入稳定错误路径。禁止在 `src` 中重复实现以下逻辑：

- SSE 行分块、`data:` 拼接和跨网络 chunk 缓冲。
- 把第三方工具包的内部消息对象当成 Zustand 业务状态。
- 为同一份文本同时维护 SDK 消息、Store 消息和组件本地消息。
- 把外部项目的工具调用、Artifact 或多 Agent 编排原样复制进当前 Runtime。

依赖选择遵循以下顺序：

1. 只有通用、边界清晰且不拥有业务语义的工具包可以直接依赖。
2. 协议、消息状态、会话状态和 Agent 编排必须由项目自己的类型与测试定义。
3. 参考实现用于研究，不直接引入其业务模块、状态容器或目录结构。
4. 外部类型止步于 Adapter 边界，不进入 Store、领域对象或跨端契约。
5. 依赖必须锁定精确版本或 commit，并记录许可证和第三方声明。
6. 依赖升级必须通过 Agent 协议 fixture、浏览器 E2E 和 Desktop bridge 测试。

当前实现基线：

- 前端直接依赖 `eventsource-parser`，只使用其 SSE 清理能力。
- `agentTransport.ts`、`agentStreamProtocol.ts` 和 `apiAgentModel.ts` 由项目维护，吸收 Vercel AI SDK 与 `mimo-chat` 的分层方法。
- Go 侧在 `server/internal/agent/access` 中实现最小的 UI Message Stream 编码，不直接依赖完整 AI SDK 类库。
- Vercel AI SDK、`coder/aisdk-go` 和 `nmn28/aisdk-go` 仅作为协议研究与测试对照，不进入生产依赖。

## 7. Go 规范

- 使用 `gofmt` 和 `go vet`，不手工对抗标准格式。
- 导出类型和函数使用中文 Go Doc；复杂内部函数也必须有职责说明。
- 领域错误使用 `error` 值并通过 `errors.Is`、`errors.As` 判断，不依赖错误字符串。
- 对外错误必须有稳定错误码；内部错误使用 `%w` 保留因果链。
- 只有启动期不变量被破坏时才允许 `panic`，请求处理路径不得 panic 作为控制流。
- 所有外部调用接受 `context.Context`，取消和超时向下传递。
- 执行取消与终态持久化使用不同 context。执行 context 取消后，终态写入使用 `context.WithoutCancel` 派生出的有界持久化 context，确保取消、失败和中断状态能够落库。
- 数据库查询必须有边界，列表默认分页或限量，禁止无界全表读取。
- 会话、消息、事件和日志查询都必须使用 limit 或游标分页；默认上限写入 API 契约，不能只靠调用方自觉传参。
- 事务内只执行数据库操作，不调用模型、HTTP、文件系统或对象存储。
- 自建 Go 文件使用 lower_snake_case，例如 `agent_controller.go`；数据库表和列使用 snake_case。

## 8. Result 与错误契约

### 8.1 前端边界结果

Model 和协议适配器统一返回：

```ts
type Result<T, E extends CraftingTableError = CraftingTableError> =
  | readonly [data: T, error: null]
  | readonly [data: null, error: E]
```

错误不是裸 `Error`：

```ts
type CraftingTableErrorCode =
  | "VALIDATION_ERROR"
  | "NETWORK_ERROR"
  | "REQUEST_CANCELLED"
  | "AGENT_SESSION_NOT_FOUND"
  | "AGENT_SESSION_BUSY"
  | "AGENT_REQUEST_ALREADY_PROCESSED"
  | "AGENT_REQUEST_ID_CONFLICT"
  | "AGENT_RUN_NOT_FOUND"
  | "AGENT_MESSAGE_NOT_RETRYABLE"
  | "AGENT_NOT_CONFIGURED"
  | "AGENT_PROVIDER_FAILED"
  | "AGENT_PROVIDER_TIMEOUT"
  | "AGENT_STEP_LIMIT_EXCEEDED"
  | "AGENT_INTERNAL_FAILED"
  | "PROTOCOL_INVALID_METADATA"
  | "PROTOCOL_UNEXPECTED_EOF"
  | "INTERNAL_ERROR"

type CraftingTableErrorKind =
  | "network"
  | "http"
  | "validation"
  | "not_found"
  | "protocol"
  | "auth"
  | "conflict"
  | "cancelled"
  | "server"
  | "unknown"

type CraftingTableError = {
  code: CraftingTableErrorCode
  kind: CraftingTableErrorKind
  message: string
  retryable: boolean
  requestId?: string
  details?: Record<string, unknown>
  cause?: unknown
}
```

规则：

- `message` 面向用户，不包含堆栈、密钥或第三方响应原文。
- `code` 面向程序分支，必须稳定、可枚举。
- `details` 只承载已经 Schema 化的业务恢复数据，不能放堆栈、第三方原文或任意字符串。
- `cause` 只用于开发日志和测试，不直接渲染。
- `cancelled` 是用户主动取消的正常结果，不显示错误 Toast。
- Effect 使用 `const [data, error] = await model.action()` 分支，不在每个调用点重复 `try/catch`。

### 8.2 异常边界

`Result<T>` 只覆盖 I/O 和协议错误。以下情况仍使用异常：

- 不可能状态被破坏。
- React 渲染错误。
- 程序员错误，例如 Store action 收到非法内部参数。
- 未预期的第三方库缺陷。

异常由 React ErrorBoundary、HTTP 全局恢复中间件或进程级错误处理接管。

### 8.3 HTTP 错误格式

非流式 API 使用：

```json
{
  "error": {
    "code": "AGENT_SESSION_NOT_FOUND",
    "kind": "not_found",
    "message": "会话不存在",
    "retryable": false,
    "requestId": "req_123",
    "details": {
      "sessionId": "session-id"
    }
  }
}
```

成功响应使用：

```json
{
  "data": {}
}
```

HTTP 状态码表达传输和资源状态，`code` 表达业务分支，两者不能互相替代。

Agent 错误矩阵固定如下；“流内”表示 HTTP 已经进入 SSE，只能通过 `data-agent-error` 和 `error` chunk 表达：

| code | HTTP 或流内 | kind | retryable | details |
| --- | --- | --- | --- | --- |
| `VALIDATION_ERROR` | 400 | validation | false | 字段路径和可公开原因 |
| `REQUEST_CANCELLED` | 客户端流桥 | cancelled | true | `requestId` |
| `AGENT_SESSION_NOT_FOUND` | 404 | not_found | false | `sessionId` |
| `AGENT_SESSION_BUSY` | 409 | conflict | true | `sessionId`、当前 `runId` |
| `AGENT_REQUEST_ALREADY_PROCESSED` | 409 | conflict | false | 原 `runId`、最终 `status` |
| `AGENT_REQUEST_ID_CONFLICT` | 409 | conflict | false | `clientRequestId` |
| `AGENT_RUN_NOT_FOUND` | 404 | not_found | false | `runId` |
| `AGENT_MESSAGE_NOT_RETRYABLE` | 409 | conflict | false | `assistantMessageId`、当前 `status` |
| `AGENT_NOT_CONFIGURED` | 503 | server | false | 缺失的配置项名称，不含值 |
| `AGENT_PROVIDER_FAILED` | 502 或流内 | server | true | 稳定供应商类别，不含原始响应 |
| `AGENT_PROVIDER_TIMEOUT` | 504 或流内 | server | true | `phase`、`timeoutMs` |
| `AGENT_STEP_LIMIT_EXCEEDED` | 流内 | server | false | `maxSteps`、`completedSteps` |
| `AGENT_INTERNAL_FAILED` | 500 或流内 | server | true | `runId` 和可公开阶段，不含堆栈 |
| `PROTOCOL_INVALID_METADATA` | 前端适配器 | protocol | false | 缺失字段和 chunk 类型 |
| `PROTOCOL_UNEXPECTED_EOF` | 前端适配器 | protocol | true | 最后收到的 chunk 类型 |

未知服务端错误统一映射为 `AGENT_INTERNAL_FAILED` 或 `INTERNAL_ERROR`。任何新增错误码都必须同时补入此表、前端映射和契约测试。

## 9. API 与契约

- API 前缀固定为 `/api/v1`。
- 只使用 `GET` 和 `POST`。
- `GET` 只读取；所有写入使用 `POST`。
- API 字段使用 camelCase，数据库字段使用 snake_case。
- 时间以 UTC ISO 8601 传输和保存，展示时转换为用户本地时区。
- 可能产生重复数据的写请求携带 `clientRequestId`，用于重试和幂等判定；天然幂等的取消操作可以不携带。
- 创建会话、发送消息和重试必须携带 `clientRequestId`；同一标识只能对应一个请求语义，不能拿相同标识发送不同正文或不同助手消息。
- 幂等记录必须保存规范化请求摘要 `requestHash`，至少覆盖正文、消息 ID、模型、模式和附件引用；不得只凭唯一键判断是否为同一请求。
- 请求和响应必须经过校验后才能进入领域层。
- 列表和会话详情必须有明确上限。默认会话列表每次 30 条，默认消息详情每次 40 条，使用游标读取更早数据。
- API 契约变更必须同时更新类型、Schema、测试和文档。

跨语言契约在生成链建立前可以先手写，但必须满足：

- Go 响应用例保存在 `contracts` 或 `tests/e2e/support`。
- TypeScript Schema 对同一用例校验。
- 契约测试失败时，不允许只修改前端类型来“适配”。

接口稳定后必须增加 TypeScript 类型生成和漂移检查，不长期手工维护两份同义响应类型。

## 10. 状态与数据所有权

- Zustand Store 是前端业务状态唯一事实源。
- `AgentStreamEvent` 只作为协议映射对象，不成为第二条可修改业务状态。
- React Query、SWR、组件本地 state、第三方 Chat Hook 和 SDK 内置 Store 不得复制会话、消息或发送状态。
- `agentTransport.ts` 只负责请求与错误 envelope，`agentStreamProtocol.ts` 只负责 SSE 和协议映射；CraftingTable 的 Store action 只由 Agent Effect 和领域 Adapter 调用。
- 服务端状态加载后先转换为领域对象，再写入 Store。
- 临时 UI 状态，例如菜单是否打开、输入框草稿和 hover 状态，可以留在组件或专用 UI Store。
- 不在 Store 中保存 AbortController、ReadableStream、DOM 节点或不可序列化对象。

## 11. 设计系统与 UI

- 颜色、表面、边框、文字和强调色必须使用 `src/styles/tokens.css` 中的语义 Token。
- Light 和 Dark 必须使用同一语义 Token，不在组件内写 `if (dark)` 颜色分支。
- ShadcnUI 组件允许本地修改，但必须保留键盘、焦点和 aria 行为。
- 图标按钮必须有可访问名称，未知图标需要 Tooltip。
- 390px、768px、1024px 和 1440px 不能出现全局横向滚动。
- 输入框发送必须判断 IME 组合状态，避免中文选词时误提交。
- 禁止新增业务硬编码颜色；例外只允许二维码、遮罩、SVG 资产等明确场景。

`src/styles/tokens.css` 是颜色值的权威来源。文档和原型只使用以下语义 Token：

| Token | Light | Dark |
| --- | --- | --- |
| `background` | `#F2F5F8` | `#17191D` |
| `surface` | `#FFFFFF` | `#1E2126` |
| `surfaceSubtle` | `#F7F8FA` | `#25292F` |
| `textPrimary` | `#1D2129` | `#F1F3F5` |
| `textBody` | `#4E5969` | `#C9CDD4` |
| `textSecondary` | `#86909C` | `#969CA6` |
| `border` | `#D8DEE9` | `#30353C` |
| `brand` | `#5468FF` | `#7B8AFF` |
| `brandHover` | `#7080FF` | `#98A4FF` |
| `brandSoft` | `#EEF0FF` | `rgba(123, 138, 255, 0.14)` |
| `success` | `#00B42A` | `#46D16C` |
| `destructive` | `#D92D20` | `#F97066` |
| `radius` | `8px` | `8px` |

颜色值变更必须同时更新 Token 源文件、本文表格、主题测试和受影响的视觉原型。禁止只改截图或文档中的一处分叉值。

工程检查入口：

- `pnpm check:tokens` 扫描新增硬编码颜色。
- `pnpm test:e2e` 覆盖 Light / Dark、响应式和无障碍关键路径。

## 12. 测试规范

### 12.1 目录

这是 CraftingTable 有意采用的集中测试布局：面向产品行为、API、契约和桌面能力的测试统一位于 `tests/e2e`。集中布局不降低测试层级，`unit`、`integration`、浏览器流程和 Electron 流程必须分别可独立执行。

```text
tests/e2e/
  unit/                 Store、领域 Adapter 和纯函数
  integration/          HTTP 黑盒、契约、流协议和持久化行为
  modules/              浏览器用户流程
  desktop/              Electron 启动、IPC 和打包后流程
  support/              Fixture、Fake Runtime 和断言工具
```

默认不在 `src`、`server` 或 `electron` 内创建同置测试文件。Go 只有一种例外：必须访问未导出实现细节的纯逻辑测试，可以在对应 package 内使用 `_test.go`。这类测试不能替代 `tests/e2e` 中的黑盒验收，且不能把框架启动、数据库连接或用户流程塞回 package 测试。

`tests/e2e/integration/agent` 优先启动真实 Go 测试进程并通过 HTTP 观察行为。它不导入 `server/internal`；需要观察内部状态时，通过 API、固定时钟或测试数据库断言，而不是绕过应用边界。

根级 `go.mod` 必须覆盖 `server/` 和 `tests/e2e/integration`。若未来改用子目录 Go 模块，则必须在同一变更中更新测试命令和 CI，不能让 Go 测试落在模块外。

### 12.2 命名

- Vitest：`*.test.ts` 或 `*.test.tsx`
- Playwright：`*.e2e.ts`
- Electron：`*.desktop.ts`
- Go 外部集成测试：`*.integration_test.go`
- Go package 白盒测试：`*_test.go`
- 测试支持文件不以测试后缀命名，避免被误执行

Playwright 配置必须同时收集 `**/*.e2e.ts` 和 `**/*.desktop.ts`。浏览器项目不得加载 `.desktop.ts`，Electron 项目不得用浏览器页面冒充桌面测试。

### 12.3 验证矩阵

| 改动范围 | 最低验证 |
| --- | --- |
| Store action、selector | 对应 `tests/e2e/unit` 测试 |
| Effect、Model、HTTP 错误 | 对应 unit 测试和契约 fixture |
| Agent 三层 pipe-through | 用项目 Transport 和协议 Adapter 消费 Go fixture，覆盖终止、取消和畸形事件 |
| `AgentStreamAdapter` | 协议事件到领域消息的字段、归属和状态映射测试 |
| Go Handler / Repository | `tests/e2e` HTTP 黑盒测试；未导出纯逻辑可加 package `_test.go` |
| API 契约 | TypeScript Schema 与 Go 响应 fixture 双向验证 |
| 页面交互 | 相关 Playwright module 测试 |
| 主题或响应式 | 布局冒烟覆盖 390px 与 1440px；布局改动必须覆盖 390px、768px、1024px 和 1440px 的 Light / Dark |
| Electron 或 preload | 真实 Electron 进程中的 `.desktop.ts` 测试 |
| 跨模块或发布前 | 类型检查、全部 unit/integration、目标 E2E 和构建 |

全量 `pnpm test`、`go test ./server/... ./tests/e2e/integration/...` 和全部 Playwright 项目属于发布与合并门禁。开发期可以只运行相关子集，但不得删除最终门禁。

### 12.4 测试数据

- 自动化测试不调用真实模型。
- Agent 测试通过可编排的 Fake Runtime 按脚本发送事件。
- Fixture 必须覆盖正常、失败、取消、断线、乱序、重复终止和未知事件。
- 测试使用固定时间、固定 ID 和可控时钟，避免依赖真实等待和随机顺序。

## 13. 日志、安全与隐私

### 13.1 日志

- 使用统一 logger，不在业务代码直接使用 `console.log`。
- 服务端错误日志包含 `requestId`、`runId`、`sessionId`、稳定 `code` 和可枚举 `reason`。
- 不记录 API Key、Token、Cookie、完整本地路径、用户正文、模型原始响应和文件内容。
- 预期外部失败使用 `warn`，本地程序错误使用 `error`，开发追踪使用 `debug`。

### 13.2 Electron

- `contextIsolation: true`
- `nodeIntegration: false`
- `sandbox: true`
- Renderer 不加载远程页面。
- 外链通过受控 `openExternal` 打开。
- 文件选择和保存通过 preload 暴露的最小 IPC 接口完成。
- IPC 输入使用 Schema 校验，禁止暴露通用 `ipcRenderer`、`fs` 或 `shell`。
- 本地 Go sidecar 只监听 `127.0.0.1`，使用随机端口和短期鉴权 Token。
- 生产 Desktop 中，Renderer 不读取 sidecar 端口和 Token。preload 只暴露窄范围的 Agent transport bridge；主进程持有端点和凭据，并负责流式请求代理。
- Web 直接请求同源 `/api/v1`；Desktop 通过注入给 `agentTransport.ts` 的受控 fetch 适配器请求主进程。两条路径最终返回同一标准 `Response`。
- 主进程校验每次请求的会话、路径、方法、请求大小和取消信号；不为 Renderer 暴露通用网络代理。
- 生产包启用 CSP，并拒绝未登记权限请求。

## 14. 文档治理

### 14.1 文档分层

- `docs/harness`：工程规则、验证方式和开发入口。
- `docs/architecture`：系统结构、依赖方向和数据流。
- `docs/superpowers/specs`：产品和技术设计。
- `docs/superpowers/plans`：可执行实施计划。
- `docs/glossary.md`：业务词、协议词和架构词的唯一解释。

### 14.2 文档状态

每份设计文档头部至少包含：

- 日期
- 状态
- 范围
- 关联设计
- 术语基线

允许状态：

- `草稿`
- `待评审`
- `已确认`
- `实施中`
- `已实施`
- `已取代`

被新方案替代的文档必须标记 `已取代` 并指向新文档，不继续作为当前事实。

### 14.3 更新规则

- 修改公共契约时同步更新术语表和契约测试。
- 修改架构方向时同步更新本文和 Agent 设计。
- 修改 UI Token 时同步更新原型、主题测试和 Token 文档。
- 文档中的本地链接必须可访问。
- 根级 `AGENTS.md` 只索引规则，不复制整份规范。

## 15. 质量门禁

提交前根据改动范围执行：

```text
pnpm typecheck
gofmt / go vet
相关 Vitest
相关 Go test
相关 Playwright
pnpm build
```

发布或合并前执行：

```text
pnpm typecheck
pnpm test
go test ./server/... ./tests/e2e/integration/...
pnpm test:e2e
pnpm build
```

任何被跳过的验证必须在交付说明中写明原因，不能把“没有运行”描述成“已通过”。

## 16. 分阶段实施

以下十项全部是最终必须满足的规范目标。`bootstrap` 表示需要先建立骨架，`enforcing` 表示已经作为阻塞门禁执行；任何阶段都不能删除目标本身。

1. 根级 `AGENTS.md` 和文档索引：`bootstrap -> enforcing`。
2. MVC 依赖方向静态检查：`bootstrap -> enforcing`。
3. `Result<T>` 与统一错误映射：`bootstrap -> enforcing`。
4. Agent 契约 fixture 和 Fake Runtime：`bootstrap -> enforcing`。
5. Design Token 颜色检查：`bootstrap -> enforcing`。
6. `tests/e2e` 集中目录和完整验证矩阵：`bootstrap -> enforcing`。
7. Electron 安全基线和真实进程测试：`bootstrap -> enforcing`。
8. Go sidecar 启动、健康检查、鉴权、优雅退出和崩溃重启：`bootstrap -> enforcing`。
9. Go Agent 分层依赖静态检查：`bootstrap -> enforcing`。
10. Agent 前端三层 pipe-through 和 Go 协议一致性测试：`bootstrap -> enforcing`。

允许按依赖关系并行建立骨架，但不允许把尚未启用的门禁描述成非目标。每个阶段结束时必须更新能力状态、验收命令和剩余风险。

架构能力按以下顺序渐进引入。入口条件未满足时，后续能力不得提前进入生产实现：

| 阶段 | 增量能力 | 进入下一阶段的条件 |
| --- | --- | --- |
| 当前 | Go 模块化单体、标准库 HTTP、SQLite、直接 SSE | 现有 Today 和 Agent 交互稳定且有回归测试 |
| P1 | Application 用例层、Agent Run/Message/Part 持久化 | 刷新后必须恢复已提交消息，并准确识别遗留 Run |
| P2 | Run Coordinator、取消、重试和可恢复事件边界 | 浏览器断开后服务端仍需继续执行或客户端需要重连 |
| P3 | 本地 Daemon/Sidecar、隔离工作目录和工具执行边界 | Agent 需要访问本地文件、执行命令或接入工具 |
| P4 | PostgreSQL、任务队列、Attempt 和 Lease | Web 多实例、跨客户端并发或故障接管成为真实需求 |
| P5 | Redis、跨实例实时转发和可恢复流缓存 | Run 事件语义稳定，并完成 Redis 职责决策记录 |
| P6 | Skills、MCP、多 Agent 和独立评测平台 | 单 Agent Loop、Trace、Artifact 和权限模型稳定 |

表中“当前”不是永久架构。任何阶段升级都先保留既有 API 和状态含义，再替换底层实现；不得为了引入某件基础设施重新定义用户已经依赖的业务契约。

首个纵向切片按以下顺序实施：

1. 建立 Go sidecar 骨架，同时注册今日和 Agent 两个模块。
2. 迁移今日现有读写接口，并以现有浏览器行为作为回归基线。
3. 用 Go Repository 和 Fake Runtime 完成 Agent 会话、消息、Run 和 SSE。
4. 接入 Electron 主进程代理和固定路径 preload 桥。
5. 前端从 Fastify 原型切换到 Go API；Today 回归通过后删除旧 API。

Fastify 原型已由 Go 服务替换，不属于目标架构。若旧原型中存在需要保留的用户数据，必须在后续数据结构变化前单独编写导出、导入和校验步骤；没有数据校验通过，就不能声称迁移无损。

## 17. 架构决策门

决策门不是非目标，也不是可以跳过的未完成项。受影响平台在对应决策完成前不得进入生产实现或宣称能力已完成。

| 决策门 | 必须回答 | 不满足时的门槛 |
| --- | --- | --- |
| Web 身份与多端数据 | `AuthSession` 模型、设备标识、服务端与本地数据所有权、同步冲突、删除、导出、备份和密钥轮换 | Web 或手机端不得上线同步能力 |
| Web 多实例一致性 | 会话执行租约、Run 事件分配、任务队列和故障接管；候选至少包括 PostgreSQL 租约与 Redis 租约 | 不得宣称支持多实例部署 |
| 可恢复流与 Run 恢复 | 稳定订阅游标、断线重连、检查点边界、重复事件处理、至少一次或至多一次语义、恢复失败收敛 | 不得把“刷新后继续接收”描述为已支持 |
| Redis 职责 | 缓存、SSE 回放、队列、租约、限流中哪些由 Redis 承担，哪些仍由数据库承担 | 不得先接 Redis 再反推职责 |
| 工具、Skills、MCP 与多 Agent | 领域 Schema、权限、超时、取消、重试、Artifact/Trace 与父子 Run 关系 | 不得在 Runtime 中直接塞入未建模的前端特殊分支 |

每个决策门必须产出一份 `docs/architecture` 下的决策记录，包含候选方案、约束、故障语义、迁移方式、验收测试和未选方案原因。决策完成后再把结果写入本规范和 Agent 设计，不能只存在于代码或聊天记录中。

## 18. 完成标准

- 工程目录与职责符合本文。
- 前端 Store、Effect、Model、View 没有反向依赖。
- Go Agent 的 Access、Application、Core、Ports 和 Adapters 没有反向导入。
- 外部数据在边界完成校验。
- Model 边界使用 `Result<T>`，组件不再重复处理底层网络异常。
- Agent 流协议有独立 fixture 和失败场景测试。
- 前端通过项目 Transport、`eventsource-parser` 和协议 Adapter 消费流，`src` 不包含自建 SSE 清理器。
- 流协议 fixture 能同时验证前端 Adapter 和 Go 输出，第三方工具包已锁定版本并保留许可证说明。
- 主题色没有新增业务硬编码。
- 日志不包含敏感内容。
- Electron 在真实进程中通过启动、关键交互和 IPC 测试。
- 产品行为、API、契约和桌面验收测试位于 `tests/e2e`；Go package 白盒测试只作为解释性补充。
- 当前文档与代码事实一致，过期文档已标记状态。

## 19. 当前验证基线

2026-11-05 的 Agent 持久化与恢复改动已通过以下门禁：

```powershell
pnpm typecheck
pnpm test
go test ./server/... ./tests/e2e/integration/... ./tests/e2e/unit/agent -count=1
go vet ./server/...
gofmt -d
pnpm build
pnpm test:e2e
git diff --check
```

验证结果：Vitest 80 个测试通过，Playwright 在桌面和移动项目中共 28 个测试通过，Go 测试、类型检查、格式检查、静态检查和生产构建通过。`pnpm build` 仍报告单个前端 chunk 超过 500 kB 的警告，这是当前性能待办，不是构建失败。

Agent 恢复的实现事实：

- Session 详情通过只读事务读取一致快照。
- SSE 游标只来自进程内 Event Journal，不复用 durable RunEvent 序号。
- 旧 Run 恢复必须校验请求 `runId`，不能读取同一助手消息后续 Run 的内容。
- 完成、取消和超时竞争只有一个终态胜出。
- Finalize 失败回退为 `interrupted`，启动失败清理使用脱离 HTTP Context 的有界写入。
- Go 进程重启后旧 Run 不做跨进程续传。
