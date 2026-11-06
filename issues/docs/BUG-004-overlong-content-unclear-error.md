# BUG-004 正文超限提示「参数错误」且无前端预校验

| 项目 | 内容 |
| --- | --- |
| 状态 | `已修复待验证` |
| 严重级别 | `P2` |
| 模块 | `docs` |
| 发现日期 | 2026-11-06 |
| 发现方式 | 页面复现 |
| 影响版本 | `feat/docs-writing` @ `668ef11` |
| 修复版本 | `feat/docs-writing` @ `2263272` |
| 当前分支 | `feat/docs-writing` |

## S — 背景

正文长度上限在两侧各定义一次：

| 位置 | 常量 / 校验 |
| --- | --- |
| 后端领域常量 | `server/internal/docs/node.go`：`MaxContentRunes = 200000` |
| 后端校验 | `server/internal/docs/service.go` 的 `SaveContent`：超限返回 `ErrInvalidInput` |
| 前端导入校验 | `src/components/docs/docFileTransfer.ts`：`MAX_CONTENT_RUNES = 200000`（修复前叫 `MAX_IMPORT_RUNES`），`importMarkdownError` |
| 前端编辑校验 | **不存在**——`applyContentChange` 不判长度 |

后端错误映射（`server/internal/docs/controller.go:217-218`）：

```go
case errors.Is(err, ErrInvalidInput):
    c.validationError(w, r, "参数错误")
```

## T — 期望

规范 [§7.3 校验规则](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md) 明确要求：

> `content` ≤ 200000 rune → `400 VALIDATION_ERROR`，**消息「文档内容过长」**

规范 [§13 错误处理](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md) 要求错误文案为中文且不暴露技术细节。

期望行为：

1. 正文超限时提示明确指出「文档内容过长」，而不是笼统的「参数错误」。
2. 前端在提交前就知道超限，避免每 400ms 防抖都发一次注定失败的请求。
3. 用户能知道「再删多少字才能保存」，而不是面对一个永远失败的保存状态。

## A — 复现

前置条件：`pnpm dev` 已启动，已打开任意文档（例如 `import-spec`）。

步骤：

1. 打开 `http://localhost:5173/#/docs/<任意文档 id>`。
2. 在正文编辑区聚焦。
3. 粘贴一段长度超过 200000 字符的纯文本（例如 `'x'.repeat(200050)`）。
4. 观察标题栏右侧状态与 toast。
5. 继续在正文里输入任意字符。
6. 再次观察状态与 toast。

页面注入脚本：

```ts
await page.evaluate(() => {
  const editor = document.querySelector('[aria-label="文档正文"]');
  editor.focus();
  const huge = 'x'.repeat(200050);
  const dt = new DataTransfer();
  dt.setData('text/plain', huge);
  editor.dispatchEvent(
    new ClipboardEvent('paste', {
      clipboardData: dt,
      bubbles: true,
      cancelable: true,
    }),
  );
});
```

## R — 实际

第 4 步：

```text
状态    ：保存失败，点击重试
toast   ：参数错误
控制台  ：POST /api/v1/docs/save → 400 (Bad Request)
正文字符：200077
```

第 5-6 步继续输入后，状态仍为「保存失败，点击重试」，并且**只能看到「参数错误」这一条信息**——用户无法从文案判断是长度问题，也不知道上限是多少。

补充观察：单测里每次失败保存都会调用一次 `notify.error`（3 次输入 → 3 次调用），页面上未观察到 toast 堆积（Sonner 对相同文案做了视觉合并），因此「toast 刷屏」未在页面复现，**不登记为独立缺陷**。

## 根因

1. **后端文案与规范不一致**：`SaveContent` 的长度超限与其它参数错误共用 `ErrInvalidInput`，`controller.go` 统一映射成「参数错误」，规范要求的「文档内容过长」没有实现。
2. **前端无编辑期校验**：`applyContentChange`（`src/controllers/effects/useDocsEffect.ts`）只写草稿与防抖，不判 `MAX_IMPORT_RUNES`。只有 `importFile` 走了 `importMarkdownError`。
3. **上限常量跨语言重复**：`MAX_IMPORT_RUNES`（TS）与 `MaxContentRunes`（Go）各自硬编码 200000，改动时容易漂移。

## 修复方式（已实施）

`2263272` 三部分：

1. **后端可区分错误**：新增 `ErrContentTooLong`（`server/internal/docs/node.go`），`Service.SaveContent` 返回它，Controller 映射为规范要求的文案。

```go
case errors.Is(err, ErrContentTooLong):
    c.validationError(w, r, "文档内容过长（上限 200000 字符）")
```

2. **前端编辑期预校验**：`applyContentChange` 在入队前判长度，超限清掉待发草稿与防抖定时器，状态切到新增的 `too-long`，并用一次性标记保证同一段超限过程只提示一次；改回上限内后标记复位，自动保存恢复。

```ts
if ([...content].length > MAX_CONTENT_RUNES) {
  pendingSave = null;
  if (contentTimer) { clearTimeout(contentTimer); contentTimer = null; }
  if (useDocsStore.getState().activeNodeId === id) {
    useDocsStore.getState().setSaveStatus("too-long");
  }
  if (!contentTooLongNotified) {
    contentTooLongNotified = true;
    notify.error(CONTENT_TOO_LONG_MESSAGE);
  }
  return;
}
```

3. **上限单一来源**：`MAX_CONTENT_RUNES` 与 `CONTENT_TOO_LONG_MESSAGE` 落在契约层 `src/models/docContracts.ts`（注释指向后端 `MaxContentRunes`），`docFileTransfer.ts` 与 Effect 都从这里取，不再各自硬编码。

配套的展示改动：`SaveStatus` 增加 `too-long`，标题栏渲染「内容过长，无法保存」（红色、不可点击——重试也救不了超限正文），`docPage` 的导入/导出拦截改为 `isSaveBlocked(status)` 同时覆盖 `error` 与 `too-long`。

## 验证方式

```bash
pnpm test:go
pnpm vitest run tests/e2e/unit/docs/useDocsEffect.test.ts
```

### 回归用例

| 用例 | 覆盖点 | 修复前 | 修复后 |
| --- | --- | --- | --- |
| `TestDocsAPIRejectsOverlongContent`（Go 集成） | 200001 字符 → 400 且消息为「文档内容过长（上限 200000 字符）」 | 失败（「参数错误」） | 通过 |
| `docsEffect > refuses to save overlong content and warns only once` | 超限不入队、不发请求、状态 `too-long`、提示仅一次 | 失败（发请求） | 通过 |
| `docsEffect > saves again once the content is trimmed back under the limit` | 删短后自动保存恢复，状态回到 `saved` | — | 通过 |
| `docFileTransfer` 上传超限用例 | 改用 `MAX_CONTENT_RUNES` 后行为不变 | 通过 | 通过 |

### 门禁结果

- `pnpm typecheck`：通过
- `pnpm vitest run`：24 个文件 / 127 个用例全部通过
- `go build ./...`、`go vet ./...`：通过
- `go test ./server/... ./tests/e2e/integration/...`：全部通过

### 页面验证

打开 `import-spec`，在正文粘贴 200050 个 `x`，再粘一次：

```text
状态栏  ：内容过长，无法保存
toast   ：文档内容过长（上限 200000 字符）
第二次粘贴后 toast 数量：1（未重复提示）
正文长度：200105（草稿保留在编辑器里，没有被清空）
```

探针有效性单独做了对照组：同一次注入的 `fetch` 探针成功记录了 `/api/v1/docs/tree` 与 `/api/v1/docs/search?q=import`，说明探针本身工作正常；在此基础上，两次超限粘贴之后 `/docs/save` 的调用数为 **0**——超限确实没有发请求。

后端文案在运行中的服务上直接验证：

```text
POST /api/v1/docs/save  { content: 'x'.repeat(200001) }
→ 400 VALIDATION_ERROR  文档内容过长（上限 200000 字符）
```

## 关联

- 设计：[§7.3 校验规则](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)、[§10 自动保存](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)、[§13 错误处理](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)
- 计划：Task 7（HTTP Controller）、Task 11（文档 Effect）、Task 15（导入导出）
- 相关常量：`server/internal/docs/node.go` 的 `MaxContentRunes`、`src/models/docContracts.ts` 的 `MAX_CONTENT_RUNES`
