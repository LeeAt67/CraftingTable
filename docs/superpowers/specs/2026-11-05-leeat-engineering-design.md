# Leeat 工程化设计规范

日期：2026-11-05
状态：待评审
范围：Leeat 的代码组织、架构边界、错误处理、契约、测试、安全与文档治理
适用范围：Desktop、Web，以及后续移动端共用的前端代码
关联设计：[Leeat Agent 设计](./2026-11-05-leeat-agent-design.md)
术语基线：[Leeat 术语表](../../glossary.md)

## 1. 目的

本文定义 Leeat 在进入实现和持续迭代后必须遵守的工程规则。

规范的目标不是复制大型团队流程，而是让以下问题在提交代码前就能被发现：

- View、Store、Effect 和 Model 的依赖方向被打乱。
- API、流事件和前端类型逐渐不一致。
- 网络错误、取消和协议错误混进组件。
- 主题值、日志和 Electron 安全边界被绕过。
- 文档继续描述已经废弃的技术方案。
- 测试数量增加，但关键失败路径仍没有覆盖。

本文只保留能够降低 Leeat 返工成本的规则。每条规则都应能通过代码结构、类型、测试、脚本或评审清单验证。

## 2. 核心原则

1. **边界优先于抽象数量**：只有承担真实职责的模块才创建接口和目录，不为形式完整增加空层。
2. **单一事实源**：业务状态、错误码、协议、术语和设计 Token 都只能有一个权威来源。
3. **错误是数据**：I/O 和协议失败通过 `Result<T>` 返回；程序缺陷仍使用异常，由错误边界处理。
4. **Parse，don't cast**：外部数据先校验再使用，禁止把 `unknown`、JSON 或第三方响应直接断言成领域类型。
5. **默认增量验证**：修改什么就验证什么；全量门禁只在合并、发布或跨模块高风险改动时运行。
6. **桌面端不是浏览器**：Electron 的 IPC、文件、外链、快捷键、离线启动和打包行为必须在真实桌面进程中验证。
7. **文档描述当前事实或明确标注状态**：过时方案不继续伪装成当前设计。

## 3. 目标与非目标

### 3.1 目标

- 前端保持 React、TypeScript、Vite、ShadcnUI、Zustand 和 MVC。
- 后端保持 Go 模块化单体，不为 MVP 拆分微服务。
- Desktop 采用本地优先架构，Web 复用同一套 API 和领域契约。
- MVC 的 Model、Effect、Store、View 有明确且可检查的依赖方向。
- Agent 流式协议能够独立测试，不依赖真实模型。
- 产品行为、API、契约和桌面验收测试统一放在 `tests/e2e`。
- 设计系统、日志安全和文档链接有自动检查入口。
- 首个可交付版本是 Windows Desktop，Go sidecar 同时承载今日和 Agent；现有 Fastify 原型只作为迁移期间的对照实现。

### 3.2 非目标

- 当前 MVP 不引入 Monorepo、Turborepo、Feature Flag、MR Gate、多环境分支或生产从库。
- 不引入 TanStack Query 代替 Zustand 业务状态。
- 不强制所有函数都返回 `Result<T>`。
- 不为个人项目复制大型团队的组织流程和发布审批。
- 不在此阶段建设 Agent Evaluation Platform。

### 3.3 反目标

以下结果即使功能可用，也视为工程失败：

- 页面可以运行，但 View 直接请求 API 或导入 Model。
- Store 保存一份状态，AI SDK Hook 又保存另一份可修改状态。
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
  cmd/leeatd/
  internal/
    agent/
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
HTTP Controller
  -> Application Controller
  -> Domain Model
  -> Repository
  -> SQLite / PostgreSQL
```

HTTP Controller 只负责解码、校验、调用应用逻辑和序列化响应。Repository 不依赖 HTTP，Agent Runtime 不直接访问数据库，数据库事务内不调用模型或其他外部 I/O。

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
- 所有函数使用中文 TSDoc，包含职责、`@param`、`@returns`、副作用和失败语义。
- 注释解释原因、约束、竞态和业务规则，不复述代码字面含义。
- 注释与实现不一致视为缺陷。

## 7. Go 规范

- 使用 `gofmt` 和 `go vet`，不手工对抗标准格式。
- 导出类型和函数使用中文 Go Doc；复杂内部函数也必须有职责说明。
- 领域错误使用 `error` 值并通过 `errors.Is`、`errors.As` 判断，不依赖错误字符串。
- 对外错误必须有稳定错误码；内部错误使用 `%w` 保留因果链。
- 只有启动期不变量被破坏时才允许 `panic`，请求处理路径不得 panic 作为控制流。
- 所有外部调用接受 `context.Context`，取消和超时向下传递。
- 数据库查询必须有边界，列表默认分页或限量，禁止无界全表读取。
- 会话、消息、事件和日志查询都必须使用 limit 或游标分页；默认上限写入 API 契约，不能只靠调用方自觉传参。
- 事务内只执行数据库操作，不调用模型、HTTP、文件系统或对象存储。
- 自建 Go 文件使用 lower_snake_case，例如 `agent_controller.go`；数据库表和列使用 snake_case。

## 8. Result 与错误契约

### 8.1 前端边界结果

Model 和协议适配器统一返回：

```ts
type Result<T, E extends LeeatError = LeeatError> =
  | readonly [data: T, error: null]
  | readonly [data: null, error: E]
```

错误不是裸 `Error`：

```ts
type LeeatErrorKind =
  | "network"
  | "http"
  | "validation"
  | "protocol"
  | "auth"
  | "conflict"
  | "cancelled"
  | "server"
  | "unknown"

type LeeatError = {
  code: string
  kind: LeeatErrorKind
  message: string
  retryable: boolean
  requestId?: string
  cause?: unknown
}
```

规则：

- `message` 面向用户，不包含堆栈、密钥或第三方响应原文。
- `code` 面向程序分支，必须稳定、可枚举。
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
    "kind": "http",
    "message": "会话不存在",
    "retryable": false,
    "requestId": "req_123"
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

## 9. API 与契约

- API 前缀固定为 `/api/v1`。
- 只使用 `GET` 和 `POST`。
- `GET` 只读取；所有写入使用 `POST`。
- API 字段使用 camelCase，数据库字段使用 snake_case。
- 时间以 UTC ISO 8601 传输和保存，展示时转换为用户本地时区。
- 可能产生重复数据的写请求携带 `clientRequestId`，用于重试和幂等判定；天然幂等的取消操作可以不携带。
- 创建会话、发送消息和重试必须携带 `clientRequestId`；同一标识只能对应一个请求语义，不能拿相同标识发送不同正文或不同助手消息。
- 请求和响应必须经过校验后才能进入领域层。
- 列表和会话详情必须有明确上限。默认会话列表每次 30 条，默认消息详情每次 40 条，使用游标读取更早数据。
- API 契约变更必须同时更新类型、Schema、测试和文档。

跨语言契约在 MVP 阶段可以保持手写，但必须满足：

- Go 响应用例保存在 `contracts` 或 `tests/e2e/support`。
- TypeScript Schema 对同一用例校验。
- 契约测试失败时，不允许只修改前端类型来“适配”。

接口稳定后增加 TypeScript 类型生成和漂移检查，不手工维护两份同义响应类型。

## 10. 状态与数据所有权

- Zustand Store 是前端业务状态唯一事实源。
- AI SDK 的 `UIMessage` 只作为协议映射对象，不成为第二条可修改业务状态。
- React Query、SWR、组件本地 state 和 AI SDK Hook 不得复制会话、消息或发送状态。
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

工程检查入口：

- `pnpm check:tokens` 扫描新增硬编码颜色。
- `pnpm test:e2e` 覆盖 Light / Dark、响应式和无障碍关键路径。

## 12. 测试规范

### 12.1 目录

面向产品行为、API、契约和桌面能力的测试统一位于 `tests/e2e`：

```text
tests/e2e/
  unit/                 Store、Reducer、Parser 和纯函数
  integration/          HTTP 黑盒、契约、流协议和持久化行为
  modules/              浏览器用户流程
  desktop/              Electron 启动、IPC 和打包后流程
  support/              Fixture、Fake Runtime 和断言工具
```

默认不在 `src`、`server` 或 `electron` 内创建同置测试文件。Go 只有一种例外：必须访问未导出实现细节的纯逻辑测试，可以在对应 package 内使用 `_test.go`。这类测试不能替代 `tests/e2e` 中的黑盒验收，且不能把框架启动、数据库连接或用户流程塞回 package 测试。

`tests/e2e/integration/agent` 优先启动真实 Go 测试进程并通过 HTTP 观察行为。它不导入 `server/internal`；需要观察内部状态时，通过 API、固定时钟或测试数据库断言，而不是绕过应用边界。

### 12.2 命名

- Vitest：`*.test.ts` 或 `*.test.tsx`
- Playwright：`*.e2e.ts`
- Electron：`*.desktop.ts`
- Go 外部集成测试：`*.integration_test.go`
- Go package 白盒测试：`*_test.go`
- 测试支持文件不以测试后缀命名，避免被误执行

### 12.3 验证矩阵

| 改动范围 | 最低验证 |
| --- | --- |
| Store action、selector | 对应 `tests/e2e/unit` 测试 |
| Effect、Model、HTTP 错误 | 对应 unit 测试和契约 fixture |
| Agent SSE Parser / Reducer | 事件顺序、终止、取消和畸形事件测试 |
| Go Handler / Repository | `tests/e2e` HTTP 黑盒测试；未导出纯逻辑可加 package `_test.go` |
| API 契约 | TypeScript Schema 与 Go 响应 fixture 双向验证 |
| 页面交互 | 相关 Playwright module 测试 |
| 主题或响应式 | 390px 与 1440px 的 Light / Dark 测试 |
| Electron 或 preload | 真实 Electron 进程中的 `.desktop.ts` 测试 |
| 跨模块或发布前 | 类型检查、全部 unit/integration、目标 E2E 和构建 |

全量 `pnpm test`、`go test ./...` 和全部 Playwright 项目只在发布、合并或用户明确要求时运行。

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
- Web 直接请求同源 `/api/v1`；Desktop 通过注入给 `DefaultChatTransport` 的受控 fetch 适配器请求主进程。两条路径最终返回同一 `ReadableStream<UIMessageChunk>`。
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
go test ./...
pnpm test:e2e
pnpm build
```

任何被跳过的验证必须在交付说明中写明原因，不能把“没有运行”描述成“已通过”。

## 16. 首期实施范围

第一批只建立以下工程能力：

1. 根级 `AGENTS.md` 和文档索引。
2. MVC 依赖方向静态检查。
3. `Result<T>` 与统一错误映射。
4. Agent 契约 fixture 和 Fake Runtime。
5. Design Token 颜色检查。
6. `tests/e2e` 统一目录和基础验证矩阵。
7. Electron 最小安全基线和真实进程冒烟测试。
8. Go sidecar 启动、健康检查、鉴权、优雅退出和崩溃重启。

不等待全部规范就绪后再开始开发。上述能力优先服务于当前工作台和单 Agent 会话。

首个纵向切片按以下顺序实施：

1. 建立 Go sidecar 骨架，同时注册今日和 Agent 两个模块。
2. 迁移今日现有读写接口，并以现有浏览器行为作为回归基线。
3. 用 Go Repository 和 Fake Runtime 完成 Agent 会话、消息、Run 和 SSE。
4. 接入 Electron 主进程代理和固定路径 preload 桥。
5. 前端从 Fastify 原型切换到 Go API；Today 回归通过后才删除旧 API。

现有 Fastify 原型不作为目标架构的一部分。若原型中存在需要保留的用户数据，必须在切换前单独编写导出、导入和校验步骤；没有数据校验通过，就不能声称迁移无损。

## 17. 完成标准

- 工程目录与职责符合本文。
- 前端 Store、Effect、Model、View 没有反向依赖。
- 外部数据在边界完成校验。
- Model 边界使用 `Result<T>`，组件不再重复处理底层网络异常。
- Agent 流协议有独立 fixture 和失败场景测试。
- 主题色没有新增业务硬编码。
- 日志不包含敏感内容。
- Electron 在真实进程中通过启动、关键交互和 IPC 测试。
- 产品行为、API、契约和桌面验收测试位于 `tests/e2e`；Go package 白盒测试只作为解释性补充。
- 当前文档与代码事实一致，过期文档已标记状态。
