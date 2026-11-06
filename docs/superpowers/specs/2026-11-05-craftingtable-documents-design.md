# CraftingTable Documents Design

- **Status:** Implemented and verified on 2026-11-06
- **Date:** 2026-11-05
- **Scope:** 工作台「文档」工作区：目录树、Markdown 文档编辑、自动保存、服务端搜索、Markdown 导入导出
- **Depends on:** [工程化设计规范](./2026-11-05-craftingtable-engineering-design.md)、[Leeat 个人工作台设计](./2026-11-04-leeat-personal-workbench-design.md)
- **Supersedes:** [Leeat 个人工作台设计](./2026-11-04-leeat-personal-workbench-design.md) 第 3 节「不实现……笔记……」的边界

## 1. 决策摘要

在工作台中新增独立的「文档」工作区，采用以下方案：

```text
一级图标栏新增「文档」
  -> 二级侧栏：搜索框 + 两级目录树
  -> 右侧主内容区：标题输入 + Tiptap WYSIWYG 编辑器
  -> Go docs 模块（Repository port + Controller）
  -> SQLite doc_nodes 单表自引用
```

关键决策：

- 目录节点用**单表 `doc_nodes`** 承载文件夹与文档，以 `kind` 区分，`parent_id` 自引用。表结构从第一天支持任意层级，UI 首版只呈现两级。
- 正文以 **Markdown 字符串**保存（`content` 字段），编辑器使用 Tiptap 3 + 官方 `@tiptap/markdown`，载入用 `contentType: 'markdown'`，保存用 `editor.getMarkdown()`。与参考项目的既有结论保持一致：Markdown 是唯一事实源，不保留 HTML 中间态。
- 正文只存 SQLite。导入导出走浏览器 File API，服务端不实现文件系统读写。
- 搜索走**服务端接口**，不采用前端过滤，为后续 PostgreSQL 全文检索预留稳定契约。
- 删除为软删除（`deleted_at`），首版不提供回收站界面。
- 并发策略明确为 last-write-wins，不实现 revision 乐观锁。
- 不接入 Agent：本期不支持 AI 生成、改写或续写文档。
- 不实现预览模式：WYSIWYG 本身即所见即所得，主内容区单区呈现。

## 2. 当前问题

1. 工作台只有「今日」和「Agent」两个工作区，没有任何长文本写作能力。
2. 早期规范把「笔记」列为非目标，导致文档能力缺少设计与契约，直接实现会与工程规范冲突。
3. 现有依赖中没有 Markdown 编辑器或渲染器，编辑体验需要一次明确的技术选型。
4. 今日模块的 `items` 只适合短文本，`description` 上限 2000 字符，不能承载文档正文。
5. 文档数量增长后，前端全量过滤无法支撑，需要服务端搜索契约。

## 3. 目标与非目标

### 3.1 目标

1. 提供独立的 `/docs` 工作区，包含搜索、两级目录树和编辑区。
2. 文档正文可靠自动保存，刷新页面不丢内容。
3. 目录树支持新建文件夹、新建文档、重命名、移动、删除。
4. 搜索在服务端完成，接口预留游标分页，切到 PostgreSQL 时不改上层契约。
5. 支持导入 `.md` 文件、导出当前文档为 `.md`。
6. 沿用现有 MVC 分层：View 不直接请求 API，Store 是前端业务状态唯一事实源。
7. 视觉完全消费 `src/styles/tokens.css` 的语义 Token，深浅色自动跟随。

### 3.2 非目标

| # | 不做的事 | 说明 / 触发条件 |
|---|----------|-----------------|
| 1 | Agent 生成、改写、续写文档 | 编辑稳定后再复用现有 run 接口 |
| 2 | 多人协同、实时编辑、Yjs | 单用户本地应用 |
| 3 | 版本历史、时间线、差异对比 | 需要回滚时增加内容快照表 |
| 4 | 回收站界面与恢复 | 数据已软删除，等误删真实发生再做 |
| 5 | 无限层级、拖拽排序、拖拽移动 | 表结构已支持，仅 UI 后置 |
| 6 | 文档下挂子文档（文档即文件夹） | 交互成本高，无明确需求 |
| 7 | 图片/附件上传、表格、Mermaid、公式、代码高亮主题 | 需要文件存储与额外扩展 |
| 8 | 预览 / 阅读模式 | 已确认单区 WYSIWYG |
| 9 | 正文全文搜索 | 文档数超过 500 或接入 PostgreSQL 全文索引时再做 |
| 10 | 收藏、置顶、标签、分类元数据 | 加字段即可 |
| 11 | 权限、分享链接、公开只读页 | 单用户本地 |
| 12 | 多标签页并发冲突处理 | 本期明确 last-write-wins |
| 13 | 离线/PWA、Electron 打包适配 | 不在本阶段 |
| 14 | 移动端专属手势交互 | 只做响应式收敛 |
| 15 | 服务端直接读写磁盘 `.md` 目录 | 导入导出走浏览器 File API |
| 16 | 文档模板、大纲 TOC、字数统计 | YAGNI |

### 3.3 阶段边界

本期只实现 SQLite 适配器。接入 PostgreSQL 时只新增 store 适配器与迁移脚本，**不修改** Controller、`docs.Repository` 接口和前端契约；`limit`/`cursor` 语义已按可移植方式定义。

## 4. 信息架构与界面结构

```text
[一级图标栏] 首页 | 文档 | Agent            [二级侧栏] 搜索 + 目录树        [主内容区] 标题 + 编辑器
```

路由：

| 路径 | 内容 |
|------|------|
| `/docs` | 未选中节点时的空状态 |
| `/docs/:nodeId` | 打开指定文档 |

布局规则：

1. 一级图标栏复用 `NavRail`，新增 `FileText` 图标的「文档」入口，`aria-label="文档"`。
2. 二级侧栏沿用今日页的响应式宽度阶梯：`88px → 104px(min 461px) → 184px(min 768px) → 232px(min 1280px)`。
3. 侧栏自上而下：搜索输入框、「新建文档」「新建文件夹」按钮、目录树。
4. 宽度小于 `461px` 时侧栏收为窄栏，仅显示图标与首字，搜索框折叠为图标按钮。
5. 主内容区顶部为标题输入与保存状态，下方为编辑器滚动区。
6. 目录树首版只渲染两级；文件夹展开状态只保存在组件内存，不持久化。

目录树排序规则：

```text
同层排序：kind = folder 优先
        -> sort_order ASC
        -> updated_at DESC
        -> id ASC
```

当前节点高亮使用 `bg-brandSoft text-brand`，与既有导航一致。

## 5. 数据模型

新增迁移 `server/internal/platform/store/migrations/003_docs.sql`：

```sql
CREATE TABLE IF NOT EXISTS doc_nodes (
  id                TEXT PRIMARY KEY,
  parent_id         TEXT REFERENCES doc_nodes(id),
  kind              TEXT NOT NULL CHECK (kind IN ('folder', 'document')),
  title             TEXT NOT NULL,
  content           TEXT NOT NULL DEFAULT '',
  sort_order        INTEGER NOT NULL DEFAULT 0,
  client_request_id TEXT,
  deleted_at        TEXT,
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_doc_nodes_request
  ON doc_nodes(client_request_id) WHERE client_request_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_doc_nodes_parent ON doc_nodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_doc_nodes_updated ON doc_nodes(updated_at DESC);
```

字段语义：

| 字段 | 语义 |
|------|------|
| `id` | 服务端生成 UUID，前端不得协商 |
| `parent_id` | null 表示根节点；只能是未删除的 `folder` |
| `kind` | `folder` 无正文；`document` 可写正文 |
| `content` | Markdown 源码，文件夹恒为空串 |
| `sort_order` | 同级手动排序预留；首版统一写入 `0`，排序退化为按 `updated_at DESC` |
| `client_request_id` | 创建幂等键，仅 create 写入 |
| `deleted_at` | 非空表示软删除；查询一律过滤 |

可移植性约束（写入工程规范）：

1. 不使用 `AUTOINCREMENT`、`rowid`、`json1`、`INSERT OR REPLACE` 等 SQLite 专属语义。
2. 时间统一 UTC ISO 8601 文本，与现有 Agent 表一致。
3. 查询显式 `ORDER BY`，不依赖隐式顺序。
4. 删除文件夹在单个事务内递归标记子孙。
5. 大小写不敏感匹配由 `docs.Repository` 契约定义语义，SQLite 适配器用 `LIKE`，PostgreSQL 适配器用 `ILIKE`。

## 6. 后端模块结构

```text
server/internal/docs/
  node.go        NodeKind、DocNode、CreateNodeInput、RenameNodeInput、MoveNodeInput
  search.go      SearchInput、SearchCursor、SearchHit、游标编解码
  repository.go  Repository port
  service.go     领域校验、防环、递归删除、幂等
  controller.go  HTTP 适配与注册
```

依赖方向：`controller -> service -> Repository`，实现位于 `server/internal/platform/store`，在 `server/cmd/craftingtabled/main.go` 注册一行：

```go
docs.NewController(sqliteStore, logger).Register(router)
```

## 7. API 契约

前缀固定 `/api/v1`，只使用 `GET` 与 `POST`，字段 camelCase，成功后统一 `{ "data": ... }`。

### 7.1 接口清单

| 方法 | 路径 | 请求 | 响应 |
|------|------|------|------|
| GET | `/api/v1/docs/tree` | — | `DocNodeSummary[]`（不含 content） |
| GET | `/api/v1/docs/item?id=` | query | `DocNode`（含 content） |
| GET | `/api/v1/docs/search?q=&limit=&cursor=` | query | `{ items: SearchHit[], nextCursor: string \| null }` |
| POST | `/api/v1/docs/create` | `{ kind, parentId?, title?, clientRequestId }` | `DocNode` |
| POST | `/api/v1/docs/rename` | `{ id, title }` | `DocNodeSummary` |
| POST | `/api/v1/docs/move` | `{ id, parentId }` | `DocNodeSummary` |
| POST | `/api/v1/docs/save` | `{ id, content }` | `{ id, updatedAt }` |
| POST | `/api/v1/docs/delete` | `{ id }` | `{ deletedIds: string[] }` |

### 7.2 类型

```ts
type DocNodeKind = "folder" | "document";

type DocNodeSummary = {
  id: string;
  parentId: string | null;
  kind: DocNodeKind;
  title: string;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
};

type DocNode = DocNodeSummary & {
  content: string;
};

type SearchHit = {
  id: string;
  kind: DocNodeKind;
  title: string;
  parentId: string | null;
  path: string[];
  updatedAt: string;
};
```

### 7.3 校验规则

| 规则 | 失败响应 |
|------|----------|
| `title` trim 后 1–200 rune | `400 VALIDATION_ERROR` |
| 缺省标题：文档 `无标题文档`，文件夹 `未命名文件夹` | — |
| `content` ≤ 200000 rune | `400 VALIDATION_ERROR`，消息「文档内容过长」 |
| `kind` 只能是 `folder` / `document` | `400 VALIDATION_ERROR` |
| `parentId` 必须存在、未删除、kind 为 `folder` | `404 NOT_FOUND` 或 `400 VALIDATION_ERROR` |
| `move` 禁止目标为自己或自己的子孙 | `400 VALIDATION_ERROR`，消息「不能移动到自身或子节点」 |
| 文件夹不允许写正文 | `400 VALIDATION_ERROR` |
| `create` 相同 `clientRequestId` 返回已存在节点 | `200`，不重复插入 |
| 目标节点不存在或已删除 | `404 NOT_FOUND` |

### 7.4 搜索接口

`GET /api/v1/docs/search?q=<keyword>&limit=20&cursor=<opaque>`

| 参数 | 规则 |
|------|------|
| `q` | trim 后 1–100 rune；空串直接返回 `{ items: [], nextCursor: null }`，不视为错误 |
| `limit` | 默认 20，范围 1–50，超范围按边界截断 |
| `cursor` | 上一次响应的 `nextCursor`，opaque 字符串；非法或与 `q` 不匹配则 `400 VALIDATION_ERROR` |

匹配与排序：

1. 仅匹配 `title`，大小写不敏感子串匹配；`%`、`_`、`\` 按字面量处理，使用 `ESCAPE '\'`。
2. 排序：前缀命中优先 → `updated_at DESC` → `id DESC`，保证分页稳定。
3. 文件夹与文档同时返回，由 `kind` 区分；前端只在展示上把文档排在文件夹之前。
4. 只查 `deleted_at IS NULL`。
5. 响应不含正文，避免搜索拉回大字段。

分页实现：

1. 查询 `limit + 1` 条，多出则说明还有下一页；返回前截断为 `limit` 条，`nextCursor` 指向第 `limit` 条。不额外执行 `COUNT`，避免统计与取页之间的竞态。
2. 没有下一页时 `nextCursor` 显式返回 `null`，不使用字段省略。
3. `limit` 默认 20、上限 50 为侧栏搜索的刻意选择，与 IM 频道搜索（默认 50、上限 100）不同：侧栏宽度 232px，20 条已超过一屏。

`path` 通过 `WITH RECURSIVE` 向上收集祖先标题，从根到父，最多 8 层，超过则截断。SQLite 与 PostgreSQL 均支持该语法。

游标编码：`base64.RawURLEncoding(JSON{ rank, updatedAt, id, qHash })`，服务端自行解析，前端不解释内容。`rank` 由 SQL 计算，前缀命中为 `0`，其余为 `1`：

```sql
CASE WHEN lower(title) LIKE lower(?) ESCAPE '\' THEN 0 ELSE 1 END AS rank
```

`qHash` 是规范化查询词（trim 后大小写折叠）的 SHA-256 前 8 字节十六进制。服务端必须先校验 `qHash` 与本次请求的 `q` 一致，否则返回 `400 VALIDATION_ERROR`，消息「搜索游标与查询词不匹配」。

不匹配校验的必要性：`rank` 依赖查询词，若客户端用 `q=A` 的游标请求 `q=B`，位置比较会跳过或重复结果且不报错。用位置拼接的游标无法承载这个字段，因此游标采用命名 JSON 载荷。

游标字段缺失、`rank` 不在 `{0,1}`、`updatedAt` 不是 RFC3339、`id` 不是 UUID 之一时，一律视为非法游标。

分页使用 keyset 比较，排序方向为 `rank ASC, updated_at DESC, id DESC`，predicate 必须显式展开：

```sql
WHERE rank > :rank
   OR (rank = :rank AND updated_at < :updatedAt)
   OR (rank = :rank AND updated_at = :updatedAt AND id < :id)
ORDER BY rank ASC, updated_at DESC, id DESC
LIMIT :limit + 1
```

`rank` 是查询内表达式，SQLite 不保证 `WHERE` 可引用别名，因此实际实现把 `CASE` 表达式重复在 `WHERE` 中，或在子查询里先投影后再过滤；两种写法在 SQLite 与 PostgreSQL 上语义一致。

### 7.5 Repository 契约

```go
type Repository interface {
    ListTree(ctx context.Context) ([]DocNodeSummary, error)
    GetNode(ctx context.Context, id string) (DocNode, error)
    CreateNode(ctx context.Context, input CreateNodeInput) (DocNode, error)
    RenameNode(ctx context.Context, input RenameNodeInput) (DocNodeSummary, error)
    MoveNode(ctx context.Context, input MoveNodeInput) (DocNodeSummary, error)
    SaveContent(ctx context.Context, id, content, updatedAt string) error
    SoftDelete(ctx context.Context, id, deletedAt string) ([]string, error)
    Search(ctx context.Context, input SearchInput) ([]SearchHit, *string, error)
}
```

约定：

1. `Search` 的大小写不敏感语义由接口注释定义，实现各自满足。
2. `SoftDelete` 必须在单个事务内完成递归标记，并返回全部受影响 id。
3. 所有方法不得返回已软删除节点。
4. `Search` 内部取 `limit + 1` 条判定是否有下一页，返回给上层的是截断后的 `limit` 条与 `nextCursor`。
5. 游标 `qHash` 校验在 `search.go` 的编解码层完成；Controller 只负责把解析失败映射为 `400 VALIDATION_ERROR`。

## 8. 前端 MVC 结构

```text
src/models/docContracts.ts              DocNode、SearchHit、DocsModel
src/models/apiDocsModel.ts              fetch + envelope，复用 apiModel 风格
src/models/docsModel.ts                 api / fake 选择，与 todayModel 同构
src/controllers/stores/docsStore.ts     树、当前节点、草稿、保存状态、搜索状态
src/controllers/effects/useDocsEffect.ts 加载、创建、重命名、移动、保存、删除、搜索
src/layouts/docsWorkspaceLayout.tsx
src/components/docs/docSidebar.tsx      搜索框 + 新建按钮 + 树容器
src/components/docs/docTree.tsx         递归渲染，UI 限两级
src/components/docs/docTreeItem.tsx     行内菜单：重命名 / 移动 / 删除
src/components/docs/docEditor.tsx       Tiptap 封装 + BubbleMenu
src/components/docs/docTitleInput.tsx   标题输入 + 保存状态
src/pages/docs/index.tsx                空状态
src/pages/docs/docPage.tsx              /docs/:nodeId
src/styles/docsProse.css                编辑器排版样式（消费 tokens）
```

Store 状态：

```ts
type DocsState = {
  nodes: DocNodeSummary[];
  activeNodeId: string | null;
  activeNode: DocNode | null;
  draftContent: string;
  saveStatus: "idle" | "saving" | "saved" | "error";
  phase: "idle" | "loading" | "ready" | "error";
  errorMessage: string | null;
  searchQuery: string;
  searchResults: SearchHit[];
  searchPhase: "idle" | "loading" | "ready" | "error";
  searchCursor: string | null;
};
```

约束：

1. View 只读 Store、只调用 Effect 暴露的动作，不导入 Model。
2. Effect 负责所有异步与错误处理，写回 Store。
3. Store 不保存 `Editor` 实例、`AbortController`、DOM 节点等不可序列化对象。
4. 编辑器实例由 `docEditor.tsx` 内部持有，通过 `onChange` 回调把 Markdown 交给 Effect。

## 9. 编辑器选型

已核实的依赖（npm registry，2026-11-05）：

| 包 | 版本 | 许可证 |
|----|------|--------|
| `@tiptap/react` | 3.32.3 | MIT |
| `@tiptap/pm` | 3.32.3 | MIT |
| `@tiptap/starter-kit` | 3.32.3 | MIT |
| `@tiptap/markdown` | 3.32.3 | MIT |
| `@tiptap/extensions` | 3.32.3 | MIT |
| `@radix-ui/react-dropdown-menu` | 2.2.1 | MIT |

选型依据：

1. 参考项目中该组合已在生产使用，并将 Markdown 作为唯一事实源，双向转换链已验证。
2. 只读渲染不引入 `react-markdown`，因为本期不做预览模式。
3. 工具栏使用 `@tiptap/react/menus` 的 `BubbleMenu`（已确认该 subpath 存在），不手写浮层定位。
4. 不引入 Mantine、BlockNote 等携带自有设计系统的编辑器，避免与 `tokens.css` 形成双轨。
5. Tiptap 3 已把 Placeholder 收敛进 `@tiptap/extensions`（`@tiptap/extension-placeholder` 在 3.32.3 仅是对它的 peer 转发），因此不单独安装 `extension-placeholder`；`StarterKit` 自身已依赖 `@tiptap/extensions`，显式声明该依赖只是为了从它导入 `Placeholder`。
6. 链接、列表、代码块、引用、分割线均在 `StarterKit` 内，无需逐个安装子扩展。

启用的能力（全部来自 StarterKit）：

```text
段落、H1–H3、粗体、斜体、删除线、行内代码、代码块、
引用、有序列表、无序列表、分割线、链接
```

明确不启用：表格、图片、Mermaid、数学公式、代码高亮主题、mention、slash 命令。

Markdown 往返约定：

1. 载入：`contentType: 'markdown'`，内容来自 `GET /docs/item`。
2. 保存：`editor.getMarkdown()` 输出。
3. 不保留 HTML 中间态，不使用正则回写 Markdown。
4. 若出现往返不一致，以「保存后再载入结果一致」为验收标准补测试。

## 10. 自动保存

| 动作 | 触发 |
|------|------|
| 正文保存 | 编辑后 400ms trailing debounce |
| 标题保存 | 输入后 600ms trailing debounce |
| 立即保存 | `Ctrl/Cmd + S` |
| 强制 flush | 切换文档、路由离开、`beforeunload` |

状态机：

```text
idle -> saving -> saved
              -> error -> saving（用户重试或再次编辑）
```

要求：

1. `保存中…` 提示至少显示 500ms，避免闪烁（沿用参考项目验证过的经验值）。
2. 保存失败时保留草稿在 Store 中，并显示「保存失败，点击重试」。
3. 保存请求携带最后一次编辑的 `content` 快照，响应返回 `updatedAt` 后更新树顺序。
4. 切换文档前必须 flush；若 flush 失败，阻止切换并提示。
5. 同一文档的保存请求串行化，避免后发先至覆盖。

## 11. 导入与导出

导入：

1. 入口为隐藏的 `<input type="file" accept=".md,.markdown,.txt,text/markdown">`。
2. 读取文本后调用 `POST /docs/create`，`kind=document`，标题取文件名去掉扩展名。
3. 导入位置为当前选中文件夹；未选中时为根。
4. 失败场景给中文提示：非文本、空文件、超过 200000 rune、读取异常。
5. 允许重复标题，不做同名冲突校验。

导出：

1. 前端用 `Blob` + `a[download]`，文件名 `{title}.md`，非法文件名字符替换为 `-`。
2. 导出内容取 Store 中最新草稿；若存在未保存改动，先 flush 再导出。
3. 后端不参与文件读写。

## 12. 搜索交互

1. 侧栏搜索框输入后 300ms trailing debounce 调 `GET /docs/search`。
2. 每次请求携带 `AbortController`，新请求发出前中止上一个，响应只接受最后一次请求的结果。
3. 搜索态下侧栏展示扁平结果，每行显示标题与父级路径（`path.join(" / ")`）。
4. 清空 `q` 立即回到目录树，不发请求。
5. 结果为空时显示「没有匹配的文档」。
6. 目录树仍由 `GET /docs/tree` 提供，与搜索互不影响。

## 13. 错误处理

| 场景 | 行为 |
|------|------|
| 树加载失败 | 主区域显示错误卡片 + 「重试」 |
| 节点不存在或已删除 | 清空当前节点，显示「文档不存在或已被删除」 |
| 保存失败 | 顶部状态变为可点击重试，草稿保留 |
| 创建/重命名/移动/删除失败 | toast 中文错误，状态回滚 |
| 删除确认 | Dialog 二次确认，文件夹文案说明会一并删除内部内容 |
| 搜索失败 | 侧栏内联错误 + 重试按钮，不清空已有结果 |

所有错误文案使用中文；错误信息来自 `ErrorBody.message`，前端不拼接技术细节。

## 14. 测试与验收

### 14.1 测试矩阵

| 层 | 覆盖 |
|----|------|
| Go 单测 | 标题与内容校验、防环移动、文件夹递归软删除、游标编解码 |
| Go 集成 | 迁移幂等、tree/item/create/rename/move/save/delete 全链路、create 幂等、搜索分页与转义 |
| Vitest | store 归约、effect debounce（fake timers）、树扁平化与排序、搜索乱序保护、导入文件名解析 |
| Playwright | 新建文档→输入→刷新内容仍在；新建文件夹→移动文档；搜索过滤；删除二次确认；导入导出 |

### 14.2 验收标准

1. 输入后 500ms 内落库，刷新页面内容不丢。
2. 目录树两级结构稳定，排序符合第 4 节规则。
3. 搜索 200 篇内即时响应；`%`、`_`、`\` 按字面量匹配，不返回全量。
4. 连续快速输入只发一次搜索请求，无乱序覆盖。
5. 导入 `.md` 后标题、列表、代码块、引用在编辑器中正确还原，导出后重新导入结构一致。
6. 深浅色下正文、代码块、引用块配色均来自 Token。
7. 删除文件夹后其子孙不再出现在树与搜索结果中。
8. `pnpm typecheck`、`pnpm test`、`pnpm test:go` 全绿。

## 15. 安全与隐私

1. 日志只记录 `request_id`、路由、状态码和耗时，不记录标题与正文。
2. 正文不进入 URL，不使用 GET 传输。
3. 前端渲染标题时使用 React 文本节点，不使用 `dangerouslySetInnerHTML`。
4. 搜索关键词在 SQL 中使用参数绑定，不拼接字符串。

## 16. 文档治理

1. 本文为文档能力的唯一设计事实源。
2. 更新 [Leeat 个人工作台设计](./2026-11-04-leeat-personal-workbench-design.md) 第 3 节，在「不实现……笔记……」处标注该限制已由本文取代。
3. `docs/glossary.md` 新增术语：文档节点、文件夹、目录树、软删除。
4. API 契约变更必须同时更新本文、类型、Schema 与测试。

## 17. 实施顺序

```text
1. 迁移与 Repository 契约
2. service 校验 + Go 单测
3. Controller + 集成测试
4. 前端 contracts / apiDocsModel / docsModel
5. docsStore + useDocsEffect + Vitest
6. 布局、侧栏、目录树
7. Tiptap 编辑器与 BubbleMenu
8. 自动保存与保存状态
9. 搜索交互接入
10. 导入导出
11. Playwright 端到端
12. 文档更新与术语表
```
