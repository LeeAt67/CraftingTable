# BUG-001 快速切换文档时旧响应覆盖新文档

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

文档工作区打开一篇文档的流程：

```text
点击目录树一行
  → navigate(`/docs/:nodeId`)
  → DocPage 的 useEffect 监听 nodeId 变化
  → effect.openNode(nodeId)
  → model.getNode(id) 请求 /api/v1/docs/item
  → store.setActiveNode(node) 写入编辑器内容
```

相关代码：

- 打开逻辑：`src/controllers/effects/useDocsEffect.ts:49-67`
- 触发位置：`src/pages/docs/docPage.tsx` 的 `useEffect(() => { if (nodeId) void effect.openNode(nodeId) }, [effect, nodeId])`
- 写入状态：`src/controllers/stores/docsStore.ts` 的 `setActiveNode`

同一文件中的搜索路径（`useDocsEffect.ts:216-250`）已经用 `searchSequence` 做乱序保护，但 `openNode` 没有对应机制——同一个文件存在两套标准。

## T — 期望

规范 [文档工作区设计 §8](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md) 要求 View 始终渲染当前路由对应的节点。

无论两个 `getNode` 请求以什么顺序返回，最终 `activeNode` 必须是 URL 中 `nodeId` 对应的那篇文档。晚到的旧响应必须被丢弃。

## A — 复现

前置条件：

- 至少两篇文档，`doc-a` 与 `doc-b`。
- 能控制 `getNode` 的返回时机（真实网络慢，或在测试里手动档住 Promise）。

步骤：

1. 打开 `/docs`，不选中任何文档。
2. 触发 `openNode('doc-a')`，并让它的 `getNode` 挂起（延迟 3000ms 或手动 resolve）。
3. 300ms 内触发 `openNode('doc-b')`，`doc-b` 立即返回。
4. 等 `doc-a` 的响应最后返回。
5. 读 `useDocsStore.getState().activeNode.id`。

最小复现用例（Vitest）：

```ts
it("慢响应的旧文档不覆盖后打开的新文档", async () => {
  let resolveA: (node: DocNode) => void = () => undefined;
  const slowA = new Promise<DocNode>((resolve) => {
    resolveA = resolve;
  });

  const model = {
    listTree: vi.fn().mockResolvedValue([]),
    getNode: vi.fn((id: string) =>
      id === "doc-a" ? slowA : Promise.resolve(nodeB),
    ),
    createNode: vi.fn(),
    renameNode: vi.fn(),
    moveNode: vi.fn(),
    saveContent: vi.fn(),
    deleteNode: vi.fn(),
    search: vi.fn(),
  } as unknown as DocsModel;

  const effect = createDocsEffect(model, notify);

  const first = effect.openNode("doc-a");
  const second = effect.openNode("doc-b");
  await second;
  expect(useDocsStore.getState().activeNode?.id).toBe("doc-b");

  resolveA(nodeA);
  await first;
  expect(useDocsStore.getState().activeNode?.id).toBe("doc-b");
});
```

## R — 实际

断言失败，晚到的 `doc-a` 覆盖了 `doc-b`：

```text
× openNode 竞态 > 慢响应的旧文档不覆盖后打开的新文档 8ms
  → expected 'doc-a' to be 'doc-b' // Object.is equality

AssertionError: expected 'doc-a' to be 'doc-b' // Object.is equality
Expected: "doc-b"
Received: "doc-a"
```

用户可见后果：URL 是 `#/docs/<doc-b>`，但编辑器里显示的是 `doc-a` 的标题与正文。此时编辑会写向 `doc-a`，与界面所显示的路由不一致。

## 根因

`useDocsEffect.ts:49-67` 的 `openNode` 在 `await model.getNode(id)` 之后无条件调用 `setActiveNode`：

```ts
const openNode = async (id: string) => {
  if (useDocsStore.getState().activeNodeId === id) {
    return;
  }
  await flushSave();
  useDocsStore.getState().setLoading();
  try {
    const node = await model.getNode(id);   // 没有时序校验
    useDocsStore.getState().setActiveNode(node);
  } catch (error) {
    ...
  }
};
```

没有任何机制判断「这个响应是否仍属于当前请求」。前一个 `openNode` 的闭包在 await 结束后继续执行，把过期结果写入唯一的状态源。

## 修复建议

沿用搜索已有的做法，加单调递增的 `loadSequence`：

```ts
let loadSequence = 0;

const openNode = async (id: string) => {
  if (useDocsStore.getState().activeNodeId === id) {
    return;
  }
  await flushSave();
  loadSequence += 1;
  const sequence = loadSequence;
  useDocsStore.getState().setLoading();
  try {
    const node = await model.getNode(id);
    if (sequence !== loadSequence) {
      return;               // 已被更新的请求取代
    }
    useDocsStore.getState().setActiveNode(node);
  } catch (error) {
    if (sequence !== loadSequence) {
      return;
    }
    ...
  }
};
```

注意 `catch` 分支也要校验：旧请求失败不应把新文档的 `phase` 置为 `error`。

涉及文件：`src/controllers/effects/useDocsEffect.ts`。

## 验证方式

```bash
pnpm vitest run tests/e2e/unit/docs/useDocsEffect.test.ts
```

回归用例应断言：

1. 慢响应的 `doc-a` 返回后，`activeNode.id` 仍为 `doc-b`。
2. 旧请求失败时，`phase` 不变成 `error`。

修复后追加到 `tests/e2e/unit/docs/useDocsEffect.test.ts`，不要新增独立文件。

## 关联

- 设计：[2026-11-05-craftingtable-documents-design.md §8 前端 MVC 结构](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)
- 计划：Task 11（文档 Effect）
- 同类实现参考：`useDocsEffect.ts:216` 搜索的 `searchSequence` 守卫
- 相关 issue：[BUG-002](./BUG-002-save-status-cross-document.md)（同一 Effect 的状态归属问题）
