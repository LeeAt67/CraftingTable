# BUG-003 目录树加载失败后无法恢复且无错误提示

| 项目 | 内容 |
| --- | --- |
| 状态 | `已修复待验证` |
| 严重级别 | `P1` |
| 模块 | `docs` |
| 发现日期 | 2026-11-06 |
| 发现方式 | 页面复现 |
| 影响版本 | `feat/docs-writing` @ `668ef11` |
| 修复提交 | `f13c531`（依赖 `e7850a1` 的 `nodePhase` 拆分） |
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

## 修复方式（已实施，`f13c531`）

四项改动：

1. **状态拆分**（`e7850a1` 已完成）：`phase` 只表示目录树状态，当前文档改用 `nodePhase`。树加载不再被打开文档的失败污染。
2. **侧栏错误态**：`docSidebar.tsx` 在 `phase === "error"` 时渲染 `role="alert"` 卡片，包含标题「加载文档目录失败」、中文原因与「重试」按钮；`phase === "loading"` 时渲染三个骨架行。
3. **守卫允许重试**：挂载守卫从 `phase === "idle"` 放宽为 `phase === "idle" || phase === "error"`，接口恢复后导航回 `#/docs` 即可自动重建目录树，不必整页刷新。
4. **错误文案本地化**：`messageOf` 识别 `Failed to fetch` / `Load failed` / `NetworkError` 等浏览器原始异常，统一返回「网络异常，请检查服务是否可用后重试」。

涉及文件：

- `src/components/docs/docSidebar.tsx`
- `src/controllers/effects/useDocsEffect.ts`
- `src/controllers/stores/docsStore.ts`（`nodePhase` 拆分，见 BUG-001）

## 验证方式

```bash
pnpm typecheck
pnpm vitest run tests/e2e/unit/docs/docSidebar.test.tsx
```

回归用例：

| 用例 | 断言 |
| --- | --- |
| `shows a Chinese error with retry when the tree fails to load` | 失败时出现「加载文档目录失败」+「重试」，原英文 `Failed to fetch` 不出现，点击重试后目录树渲染成功 |
| `retries the tree automatically after remounting from an error` | 处于 `error` 状态时重新挂载会自动再调一次 `listTree` 并恢复 |

门禁结果（2026-11-06）：`pnpm typecheck` 通过，`pnpm vitest run` 122 个用例全部通过。

页面验证（修复后实测）：

1. 拦截 `/api/v1/docs/tree` 并访问 `#/docs` → 侧栏显示 `加载文档目录失败 / 网络异常，请检查服务是否可用后重试 / 重试`，toast 为中文。
2. 取消拦截后点击「重试」→ 目录树恢复正常。
3. 再次拦截制造失败，取消拦截后在应用内导航到 `#/` 再回 `#/docs` → 目录树自动恢复，无需整页刷新（修复前 `rows` 恒为 0）。

## 关联

- 设计：[§13 错误处理](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)、[§12 搜索交互](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)
- 计划：Task 14（侧栏与目录树）
- 对比实现：搜索失败已有内联错误与重试（`docSidebar.tsx`），目录树缺失同等处理
