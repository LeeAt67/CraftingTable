# BUG-002 切文档后保存状态串到另一篇文档

| 项目 | 内容 |
| --- | --- |
| 状态 | `已修复待验证` |
| 严重级别 | `P1` |
| 模块 | `docs` |
| 发现日期 | 2026-11-06 |
| 发现方式 | 确定性单测（红）+ 页面复现 |
| 影响版本 | `feat/docs-writing` @ `668ef11` |
| 修复版本 | `feat/docs-writing` @ `0b97df7` |
| 当前分支 | `feat/docs-writing` |

## S — 背景

文档的保存状态是一个全局单值 `saveStatus`，由 `DocTitleInput` 渲染为 `保存中…` / `已保存` / `保存失败，点击重试`。

相关代码：

- 状态定义：`src/controllers/stores/docsStore.ts:11`（`SaveStatus = "idle" | "saving" | "saved" | "error"`）
- 写入点：`src/controllers/stores/docsStore.ts:89-113` 的 `markSaved`
- 触发点：`src/controllers/effects/useDocsEffect.ts` 的 `persistContent`
- 展示点：`src/components/docs/docTitleInput.tsx`
- 切换文档：`setActiveNode` 会把 `saveStatus` 重置为 `idle`（`docsStore.ts:60-70`）

保存请求是异步的，且用 `saveSequence` 串行化；切换文档不会取消已在飞行中的请求。

## T — 期望

规范 [文档工作区设计 §10 自动保存](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md) 规定：保存状态描述的是**当前打开的文档**的保存结果。

文档 A 的保存响应返回时，如果用户已经切到文档 B，那么 B 的保存状态不得被改变——B 从未被编辑，应保持 `idle`。

## A — 复现

前置条件：两篇文档 `doc-a`、`doc-b`，且能让 `saveContent` 返回受控的延迟 Promise。

步骤：

1. 打开 `doc-a`，在正文里输入任意内容（触发 400ms 防抖）。
2. 在防抖窗口内（或保存请求飞行中）让 `saveContent` 挂起。
3. 立即切到 `doc-b`（`setActiveNode(doc-b)`），此时 `saveStatus` 为 `idle`。
4. 放行 `doc-a` 的保存响应。
5. 读 `useDocsStore.getState().saveStatus`。

最小复现用例（Vitest）：

```ts
it("A 的保存响应不应把 B 的 saveStatus 改成 saved", async () => {
  let resolveSave: (value: { id: string; updatedAt: string }) => void =
    () => undefined;
  const slowSave = new Promise<{ id: string; updatedAt: string }>((resolve) => {
    resolveSave = resolve;
  });

  const model = createModel({ saveContent: vi.fn(() => slowSave) });
  const effect = createDocsEffect(model, notify);

  useDocsStore.getState().setActiveNode(nodeA);
  effect.applyContentChange("doc-a", "# 改过的 A");
  const flushing = effect.flushSave();

  useDocsStore.getState().setActiveNode(nodeB);
  expect(useDocsStore.getState().saveStatus).toBe("idle");

  resolveSave({ id: "doc-a", updatedAt: "2026-11-06T00:00:09Z" });
  await flushing;

  expect(useDocsStore.getState().activeNode?.id).toBe("doc-b");
  expect(useDocsStore.getState().saveStatus).toBe("idle");
});
```

## R — 实际

断言失败：

```text
× 保存状态串文档 > A 的保存响应不应把 B 的 saveStatus 改成 saved 6ms
  → expected 'saved' to be 'idle' // Object.is equality

AssertionError: expected 'saved' to be 'idle' // Object.is equality
Expected: "idle"
Received: "saved"
```

用户可见后果：切到一篇从未编辑过的新文档，标题栏直接显示「已保存」，让人误以为该文档刚保存过；如果 A 的保存失败，B 会显示「保存失败，点击重试」，点击后重试的是 B 的保存队列（空的），什么都不会发生。

### 页面复现（修复前实测）

在真实页面里，单测的「保存请求飞行中切文档」不容易出现，因为 `openNode` 会 `await flushSave()`。真正可达的路径是**离开文档页**：卸载时 `dispose()` 触发的 flush 不等待响应，用户可以在响应回来之前打开另一篇文档。

操作序列（用 `addInitScript` 给 `POST /api/v1/docs/save` 注入 2.5s 延迟，其余请求不受影响）：

1. 打开 `import-spec`（`db839f15`），在正文输入标记文本，等 400ms 防抖。
2. `location.hash = '#/agent'` —— 文档页卸载，`dispose()` 发起一次非等待保存。
3. 立刻 `location.hash = '#/docs/db0fd6ea'` 打开「接口设计」（`db0fd6ea`）。
4. 观察标题栏 `role="status"`。

实测时间线（修复前 `4a7b366`）：

```text
step: left to agent     saveCalls: 1
step: opened 接口设计    title: 接口设计   status: ""
step: t=1.8s            title: 接口设计   status: "已保存"     ← 泄漏
step: t=3.8s            title: 接口设计   status: "已保存"

saveLog: [{ body.id: "db839f15-23ca-40a0-a6f5-60ad56b7fa3e",
            start: 5669, end: 8717 }]
```

保存请求的 `body.id` 是 `import-spec`，但「已保存」出现在「接口设计」上 —— 用户从未编辑过「接口设计」。这是页面级的确定性复现，不是推测。


## 根因

`docsStore.ts:89-95` 的 `markSaved` 在检测到「响应不属于当前文档」时，仍然写入了 `saveStatus`：

```ts
markSaved: ({ id, content, updatedAt }) =>
  set((state) => {
    if (state.activeNodeId !== id) {
      return { saveStatus: "saved" as const };   // 明知不是当前文档，仍然改状态
    }
    ...
  }),
```

`return` 只阻止了 `activeNode` 与 `nodes` 的更新，没有阻止 `saveStatus`。这个分支的本意应是「不触碰任何状态」。

## 修复方式（已实施）

`0b97df7` 两处改动：

1. `src/controllers/stores/docsStore.ts` 的 `markSaved`：非当前文档只更新树摘要 `updatedAt`，不再碰 `saveStatus` 与 `activeNode`。

```ts
markSaved: ({ id, content, updatedAt }) =>
  set((state) => {
    const nodes = state.nodes.map((node) =>
      node.id === id ? { ...node, updatedAt } : node,
    );
    if (state.activeNodeId !== id) {
      return { nodes };        // 只同步树摘要，保存状态归当前文档所有
    }
    ...
  }),
```

2. `src/controllers/effects/useDocsEffect.ts` 的 `persistContent`：`saving` 与 `error` 两个状态写入都加了归属判断，避免旧文档的失败把新文档置为「保存失败」。

```ts
if (useDocsStore.getState().activeNodeId === target.id) {
  useDocsStore.getState().setSaveStatus("saving");
}
// ...
} catch (error) {
  const message = messageOf(error);
  if (useDocsStore.getState().activeNodeId === target.id) {
    useDocsStore.getState().setSaveStatus("error");
  }
  notify.error(message);   // 失败提示仍然弹出，只是不落到另一篇文档上
}
```

`notify.error` 刻意保留在归属判断之外：失败必须让用户知道，只是不该污染另一篇文档的状态栏。

## 验证方式

```bash
pnpm vitest run tests/e2e/unit/docs/docsStore.test.ts
pnpm vitest run tests/e2e/unit/docs/useDocsEffect.test.ts
```

### 回归用例

| 用例 | 覆盖点 | 修复前 | 修复后 |
| --- | --- | --- | --- |
| `docsStore > does not mark the active document saved when another document finishes saving` | 非当前文档的 `markSaved` 不改 `saveStatus` 与 `activeNode` | 失败（`saved`） | 通过 |
| `docsEffect > keeps the new document idle when the previous document finishes saving` | 保存成功但归属已切换 | 失败 | 通过 |
| `docsEffect > keeps the new document idle when the previous document fails to save` | 保存失败但归属已切换，且仍弹一次错误提示 | 失败 | 通过 |

### 门禁结果

- `pnpm typecheck`：通过
- `pnpm vitest run`：24 个文件 / 125 个用例全部通过

### 页面验证

同一注入延迟的序列在修复后重跑（`0b97df7`）：

```text
step: typed on 接口设计  status: ""
step: left to agent     saveCalls: 1
step: opened import-spec title: import-spec  status: ""
step: t=1.8s            title: import-spec  status: ""
step: t=3.8s            title: import-spec  status: ""

saveLog: [{ body.id: "db0fd6ea-14de-443e-9d6a-191b0d4118c3",
            start: 42856, end: 45845 }]
```

保存请求属于「接口设计」，而当前打开的 `import-spec` 在 3.8s 内始终保持空状态 —— 泄漏消失。

## 关联

- 设计：[§10 自动保存](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)、[§13 错误处理](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)
- 计划：Task 10（文档 Store）、Task 11（文档 Effect）
- 相关 issue：[BUG-001](./BUG-001-opennode-race.md)（同为「异步结果未校验归属」）
