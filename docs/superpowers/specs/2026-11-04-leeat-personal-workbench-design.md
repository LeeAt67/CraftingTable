# Leeat 个人工作台 MVP 设计规范

日期：2026-11-04  
状态：已确认，进入实施计划  
视觉基线：当前 HTML 原型  
架构：MVC  
范围：MVP

## 1. 产品定义

Leeat 是一个单用户个人事务工作台。MVP 只实现首页的“今日”视图，用时间流集中展示今天的任务和日程，并支持新建待办、完成待办和切换 Light / Dark 主题。

一级导航只保留：

1. 首页 `/`
2. 设置 `/settings`

设置页仅做占位，不实现账号、权限、同步、Agent、多智能体或 AI 对话。

## 2. MVP 范围

### 首页 `/`

- 一级图标导航：首页、设置。
- 二级上下文侧栏：全部、已安排、已完成、分类和今日完成度。
- 页面标题、日期和新建按钮。
- 摘要：待办数量、日程数量、专注时间。
- 今日时间流：任务和日程混排。
- 完成任务与取消完成。
- Quick Capture：新增一条未安排的今日待办。
- Light / Dark 切换并保存到 `localStorage`。

### 设置 `/settings`

- 保留路由和一级导航入口。
- 显示标题、占位说明和“返回今日”按钮。
- 不实现表单、账号或业务数据。

## 3. 非目标

- 不实现收件箱、日历、笔记、收藏和分类管理页面。
- 不实现登录、账号、权限、云同步和多人协作。
- 不实现 Agent、AI 对话、自动规划和工具调用。
- 不实现全局搜索、通知、插件和复杂设置。
- 不引入 ORM、Redis、消息队列、微服务、Docker。
- 当前 MVP 不安装 Electron，也不生成桌面安装包。

## 4. MVC 架构

### 4.1 Model

Model 负责数据类型、数据读写和持久化。

前端 Model：

- TypeScript 类型。
- Model 接口。
- Mock Model。
- API Model。
  - 后续 Electron 使用的 IPC Model。
- 主题存储。

后端 Model：

- SQLite 表结构。
- SQL 查询与写入。
- 查询结果到业务对象的转换。

### 4.2 Controller

Controller 固定拆成 `effects` 和 `stores` 两部分：

- `stores` 只保存状态，并暴露同步状态动作。
- `effects` 负责异步动作、调用 Model、错误处理，以及把结果写入 Store。
- View 读取 Store，并调用 Effect 暴露的动作。
- Model 不感知 Store，Effect 不直接渲染 UI。

前端 Controller：

- `controllers/stores/todayStore.ts`：保存今日条目、分类、加载状态和错误状态。
- `controllers/stores/themeStore.ts`：保存当前主题，并通过 Zustand persist 写入 `localStorage`。
- `controllers/effects/useTodayEffect.ts`：加载今日数据、创建待办、完成和取消完成。
- `controllers/effects/useThemeEffect.ts`：把主题状态同步到根节点，不直接操作业务数据。

设置页只显示静态占位内容，不创建 Store 或 Effect。不要为了形式完整而增加空控制器。

后端 Controller：

- 校验请求。
- 调用 Model。
- 处理成功和错误。
- 返回统一 JSON。

### 4.3 View

View 只负责展示和触发 Controller 动作。

前端 View：

- AppLayout、TodayPage、SettingsPage。
- AppShell、NavRail、ContextSidebar。
- 时间流、摘要条、Quick Capture、新建 Dialog。

后端 View：

- JSON serializer。
- 成功响应：`{ "data": ... }`。
- 错误响应：`{ "error": { "code", "message" } }`。

View 不直接执行 SQL，也不直接拼接 API 地址。

## 5. 技术栈

### 前端

- React + TypeScript + Vite。
- Tailwind CSS。
- shadcn/ui。
- React Router。
- Zustand。
- Lucide React。
- Zod。
- Vitest + Testing Library。
- Playwright。

### 后端

- Node.js 24 LTS。
- TypeScript 严格模式。
- Fastify 5。
- Node.js 内置 `node:sqlite`。
- Zod。
- Vitest + Fastify `inject()`。

后端只运行一个 Node 进程，SQLite 文件作为唯一持久化存储。

### 后续 Electron

Electron 只作为后续运行容器，不改变现有 React View 和 Controller。

- 浏览器模式：`ApiModel` 调用 Fastify API。
- Electron 模式：`IpcModel` 调用主进程 IPC。
- Electron 主进程负责窗口、SQLite 和业务动作。
- Renderer 不直接访问 Node.js、SQLite 或文件系统。
- 当前 MVP 不安装 Electron 依赖，只保留可迁移边界。
- 路由从 MVP 开始使用 `HashRouter`，保证浏览器和 Electron 的 `file://` 加载方式一致。
- 逻辑路由仍然是 `/` 和 `/settings`，浏览器地址栏可显示 `/#/settings`。

## 6. 目录结构

```text
src/
  components/
    shell/
  controllers/
    effects/
      useTodayEffect.ts
      useThemeEffect.ts
    stores/
      todayStore.ts
      themeStore.ts
  layouts/
    appLayout.tsx
  models/
    contracts.ts
    todayModel.ts
    mockModel.ts
    apiModel.ts
  pages/
    today/
      index.tsx
    settings/
      index.tsx
  router/
    index.tsx
  styles/
    globals.css
    tokens.css
  app.tsx
  main.tsx

api/
  src/
    models/
      db.ts
      itemModel.ts
      categoryModel.ts
    controllers/
      todayController.ts
      itemController.ts
      categoryController.ts
    views/
      jsonView.ts
    routes.ts
    app.ts
    server.ts
  migrations/
  test/

electron/
  main.ts
  preload.ts
  ipcHandlers.ts
  sqliteStore.ts
```

前端映射固定为：`models` 是 Model，`controllers/effects + controllers/stores` 是 Controller，`pages + layouts + components` 是 View。Store 不调用 Model，Effect 不保存长期状态，每个文件只负责一个 MVC 角色。

`electron` 目录只在后续桌面版实施时创建，不属于当前 MVP 交付。

### Store 状态

`todayStore` 至少保存：

- `items`
- `categories`
- `phase`：`idle | loading | ready | error`
- `errorMessage`

`todayStore` 只提供同步动作，例如 `setLoading`、`setReady`、`setError`、`addItem`、`replaceItem`。它不调用 `fetch`，不调用 Model，也不显示 Toast。

`themeStore` 保存 `light | dark`，通过 Zustand persist 持久化到 `localStorage`。根节点的 `data-theme` 由 `useThemeEffect.ts` 同步。

### Effect 动作

`useTodayEffect.ts` 暴露：

- `loadToday`
- `retryToday`
- `createItem`
- `completeItem`
- `uncompleteItem`

Effect 负责调用 `TodayModel`，成功时更新 Store，失败时写入错误状态并反馈 Toast。Effect 函数接受可选的 `TodayModel` 参数，默认使用当前运行环境选择的实现，便于测试时注入 `MockModel`。

## 7. UI 结构

桌面端三列：

```text
┌────┬──────────────┬──────────────────────────────┐
│一级│ 二级上下文栏   │ 今日标题、摘要和时间流          │
│导航│              │                              │
└────┴──────────────┴──────────────────────────────┘
```

- 顶部应用栏只显示 `L / LEEAT / 首页`。
- 一级导航宽 56px，只显示首页和设置。
- 二级侧栏桌面宽 232px，窄屏压缩为 88px。
- 主内容区禁止出现全局横向滚动。
- 设置入口固定在一级导航底部。

### 响应式

| 宽度 | 布局 |
| --- | --- |
| `>= 1280px` | 一级导航 56px，二级侧栏 232px |
| `768px - 1279px` | 一级导航 52px，二级侧栏 184px |
| `461px - 767px` | 一级导航 44px，二级侧栏 104px |
| `<= 460px` | 一级导航 34px，二级侧栏 88px |

## 8. 首页视觉

- 页面底色使用中性灰，内容表面使用白色或深灰。
- 主按钮、导航选中态、当前时间节点使用品牌色。
- 完成任务使用成功色，并降低文字强调。
- 时间流使用一条 1px 轴线贯穿当天。
- 任务与日程使用独立横向表面，不嵌套卡片。
- 字体层级只保留标题、正文和辅助文字三级。

### Light Token

| Token | Value |
| --- | --- |
| background | `#F2F5F8` |
| surface | `#FFFFFF` |
| surfaceSubtle | `#F7F8FA` |
| textPrimary | `#1D2129` |
| textBody | `#4E5969` |
| textSecondary | `#86909C` |
| border | `#D8DEE9` |
| brand | `#5468FF` |
| brandHover | `#7080FF` |
| brandSoft | `#EEF0FF` |
| success | `#00B42A` |

### Dark Token

| Token | Value |
| --- | --- |
| background | `#17191D` |
| surface | `#1E2126` |
| surfaceSubtle | `#25292F` |
| textPrimary | `#F1F3F5` |
| textBody | `#C9CDD4` |
| textSecondary | `#969CA6` |
| border | `#30353C` |
| brand | `#7B8AFF` |
| brandHover | `#98A4FF` |
| brandSoft | `rgba(123, 138, 255, 0.14)` |
| success | `#46D16C` |

## 9. 数据模型

```ts
type ItemKind = "task" | "event";
type ItemStatus = "unplanned" | "planned" | "done";

type Category = {
  id: string;
  name: string;
  color: string;
};

type Item = {
  id: string;
  kind: ItemKind;
  title: string;
  description?: string;
  status: ItemStatus;
  categoryId?: string;
  startAt?: string;
  endAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
};
```

SQLite 只建立两张表：

- `categories`
- `items`

数据库变更使用普通 SQL migration 文件，不使用 ORM。

## 10. API

- 前缀：`/api/v1`。
- 只使用 `GET` 和 `POST`。
- `GET` 只读取。
- 所有写操作使用 `POST`。
- 不使用 `PUT`、`PATCH`、`DELETE`。
- 写操作通过 body 传递参数和 `id`。
- 请求和响应使用 `camelCase`。
- 时间使用 UTC ISO 8601。

```text
GET  /api/v1/today
GET  /api/v1/categories
POST /api/v1/item/create
POST /api/v1/item/update
POST /api/v1/item/complete
POST /api/v1/item/uncomplete
```

设置页不新增 API。

## 11. Model 接口

```ts
interface TodayModel {
  listToday(): Promise<Item[]>;
  listCategories(): Promise<Category[]>;
  createItem(input: CreateItemInput): Promise<Item>;
  updateItem(input: UpdateItemInput): Promise<Item>;
  completeItem(id: string): Promise<Item>;
  uncompleteItem(id: string): Promise<Item>;
}
```

MVP 同时提供：

- `MockModel`：无后端时运行 UI。
- `ApiModel`：后端可用时调用 API。

两种实现使用同一个接口，View 和 Controller 不感知数据来源。后续增加 `IpcModel` 时，也不修改页面组件和 Controller。

## 12. Electron 兼容方案

### 数据流

浏览器模式：

```text
View
  -> Controller (Effect / Store)
  -> TodayModel
  -> ApiModel
  -> GET / POST
  -> Fastify
  -> SQLite
```

Electron 模式：

```text
View
  -> Controller (Effect / Store)
  -> TodayModel
  -> IpcModel
  -> preload
  -> ipcMain
  -> SQLite
```

### IPC 接口

IPC 动作与 API 的业务动作保持一致：

```text
todayList
categoryList
itemCreate
itemUpdate
itemComplete
itemUncomplete
```

`preload.ts` 使用 `contextBridge` 暴露唯一的 `window.leeat` 类型化接口。`IpcModel` 只调用该接口，不读取 Electron 全局对象。

### 安全边界

- `contextIsolation: true`
- `nodeIntegration: false`
- `sandbox: true`
- Renderer 不加载远程页面。
- IPC 输入使用 Zod 校验。
- IPC 通道名集中定义，不在组件内直接拼接。
- SQLite 文件放在 Electron `app.getPath("userData")` 目录。

### 主进程职责

- 创建 `BrowserWindow`。
- 加载 Vite 开发地址或构建后的 `dist/index.html`。
- 注册 IPC handlers。
- 复用现有 SQLite Model 和 migration。
- 通过 `app.requestSingleInstanceLock()` 保证同一用户只运行一个实例。

### 当前必须预留的边界

- 页面不直接 `fetch`，只通过 Controller 和 Model 获取数据。
- `IpcModel` 只替换 Effect 使用的 Model 实例，不修改 Store 和 View。
- API 类型与 IPC 类型共用 `contracts.ts`。
- 路由使用 `HashRouter`。
- 主题继续保存在 `localStorage`，迁移到 Electron 时无需修改页面。
- 不在 Renderer 中导入 `node:sqlite`、`fs`、`path` 或 Electron 主进程模块。

Electron 打包、自动更新、系统托盘、原生菜单和多窗口不属于 MVP，也不写入当前实施范围。

## 13. 页面状态

首页必须处理：

- Loading：摘要和时间流显示 Skeleton。
- Empty：显示“今天还没有安排”，保留新建入口。
- Error：显示错误说明和重试按钮。
- Populated：显示摘要和时间流。

新建和完成后使用轻量 Toast 反馈。

## 14. 交互与无障碍

- 所有图标按钮有 `aria-label`。
- 当前导航使用 `aria-current="page"`。
- Dialog 和 Drawer 关闭后焦点回到触发按钮。
- `Escape` 关闭 Dialog。
- `Space` 切换任务完成状态。
- 焦点环使用品牌色。
- 动画遵循 `prefers-reduced-motion`。
- Light / Dark 不依赖颜色单独表达状态。

## 15. 测试

- TypeScript 严格模式无错误。
- Vitest 覆盖 Model 接口、Store 同步动作和 Today Effect 异步动作。
- Store 测试不启动 React，不访问网络。
- Effect 测试注入 MockModel，并断言成功、失败和重试状态。
- Fastify `inject()` 覆盖今日读取、创建和完成接口。
- Playwright 覆盖 `/`、`/settings`、Light / Dark、390px 和 1440px。
- 检查 `scrollWidth === clientWidth`。
- 检查 `HashRouter` 下首页与设置页可直接访问。

## 16. 实施顺序

1. 初始化 React、Tailwind、shadcn/ui 和 MVC 目录。
2. 完成 AppShell、导航、主题和响应式骨架。
3. 完成 Today Store、Theme Store、Today Effect 和 MockModel。
4. 完成首页 View 并接入 Effect 与 Store。
5. 完成 Fastify、SQLite、API Controller 和 JSON View。
6. 接入 ApiModel。
7. 完成设置占位页。
8. 补齐测试和视觉验收。

## 17. MVP 完成标准

- 一级导航只有首页和设置。
- 首页可加载今日摘要和时间流。
- 首页可新建待办。
- 待办可完成和取消完成。
- Light / Dark 可切换并持久化。
- `/settings` 显示统一占位页。
- 390px、768px、1024px、1440px 无全局横向滚动。
- MockModel 可独立运行 UI。
- Fastify API 可使用 SQLite 完成相同数据动作。
- 前端数据读取只通过 `TodayModel`，页面不直接请求 API。
- Controller 明确拆分为 `effects` 和 `stores`，Store 不调用 Model。
- 异步动作只存在于 Effect，长期页面状态只存在于 Store。
- 路由使用 `HashRouter`，后续 Electron 无需重写页面路由。
- 当前依赖和构建产物不包含 Electron。
- 不包含 Agent、多 Agent 或 AI 对话入口。
