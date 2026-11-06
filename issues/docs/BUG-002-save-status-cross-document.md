# BUG-002 切文档后保存状态串到另一篇文档

| 项目 | 内容 |
| --- | --- |
| 状态 | `待修复` |
| 严重级别 | `P1` |
| 模块 | `docs` |
| 发现日期 | 2026-11-06 |
| 发现方式 | 确定性单测（红） |
| 影响版本 | `feat/docs-writing` @ `668ef11` |
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

## 修复建议

非当前文档的保存结果直接返回空补丁：

```ts
markSaved: ({ id, content, updatedAt }) =>
  set((state) => {
    if (state.activeNodeId !== id) {
      return {};
    }
    ...
  }),
```

同时建议在 `persistContent` 的 catch 分支做同样判断，避免旧文档的失败把新文档置为 `error`：

```ts
} catch (error) {
  const message = messageOf(error);
  if (useDocsStore.getState().activeNodeId === target.id) {
    useDocsStore.getState().setSaveStatus("error");
  }
  notify.error(message);
}
```

涉及文件：`src/controllers/stores/docsStore.ts`、`src/controllers/effects/useDocsEffect.ts`。

## 验证方式

```bash
pnpm vitest run tests/e2e/unit/docs/docsStore.test.ts
pnpm vitest run tests/e2e/unit/docs/useDocsEffect.test.ts
```

回归用例应断言：

1. 非当前文档的 `markSaved` 不改变 `saveStatus`。
2. 非当前文档的保存失败不把当前文档置为 `error`。
3. 当前文档的正常保存仍然从 `saving` 变为 `saved`。

## 关联

- 设计：[§10 自动保存](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)、[§13 错误处理](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)
- 计划：Task 10（文档 Store）、Task 11（文档 Effect）
- 相关 issue：[BUG-001](./BUG-001-opennode-race.md)（同为「异步结果未校验归属」）
