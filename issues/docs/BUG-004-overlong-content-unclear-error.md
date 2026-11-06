# BUG-004 正文超限提示「参数错误」且无前端预校验

| 项目 | 内容 |
| --- | --- |
| 状态 | `待修复` |
| 严重级别 | `P2` |
| 模块 | `docs` |
| 发现日期 | 2026-11-06 |
| 发现方式 | 页面复现 |
| 影响版本 | `feat/docs-writing` @ `668ef11` |
| 当前分支 | `feat/docs-writing` |

## S — 背景

正文长度上限在两侧各定义一次：

| 位置 | 常量 / 校验 |
| --- | --- |
| 后端领域常量 | `server/internal/docs/node.go`：`MaxContentRunes = 200000` |
| 后端校验 | `server/internal/docs/service.go` 的 `SaveContent`：超限返回 `ErrInvalidInput` |
| 前端导入校验 | `src/components/docs/docFileTransfer.ts`：`MAX_IMPORT_RUNES = 200000`，`importMarkdownError` |
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

## 修复建议

1. **后端拆分错误**：为超限单独返回一个可区分的错误（如 `ErrContentTooLong`），`controller.go` 映射为「文档内容过长（上限 200000 字符）」。涉及 `server/internal/docs/node.go`、`service.go`、`controller.go`。
2. **前端预校验**：`applyContentChange` 里按 `MAX_IMPORT_RUNES` 判断，超限时不入队保存，直接 `setSaveStatus("error")` 并只提示一次（用一次性标记去重）。涉及 `src/controllers/effects/useDocsEffect.ts`、`src/components/docs/docFileTransfer.ts`。
3. **文案统一**：上限值在一处导出（前端 `MAX_IMPORT_RUNES`，后端 `MaxContentRunes`），并在两边注释里互相指向。

## 验证方式

```bash
pnpm test:go
pnpm vitest run tests/e2e/unit/docs/useDocsEffect.test.ts
```

回归用例应断言：

1. Go 集成测试：发送 200001 个字符的正文 → 400，`error.message` 为「文档内容过长」。
2. 前端：`applyContentChange` 收到超限内容时**不调用** `model.saveContent`，`saveStatus` 立即为 `error`，`notify.error` 只调用一次。

页面验证：粘贴超长文本后，toast 与状态都应为「文档内容过长」类文案，而不是「参数错误」。

## 关联

- 设计：[§7.3 校验规则](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)、[§13 错误处理](../../docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md)
- 计划：Task 7（HTTP Controller）、Task 11（文档 Effect）、Task 15（导入导出）
- 相关常量：`server/internal/docs/node.go` 的 `MaxContentRunes`、`src/components/docs/docFileTransfer.ts` 的 `MAX_IMPORT_RUNES`
