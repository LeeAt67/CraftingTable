# BUG-003 目录树加载失败后无法恢复且无错误提示

| 项目 | 内容 |
| --- | --- |
| 状态 | `待修复` |
| 严重级别 | `P1` |
| 模块 | `docs` |
| 发现日期 | 2026-11-06 |
| 发现方式 | 页面复现 |
| 影响版本 | `feat/docs-writing` @ `668ef11` |
| 当前分支 | `feat/docs-writing` |

## S — 背景

文档工作区进入 `/docs` 时，侧栏在挂载时拉取一次目录树：

```ts
// src/components/docs/docSidebar.tsx:31-36
useEffect(() => {
  // 目录树只拉取一次：store 已有数据时不再覆盖，避免重复请求。
  if (useDocsStore.getState().phase === "idle") {
    void effect.loadTree();
  }
}, [effect]);
```

相关代码：

- 侧栏加载与守卫：`src/components/docs/docSidebar.tsx:31-36`
- `loadTree`：`src/controllers/effects/useDocsEffect.ts:38-47`，失败时调用 `setError`
- 状态写入：`src/controllers/stores/docsStore.ts`
  - `setLoading` → `phase = "loading"`
  - `setTree` → `phase = "ready"`
  - `setError` → `phase = "error"`
- 侧栏渲染：`src/components/docs/docSidebar.tsx:100-160`，只处理「搜索」与「目录树」两种视图，**没有树错误态**

`phase` 是单一字段，同时被「目录树加载」和「当前文档加载」（`openNode`）复用。

## T — 期望

规范 [§13 错误处理](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md) 规定：

> 树加载失败 → 主区域显示错误卡片 + 「重试」

同时 §12 规定搜索失败要有内联错误与重试。目录树加载失败应满足：

1. 侧栏显示中文错误提示，不是英文原始异常。
2. 提供「重试」入口，点击后重新拉取。
3. API 恢复后无需刷新整页即可恢复。

## A — 复现

前置条件：`pnpm dev` 已启动（Web 5173 + API 3001）。

步骤：

1. 在浏览器开发者工具里把 `/api/v1/docs/tree` 置为失败（Network 面板 Block request URL，或用 Playwright 拦截 abort）。
2. 访问 `http://localhost:5173/#/docs`。
3. 观察侧栏与 toast。
4. 恢复该接口（取消拦截）。
5. 在应用内切换到 `#/` 再回到 `#/docs`。
6. 再次观察侧栏。

Playwright 复现脚本：

```ts
await page.route('**/api/v1/docs/tree**', (route) => route.abort('failed'));
await page.goto('http://localhost:5173/#/docs');
await page.waitForTimeout(2500);
```

## R — 实际

第 3 步观察到的结果：

```text
侧栏文本      ：新建文档 / 新建文件夹        ← 树区域完全空白
树行数        ：0
主区域        ：选择或新建一篇文档
可点击按钮    ：["新建文档", "新建文件夹"]    ← 没有「重试」
toast         ：Failed to fetch              ← 英文原始异常
控制台        ：GET http://localhost:5173/api/v1/docs/tree net::ERR_FAILED
```

第 4-6 步（接口已恢复，并在应用内来回导航）之后：

```text
树行数        ：0
侧栏文本      ：新建文档 / 新建文件夹
```

树依然为空。只有整页刷新（`F5`）才能恢复。

## 根因

两个问题叠加：

1. **守卫条件永久失效**：`loadTree` 失败后 `phase` 变成 `"error"`，而挂载守卫要求 `phase === "idle"`。侧栏重新挂载时（路由切换会重建组件）不会再次请求，因此状态卡死。
2. **侧栏没有错误态 UI**：`docSidebar.tsx` 的树视图没有读取 `phase` / `errorMessage`，也没有重试按钮。主区域的「重试」按钮调用的是 `openNode`（`docPage.tsx`），不是 `loadTree`。

`phase` 被树加载与节点加载共用，进一步放大了问题：打开某篇已删除文档失败后，`phase` 也变成 `"error"`，同样会阻止树重新加载。

英文 toast 来自 `messageOf(error)`：`fetch` 抛出的 `TypeError("Failed to fetch")` 被原样展示，没有做本地化映射。

## 修复建议

1. **拆分状态**：把单一 `phase` 拆成 `treePhase` 与 `nodePhase`，让树加载与节点加载互不影响。涉及 `src/controllers/stores/docsStore.ts`、`src/controllers/effects/useDocsEffect.ts`、`src/pages/docs/docPage.tsx`。
2. **侧栏补错误态**：树视图在 `treePhase === "error"` 时渲染中文错误行与「重试」按钮，调用 `effect.loadTree()`。涉及 `src/components/docs/docSidebar.tsx`。
3. **守卫改为允许重试**：挂载守卫从 `phase === "idle"` 改为 `treePhase === "idle"`；重试时无条件发起请求。
4. **错误文案本地化**：`messageOf` 对非 `Error` 或网络类异常返回中文兜底，例如「网络异常，请稍后重试」。涉及 `src/controllers/effects/useDocsEffect.ts`。

## 验证方式

```bash
pnpm typecheck
pnpm vitest run tests/e2e/unit/docs/docSidebar.test.tsx
```

回归用例应断言：

1. `loadTree` 失败时侧栏出现「加载文档目录失败」与「重试」按钮。
2. 点击重试后会再次调用 `model.listTree`。
3. 重试成功后渲染出目录树。
4. 失败提示为中文。

页面验证：

1. 拦截 `/api/v1/docs/tree` 并访问 `#/docs`。
2. 确认侧栏出现中文错误与重试按钮。
3. 取消拦截，点击重试，确认目录树出现（不刷新整页）。

## 关联

- 设计：[§13 错误处理](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)、[§12 搜索交互](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)
- 计划：Task 14（侧栏与目录树）
- 对比实现：搜索失败已有内联错误与重试（`docSidebar.tsx`），目录树缺失同等处理
