# Leeat Agent 工作区设计规范

日期：2026-11-04
状态：待评审
范围：Agent 会话 UI 骨架

## 1. 产品定义

Leeat 新增一个独立的 Agent 工作区，用于容纳未来的 Agent 会话。当前版本只交付会话管理外壳和会话占位界面，不实现 Agent 推理、工具调用、多 Agent、消息持久化或后端接口。

Agent 工作区与今日工作区平级，不从属于今日页面：

- 今日：`/`
- Agent：`/agent`
- 设置：`/settings`

进入 Agent 后，一级导航选中 Agent，二级侧边栏切换为会话管理面板，主内容区显示当前会话。

## 2. 目标

- 提供独立的 Agent 一级入口。
- 提供专用二级侧边栏，用于新建会话和会话管理。
- 提供当前会话的主内容区域和输入框占位。
- 保持现有今日页和设置页行为不变。
- 保持 Light / Dark、响应式布局和现有视觉 token 一致。
- 为未来接入真实 Agent 保留清晰边界，不提前实现 Agent 逻辑。

## 3. 非目标

- 不实现真实 Agent 对话。
- 不实现多 Agent 编排。
- 不实现模型选择、工具选择、系统提示词或运行参数。
- 不新增 Agent API。
- 不新增数据库表。
- 不做消息持久化、会话搜索、重命名或删除。
- 不接入 Electron IPC。
- 不改变今日页面的数据模型和 API。

## 4. 信息架构

### 4.1 一级侧边栏

一级侧边栏继续承担全局导航，现有条目为：

1. 首页
2. Agent
3. 设置
4. Light / Dark 切换

Agent 使用会话或消息类图标，并具有 `aria-label="Agent"`。当前路由为 `/agent` 或其子路由时显示选中态和 `aria-current="page"`。

### 4.2 二级侧边栏

Agent 路由下显示 `AgentSessionSidebar`，替代今日页的 `ContextSidebar`。

结构从上到下为：

1. 标题 `Agent`
2. 新建会话按钮
3. 当前会话数量摘要 `今天 2 个会话`
4. 按“今天 / 更早”分组的会话列表
5. 底部状态文案 `Agent 尚未接入`

桌面端显示完整文字；窄屏保持侧边栏可见，压缩为 88px，只显示新建图标和会话首字或图标。

### 4.3 主内容区

Agent 主内容区从上到下为：

1. 当前会话标题和状态
2. 空会话状态
3. 底部输入框和发送按钮

未选择会话时，主内容区显示“选择一个会话或新建会话”。

选择 `/#/agent/new` 时，显示“新会话”和空白消息状态。

## 5. 路由与状态

### 5.1 路由

使用现有 `HashRouter`：

- `/agent`：Agent 工作区入口，默认显示会话选择空状态。
- `/agent/:sessionId`：显示指定会话的占位内容。
- `/agent/new`：新会话草稿状态。

MVP 中会话列表使用固定的 UI 示例数据，用于呈现会话管理结构。示例数据不代表真实业务数据，也不写入 Store 或后端。

### 5.2 新建会话

点击“新建会话”导航到 `/agent/new`：

- 会话管理面板显示一个新会话选中项。
- 主内容区标题为“新会话”。
- 输入框可输入文字。
- 发送按钮使用明确禁用态，不发出消息。
- 刷新页面后仍回到空白新会话，不保存草稿。

### 5.3 会话选择

点击会话列表项导航到对应的 `/agent/:sessionId`。

- 当前会话使用 `aria-current="page"`。
- 主内容区标题更新为会话标题。
- MVP 中所有会话内容均为空状态，不包含真实历史消息。

## 6. MVC 边界

当前功能只增加 View：

- `src/pages/agent/index.tsx`：Agent 页面入口。
- `src/components/agent/agentSessionSidebar.tsx`：会话管理二级侧栏。
- `src/components/agent/agentWorkspace.tsx`：当前会话主内容。
- `src/components/agent/agentComposer.tsx`：输入框和发送按钮。
- `src/components/agent/agentSessionFixtures.ts`：仅用于 UI 骨架的示例会话。

布局修改：

- `AppLayout` 根据当前路径选择今日上下文侧栏或 Agent 会话侧栏。
- `AppShell` 接受二级侧栏插槽，不再硬编码今日上下文侧栏。
- `NavRail` 增加 Agent 入口。
- `routes` 增加 `/agent` 和 `/agent/:sessionId`。

当前不新增 Agent Store、Effect、Model 或 API。只有在接入真实会话数据时才增加对应 Model 和 Controller。

## 7. 视觉规范

### 7.1 布局

桌面端：

```text
┌──────┬──────────────────┬────────────────────────────┐
│ 一级 │ Agent 二级侧栏    │ 当前会话                   │
│ 首页 │ Agent     +      │ 标题                       │
│ Agent│ 今天             │ 空状态                     │
│ 设置 │   示例会话 A      │                            │
│      │ 更早             │ 输入框             发送     │
│      │   示例会话 B      │                            │
└──────┴──────────────────┴────────────────────────────┘
```

宽度沿用现有响应式规则：

- `>=1280px`：一级 56px，二级 232px
- `768px - 1279px`：一级 52px，二级 184px
- `461px - 767px`：一级 44px，二级 104px
- `<=460px`：一级 34px，二级 88px

### 7.2 会话侧栏

- 标题与二级侧栏原有字号保持一致。
- “新建会话”为主按钮，桌面显示图标和文字，窄屏只显示图标。
- 会话按小标题分组。
- 当前会话使用 `brandSoft` 背景和 `brand` 文字。
- 非当前会话使用 `textSecondary`，悬停使用 `surface`。
- 列表必须支持 `min-w-0`、`truncate` 和长标题换行边界。

### 7.3 会话主区域

- 主内容背景使用 `background`。
- 会话标题在内容最大宽度内左对齐。
- 空状态使用轻量描边容器，不使用嵌套卡片。
- 输入区固定在主内容底部，滚动发生在消息区域。
- 输入框使用现有边框、表面和焦点环 token。
- 发送按钮使用品牌色，具有 `aria-label="发送消息"`。

### 7.4 主题

不新增颜色 token，继续使用：

- Light：`background #F2F5F8`、`surface #FFFFFF`、`brand #5468FF`
- Dark：`background #17191D`、`surface #1E2126`、`brand #7B8AFF`

## 8. 响应式

- 390px、768px、1024px、1440px 均不能出现全局横向滚动。
- 二级会话侧栏不能消失。
- 390px 下新建按钮只显示图标，会话标题使用首字或图标表示。
- 输入框和发送按钮不得超出主内容宽度。
- 空状态、标题和输入框不能互相遮挡。

## 9. 无障碍

- Agent 一级入口有 `aria-label="Agent"`。
- 新建会话按钮有 `aria-label="新建会话"`。
- 发送按钮有 `aria-label="发送消息"`。
- 当前导航和当前会话使用 `aria-current="page"`。
- 输入框有可访问名称。
- 图标按钮均包含隐藏文本或 `aria-label`。

## 10. 测试与验收

测试继续统一放在 `tests/e2e`，按模块新增：

- `tests/e2e/modules/agent/navigation.e2e.ts`
- `tests/e2e/modules/agent/sessionSidebar.e2e.ts`
- `tests/e2e/modules/agent/responsive.e2e.ts`

验收标准：

1. 首页、Agent、设置均可从一级导航到达。
2. 进入 Agent 后显示专用二级侧边栏，而不是今日筛选。
3. 点击新建会话后 URL 为 `/#/agent/new`，标题显示“新会话”。
4. 点击示例会话后标题更新且选中态正确。
5. 390px 下两级侧边栏都保持可见。
6. 390px、768px、1024px、1440px 无全局横向滚动。
7. Light / Dark 切换在 Agent 页面正常。
8. 原今日页面的 Quick Capture、完成状态和主题测试继续通过。

## 11. 完成标准

- Agent 一级入口存在且状态正确。
- Agent 二级会话侧栏包含新建会话和会话管理结构。
- Agent 主区域包含当前会话标题、空状态和输入框。
- 所有新增交互均有可访问标签。
- 所有测试位于 `tests/e2e`。
- 当前实现不包含 Agent API、Agent Model、多 Agent 或消息持久化。
