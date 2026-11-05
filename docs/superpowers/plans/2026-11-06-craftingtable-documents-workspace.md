# CraftingTable Documents Workspace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在工作台中交付 `/docs` 文档工作区，包含两级目录树、Markdown 所见即所得编辑、自动保存、服务端搜索和 Markdown 导入导出。

**Architecture:** 后端新增 `server/internal/docs` 模块（Repository 端口 + Service 领域校验 + Controller HTTP 适配），持久化由 `store` 的 SQLite 适配器实现，数据落在单表自引用的 `doc_nodes`。前端沿用既有 MVC：`docContracts`/`apiDocsModel` 负责协议，`docsStore` 是唯一业务状态源，`useDocsEffect` 承担全部异步与错误处理，View 只读 Store 并调用 Effect。

**Tech Stack:** Go 1.2x + `net/http` ServeMux + modernc.org/sqlite；React 19 + TypeScript 5.9 + Zustand 5 + React Router 7 + Tailwind 3 + Radix Dialog/DropdownMenu；Tiptap 3.32.3（`@tiptap/react`、`@tiptap/pm`、`@tiptap/starter-kit`、`@tiptap/markdown`、`@tiptap/extensions`）。

**Design Spec:** [2026-11-05-craftingtable-documents-design.md](../specs/2026-11-05-craftingtable-documents-design.md)

## Global Constraints

- API 前缀固定 `/api/v1`；只使用 `GET` 与 `POST`；成功后统一 `{ "data": ... }`，失败统一 `{ "error": { code, kind, message, retryable, requestId } }`。
- API 字段 camelCase，数据库字段 snake_case；时间统一 UTC ISO 8601 文本（`time.RFC3339Nano`）。
- 不使用 `AUTOINCREMENT`、`rowid`、`json1`、`INSERT OR REPLACE` 等 SQLite 专属语义。
- 所有查询显式 `ORDER BY`，并过滤 `deleted_at IS NULL`。
- 标题 1–200 rune；正文 ≤ 200000 rune；搜索关键词 1–100 rune。
- 搜索默认 `limit=20`、上限 50；游标为 `base64.RawURLEncoding(JSON{rank, updatedAt, id, qHash})`。
- 前端 View 不得导入 Model；Store 不保存 `Editor`、`AbortController`、DOM 节点。
- 颜色只用 `src/styles/tokens.css` 的语义 Token（`bg-surface`、`text-textPrimary`、`border-border`、`bg-brandSoft` 等）。
- 所有用户可见文案为中文。
- 文档与提交信息中不得出现内部项目代号与模型供应商名，统一写「参考项目」「上游模型」。
- 提交信息格式：`feat: ...` / `test: ...` / `docs: ...` / `chore: ...`。

---

## File Structure

**后端**

| 文件 | 职责 |
|---|---|
| `server/internal/platform/store/migrations/003_docs.sql` | 建表与索引 |
| `server/internal/docs/node.go` | 节点类型、错误、标题归一化 |
| `server/internal/docs/repository.go` | `Repository` 端口 |
| `server/internal/docs/search.go` | 搜索入参、游标编解码、关键词清洗 |
| `server/internal/docs/service.go` | 领域校验与用例编排 |
| `server/internal/docs/controller.go` | HTTP 适配与路由注册 |
| `server/internal/platform/store/doc_node_store.go` | 节点读写实现 |
| `server/internal/platform/store/doc_search_store.go` | 搜索实现 |

**前端**

| 文件 | 职责 |
|---|---|
| `src/models/docContracts.ts` | 契约类型与 `DocsModel` |
| `src/models/apiDocsModel.ts` | API 适配器 |
| `src/models/fakeDocsModel.ts` | 测试与 mock 模式的内存实现 |
| `src/models/docsModel.ts` | api / fake 选择 |
| `src/controllers/stores/docsStore.ts` | 业务状态 |
| `src/controllers/effects/useDocsEffect.ts` | 异步动作与防抖 |
| `src/layouts/docsWorkspaceLayout.tsx` | 侧栏 + 内容区布局 |
| `src/components/docs/docSidebar.tsx` | 搜索框、新建按钮、目录树宿主 |
| `src/components/docs/docTree.tsx` | 目录树渲染 |
| `src/components/docs/docTreeItem.tsx` | 单行节点与操作菜单 |
| `src/components/docs/docNodeDialog.tsx` | 重命名 / 移动 / 删除对话框 |
| `src/components/docs/docEditor.tsx` | Tiptap 编辑器与 BubbleMenu |
| `src/components/docs/docTitleInput.tsx` | 标题与保存状态 |
| `src/components/docs/docFileTransfer.ts` | 文件名解析与下载 |
| `src/pages/docs/index.tsx` | 未选中文档的空状态 |
| `src/pages/docs/docPage.tsx` | 文档编辑页 |
| `src/styles/docsProse.css` | 编辑器排版样式 |

---

## Task 1: 数据表与迁移护栏

**Files:**
- Create: `server/internal/platform/store/migrations/003_docs.sql`
- Modify: `tests/e2e/integration/migrations.integration_test.go:50`

**Interfaces:**
- Consumes: 无
- Produces: 表 `doc_nodes(id, parent_id, kind, title, content, sort_order, client_request_id, deleted_at, created_at, updated_at)`；索引 `idx_doc_nodes_request`、`idx_doc_nodes_parent`、`idx_doc_nodes_updated`

- [ ] **Step 1: 写迁移文件**

创建 `server/internal/platform/store/migrations/003_docs.sql`：

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

- [ ] **Step 2: 扩展迁移测试覆盖新迁移**

在 `tests/e2e/integration/migrations.integration_test.go` 中把断言列表改为三个迁移：

```go
	for _, name := range []string{
		"001_initial.sql",
		"002_agent.sql",
		"003_docs.sql",
	} {
		if len(applied[name]) != 64 {
			t.Fatalf("migration %s checksum = %q", name, applied[name])
		}
	}
```

- [ ] **Step 3: 运行迁移测试**

Run: `go test ./tests/e2e/integration/ -run TestSQLiteMigrations -v`
Expected: PASS，两个迁移测试均通过

- [ ] **Step 4: 提交**

```bash
git add server/internal/platform/store/migrations/003_docs.sql tests/e2e/integration/migrations.integration_test.go
git commit -m "feat: add document node table migration"
```

---

## Task 2: 领域类型与 Repository 端口

**Files:**
- Create: `server/internal/docs/node.go`
- Create: `server/internal/docs/repository.go`
- Test: `server/internal/docs/node_test.go`

**Interfaces:**
- Consumes: Task 1 的 `doc_nodes` 表
- Produces:
  - `docs.NodeKind`（`NodeKindFolder`、`NodeKindDocument`）
  - `docs.NodeSummary{ID, ParentID *string, Kind, Title, SortOrder, CreatedAt, UpdatedAt}`
  - `docs.Node`（`NodeSummary` + `Content string`）
  - `docs.CreateNodeInput{Kind, ParentID *string, Title, ClientRequestID *string}`
  - `docs.RenameNodeInput{ID, Title}`、`docs.MoveNodeInput{ID, ParentID *string}`、`docs.SaveContentInput{ID, Content}`
  - `docs.NormalizeTitle(kind, value) string`
  - 错误 `ErrNodeNotFound`、`ErrInvalidInput`、`ErrInvalidCursor`、`ErrParentNotFolder`、`ErrInvalidMove`、`ErrFolderHasContent`
  - `docs.Repository` 端口

- [ ] **Step 1: 写失败测试**

创建 `server/internal/docs/node_test.go`：

```go
package docs

import "testing"

func TestNormalizeTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		kind  NodeKind
		value string
		want  string
	}{
		{name: "trims document title", kind: NodeKindDocument, value: "  接口设计  ", want: "接口设计"},
		{name: "document default", kind: NodeKindDocument, value: "   ", want: DefaultDocumentTitle},
		{name: "folder default", kind: NodeKindFolder, value: "", want: DefaultFolderTitle},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeTitle(test.kind, test.value); got != test.want {
				t.Fatalf("NormalizeTitle() = %q, want %q", got, test.want)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./server/internal/docs/ -run TestNormalizeTitle -v`
Expected: FAIL，`undefined: NormalizeTitle`

- [ ] **Step 3: 写领域类型**

创建 `server/internal/docs/node.go`：

```go
// Package docs 实现工作台文档与目录树的领域模型。
package docs

import (
	"context"
	"errors"
	"strings"
)

// NodeKind 是目录节点类型。
type NodeKind string

const (
	// NodeKindFolder 表示文件夹，没有正文。
	NodeKindFolder NodeKind = "folder"
	// NodeKindDocument 表示文档，正文为 Markdown。
	NodeKindDocument NodeKind = "document"
)

var (
	// ErrNodeNotFound 表示目录节点不存在或已被删除。
	ErrNodeNotFound = errors.New("doc node not found")
	// ErrInvalidInput 表示请求不满足领域约束。
	ErrInvalidInput = errors.New("invalid doc node input")
	// ErrInvalidCursor 表示搜索游标非法或与查询词不匹配。
	ErrInvalidCursor = errors.New("invalid search cursor")
	// ErrParentNotFolder 表示目标父节点不是文件夹。
	ErrParentNotFolder = errors.New("parent is not a folder")
	// ErrInvalidMove 表示移动目标为自身或自身子孙。
	ErrInvalidMove = errors.New("invalid node move")
	// ErrFolderHasContent 表示试图给文件夹写入正文。
	ErrFolderHasContent = errors.New("folder cannot store content")
)

const (
	// MaxTitleRunes 是标题允许的最大字符数。
	MaxTitleRunes = 200
	// MaxContentRunes 是文档正文允许的最大字符数。
	MaxContentRunes = 200000
	// DefaultDocumentTitle 是文档缺省标题。
	DefaultDocumentTitle = "无标题文档"
	// DefaultFolderTitle 是文件夹缺省标题。
	DefaultFolderTitle = "未命名文件夹"
)

// NodeSummary 是不含正文的目录节点。
type NodeSummary struct {
	ID        string   `json:"id"`
	ParentID  *string  `json:"parentId"`
	Kind      NodeKind `json:"kind"`
	Title     string   `json:"title"`
	SortOrder int      `json:"sortOrder"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

// Node 是包含正文的目录节点。
type Node struct {
	NodeSummary
	Content string `json:"content"`
}

// CreateNodeInput 描述创建节点的允许字段。
type CreateNodeInput struct {
	Kind            NodeKind
	ParentID        *string
	Title           string
	ClientRequestID *string
}

// RenameNodeInput 描述重命名节点的允许字段。
type RenameNodeInput struct {
	ID    string
	Title string
}

// MoveNodeInput 描述移动节点的允许字段。
type MoveNodeInput struct {
	ID       string
	ParentID *string
}

// SaveContentInput 描述保存正文的允许字段。
type SaveContentInput struct {
	ID      string
	Content string
}

// NormalizeTitle 去除标题两端空白，并按节点类型补缺省标题。
func NormalizeTitle(kind NodeKind, value string) string {
	title := strings.TrimSpace(value)
	if title != "" {
		return title
	}
	if kind == NodeKindFolder {
		return DefaultFolderTitle
	}
	return DefaultDocumentTitle
}

// IsValidKind 判断节点类型是否合法。
func IsValidKind(kind NodeKind) bool {
	return kind == NodeKindFolder || kind == NodeKindDocument
}

// Repository 是文档模块拥有的持久化端口。
type Repository interface {
	ListTree(ctx context.Context) ([]NodeSummary, error)
	GetNode(ctx context.Context, id string) (Node, error)
	CreateNode(ctx context.Context, input CreateNodeInput) (Node, error)
	RenameNode(ctx context.Context, input RenameNodeInput) (NodeSummary, error)
	MoveNode(ctx context.Context, input MoveNodeInput) (NodeSummary, error)
	SaveContent(ctx context.Context, input SaveContentInput) (string, error)
	SoftDelete(ctx context.Context, id, deletedAt string) ([]string, error)
	AncestorIDs(ctx context.Context, id string) ([]string, error)
	Search(ctx context.Context, input SearchInput) (SearchPage, error)
}
```

创建 `server/internal/docs/repository.go`，只放端口注释，避免与 `node.go` 重复声明：

```go
package docs

// 本文件保留 Repository 端口的位置说明。
// 端口声明位于 node.go，与领域类型同处一个包以便实现方直接引用。
//
// 约定：
//   1. GetNode、RenameNode、MoveNode、SaveContent、SoftDelete 对已软删除节点返回 ErrNodeNotFound。
//   2. SoftDelete 必须在单个事务内递归标记子孙，并返回全部受影响 id。
//   3. Search 的大小写不敏感语义由实现保证：SQLite 用 LIKE，其他实现可用 ILIKE。
//   4. Search 内部取 limit+1 条判定是否有下一页，返回给上层的是截断后的结果与 nextCursor。
//   5. AncestorIDs 返回从直接父节点到根的 id 列表，不包含自身。
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./server/internal/docs/ -run TestNormalizeTitle -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/docs/node.go server/internal/docs/repository.go server/internal/docs/node_test.go
git commit -m "feat: add document node domain types"
```

---

## Task 3: 搜索游标与入参清洗

**Files:**
- Create: `server/internal/docs/search.go`
- Test: `server/internal/docs/search_test.go`

**Interfaces:**
- Consumes: `docs.ErrInvalidCursor`、`docs.ErrInvalidInput`
- Produces:
  - `docs.SearchCursor{Rank int, UpdatedAt, ID, QueryHash string}`
  - `docs.SearchInput{Query string, Limit int, Cursor *SearchCursor}`
  - `docs.SearchHit{ID, Kind, Title, ParentID *string, Path []string, UpdatedAt}`
  - `docs.SearchPage{Items []SearchHit, NextCursor *string}`
  - `docs.DefaultSearchLimit = 20`、`docs.MaxSearchLimit = 50`、`docs.MaxQueryRunes = 100`
  - `docs.QueryHash(query string) string`
  - `docs.EncodeCursor(cursor SearchCursor) string`
  - `docs.DecodeCursor(raw, query string) (*SearchCursor, error)`
  - `docs.NormalizeLimit(limit int) int`
  - `docs.NormalizeQuery(raw string) (string, error)`
  - `docs.LikePattern(query string) string`

- [ ] **Step 1: 写失败测试**

创建 `server/internal/docs/search_test.go`：

```go
package docs

import (
	"errors"
	"testing"
)

func TestNormalizeQuery(t *testing.T) {
	t.Parallel()

	if _, err := NormalizeQuery("   "); err != nil {
		t.Fatalf("blank query error = %v, want nil", err)
	}
	value, err := NormalizeQuery("  接口  ")
	if err != nil || value != "接口" {
		t.Fatalf("NormalizeQuery() = %q, %v", value, err)
	}
	long := make([]rune, MaxQueryRunes+1)
	for index := range long {
		long[index] = 'a'
	}
	if _, err := NormalizeQuery(string(long)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("long query error = %v, want ErrInvalidInput", err)
	}
}

func TestNormalizeLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int
		want  int
	}{
		{input: 0, want: DefaultSearchLimit},
		{input: -5, want: DefaultSearchLimit},
		{input: 1, want: 1},
		{input: MaxSearchLimit, want: MaxSearchLimit},
		{input: 999, want: MaxSearchLimit},
	}

	for _, test := range tests {
		if got := NormalizeLimit(test.input); got != test.want {
			t.Fatalf("NormalizeLimit(%d) = %d, want %d", test.input, got, test.want)
		}
	}
}

func TestLikePatternEscapesWildcards(t *testing.T) {
	t.Parallel()

	if got := LikePattern(`50%_off\`); got != `50\%\_off\\` {
		t.Fatalf("LikePattern() = %q", got)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()

	cursor := SearchCursor{
		Rank:      1,
		UpdatedAt: "2026-11-06T01:02:03.000000004Z",
		ID:        "6f1b0d5e-3f4a-4f7b-8c9d-0a1b2c3d4e5f",
		QueryHash: QueryHash("接口"),
	}
	raw := EncodeCursor(cursor)
	decoded, err := DecodeCursor(raw, "接口")
	if err != nil {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
	if *decoded != cursor {
		t.Fatalf("decoded = %#v, want %#v", *decoded, cursor)
	}
}

func TestDecodeCursorRejectsMismatchedQuery(t *testing.T) {
	t.Parallel()

	raw := EncodeCursor(SearchCursor{
		Rank:      0,
		UpdatedAt: "2026-11-06T01:02:03Z",
		ID:        "6f1b0d5e-3f4a-4f7b-8c9d-0a1b2c3d4e5f",
		QueryHash: QueryHash("接口"),
	})
	if _, err := DecodeCursor(raw, "文档"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("mismatched query error = %v, want ErrInvalidCursor", err)
	}
}

func TestDecodeCursorRejectsMalformedPayload(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "not-base64!", "e30"} {
		if _, err := DecodeCursor(raw, "接口"); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("DecodeCursor(%q) error = %v, want ErrInvalidCursor", raw, err)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./server/internal/docs/ -run 'TestNormalizeQuery|TestNormalizeLimit|TestLikePattern|TestCursor|TestDecodeCursor' -v`
Expected: FAIL，`undefined: NormalizeQuery`

- [ ] **Step 3: 实现游标与清洗**

创建 `server/internal/docs/search.go`：

```go
package docs

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultSearchLimit 是搜索默认返回条数。
	DefaultSearchLimit = 20
	// MaxSearchLimit 是搜索允许的最大返回条数。
	MaxSearchLimit = 50
	// MaxQueryRunes 是搜索关键词最大字符数。
	MaxQueryRunes = 100
)

var uuidPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

// SearchCursor 是搜索分页游标的载荷。
type SearchCursor struct {
	Rank      int    `json:"rank"`
	UpdatedAt string `json:"updatedAt"`
	ID        string `json:"id"`
	QueryHash string `json:"qHash"`
}

// SearchInput 描述一次搜索请求。
type SearchInput struct {
	Query     string
	Limit     int
	Cursor    *SearchCursor
	QueryHash string
}

// SearchHit 是搜索命中的目录节点。
type SearchHit struct {
	ID        string   `json:"id"`
	Kind      NodeKind `json:"kind"`
	Title     string   `json:"title"`
	ParentID  *string  `json:"parentId"`
	Path      []string `json:"path"`
	UpdatedAt string   `json:"updatedAt"`
}

// SearchPage 是搜索结果与下一页游标。
type SearchPage struct {
	Items      []SearchHit
	NextCursor *string
}

// QueryHash 返回规范化查询词的短哈希，用于绑定游标与查询词。
func QueryHash(query string) string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:8])
}

// EncodeCursor 把游标载荷编码为 URL 安全的 opaque 字符串。
func EncodeCursor(cursor SearchCursor) string {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

// DecodeCursor 解析游标并校验其与查询词一致。
func DecodeCursor(raw string, query string) (*SearchCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var cursor SearchCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return nil, ErrInvalidCursor
	}
	if cursor.Rank != 0 && cursor.Rank != 1 {
		return nil, ErrInvalidCursor
	}
	if !uuidPattern.MatchString(cursor.ID) {
		return nil, ErrInvalidCursor
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.UpdatedAt); err != nil {
		return nil, ErrInvalidCursor
	}
	if cursor.QueryHash != QueryHash(query) {
		return nil, ErrInvalidCursor
	}
	return &cursor, nil
}

// NormalizeQuery 去除空白并校验长度；空串表示无结果，不视为错误。
func NormalizeQuery(raw string) (string, error) {
	query := strings.TrimSpace(raw)
	if query == "" {
		return "", nil
	}
	if utf8.RuneCountInString(query) > MaxQueryRunes {
		return "", ErrInvalidInput
	}
	return query, nil
}

// NormalizeLimit 把越界的 limit 收敛到允许区间。
func NormalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		return MaxSearchLimit
	}
	return limit
}

// LikePattern 转义 LIKE 通配符，配合 ESCAPE '\' 使用。
func LikePattern(query string) string {
	escaped := strings.ReplaceAll(query, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `%`, `\%`)
	escaped = strings.ReplaceAll(escaped, `_`, `\_`)
	return escaped
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./server/internal/docs/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/docs/search.go server/internal/docs/search_test.go
git commit -m "feat: add document search cursor encoding"
```

---

## Task 4: 领域校验与用例编排

**Files:**
- Create: `server/internal/docs/service.go`
- Test: `server/internal/docs/service_test.go`

**Interfaces:**
- Consumes: `docs.Repository`、`docs.NormalizeTitle`、`docs.NormalizeQuery`、`docs.DecodeCursor`、`docs.NormalizeLimit`
- Produces:
  - `docs.NewService(repository Repository) *Service`
  - `(*Service).ListTree(ctx) ([]NodeSummary, error)`
  - `(*Service).GetNode(ctx, id string) (Node, error)`
  - `(*Service).CreateNode(ctx, CreateNodeInput) (Node, error)`
  - `(*Service).RenameNode(ctx, RenameNodeInput) (NodeSummary, error)`
  - `(*Service).MoveNode(ctx, MoveNodeInput) (NodeSummary, error)`
  - `(*Service).SaveContent(ctx, SaveContentInput) (string, error)`
  - `(*Service).DeleteNode(ctx, id string) ([]string, error)`
  - `(*Service).Search(ctx, rawQuery string, rawLimit int, rawCursor string) (SearchPage, error)`

- [ ] **Step 1: 写失败测试**

创建 `server/internal/docs/service_test.go`：

```go
package docs

import (
	"context"
	"errors"
	"testing"
)

type stubRepository struct {
	nodes       map[string]Node
	ancestors   map[string][]string
	created     []CreateNodeInput
	saved       []SaveContentInput
	deleted     []string
	searchInput SearchInput
	searchPage  SearchPage
}

func newStubRepository() *stubRepository {
	return &stubRepository{
		nodes:     map[string]Node{},
		ancestors: map[string][]string{},
	}
}

func (s *stubRepository) ListTree(context.Context) ([]NodeSummary, error) {
	result := make([]NodeSummary, 0, len(s.nodes))
	for _, node := range s.nodes {
		result = append(result, node.NodeSummary)
	}
	return result, nil
}

func (s *stubRepository) GetNode(_ context.Context, id string) (Node, error) {
	node, ok := s.nodes[id]
	if !ok {
		return Node{}, ErrNodeNotFound
	}
	return node, nil
}

func (s *stubRepository) CreateNode(_ context.Context, input CreateNodeInput) (Node, error) {
	s.created = append(s.created, input)
	node := Node{
		NodeSummary: NodeSummary{
			ID:       "created-id",
			ParentID: input.ParentID,
			Kind:     input.Kind,
			Title:    input.Title,
		},
	}
	s.nodes["created-id"] = node
	return node, nil
}

func (s *stubRepository) RenameNode(_ context.Context, input RenameNodeInput) (NodeSummary, error) {
	node, ok := s.nodes[input.ID]
	if !ok {
		return NodeSummary{}, ErrNodeNotFound
	}
	node.Title = input.Title
	s.nodes[input.ID] = node
	return node.NodeSummary, nil
}

func (s *stubRepository) MoveNode(ctx context.Context, input MoveNodeInput) (NodeSummary, error) {
	return s.RenameNode(ctx, RenameNodeInput{ID: input.ID, Title: s.nodes[input.ID].Title})
}

func (s *stubRepository) SaveContent(_ context.Context, input SaveContentInput) (string, error) {
	s.saved = append(s.saved, input)
	return "2026-11-06T00:00:00Z", nil
}

func (s *stubRepository) SoftDelete(_ context.Context, id string, _ string) ([]string, error) {
	if _, ok := s.nodes[id]; !ok {
		return nil, ErrNodeNotFound
	}
	s.deleted = append(s.deleted, id)
	return []string{id}, nil
}

func (s *stubRepository) AncestorIDs(_ context.Context, id string) ([]string, error) {
	return s.ancestors[id], nil
}

func (s *stubRepository) Search(_ context.Context, input SearchInput) (SearchPage, error) {
	s.searchInput = input
	return s.searchPage, nil
}

func folderNode(id string, parentID *string) Node {
	return Node{NodeSummary: NodeSummary{ID: id, Kind: NodeKindFolder, ParentID: parentID}}
}

func documentNode(id string, parentID *string) Node {
	return Node{NodeSummary: NodeSummary{ID: id, Kind: NodeKindDocument, ParentID: parentID}}
}

func TestServiceCreateNodeValidatesParent(t *testing.T) {
	t.Parallel()

	repository := newStubRepository()
	repository.nodes["doc"] = documentNode("doc", nil)
	service := NewService(repository)

	if _, err := service.CreateNode(context.Background(), CreateNodeInput{
		Kind:     NodeKindDocument,
		ParentID: stringPointer("doc"),
		Title:    "子文档",
	}); !errors.Is(err, ErrParentNotFolder) {
		t.Fatalf("error = %v, want ErrParentNotFolder", err)
	}

	if _, err := service.CreateNode(context.Background(), CreateNodeInput{
		Kind:  NodeKindDocument,
		Title: "   ",
	}); err != nil {
		t.Fatalf("create error = %v", err)
	}
	if repository.created[0].Title != DefaultDocumentTitle {
		t.Fatalf("title = %q, want %q", repository.created[0].Title, DefaultDocumentTitle)
	}
}

func TestServiceCreateNodeRejectsInvalidKind(t *testing.T) {
	t.Parallel()

	service := NewService(newStubRepository())
	if _, err := service.CreateNode(context.Background(), CreateNodeInput{
		Kind:  NodeKind("page"),
		Title: "错类型",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestServiceMoveNodeRejectsSelfAndDescendant(t *testing.T) {
	t.Parallel()

	repository := newStubRepository()
	repository.nodes["a"] = folderNode("a", nil)
	repository.nodes["b"] = folderNode("b", stringPointer("a"))
	repository.ancestors["b"] = []string{"a"}
	service := NewService(repository)

	if _, err := service.MoveNode(context.Background(), MoveNodeInput{
		ID:       "a",
		ParentID: stringPointer("a"),
	}); !errors.Is(err, ErrInvalidMove) {
		t.Fatalf("self move error = %v, want ErrInvalidMove", err)
	}

	if _, err := service.MoveNode(context.Background(), MoveNodeInput{
		ID:       "a",
		ParentID: stringPointer("b"),
	}); !errors.Is(err, ErrInvalidMove) {
		t.Fatalf("descendant move error = %v, want ErrInvalidMove", err)
	}

	if _, err := service.MoveNode(context.Background(), MoveNodeInput{
		ID:       "b",
		ParentID: nil,
	}); err != nil {
		t.Fatalf("move to root error = %v", err)
	}
}

func TestServiceSaveContentValidatesKindAndLength(t *testing.T) {
	t.Parallel()

	repository := newStubRepository()
	repository.nodes["folder"] = folderNode("folder", nil)
	repository.nodes["doc"] = documentNode("doc", nil)
	service := NewService(repository)

	if _, err := service.SaveContent(context.Background(), SaveContentInput{
		ID:      "folder",
		Content: "内容",
	}); !errors.Is(err, ErrFolderHasContent) {
		t.Fatalf("folder error = %v, want ErrFolderHasContent", err)
	}

	long := make([]rune, MaxContentRunes+1)
	for index := range long {
		long[index] = 'a'
	}
	if _, err := service.SaveContent(context.Background(), SaveContentInput{
		ID:      "doc",
		Content: string(long),
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("long content error = %v, want ErrInvalidInput", err)
	}

	if _, err := service.SaveContent(context.Background(), SaveContentInput{
		ID:      "doc",
		Content: "# 标题",
	}); err != nil {
		t.Fatalf("save error = %v", err)
	}
	if repository.saved[0].Content != "# 标题" {
		t.Fatalf("saved = %#v", repository.saved[0])
	}
}

func TestServiceSearchNormalizesInput(t *testing.T) {
	t.Parallel()

	repository := newStubRepository()
	repository.searchPage = SearchPage{Items: make([]SearchHit, 0)}
	service := NewService(repository)

	page, err := service.Search(context.Background(), "   ", 0, "")
	if err != nil {
		t.Fatalf("blank search error = %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("blank search page = %#v", page)
	}

	if _, err := service.Search(context.Background(), "接口", 500, ""); err != nil {
		t.Fatalf("search error = %v", err)
	}
	if repository.searchInput.Limit != MaxSearchLimit {
		t.Fatalf("limit = %d, want %d", repository.searchInput.Limit, MaxSearchLimit)
	}

	raw := EncodeCursor(SearchCursor{
		Rank:      0,
		UpdatedAt: "2026-11-06T00:00:00Z",
		ID:        "6f1b0d5e-3f4a-4f7b-8c9d-0a1b2c3d4e5f",
		QueryHash: QueryHash("接口"),
	})
	if _, err := service.Search(context.Background(), "接口", 20, raw); err != nil {
		t.Fatalf("cursor search error = %v", err)
	}
	if repository.searchInput.Cursor == nil {
		t.Fatal("cursor was not passed to repository")
	}

	if _, err := service.Search(context.Background(), "接口", 20, "bad-cursor"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("bad cursor error = %v, want ErrInvalidCursor", err)
	}
}

func stringPointer(value string) *string {
	return &value
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./server/internal/docs/ -run TestService -v`
Expected: FAIL，`undefined: NewService`

- [ ] **Step 3: 实现 Service**

创建 `server/internal/docs/service.go`：

```go
package docs

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

// Service 承担文档领域校验并编排持久化端口。
type Service struct {
	repository Repository
}

// NewService 创建文档领域服务。
func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// ListTree 返回全量未删除节点摘要。
func (s *Service) ListTree(ctx context.Context) ([]NodeSummary, error) {
	return s.repository.ListTree(ctx)
}

// GetNode 返回单个节点及其正文。
func (s *Service) GetNode(ctx context.Context, id string) (Node, error) {
	if strings.TrimSpace(id) == "" {
		return Node{}, ErrNodeNotFound
	}
	return s.repository.GetNode(ctx, strings.TrimSpace(id))
}

// CreateNode 校验并创建目录节点。
func (s *Service) CreateNode(ctx context.Context, input CreateNodeInput) (Node, error) {
	if !IsValidKind(input.Kind) {
		return Node{}, ErrInvalidInput
	}
	title := NormalizeTitle(input.Kind, input.Title)
	if utf8.RuneCountInString(title) > MaxTitleRunes {
		return Node{}, ErrInvalidInput
	}

	parentID := trimmedPointer(input.ParentID)
	if parentID != nil {
		parent, err := s.repository.GetNode(ctx, *parentID)
		if err != nil {
			return Node{}, err
		}
		if parent.Kind != NodeKindFolder {
			return Node{}, ErrParentNotFolder
		}
	}

	input.ParentID = parentID
	input.Title = title
	input.ClientRequestID = trimmedPointer(input.ClientRequestID)
	return s.repository.CreateNode(ctx, input)
}

// RenameNode 校验并重命名节点。
func (s *Service) RenameNode(ctx context.Context, input RenameNodeInput) (NodeSummary, error) {
	current, err := s.GetNode(ctx, input.ID)
	if err != nil {
		return NodeSummary{}, err
	}
	title := NormalizeTitle(current.Kind, input.Title)
	if utf8.RuneCountInString(title) > MaxTitleRunes {
		return NodeSummary{}, ErrInvalidInput
	}
	return s.repository.RenameNode(ctx, RenameNodeInput{
		ID:    current.ID,
		Title: title,
	})
}

// MoveNode 校验目标父节点并移动节点。
func (s *Service) MoveNode(ctx context.Context, input MoveNodeInput) (NodeSummary, error) {
	current, err := s.GetNode(ctx, input.ID)
	if err != nil {
		return NodeSummary{}, err
	}

	parentID := trimmedPointer(input.ParentID)
	if parentID != nil {
		if *parentID == current.ID {
			return NodeSummary{}, ErrInvalidMove
		}
		parent, err := s.repository.GetNode(ctx, *parentID)
		if err != nil {
			return NodeSummary{}, err
		}
		if parent.Kind != NodeKindFolder {
			return NodeSummary{}, ErrParentNotFolder
		}
		ancestors, err := s.repository.AncestorIDs(ctx, current.ID)
		if err != nil {
			return NodeSummary{}, err
		}
		for _, ancestor := range ancestors {
			if ancestor == *parentID {
				return NodeSummary{}, ErrInvalidMove
			}
		}
	}

	return s.repository.MoveNode(ctx, MoveNodeInput{
		ID:       current.ID,
		ParentID: parentID,
	})
}

// SaveContent 校验并保存文档正文，返回新的更新时间。
func (s *Service) SaveContent(ctx context.Context, input SaveContentInput) (string, error) {
	current, err := s.GetNode(ctx, input.ID)
	if err != nil {
		return "", err
	}
	if current.Kind != NodeKindDocument {
		return "", ErrFolderHasContent
	}
	if utf8.RuneCountInString(input.Content) > MaxContentRunes {
		return "", ErrInvalidInput
	}
	return s.repository.SaveContent(ctx, SaveContentInput{
		ID:      current.ID,
		Content: input.Content,
	})
}

// DeleteNode 软删除节点及其子孙，返回全部受影响 id。
func (s *Service) DeleteNode(ctx context.Context, id string) ([]string, error) {
	current, err := s.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.repository.SoftDelete(ctx, current.ID, now)
}

// Search 校验搜索入参并返回一页结果。
func (s *Service) Search(
	ctx context.Context,
	rawQuery string,
	rawLimit int,
	rawCursor string,
) (SearchPage, error) {
	query, err := NormalizeQuery(rawQuery)
	if err != nil {
		return SearchPage{}, err
	}
	if query == "" {
		return SearchPage{Items: make([]SearchHit, 0)}, nil
	}

	cursor, err := DecodeCursor(rawCursor, query)
	if err != nil {
		return SearchPage{}, err
	}

	return s.repository.Search(ctx, SearchInput{
		Query:     LikePattern(query),
		Limit:     NormalizeLimit(rawLimit),
		Cursor:    cursor,
		QueryHash: QueryHash(query),
	})
}

func trimmedPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
```

`search_test.go` 已定义 `stringPointer`，`service_test.go` 再定义会重复。让 `service_test.go` 复用 `stringPointer`，删除该文件末尾的同名函数。实施时按此调整：`service_test.go` 末尾不再声明 `stringPointer`。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./server/internal/docs/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/docs/service.go server/internal/docs/service_test.go
git commit -m "feat: add document domain service validation"
```

---

## Task 5: 持久化节点读写

**Files:**
- Create: `server/internal/platform/store/doc_node_store.go`
- Test: `tests/e2e/unit/store/doc_node_store_test.go`

**Interfaces:**
- Consumes: `docs.Node`、`docs.NodeSummary`、`docs.CreateNodeInput`、`docs.RenameNodeInput`、`docs.MoveNodeInput`、`docs.SaveContentInput`、`docs.ErrNodeNotFound`；`store.SQLiteStore` 的 `db`、`newID()`、`nullableValue()`、`stringPointer()`
- Produces: `(*SQLiteStore).ListTree`、`GetNode`、`CreateNode`、`RenameNode`、`MoveNode`、`SaveContent`、`SoftDelete`、`AncestorIDs`、`docNodeByRequestID`、`scanNodeSummary`、`scanNode`

- [ ] **Step 1: 写失败测试**

创建 `tests/e2e/unit/store/doc_node_store_test.go`：

```go
package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/leeat/craftingtable/server/internal/docs"
	"github.com/leeat/craftingtable/server/internal/platform/store"
)

func openStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	ctx := context.Background()
	opened, err := store.OpenSQLite(ctx, filepath.Join(t.TempDir(), "docs.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	return opened
}

func createFolder(t *testing.T, opened *store.SQLiteStore, title string, parentID *string) docs.Node {
	t.Helper()
	node, err := opened.CreateNode(context.Background(), docs.CreateNodeInput{
		Kind:     docs.NodeKindFolder,
		Title:    title,
		ParentID: parentID,
	})
	if err != nil {
		t.Fatalf("create folder %q: %v", title, err)
	}
	return node
}

func TestDocNodeStoreCreatesTreeAndHidesContent(t *testing.T) {
	opened := openStore(t)
	ctx := context.Background()

	folder := createFolder(t, opened, "工作", nil)
	document, err := opened.CreateNode(ctx, docs.CreateNodeInput{
		Kind:     docs.NodeKindDocument,
		Title:    "接口设计",
		ParentID: &folder.ID,
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	if document.Kind != docs.NodeKindDocument || document.Content != "" {
		t.Fatalf("document = %#v", document)
	}

	tree, err := opened.ListTree(ctx)
	if err != nil {
		t.Fatalf("list tree: %v", err)
	}
	if len(tree) != 2 {
		t.Fatalf("tree size = %d, want 2", len(tree))
	}
	if tree[0].Kind != docs.NodeKindFolder {
		t.Fatalf("first node kind = %q, want folder", tree[0].Kind)
	}

	loaded, err := opened.GetNode(ctx, document.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if loaded.ParentID == nil || *loaded.ParentID != folder.ID {
		t.Fatalf("loaded parent = %#v", loaded.ParentID)
	}
}

func TestDocNodeStoreCreateIsIdempotent(t *testing.T) {
	opened := openStore(t)
	ctx := context.Background()
	requestID := "request-1"

	first, err := opened.CreateNode(ctx, docs.CreateNodeInput{
		Kind:            docs.NodeKindDocument,
		Title:           "第一篇",
		ClientRequestID: &requestID,
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := opened.CreateNode(ctx, docs.CreateNodeInput{
		Kind:            docs.NodeKindDocument,
		Title:           "第二篇",
		ClientRequestID: &requestID,
	})
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if first.ID != second.ID || second.Title != "第一篇" {
		t.Fatalf("idempotent create = %#v / %#v", first, second)
	}

	tree, err := opened.ListTree(ctx)
	if err != nil {
		t.Fatalf("list tree: %v", err)
	}
	if len(tree) != 1 {
		t.Fatalf("tree size = %d, want 1", len(tree))
	}
}

func TestDocNodeStoreMovesAndRenames(t *testing.T) {
	opened := openStore(t)
	ctx := context.Background()

	first := createFolder(t, opened, "A", nil)
	second := createFolder(t, opened, "B", nil)
	document, err := opened.CreateNode(ctx, docs.CreateNodeInput{
		Kind:  docs.NodeKindDocument,
		Title: "文档",
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	renamed, err := opened.RenameNode(ctx, docs.RenameNodeInput{
		ID:    document.ID,
		Title: "改名后的文档",
	})
	if err != nil || renamed.Title != "改名后的文档" {
		t.Fatalf("rename = %#v, %v", renamed, err)
	}

	moved, err := opened.MoveNode(ctx, docs.MoveNodeInput{
		ID:       document.ID,
		ParentID: &second.ID,
	})
	if err != nil || moved.ParentID == nil || *moved.ParentID != second.ID {
		t.Fatalf("move = %#v, %v", moved, err)
	}

	ancestors, err := opened.AncestorIDs(ctx, document.ID)
	if err != nil {
		t.Fatalf("ancestors: %v", err)
	}
	if len(ancestors) != 1 || ancestors[0] != second.ID {
		t.Fatalf("ancestors = %#v", ancestors)
	}
	if second.ID == first.ID {
		t.Fatal("fixture folders must differ")
	}
}

func TestDocNodeStoreSavesContentAndSoftDeletesSubtree(t *testing.T) {
	opened := openStore(t)
	ctx := context.Background()

	folder := createFolder(t, opened, "根目录", nil)
	child := createFolder(t, opened, "子目录", &folder.ID)
	document, err := opened.CreateNode(ctx, docs.CreateNodeInput{
		Kind:     docs.NodeKindDocument,
		Title:    "深层文档",
		ParentID: &child.ID,
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	updatedAt, err := opened.SaveContent(ctx, docs.SaveContentInput{
		ID:      document.ID,
		Content: "# 标题\n\n正文",
	})
	if err != nil || updatedAt == "" {
		t.Fatalf("save content = %q, %v", updatedAt, err)
	}
	loaded, err := opened.GetNode(ctx, document.ID)
	if err != nil || loaded.Content != "# 标题\n\n正文" {
		t.Fatalf("loaded = %#v, %v", loaded, err)
	}

	deleted, err := opened.SoftDelete(ctx, folder.ID, "2026-11-06T00:00:00Z")
	if err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if len(deleted) != 3 {
		t.Fatalf("deleted = %#v, want 3 ids", deleted)
	}

	if _, err := opened.GetNode(ctx, document.ID); !errors.Is(err, docs.ErrNodeNotFound) {
		t.Fatalf("deleted node error = %v, want ErrNodeNotFound", err)
	}
	tree, err := opened.ListTree(ctx)
	if err != nil {
		t.Fatalf("list tree: %v", err)
	}
	if len(tree) != 0 {
		t.Fatalf("tree size = %d, want 0", len(tree))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./tests/e2e/unit/store/ -run TestDocNodeStore -v`
Expected: FAIL，`opened.CreateNode undefined`

- [ ] **Step 3: 实现节点读写**

创建 `server/internal/platform/store/doc_node_store.go`：

```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leeat/craftingtable/server/internal/docs"
)

const selectDocNode = `
SELECT
  id,
  parent_id,
  kind,
  title,
  sort_order,
  created_at,
  updated_at`

const selectDocNodeWithContent = selectDocNode + `,
  content`

// ListTree 返回全量未删除节点摘要，文件夹优先。
func (s *SQLiteStore) ListTree(ctx context.Context) ([]docs.NodeSummary, error) {
	rows, err := s.db.QueryContext(
		ctx,
		selectDocNode+`
FROM doc_nodes
WHERE deleted_at IS NULL
ORDER BY CASE kind WHEN 'folder' THEN 0 ELSE 1 END, sort_order, updated_at DESC, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list doc tree: %w", err)
	}
	defer rows.Close()

	nodes := make([]docs.NodeSummary, 0)
	for rows.Next() {
		node, err := scanNodeSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("scan doc node: %w", err)
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate doc nodes: %w", err)
	}
	return nodes, nil
}

// GetNode 返回单个未删除节点。
func (s *SQLiteStore) GetNode(ctx context.Context, id string) (docs.Node, error) {
	row := s.db.QueryRowContext(
		ctx,
		selectDocNodeWithContent+`
FROM doc_nodes
WHERE id = ? AND deleted_at IS NULL`,
		id,
	)
	node, err := scanNode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return docs.Node{}, docs.ErrNodeNotFound
	}
	if err != nil {
		return docs.Node{}, fmt.Errorf("get doc node: %w", err)
	}
	return node, nil
}

// CreateNode 创建节点；相同 clientRequestId 返回已存在节点。
func (s *SQLiteStore) CreateNode(
	ctx context.Context,
	input docs.CreateNodeInput,
) (docs.Node, error) {
	if input.ClientRequestID != nil {
		existing, err := s.docNodeByRequestID(ctx, *input.ClientRequestID)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, docs.ErrNodeNotFound) {
			return docs.Node{}, err
		}
	}

	id, err := newID()
	if err != nil {
		return docs.Node{}, fmt.Errorf("create doc node id: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO doc_nodes (
		  id, parent_id, kind, title, content, sort_order,
		  client_request_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, '', 0, ?, ?, ?)`,
		id,
		nullableValue(input.ParentID),
		string(input.Kind),
		input.Title,
		nullableValue(input.ClientRequestID),
		now,
		now,
	)
	if err != nil {
		if input.ClientRequestID != nil {
			if existing, lookupErr := s.docNodeByRequestID(ctx, *input.ClientRequestID); lookupErr == nil {
				return existing, nil
			}
		}
		return docs.Node{}, fmt.Errorf("insert doc node: %w", err)
	}
	return s.GetNode(ctx, id)
}

// RenameNode 更新标题并返回节点摘要。
func (s *SQLiteStore) RenameNode(
	ctx context.Context,
	input docs.RenameNodeInput,
) (docs.NodeSummary, error) {
	return s.updateDocNodeSummary(
		ctx,
		"UPDATE doc_nodes SET title = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL",
		input.Title,
		input.ID,
	)
}

// MoveNode 更新父节点并返回节点摘要。
func (s *SQLiteStore) MoveNode(
	ctx context.Context,
	input docs.MoveNodeInput,
) (docs.NodeSummary, error) {
	return s.updateDocNodeSummary(
		ctx,
		"UPDATE doc_nodes SET parent_id = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL",
		nullableValue(input.ParentID),
		input.ID,
	)
}

// SaveContent 保存正文并返回新的更新时间。
func (s *SQLiteStore) SaveContent(
	ctx context.Context,
	input docs.SaveContentInput,
) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE doc_nodes
		 SET content = ?, updated_at = ?
		 WHERE id = ? AND kind = 'document' AND deleted_at IS NULL`,
		input.Content,
		now,
		input.ID,
	)
	if err != nil {
		return "", fmt.Errorf("save doc content: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("save doc content rows: %w", err)
	}
	if affected == 0 {
		return "", docs.ErrNodeNotFound
	}
	return now, nil
}

// SoftDelete 在单个事务内递归软删除节点与子孙。
func (s *SQLiteStore) SoftDelete(
	ctx context.Context,
	id string,
	deletedAt string,
) ([]string, error) {
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin doc delete: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()

	rows, err := transaction.QueryContext(
		ctx,
		`WITH RECURSIVE subtree(id) AS (
		   SELECT id FROM doc_nodes WHERE id = ? AND deleted_at IS NULL
		   UNION ALL
		   SELECT node.id
		   FROM doc_nodes node
		   JOIN subtree parent ON node.parent_id = parent.id
		   WHERE node.deleted_at IS NULL
		 )
		 SELECT id FROM subtree`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("collect doc subtree: %w", err)
	}
	identifiers := make([]string, 0)
	for rows.Next() {
		var identifier string
		if err := rows.Scan(&identifier); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan doc subtree: %w", err)
		}
		identifiers = append(identifiers, identifier)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate doc subtree: %w", err)
	}
	_ = rows.Close()

	if len(identifiers) == 0 {
		return nil, docs.ErrNodeNotFound
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(identifiers)), ",")
	arguments := make([]any, 0, len(identifiers)+1)
	arguments = append(arguments, deletedAt)
	for _, identifier := range identifiers {
		arguments = append(arguments, identifier)
	}
	if _, err := transaction.ExecContext(
		ctx,
		"UPDATE doc_nodes SET deleted_at = ?, updated_at = ? WHERE id IN ("+placeholders+")",
		append(arguments[:1], append([]any{deletedAt}, arguments[1:]...)...)...,
	); err != nil {
		return nil, fmt.Errorf("soft delete doc subtree: %w", err)
	}

	if err := transaction.Commit(); err != nil {
		return nil, fmt.Errorf("commit doc delete: %w", err)
	}
	return identifiers, nil
}

// AncestorIDs 返回从直接父节点到根的 id 列表。
func (s *SQLiteStore) AncestorIDs(
	ctx context.Context,
	id string,
) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`WITH RECURSIVE chain(id, parent_id, depth) AS (
		   SELECT id, parent_id, 0 FROM doc_nodes WHERE id = ?
		   UNION ALL
		   SELECT node.id, node.parent_id, chain.depth + 1
		   FROM doc_nodes node
		   JOIN chain ON node.id = chain.parent_id
		   WHERE chain.depth < 32
		 )
		 SELECT id FROM chain WHERE depth > 0`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("collect doc ancestors: %w", err)
	}
	defer rows.Close()

	ancestors := make([]string, 0)
	for rows.Next() {
		var ancestor string
		if err := rows.Scan(&ancestor); err != nil {
			return nil, fmt.Errorf("scan doc ancestor: %w", err)
		}
		ancestors = append(ancestors, ancestor)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate doc ancestors: %w", err)
	}
	return ancestors, nil
}

func (s *SQLiteStore) docNodeByRequestID(
	ctx context.Context,
	requestID string,
) (docs.Node, error) {
	row := s.db.QueryRowContext(
		ctx,
		selectDocNodeWithContent+`
FROM doc_nodes
WHERE client_request_id = ? AND deleted_at IS NULL`,
		requestID,
	)
	node, err := scanNode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return docs.Node{}, docs.ErrNodeNotFound
	}
	if err != nil {
		return docs.Node{}, fmt.Errorf("find doc node by request: %w", err)
	}
	return node, nil
}

func (s *SQLiteStore) updateDocNodeSummary(
	ctx context.Context,
	statement string,
	value any,
	id string,
) (docs.NodeSummary, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, statement, value, now, id)
	if err != nil {
		return docs.NodeSummary{}, fmt.Errorf("update doc node: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return docs.NodeSummary{}, fmt.Errorf("update doc node rows: %w", err)
	}
	if affected == 0 {
		return docs.NodeSummary{}, docs.ErrNodeNotFound
	}
	node, err := s.GetNode(ctx, id)
	if err != nil {
		return docs.NodeSummary{}, err
	}
	return node.NodeSummary, nil
}

func scanNodeSummary(scanner rowScanner) (docs.NodeSummary, error) {
	var (
		summary  docs.NodeSummary
		kind     string
		parentID sql.NullString
	)
	if err := scanner.Scan(
		&summary.ID,
		&parentID,
		&kind,
		&summary.Title,
		&summary.SortOrder,
		&summary.CreatedAt,
		&summary.UpdatedAt,
	); err != nil {
		return docs.NodeSummary{}, err
	}
	summary.Kind = docs.NodeKind(kind)
	summary.ParentID = stringPointer(parentID)
	return summary, nil
}

func scanNode(scanner rowScanner) (docs.Node, error) {
	var (
		node     docs.Node
		kind     string
		parentID sql.NullString
	)
	if err := scanner.Scan(
		&node.ID,
		&parentID,
		&kind,
		&node.Title,
		&node.SortOrder,
		&node.CreatedAt,
		&node.UpdatedAt,
		&node.Content,
	); err != nil {
		return docs.Node{}, err
	}
	node.Kind = docs.NodeKind(kind)
	node.ParentID = stringPointer(parentID)
	return node, nil
}
```

`SoftDelete` 的 `UPDATE` 参数拼接写得过于绕，实施时改用清晰的写法：

```go
	arguments := make([]any, 0, len(identifiers)+2)
	arguments = append(arguments, deletedAt, deletedAt)
	for _, identifier := range identifiers {
		arguments = append(arguments, identifier)
	}
	if _, err := transaction.ExecContext(
		ctx,
		"UPDATE doc_nodes SET deleted_at = ?, updated_at = ? WHERE id IN ("+placeholders+")",
		arguments...,
	); err != nil {
		return nil, fmt.Errorf("soft delete doc subtree: %w", err)
	}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./tests/e2e/unit/store/ -run TestDocNodeStore -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/platform/store/doc_node_store.go tests/e2e/unit/store/doc_node_store_test.go
git commit -m "feat: persist document nodes in sqlite"
```

---

## Task 6: 持久化搜索

**Files:**
- Create: `server/internal/platform/store/doc_search_store.go`
- Test: `tests/e2e/unit/store/doc_search_store_test.go`

**Interfaces:**
- Consumes: `docs.SearchInput{Query, Limit, Cursor, QueryHash}`（`Query` 已由 `LikePattern` 转义）、`docs.SearchPage`、`docs.SearchHit`、`docs.QueryHash`、`docs.EncodeCursor`
- Produces: `(*SQLiteStore).Search`、`(*SQLiteStore).docAncestorPaths`

- [ ] **Step 1: 写失败测试**

创建 `tests/e2e/unit/store/doc_search_store_test.go`：

```go
package store_test

import (
	"context"
	"testing"

	"github.com/leeat/craftingtable/server/internal/docs"
)

func TestDocSearchStoreMatchesTitlesWithPath(t *testing.T) {
	opened := openStore(t)
	ctx := context.Background()

	folder := createFolder(t, opened, "工作", nil)
	child := createFolder(t, opened, "接口", &folder.ID)
	document, err := opened.CreateNode(ctx, docs.CreateNodeInput{
		Kind:     docs.NodeKindDocument,
		Title:    "接口设计说明",
		ParentID: &child.ID,
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	page, err := opened.Search(ctx, docs.SearchInput{
		Query:     docs.LikePattern("接口"),
		Limit:     20,
		QueryHash: docs.QueryHash("接口"),
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("hit count = %d, want 2", len(page.Items))
	}
	if page.Items[0].ID != document.ID {
		t.Fatalf("first hit = %#v, want document", page.Items[0])
	}
	path := page.Items[0].Path
	if len(path) != 2 || path[0] != "工作" || path[1] != "接口" {
		t.Fatalf("path = %#v, want [工作 接口]", path)
	}
	if page.NextCursor != nil {
		t.Fatalf("next cursor = %#v, want nil", page.NextCursor)
	}
}

func TestDocSearchStoreTreatsWildcardsLiterally(t *testing.T) {
	opened := openStore(t)
	ctx := context.Background()

	if _, err := opened.CreateNode(ctx, docs.CreateNodeInput{
		Kind:  docs.NodeKindDocument,
		Title: "100% 完成",
	}); err != nil {
		t.Fatalf("create document: %v", err)
	}
	if _, err := opened.CreateNode(ctx, docs.CreateNodeInput{
		Kind:  docs.NodeKindDocument,
		Title: "100 完成",
	}); err != nil {
		t.Fatalf("create document: %v", err)
	}

	page, err := opened.Search(ctx, docs.SearchInput{
		Query:     docs.LikePattern("100%"),
		Limit:     20,
		QueryHash: docs.QueryHash("100%"),
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "100% 完成" {
		t.Fatalf("hits = %#v", page.Items)
	}
}

func TestDocSearchStorePaginatesWithCursor(t *testing.T) {
	opened := openStore(t)
	ctx := context.Background()

	for _, title := range []string{"文档一", "文档二", "文档三"} {
		if _, err := opened.CreateNode(ctx, docs.CreateNodeInput{
			Kind:  docs.NodeKindDocument,
			Title: title,
		}); err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
	}

	first, err := opened.Search(ctx, docs.SearchInput{
		Query:     docs.LikePattern("文档"),
		Limit:     2,
		QueryHash: docs.QueryHash("文档"),
	})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Items) != 2 || first.NextCursor == nil {
		t.Fatalf("first page = %#v", first)
	}

	cursor, err := docs.DecodeCursor(*first.NextCursor, "文档")
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	second, err := opened.Search(ctx, docs.SearchInput{
		Query:     docs.LikePattern("文档"),
		Limit:     2,
		Cursor:    cursor,
		QueryHash: docs.QueryHash("文档"),
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Items) != 1 {
		t.Fatalf("second page size = %d, want 1", len(second.Items))
	}
	if second.Items[0].ID == first.Items[0].ID || second.Items[0].ID == first.Items[1].ID {
		t.Fatalf("second page repeats first page: %#v", second.Items)
	}
	if second.NextCursor != nil {
		t.Fatalf("second next cursor = %#v, want nil", second.NextCursor)
	}
}

func TestDocSearchStoreSkipsDeletedNodes(t *testing.T) {
	opened := openStore(t)
	ctx := context.Background()

	folder := createFolder(t, opened, "归档", nil)
	document, err := opened.CreateNode(ctx, docs.CreateNodeInput{
		Kind:     docs.NodeKindDocument,
		Title:    "归档说明",
		ParentID: &folder.ID,
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	if _, err := opened.SoftDelete(ctx, document.ID, "2026-11-06T00:00:00Z"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	page, err := opened.Search(ctx, docs.SearchInput{
		Query:     docs.LikePattern("归档"),
		Limit:     20,
		QueryHash: docs.QueryHash("归档"),
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Kind != docs.NodeKindFolder {
		t.Fatalf("hits = %#v", page.Items)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./tests/e2e/unit/store/ -run TestDocSearchStore -v`
Expected: FAIL，`opened.Search undefined`

- [ ] **Step 3: 实现搜索**

创建 `server/internal/platform/store/doc_search_store.go`：

```go
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/leeat/craftingtable/server/internal/docs"
)

// Search 返回标题命中的节点，并附带祖先路径。
func (s *SQLiteStore) Search(
	ctx context.Context,
	input docs.SearchInput,
) (docs.SearchPage, error) {
	rank := 0
	updatedAt := ""
	id := ""
	hasCursor := 1
	if input.Cursor != nil {
		rank = input.Cursor.Rank
		updatedAt = input.Cursor.UpdatedAt
		id = input.Cursor.ID
		hasCursor = 0
	}

	rows, err := s.db.QueryContext(
		ctx,
		`WITH ranked AS (
		   SELECT id, parent_id, kind, title, updated_at,
		          CASE WHEN lower(title) LIKE lower(?) ESCAPE '\' THEN 0 ELSE 1 END AS rank
		   FROM doc_nodes
		   WHERE deleted_at IS NULL
		     AND lower(title) LIKE lower(?) ESCAPE '\'
		 ),
		 page AS (
		   SELECT * FROM ranked
		   WHERE ? = 1
		      OR rank > ?
		      OR (rank = ? AND updated_at < ?)
		      OR (rank = ? AND updated_at = ? AND id < ?)
		   ORDER BY rank, updated_at DESC, id DESC
		   LIMIT ?
		 )
		 SELECT id, parent_id, kind, title, updated_at, rank
		 FROM page
		 ORDER BY rank, updated_at DESC, id DESC`,
		input.Query,
		input.Query,
		hasCursor,
		rank,
		rank,
		updatedAt,
		rank,
		updatedAt,
		id,
		input.Limit+1,
	)
	if err != nil {
		return docs.SearchPage{}, fmt.Errorf("search doc nodes: %w", err)
	}
	defer rows.Close()

	hits := make([]docs.SearchHit, 0)
	ranks := make([]int, 0)
	for rows.Next() {
		var (
			hit      docs.SearchHit
			kind     string
			parentID sql.NullString
			hitRank  int
		)
		if err := rows.Scan(
			&hit.ID,
			&parentID,
			&kind,
			&hit.Title,
			&hit.UpdatedAt,
			&hitRank,
		); err != nil {
			return docs.SearchPage{}, fmt.Errorf("scan doc search hit: %w", err)
		}
		hit.Kind = docs.NodeKind(kind)
		hit.ParentID = stringPointer(parentID)
		hit.Path = make([]string, 0)
		hits = append(hits, hit)
		ranks = append(ranks, hitRank)
	}
	if err := rows.Err(); err != nil {
		return docs.SearchPage{}, fmt.Errorf("iterate doc search hits: %w", err)
	}

	page := docs.SearchPage{Items: hits}
	if len(hits) > input.Limit {
		page.Items = hits[:input.Limit]
		last := page.Items[len(page.Items)-1]
		cursor := docs.EncodeCursor(docs.SearchCursor{
			Rank:      ranks[input.Limit-1],
			UpdatedAt: last.UpdatedAt,
			ID:        last.ID,
			QueryHash: input.QueryHash,
		})
		page.NextCursor = &cursor
	}

	identifiers := make([]string, 0, len(page.Items))
	for _, hit := range page.Items {
		identifiers = append(identifiers, hit.ID)
	}
	paths, err := s.docAncestorPaths(ctx, identifiers)
	if err != nil {
		return docs.SearchPage{}, err
	}
	for index := range page.Items {
		page.Items[index].Path = paths[page.Items[index].ID]
	}
	return page, nil
}

func (s *SQLiteStore) docAncestorPaths(
	ctx context.Context,
	identifiers []string,
) (map[string][]string, error) {
	paths := make(map[string][]string, len(identifiers))
	if len(identifiers) == 0 {
		return paths, nil
	}

	values := ""
	arguments := make([]any, 0, len(identifiers))
	for index, identifier := range identifiers {
		if index > 0 {
			values += ","
		}
		values += "(?)"
		arguments = append(arguments, identifier)
	}

	rows, err := s.db.QueryContext(
		ctx,
		`WITH RECURSIVE seed(node_id) AS (VALUES `+values+`),
		 chain(node_id, id, title, parent_id, depth) AS (
		   SELECT seed.node_id, node.id, node.title, node.parent_id, 1
		   FROM seed
		   JOIN doc_nodes node ON node.id = seed.node_id
		   WHERE node.parent_id IS NOT NULL
		   UNION ALL
		   SELECT chain.node_id, parent.id, parent.title, parent.parent_id, chain.depth + 1
		   FROM doc_nodes parent
		   JOIN chain ON parent.id = chain.parent_id
		   WHERE chain.depth < 8
		 )
		 SELECT node_id, title, depth FROM chain ORDER BY node_id, depth DESC`,
		arguments...,
	)
	if err != nil {
		return nil, fmt.Errorf("collect doc search paths: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			nodeID string
			title  string
			depth  int
		)
		if err := rows.Scan(&nodeID, &title, &depth); err != nil {
			return nil, fmt.Errorf("scan doc search path: %w", err)
		}
		paths[nodeID] = append(paths[nodeID], title)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate doc search paths: %w", err)
	}
	return paths, nil
}
```

`ORDER BY node_id, depth DESC` 保证根节点标题先进入切片，得到「根 → 父」顺序。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./tests/e2e/unit/store/ -run TestDocSearchStore -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/platform/store/doc_search_store.go tests/e2e/unit/store/doc_search_store_test.go
git commit -m "feat: search document nodes by title"
```

---
## Task 7: HTTP Controller 与路由注册

**Files:**
- Create: `server/internal/docs/controller.go`
- Modify: `server/cmd/craftingtabled/main.go:96`

**Interfaces:**
- Consumes: `docs.NewService`、`httpapi.Router`、`httpapi.ReadJSON`、`httpapi.OK`、`httpapi.Fail`、`httpapi.ErrorBody`
- Produces: 路由 `/api/v1/docs/tree`、`/api/v1/docs/item`、`/api/v1/docs/search`、`/api/v1/docs/create`、`/api/v1/docs/rename`、`/api/v1/docs/move`、`/api/v1/docs/save`、`/api/v1/docs/delete`

- [ ] **Step 1: 写 Controller**

创建 `server/internal/docs/controller.go`：

```go
package docs

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/leeat/craftingtable/server/internal/platform/httpapi"
)

// Controller 将文档领域用例适配为 HTTP。
type Controller struct {
	service *Service
	logger  *slog.Logger
}

// NewController 创建文档 HTTP Controller。
func NewController(repository Repository, logger *slog.Logger) *Controller {
	return &Controller{
		service: NewService(repository),
		logger:  logger,
	}
}

// Register 注册文档模块的全部 HTTP 路由。
func (c *Controller) Register(router *httpapi.Router) {
	router.Handle("GET /api/v1/docs/tree", c.listTree)
	router.Handle("GET /api/v1/docs/item", c.getNode)
	router.Handle("GET /api/v1/docs/search", c.search)
	router.Handle("POST /api/v1/docs/create", c.createNode)
	router.Handle("POST /api/v1/docs/rename", c.renameNode)
	router.Handle("POST /api/v1/docs/move", c.moveNode)
	router.Handle("POST /api/v1/docs/save", c.saveContent)
	router.Handle("POST /api/v1/docs/delete", c.deleteNode)
}

type createNodeRequest struct {
	Kind            string  `json:"kind"`
	ParentID        *string `json:"parentId"`
	Title           string  `json:"title"`
	ClientRequestID *string `json:"clientRequestId"`
}

type renameNodeRequest struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type moveNodeRequest struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parentId"`
}

type saveContentRequest struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type deleteNodeRequest struct {
	ID string `json:"id"`
}

func (c *Controller) listTree(w http.ResponseWriter, r *http.Request) {
	nodes, err := c.service.ListTree(r.Context())
	if err != nil {
		c.internalError(w, r, "加载文档目录失败", err)
		return
	}
	httpapi.OK(w, http.StatusOK, nodes)
}

func (c *Controller) getNode(w http.ResponseWriter, r *http.Request) {
	node, err := c.service.GetNode(r.Context(), r.URL.Query().Get("id"))
	if err != nil {
		c.writeError(w, r, err, "加载文档失败")
		return
	}
	httpapi.OK(w, http.StatusOK, node)
}

func (c *Controller) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(strings.TrimSpace(query.Get("limit")))

	page, err := c.service.Search(
		r.Context(),
		query.Get("q"),
		limit,
		query.Get("cursor"),
	)
	if err != nil {
		c.writeError(w, r, err, "搜索文档失败")
		return
	}
	httpapi.OK(w, http.StatusOK, map[string]any{
		"items":      page.Items,
		"nextCursor": page.NextCursor,
	})
}

func (c *Controller) createNode(w http.ResponseWriter, r *http.Request) {
	var request createNodeRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		c.writeError(w, r, ErrInvalidInput, "创建文档失败")
		return
	}

	node, err := c.service.CreateNode(r.Context(), CreateNodeInput{
		Kind:            NodeKind(strings.TrimSpace(request.Kind)),
		ParentID:        request.ParentID,
		Title:           request.Title,
		ClientRequestID: request.ClientRequestID,
	})
	if err != nil {
		c.writeError(w, r, err, "创建文档失败")
		return
	}
	httpapi.OK(w, http.StatusOK, node)
}

func (c *Controller) renameNode(w http.ResponseWriter, r *http.Request) {
	var request renameNodeRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		c.writeError(w, r, ErrInvalidInput, "重命名失败")
		return
	}

	node, err := c.service.RenameNode(r.Context(), RenameNodeInput{
		ID:    request.ID,
		Title: request.Title,
	})
	if err != nil {
		c.writeError(w, r, err, "重命名失败")
		return
	}
	httpapi.OK(w, http.StatusOK, node)
}

func (c *Controller) moveNode(w http.ResponseWriter, r *http.Request) {
	var request moveNodeRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		c.writeError(w, r, ErrInvalidInput, "移动失败")
		return
	}

	node, err := c.service.MoveNode(r.Context(), MoveNodeInput{
		ID:       request.ID,
		ParentID: request.ParentID,
	})
	if err != nil {
		c.writeError(w, r, err, "移动失败")
		return
	}
	httpapi.OK(w, http.StatusOK, node)
}

func (c *Controller) saveContent(w http.ResponseWriter, r *http.Request) {
	var request saveContentRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		c.writeError(w, r, ErrInvalidInput, "保存文档失败")
		return
	}

	updatedAt, err := c.service.SaveContent(r.Context(), SaveContentInput{
		ID:      request.ID,
		Content: request.Content,
	})
	if err != nil {
		c.writeError(w, r, err, "保存文档失败")
		return
	}
	httpapi.OK(w, http.StatusOK, map[string]string{
		"id":        request.ID,
		"updatedAt": updatedAt,
	})
}

func (c *Controller) deleteNode(w http.ResponseWriter, r *http.Request) {
	var request deleteNodeRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		c.writeError(w, r, ErrInvalidInput, "删除失败")
		return
	}

	identifiers, err := c.service.DeleteNode(r.Context(), request.ID)
	if err != nil {
		c.writeError(w, r, err, "删除失败")
		return
	}
	httpapi.OK(w, http.StatusOK, map[string]any{"deletedIds": identifiers})
}

func (c *Controller) writeError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
	fallback string,
) {
	switch {
	case errors.Is(err, ErrNodeNotFound):
		httpapi.Fail(w, r, http.StatusNotFound, httpapi.ErrorBody{
			Code:      "NOT_FOUND",
			Kind:      "not_found",
			Message:   "文档不存在或已被删除",
			Retryable: false,
		})
	case errors.Is(err, ErrParentNotFolder):
		c.validationError(w, r, "目标位置不是文件夹")
	case errors.Is(err, ErrInvalidMove):
		c.validationError(w, r, "不能移动到自身或子节点")
	case errors.Is(err, ErrFolderHasContent):
		c.validationError(w, r, "文件夹不能保存正文")
	case errors.Is(err, ErrInvalidCursor):
		c.validationError(w, r, "搜索游标与查询词不匹配")
	case errors.Is(err, ErrInvalidInput):
		c.validationError(w, r, "参数错误")
	default:
		c.internalError(w, r, fallback, err)
	}
}

func (c *Controller) validationError(
	w http.ResponseWriter,
	r *http.Request,
	message string,
) {
	httpapi.Fail(w, r, http.StatusBadRequest, httpapi.ErrorBody{
		Code:      "VALIDATION_ERROR",
		Kind:      "validation",
		Message:   message,
		Retryable: false,
	})
}

func (c *Controller) internalError(
	w http.ResponseWriter,
	r *http.Request,
	message string,
	err error,
) {
	c.logger.Error(
		"docs request failed",
		"request_id", httpapi.RequestID(r),
		"error", err,
	)
	httpapi.Fail(w, r, http.StatusInternalServerError, httpapi.ErrorBody{
		Code:      "INTERNAL_ERROR",
		Kind:      "unknown",
		Message:   message,
		Retryable: true,
	})
}
```

- [ ] **Step 2: 在 main.go 注册**

在 `server/cmd/craftingtabled/main.go` 的 import 中加入 `"github.com/leeat/craftingtable/server/internal/docs"`，并在 `today.NewController(...)` 之后加一行：

```go
	today.NewController(sqliteStore, logger).Register(router)
	docs.NewController(sqliteStore, logger).Register(router)
```

- [ ] **Step 3: 编译并跑单元测试**

Run: `go build ./... ; go test ./server/... -v`
Expected: 编译通过，既有测试全部 PASS

- [ ] **Step 4: 提交**

```bash
git add server/internal/docs/controller.go server/cmd/craftingtabled/main.go
git commit -m "feat: expose document http api"
```

---

## Task 8: Go 集成测试覆盖 API 全链路

**Files:**
- Create: `tests/e2e/integration/docs.integration_test.go`

**Interfaces:**
- Consumes: `startServer`、`requestJSON`（`tests/e2e/integration/support_test.go`）
- Produces: 无（仅验证 Task 7 的 API 契约）

- [ ] **Step 1: 写集成测试**

创建 `tests/e2e/integration/docs.integration_test.go`：

```go
package integration

import (
	"net/http"
	"net/url"
	"testing"
)

type docNodeSummaryResponse struct {
	ID        string  `json:"id"`
	ParentID  *string `json:"parentId"`
	Kind      string  `json:"kind"`
	Title     string  `json:"title"`
	SortOrder int     `json:"sortOrder"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

type docNodeResponse struct {
	docNodeSummaryResponse
	Content string `json:"content"`
}

type docSearchHitResponse struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	ParentID  *string  `json:"parentId"`
	Path      []string `json:"path"`
	UpdatedAt string   `json:"updatedAt"`
}

type docSearchResponse struct {
	Items      []docSearchHitResponse `json:"items"`
	NextCursor *string                `json:"nextCursor"`
}

type docNodeEnvelope struct {
	Data docNodeResponse `json:"data"`
}

type docTreeEnvelope struct {
	Data []docNodeSummaryResponse `json:"data"`
}

type docSearchEnvelope struct {
	Data docSearchResponse `json:"data"`
}

type docSaveEnvelope struct {
	Data struct {
		ID        string `json:"id"`
		UpdatedAt string `json:"updatedAt"`
	} `json:"data"`
}

type docDeleteEnvelope struct {
	Data struct {
		DeletedIDs []string `json:"deletedIds"`
	} `json:"data"`
}

func TestDocsAPIEditsFolderAndDocument(t *testing.T) {
	server := startServer(t, "")

	status, folder := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"folder","title":"工作","clientRequestId":"req-folder"}`,
	)
	if status != http.StatusOK || folder.Data.Kind != "folder" {
		t.Fatalf("create folder = %d/%#v", status, folder.Data)
	}

	status, document := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"document","title":"接口设计","parentId":"`+folder.Data.ID+`","clientRequestId":"req-doc"}`,
	)
	if status != http.StatusOK || document.Data.ParentID == nil {
		t.Fatalf("create document = %d/%#v", status, document.Data)
	}
	if document.Data.Content != "" {
		t.Fatalf("new document content = %q, want empty", document.Data.Content)
	}

	status, duplicate := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"document","title":"重复请求","clientRequestId":"req-doc"}`,
	)
	if status != http.StatusOK || duplicate.Data.ID != document.Data.ID {
		t.Fatalf("idempotent create = %d/%#v", status, duplicate.Data)
	}

	status, saved := requestJSON[docSaveEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/save"),
		`{"id":"`+document.Data.ID+`","content":"# 接口设计\n\n- 列表\n- 详情"}`,
	)
	if status != http.StatusOK || saved.Data.UpdatedAt == "" {
		t.Fatalf("save = %d/%#v", status, saved.Data)
	}

	status, loaded := requestJSON[docNodeEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/item?id="+document.Data.ID),
		"",
	)
	if status != http.StatusOK || loaded.Data.Content != "# 接口设计\n\n- 列表\n- 详情" {
		t.Fatalf("load = %d/%#v", status, loaded.Data)
	}

	status, tree := requestJSON[docTreeEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/tree"),
		"",
	)
	if status != http.StatusOK || len(tree.Data) != 2 {
		t.Fatalf("tree = %d/%#v", status, tree.Data)
	}
	if tree.Data[0].Kind != "folder" {
		t.Fatalf("tree order = %#v", tree.Data)
	}

	status, renamed := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/rename"),
		`{"id":"`+document.Data.ID+`","title":"  改名后的文档  "}`,
	)
	if status != http.StatusOK || renamed.Data.Title != "改名后的文档" {
		t.Fatalf("rename = %d/%#v", status, renamed.Data)
	}

	status, moved := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/move"),
		`{"id":"`+document.Data.ID+`","parentId":null}`,
	)
	if status != http.StatusOK || moved.Data.ParentID != nil {
		t.Fatalf("move = %d/%#v", status, moved.Data)
	}
}

func TestDocsAPIValidatesInput(t *testing.T) {
	server := startServer(t, "")

	cases := []struct {
		name    string
		path    string
		body    string
		message string
	}{
		{
			name: "invalid kind",
			path: "/api/v1/docs/create",
			body: `{"kind":"page","title":"错类型"}`,
		},
		{
			name: "blank parent move",
			path: "/api/v1/docs/move",
			body: `{"id":"missing","parentId":null}`,
		},
	}

	for _, test := range cases {
		status, failure := requestJSON[errorEnvelope](
			t,
			http.MethodPost,
			server.URL(test.path),
			test.body,
		)
		if test.name == "invalid kind" {
			if status != http.StatusBadRequest || failure.Error.Code != "VALIDATION_ERROR" {
				t.Fatalf("%s = %d/%#v", test.name, status, failure.Error)
			}
			continue
		}
		if status != http.StatusNotFound || failure.Error.Code != "NOT_FOUND" {
			t.Fatalf("%s = %d/%#v", test.name, status, failure.Error)
		}
	}

	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/save"),
		`{"id":"missing","content":"正文"}`,
	)
	if status != http.StatusNotFound || failure.Error.Code != "NOT_FOUND" {
		t.Fatalf("save missing = %d/%#v", status, failure.Error)
	}
}

func TestDocsAPISearchesAndPaginates(t *testing.T) {
	server := startServer(t, "")

	status, folder := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"folder","title":"知识库"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create folder = %d", status)
	}

	for _, title := range []string{"接口一", "接口二", "接口三"} {
		status, created := requestJSON[docNodeEnvelope](
			t,
			http.MethodPost,
			server.URL("/api/v1/docs/create"),
			`{"kind":"document","title":"`+title+`","parentId":"`+folder.Data.ID+`"}`,
		)
		if status != http.StatusOK {
			t.Fatalf("create %q = %d", title, status)
		}
		_ = created
	}

	status, first := requestJSON[docSearchEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/search?q="+url.QueryEscape("接口")+"&limit=2"),
		"",
	)
	if status != http.StatusOK || len(first.Data.Items) != 2 {
		t.Fatalf("first page = %d/%#v", status, first.Data)
	}
	if first.Data.NextCursor == nil {
		t.Fatal("next cursor is missing")
	}
	if len(first.Data.Items[0].Path) != 1 || first.Data.Items[0].Path[0] != "知识库" {
		t.Fatalf("path = %#v", first.Data.Items[0].Path)
	}

	status, second := requestJSON[docSearchEnvelope](
		t,
		http.MethodGet,
		server.URL(
			"/api/v1/docs/search?q="+url.QueryEscape("接口")+
				"&limit=2&cursor="+url.QueryEscape(*first.Data.NextCursor),
		),
		"",
	)
	if status != http.StatusOK || len(second.Data.Items) != 1 {
		t.Fatalf("second page = %d/%#v", status, second.Data)
	}

	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/search?q="+url.QueryEscape("接口")+"&cursor=broken"),
		"",
	)
	if status != http.StatusBadRequest || failure.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("invalid cursor = %d/%#v", status, failure.Error)
	}

	status, empty := requestJSON[docSearchEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/search?q="),
		"",
	)
	if status != http.StatusOK || len(empty.Data.Items) != 0 {
		t.Fatalf("empty query = %d/%#v", status, empty.Data)
	}
}

func TestDocsAPIDeletesSubtree(t *testing.T) {
	server := startServer(t, "")

	status, folder := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"folder","title":"待删除"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create folder = %d", status)
	}
	status, document := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"document","title":"子文档","parentId":"`+folder.Data.ID+`"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create document = %d", status)
	}

	status, deleted := requestJSON[docDeleteEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/delete"),
		`{"id":"`+folder.Data.ID+`"}`,
	)
	if status != http.StatusOK || len(deleted.Data.DeletedIDs) != 2 {
		t.Fatalf("delete = %d/%#v", status, deleted.Data)
	}

	status, tree := requestJSON[docTreeEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/tree"),
		"",
	)
	if status != http.StatusOK || len(tree.Data) != 0 {
		t.Fatalf("tree after delete = %d/%#v", status, tree.Data)
	}

	status, _ = requestJSON[errorEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/item?id="+document.Data.ID),
		"",
	)
	if status != http.StatusNotFound {
		t.Fatalf("deleted item status = %d, want 404", status)
	}
}
```

- [ ] **Step 2: 运行集成测试**

Run: `go test ./tests/e2e/integration/ -run TestDocsAPI -v`
Expected: 四个测试全部 PASS

- [ ] **Step 3: 跑全量 Go 测试**

Run: `pnpm test:go`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add tests/e2e/integration/docs.integration_test.go
git commit -m "test: cover document api round trip"
```

---

## Task 9: 前端契约与数据适配器

**Files:**
- Create: `src/models/docContracts.ts`
- Create: `src/models/apiDocsModel.ts`
- Create: `src/models/fakeDocsModel.ts`
- Create: `src/models/docsModel.ts`
- Test: `tests/e2e/unit/docs/apiDocsModel.test.ts`

**Interfaces:**
- Consumes: `/api/v1/docs/*` 契约（Task 7）
- Produces:
  - `DocNodeKind`、`DocNodeSummary`、`DocNode`、`SearchHit`、`SearchPage`
  - `DocsModel` 接口：`listTree`、`getNode`、`createNode`、`renameNode`、`moveNode`、`saveContent`、`deleteNode`、`search`
  - `apiDocsModel`、`fakeDocsModel`、`docsModel`

- [ ] **Step 1: 写失败测试**

创建 `tests/e2e/unit/docs/apiDocsModel.test.ts`：

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import { apiDocsModel } from "@/models/apiDocsModel";

function mockFetch(body: unknown, status = 200) {
  const spy = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", spy);
  return spy;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("apiDocsModel", () => {
  it("loads the tree without content", async () => {
    const spy = mockFetch({
      data: [
        {
          id: "node-1",
          parentId: null,
          kind: "folder",
          title: "工作",
          sortOrder: 0,
          createdAt: "2026-11-06T00:00:00Z",
          updatedAt: "2026-11-06T00:00:00Z",
        },
      ],
    });

    const nodes = await apiDocsModel.listTree();

    expect(spy).toHaveBeenCalledWith("/api/v1/docs/tree", expect.any(Object));
    expect(nodes[0].title).toBe("工作");
  });

  it("encodes the search query", async () => {
    const spy = mockFetch({ data: { items: [], nextCursor: null } });

    await apiDocsModel.search({ q: "接口 设计", limit: 20 });

    expect(spy.mock.calls[0][0]).toBe(
      "/api/v1/docs/search?q=%E6%8E%A5%E5%8F%A3%20%E8%AE%BE%E8%AE%A1&limit=20",
    );
  });

  it("posts the save payload", async () => {
    const spy = mockFetch({
      data: { id: "node-1", updatedAt: "2026-11-06T00:00:01Z" },
    });

    const result = await apiDocsModel.saveContent({
      id: "node-1",
      content: "# 标题",
    });

    expect(result.updatedAt).toBe("2026-11-06T00:00:01Z");
    expect(JSON.parse(spy.mock.calls[0][1].body as string)).toEqual({
      id: "node-1",
      content: "# 标题",
    });
  });

  it("surfaces the server error message", async () => {
    mockFetch(
      {
        error: {
          code: "VALIDATION_ERROR",
          kind: "validation",
          message: "不能移动到自身或子节点",
          retryable: false,
        },
      },
      400,
    );

    await expect(
      apiDocsModel.moveNode({ id: "a", parentId: "a" }),
    ).rejects.toThrow("不能移动到自身或子节点");
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run tests/e2e/unit/docs/apiDocsModel.test.ts`
Expected: FAIL，无法解析 `@/models/apiDocsModel`

- [ ] **Step 3: 写契约与适配器**

创建 `src/models/docContracts.ts`：

```ts
export type DocNodeKind = "folder" | "document";

export type DocNodeSummary = {
  id: string;
  parentId: string | null;
  kind: DocNodeKind;
  title: string;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
};

export type DocNode = DocNodeSummary & {
  content: string;
};

export type DocSearchHit = {
  id: string;
  kind: DocNodeKind;
  title: string;
  parentId: string | null;
  path: string[];
  updatedAt: string;
};

export type DocSearchPage = {
  items: DocSearchHit[];
  nextCursor: string | null;
};

export type CreateDocNodeInput = {
  kind: DocNodeKind;
  parentId?: string | null;
  title?: string;
  clientRequestId: string;
};

export type SaveDocContentResult = {
  id: string;
  updatedAt: string;
};

export interface DocsModel {
  listTree(): Promise<DocNodeSummary[]>;
  getNode(id: string): Promise<DocNode>;
  createNode(input: CreateDocNodeInput): Promise<DocNode>;
  renameNode(input: { id: string; title: string }): Promise<DocNodeSummary>;
  moveNode(input: {
    id: string;
    parentId: string | null;
  }): Promise<DocNodeSummary>;
  saveContent(input: {
    id: string;
    content: string;
  }): Promise<SaveDocContentResult>;
  deleteNode(id: string): Promise<{ deletedIds: string[] }>;
  search(input: {
    q: string;
    limit?: number;
    cursor?: string | null;
    signal?: AbortSignal;
  }): Promise<DocSearchPage>;
}
```

创建 `src/models/apiDocsModel.ts`：

```ts
import type {
  CreateDocNodeInput,
  DocNode,
  DocNodeSummary,
  DocsModel,
  DocSearchPage,
  SaveDocContentResult,
} from "./docContracts";

type Envelope<T> =
  | { data: T }
  | { error: { code: string; message: string } };

const baseUrl = import.meta.env.VITE_API_BASE ?? "/api/v1";

async function readEnvelope<T>(response: Response): Promise<Envelope<T> | null> {
  const text = await response.text();
  if (!text.trim()) {
    return null;
  }
  try {
    return JSON.parse(text) as Envelope<T>;
  } catch {
    return null;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  const body = await readEnvelope<T>(response);
  if (!body) {
    throw new Error(
      response.ok
        ? "服务返回了无效响应，请稍后重试"
        : "服务暂时不可用，请稍后重试",
    );
  }
  if (!response.ok || "error" in body) {
    throw new Error("error" in body ? body.error.message : "请求失败");
  }
  return body.data;
}

export const apiDocsModel: DocsModel = {
  listTree: () => request<DocNodeSummary[]>("/docs/tree"),
  getNode: (id: string) =>
    request<DocNode>(`/docs/item?id=${encodeURIComponent(id)}`),
  createNode: (input: CreateDocNodeInput) =>
    request<DocNode>("/docs/create", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  renameNode: (input) =>
    request<DocNodeSummary>("/docs/rename", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  moveNode: (input) =>
    request<DocNodeSummary>("/docs/move", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  saveContent: (input) =>
    request<SaveDocContentResult>("/docs/save", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  deleteNode: (id: string) =>
    request<{ deletedIds: string[] }>("/docs/delete", {
      method: "POST",
      body: JSON.stringify({ id }),
    }),
  search: ({ q, limit, cursor, signal }) => {
    const params = new URLSearchParams({ q });
    if (limit !== undefined) {
      params.set("limit", String(limit));
    }
    if (cursor) {
      params.set("cursor", cursor);
    }
    return request<DocSearchPage>(`/docs/search?${params.toString()}`, {
      signal,
    });
  },
};
```

创建 `src/models/fakeDocsModel.ts`：

```ts
import type {
  CreateDocNodeInput,
  DocNode,
  DocNodeSummary,
  DocsModel,
  DocSearchPage,
} from "./docContracts";

function now() {
  return new Date().toISOString();
}

function toSummary(node: DocNode): DocNodeSummary {
  const { content, ...summary } = node;
  void content;
  return summary;
}

export function createFakeDocsModel(): DocsModel {
  const nodes = new Map<string, DocNode>();
  const requests = new Map<string, string>();

  const listTree = async () =>
    [...nodes.values()]
      .filter((node) => !("deletedAt" in node))
      .map(toSummary)
      .sort((left, right) => {
        if (left.kind !== right.kind) {
          return left.kind === "folder" ? -1 : 1;
        }
        return right.updatedAt.localeCompare(left.updatedAt);
      });

  const getNode = async (id: string) => {
    const node = nodes.get(id);
    if (!node) {
      throw new Error("文档不存在或已被删除");
    }
    return node;
  };

  const createNode = async (input: CreateDocNodeInput) => {
    const existing = input.clientRequestId
      ? requests.get(input.clientRequestId)
      : undefined;
    if (existing) {
      return getNode(existing);
    }
    const id = `fake-${nodes.size + 1}`;
    const timestamp = now();
    const node: DocNode = {
      id,
      parentId: input.parentId ?? null,
      kind: input.kind,
      title: input.title?.trim() || (input.kind === "folder" ? "未命名文件夹" : "无标题文档"),
      sortOrder: 0,
      createdAt: timestamp,
      updatedAt: timestamp,
      content: "",
    };
    nodes.set(id, node);
    if (input.clientRequestId) {
      requests.set(input.clientRequestId, id);
    }
    return node;
  };

  const renameNode = async ({ id, title }: { id: string; title: string }) => {
    const node = await getNode(id);
    node.title = title.trim() || node.title;
    node.updatedAt = now();
    return toSummary(node);
  };

  const moveNode = async ({
    id,
    parentId,
  }: {
    id: string;
    parentId: string | null;
  }) => {
    const node = await getNode(id);
    node.parentId = parentId;
    node.updatedAt = now();
    return toSummary(node);
  };

  const saveContent = async ({
    id,
    content,
  }: {
    id: string;
    content: string;
  }) => {
    const node = await getNode(id);
    node.content = content;
    node.updatedAt = now();
    return { id, updatedAt: node.updatedAt };
  };

  const deleteNode = async (id: string) => {
    const deleted: string[] = [];
    const collect = (target: string) => {
      const node = nodes.get(target);
      if (!node) {
        return;
      }
      deleted.push(target);
      nodes.delete(target);
      for (const child of [...nodes.values()]) {
        if (child.parentId === target) {
          collect(child.id);
        }
      }
    };
    collect(id);
    if (deleted.length === 0) {
      throw new Error("文档不存在或已被删除");
    }
    return { deletedIds: deleted };
  };

  const search = async ({
    q,
    limit = 20,
  }: {
    q: string;
    limit?: number;
    cursor?: string | null;
    signal?: AbortSignal;
  }): Promise<DocSearchPage> => {
    const query = q.trim().toLowerCase();
    if (!query) {
      return { items: [], nextCursor: null };
    }
    const hits = [...nodes.values()]
      .filter((node) => node.title.toLowerCase().includes(query))
      .sort((left, right) => left.title.localeCompare(right.title))
      .slice(0, limit)
      .map((node) => {
        const path: string[] = [];
        let parentId = node.parentId;
        while (parentId) {
          const parent = nodes.get(parentId);
          if (!parent) {
            break;
          }
          path.unshift(parent.title);
          parentId = parent.parentId;
        }
        return {
          id: node.id,
          kind: node.kind,
          title: node.title,
          parentId: node.parentId,
          path,
          updatedAt: node.updatedAt,
        };
      });
    return { items: hits, nextCursor: null };
  };

  return {
    listTree,
    getNode,
    createNode,
    renameNode,
    moveNode,
    saveContent,
    deleteNode,
    search,
  };
}

export const fakeDocsModel = createFakeDocsModel();
```

创建 `src/models/docsModel.ts`：

```ts
import type { DocsModel } from "./docContracts";
import { apiDocsModel } from "./apiDocsModel";
import { fakeDocsModel } from "./fakeDocsModel";

export const docsModel: DocsModel =
  import.meta.env.VITE_DATA_SOURCE === "mock" || import.meta.env.MODE === "test"
    ? fakeDocsModel
    : apiDocsModel;
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm vitest run tests/e2e/unit/docs/apiDocsModel.test.ts`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add src/models/docContracts.ts src/models/apiDocsModel.ts src/models/fakeDocsModel.ts src/models/docsModel.ts tests/e2e/unit/docs/apiDocsModel.test.ts
git commit -m "feat: add document model contracts and api adapter"
```

---

## Task 10: 文档 Store

**Files:**
- Create: `src/controllers/stores/docsStore.ts`
- Test: `tests/e2e/unit/docs/docsStore.test.ts`

**Interfaces:**
- Consumes: `DocNodeSummary`、`DocNode`、`DocSearchHit`
- Produces:
  - `useDocsStore`
  - `initialDocsState`（用于测试重置）
  - 动作：`setLoading`、`setTree`、`setError`、`setActiveNode`、`setDraftTitle`、`setDraftContent`、`setSaveStatus`、`markSaved`、`replaceSummary`、`removeNodes`、`clearActiveNode`、`setSearchQuery`、`setSearchPhase`、`setSearchResults`、`setSearchError`、`clearSearch`

- [ ] **Step 1: 写失败测试**

创建 `tests/e2e/unit/docs/docsStore.test.ts`：

```ts
import { beforeEach, describe, expect, it } from "vitest";
import { initialDocsState, useDocsStore } from "@/controllers/stores/docsStore";

const folder = {
  id: "folder-1",
  parentId: null,
  kind: "folder" as const,
  title: "工作",
  sortOrder: 0,
  createdAt: "2026-11-06T00:00:00Z",
  updatedAt: "2026-11-06T00:00:00Z",
};

const document = {
  ...folder,
  id: "doc-1",
  kind: "document" as const,
  title: "接口设计",
};

beforeEach(() => {
  useDocsStore.setState(initialDocsState, true);
});

describe("docsStore", () => {
  it("stores the tree and replaces single summaries", () => {
    useDocsStore.getState().setTree([folder, document]);
    useDocsStore.getState().replaceSummary({ ...document, title: "改名" });

    const state = useDocsStore.getState();
    expect(state.nodes).toHaveLength(2);
    expect(state.nodes[1].title).toBe("改名");
  });

  it("removes a subtree by identifiers", () => {
    useDocsStore.getState().setTree([folder, document]);
    useDocsStore.getState().removeNodes(["folder-1", "doc-1"]);

    expect(useDocsStore.getState().nodes).toHaveLength(0);
  });

  it("keeps the active node when the draft is saved", () => {
    useDocsStore.getState().setActiveNode({
      ...document,
      content: "# 旧内容",
    });
    useDocsStore.getState().setDraftContent("# 新内容");
    useDocsStore.getState().setSaveStatus("saving");
    useDocsStore.getState().markSaved({
      id: "doc-1",
      content: "# 新内容",
      updatedAt: "2026-11-06T00:00:05Z",
    });

    const state = useDocsStore.getState();
    expect(state.saveStatus).toBe("saved");
    expect(state.draftContent).toBe("# 新内容");
    expect(state.activeNode?.content).toBe("# 新内容");
    expect(state.activeNode?.updatedAt).toBe("2026-11-06T00:00:05Z");
  });

  it("clears search state when the query is emptied", () => {
    useDocsStore.getState().setSearchQuery("接口");
    useDocsStore.getState().setSearchResults(
      [
        {
          id: "doc-1",
          kind: "document",
          title: "接口设计",
          parentId: null,
          path: [],
          updatedAt: "2026-11-06T00:00:00Z",
        },
      ],
      "cursor-1",
    );
    useDocsStore.getState().clearSearch();

    const state = useDocsStore.getState();
    expect(state.searchQuery).toBe("");
    expect(state.searchResults).toHaveLength(0);
    expect(state.searchCursor).toBeNull();
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run tests/e2e/unit/docs/docsStore.test.ts`
Expected: FAIL，无法解析 `@/controllers/stores/docsStore`

- [ ] **Step 3: 实现 Store**

创建 `src/controllers/stores/docsStore.ts`：

```ts
import { create } from "zustand";
import type {
  DocNode,
  DocNodeSummary,
  DocSearchHit,
} from "@/models/docContracts";

export type DocsPhase = "idle" | "loading" | "ready" | "error";
export type SaveStatus = "idle" | "saving" | "saved" | "error";
export type SearchPhase = "idle" | "loading" | "ready" | "error";

type DocsState = {
  nodes: DocNodeSummary[];
  activeNodeId: string | null;
  activeNode: DocNode | null;
  draftTitle: string;
  draftContent: string;
  saveStatus: SaveStatus;
  phase: DocsPhase;
  errorMessage: string | null;
  searchQuery: string;
  searchResults: DocSearchHit[];
  searchPhase: SearchPhase;
  searchCursor: string | null;
};

type DocsActions = {
  setLoading: () => void;
  setTree: (nodes: DocNodeSummary[]) => void;
  setError: (message: string) => void;
  setActiveNode: (node: DocNode) => void;
  clearActiveNode: () => void;
  setDraftTitle: (title: string) => void;
  setDraftContent: (content: string) => void;
  setSaveStatus: (status: SaveStatus) => void;
  markSaved: (input: {
    id: string;
    content: string;
    updatedAt: string;
  }) => void;
  replaceSummary: (node: DocNodeSummary) => void;
  removeNodes: (identifiers: string[]) => void;
  setSearchQuery: (query: string) => void;
  setSearchPhase: (phase: SearchPhase) => void;
  setSearchResults: (items: DocSearchHit[], cursor: string | null) => void;
  setSearchError: () => void;
  clearSearch: () => void;
};

export const initialDocsState: DocsState = {
  nodes: [],
  activeNodeId: null,
  activeNode: null,
  draftTitle: "",
  draftContent: "",
  saveStatus: "idle",
  phase: "idle",
  errorMessage: null,
  searchQuery: "",
  searchResults: [],
  searchPhase: "idle",
  searchCursor: null,
};

export const useDocsStore = create<DocsState & DocsActions>()((set) => ({
  ...initialDocsState,
  setLoading: () => set({ phase: "loading", errorMessage: null }),
  setTree: (nodes) => set({ nodes, phase: "ready", errorMessage: null }),
  setError: (errorMessage) => set({ phase: "error", errorMessage }),
  setActiveNode: (node) =>
    set({
      activeNodeId: node.id,
      activeNode: node,
      draftTitle: node.title,
      draftContent: node.content,
      saveStatus: "idle",
      phase: "ready",
      errorMessage: null,
    }),
  clearActiveNode: () =>
    set({
      activeNodeId: null,
      activeNode: null,
      draftTitle: "",
      draftContent: "",
      saveStatus: "idle",
    }),
  setDraftTitle: (draftTitle) => set({ draftTitle }),
  setDraftContent: (draftContent) => set({ draftContent }),
  setSaveStatus: (saveStatus) => set({ saveStatus }),
  markSaved: ({ id, content, updatedAt }) =>
    set((state) => {
      if (state.activeNodeId !== id) {
        return { saveStatus: "saved" as const };
      }
      const activeNode = state.activeNode
        ? {
            ...state.activeNode,
            content,
            updatedAt,
          }
        : null;
      return {
        saveStatus: "saved" as const,
        activeNode,
        nodes: state.nodes.map((node) =>
          node.id === id ? { ...node, updatedAt } : node,
        ),
      };
    }),
  replaceSummary: (node) =>
    set((state) => ({
      nodes: state.nodes.map((current) =>
        current.id === node.id ? node : current,
      ),
      draftTitle:
        state.activeNodeId === node.id ? node.title : state.draftTitle,
      activeNode:
        state.activeNode && state.activeNode.id === node.id
          ? { ...state.activeNode, ...node }
          : state.activeNode,
    })),
  removeNodes: (identifiers) =>
    set((state) => {
      const removed = new Set(identifiers);
      return {
        nodes: state.nodes.filter((node) => !removed.has(node.id)),
        activeNodeId:
          state.activeNodeId && removed.has(state.activeNodeId)
            ? null
            : state.activeNodeId,
        activeNode:
          state.activeNode && removed.has(state.activeNode.id)
            ? null
            : state.activeNode,
      };
    }),
  setSearchQuery: (searchQuery) => set({ searchQuery }),
  setSearchPhase: (searchPhase) => set({ searchPhase }),
  setSearchResults: (searchResults, searchCursor) =>
    set({ searchResults, searchCursor, searchPhase: "ready" }),
  setSearchError: () => set({ searchPhase: "error" }),
  clearSearch: () =>
    set({
      searchQuery: "",
      searchResults: [],
      searchCursor: null,
      searchPhase: "idle",
    }),
}));

export function resetDocsStore() {
  useDocsStore.setState(initialDocsState, true);
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm vitest run tests/e2e/unit/docs/docsStore.test.ts`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add src/controllers/stores/docsStore.ts tests/e2e/unit/docs/docsStore.test.ts
git commit -m "feat: add document state store"
```

---

## Task 11: 文档 Effect（防抖保存与搜索乱序保护）

**Files:**
- Create: `src/controllers/effects/useDocsEffect.ts`
- Test: `tests/e2e/unit/docs/useDocsEffect.test.ts`

**Interfaces:**
- Consumes: `DocsModel`、`useDocsStore`、`toast`（sonner）
- Produces:
  - `createDocsEffect(model, notify?)`，返回 `loadTree`、`openNode`、`createNode`、`renameNode`、`moveNode`、`deleteNode`、`applyContentChange`、`applyTitleChange`、`flushSave`、`search`、`flushSearch`、`dispose`
  - `useDocsEffect(model?)`：组件内使用，卸载时自动 flush

- [ ] **Step 1: 写失败测试**

创建 `tests/e2e/unit/docs/useDocsEffect.test.ts`：

```ts
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createDocsEffect } from "@/controllers/effects/useDocsEffect";
import { initialDocsState, useDocsStore } from "@/controllers/stores/docsStore";
import type { DocsModel, DocNode } from "@/models/docContracts";

const folder = {
  id: "folder-1",
  parentId: null,
  kind: "folder" as const,
  title: "工作",
  sortOrder: 0,
  createdAt: "2026-11-06T00:00:00Z",
  updatedAt: "2026-11-06T00:00:00Z",
};

const document: DocNode = {
  ...folder,
  id: "doc-1",
  kind: "document",
  title: "接口设计",
  content: "# 旧内容",
};

function createModel(overrides: Partial<DocsModel> = {}): DocsModel {
  return {
    listTree: vi.fn().mockResolvedValue([folder, document]),
    getNode: vi.fn().mockResolvedValue(document),
    createNode: vi.fn().mockResolvedValue(document),
    renameNode: vi.fn().mockResolvedValue(document),
    moveNode: vi.fn().mockResolvedValue(document),
    saveContent: vi
      .fn()
      .mockResolvedValue({ id: "doc-1", updatedAt: "2026-11-06T00:00:09Z" }),
    deleteNode: vi.fn().mockResolvedValue({ deletedIds: ["doc-1"] }),
    search: vi.fn().mockResolvedValue({ items: [], nextCursor: null }),
    ...overrides,
  };
}

const notify = { success: vi.fn(), error: vi.fn() };

beforeEach(() => {
  vi.useFakeTimers();
  useDocsStore.setState(initialDocsState, true);
  notify.success.mockReset();
  notify.error.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("docsEffect", () => {
  it("loads the tree into the store", async () => {
    const effect = createDocsEffect(createModel(), notify);

    await effect.loadTree();

    expect(useDocsStore.getState().nodes).toHaveLength(2);
    expect(useDocsStore.getState().phase).toBe("ready");
  });

  it("debounces content saves and flushes immediately", async () => {
    const model = createModel();
    const effect = createDocsEffect(model, notify);
    useDocsStore.getState().setActiveNode(document);

    effect.applyContentChange("doc-1", "# 新内容");
    await vi.advanceTimersByTimeAsync(399);
    expect(model.saveContent).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);
    expect(model.saveContent).toHaveBeenCalledWith({
      id: "doc-1",
      content: "# 新内容",
    });
    expect(useDocsStore.getState().saveStatus).toBe("saved");
  });

  it("flushes pending content before switching documents", async () => {
    const model = createModel();
    const effect = createDocsEffect(model, notify);
    useDocsStore.getState().setActiveNode(document);

    effect.applyContentChange("doc-1", "# 待保存");
    await effect.flushSave();

    expect(model.saveContent).toHaveBeenCalledTimes(1);
    expect(useDocsStore.getState().activeNode?.content).toBe("# 待保存");
  });

  it("reports save failures without dropping the draft", async () => {
    const model = createModel({
      saveContent: vi.fn().mockRejectedValue(new Error("网络异常")),
    });
    const effect = createDocsEffect(model, notify);
    useDocsStore.getState().setActiveNode(document);

    effect.applyContentChange("doc-1", "# 保留我");
    await effect.flushSave();

    expect(useDocsStore.getState().saveStatus).toBe("error");
    expect(useDocsStore.getState().draftContent).toBe("# 保留我");
    expect(notify.error).toHaveBeenCalledWith("网络异常");
  });

  it("debounces search and ignores stale responses", async () => {
    const first = {
      items: [
        {
          id: "old",
          kind: "document" as const,
          title: "旧结果",
          parentId: null,
          path: [],
          updatedAt: "2026-11-06T00:00:00Z",
        },
      ],
      nextCursor: null,
    };
    const second = { items: [], nextCursor: null };
    const search = vi
      .fn()
      .mockImplementationOnce(
        () => new Promise((resolve) => setTimeout(() => resolve(first), 50)),
      )
      .mockResolvedValue(second);

    const effect = createDocsEffect(createModel({ search }), notify);

    effect.search("接口");
    await vi.advanceTimersByTimeAsync(300);
    effect.search("文档");
    await vi.advanceTimersByTimeAsync(300);
    await vi.advanceTimersByTimeAsync(100);

    expect(search).toHaveBeenLastCalledWith(
      expect.objectContaining({ q: "文档", limit: 20 }),
    );
    expect(useDocsStore.getState().searchResults).toHaveLength(0);
  });

  it("clears search state without a request for blank queries", async () => {
    const search = vi.fn();
    const effect = createDocsEffect(createModel({ search }), notify);

    effect.search("   ");
    await vi.advanceTimersByTimeAsync(400);

    expect(search).not.toHaveBeenCalled();
    expect(useDocsStore.getState().searchQuery).toBe("");
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run tests/e2e/unit/docs/useDocsEffect.test.ts`
Expected: FAIL，无法解析 `@/controllers/effects/useDocsEffect`

- [ ] **Step 3: 实现 Effect**

创建 `src/controllers/effects/useDocsEffect.ts`：

```ts
import { useEffect, useMemo } from "react";
import { toast } from "sonner";
import { useDocsStore } from "@/controllers/stores/docsStore";
import type { DocNodeKind, DocsModel } from "@/models/docContracts";
import { docsModel } from "@/models/docsModel";

type Notifier = {
  success: (message: string) => void;
  error: (message: string) => void;
};

const defaultNotifier: Notifier = {
  success: (message) => toast.success(message),
  error: (message) => toast.error(message),
};

const CONTENT_SAVE_DELAY = 400;
const TITLE_SAVE_DELAY = 600;
const SEARCH_DELAY = 300;

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "操作失败，请重试";
}

export function createDocsEffect(
  model: DocsModel,
  notify: Notifier = defaultNotifier,
) {
  let contentTimer: ReturnType<typeof setTimeout> | null = null;
  let titleTimer: ReturnType<typeof setTimeout> | null = null;
  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  let pendingSave: { id: string; content: string } | null = null;
  let pendingTitle: { id: string; title: string } | null = null;
  let searchSequence = 0;
  let saveSequence: Promise<void> = Promise.resolve();

  const loadTree = async () => {
    useDocsStore.getState().setLoading();
    try {
      const nodes = await model.listTree();
      useDocsStore.getState().setTree(nodes);
    } catch (error) {
      const message = messageOf(error);
      useDocsStore.getState().setError(message);
      notify.error(message);
    }
  };

  const openNode = async (id: string) => {
    if (useDocsStore.getState().activeNodeId === id) {
      return;
    }
    await flushSave();
    useDocsStore.getState().setLoading();
    try {
      const node = await model.getNode(id);
      useDocsStore.getState().setActiveNode(node);
    } catch (error) {
      const message = messageOf(error);
      useDocsStore.getState().clearActiveNode();
      useDocsStore.getState().setError(message);
      notify.error(message);
    }
  };

  const persistContent = (target: { id: string; content: string }) => {
    saveSequence = saveSequence.then(async () => {
      useDocsStore.getState().setSaveStatus("saving");
      try {
        const result = await model.saveContent(target);
        useDocsStore.getState().markSaved({
          id: target.id,
          content: target.content,
          updatedAt: result.updatedAt,
        });
        flushTitleIfPending(target.id);
      } catch (error) {
        const message = messageOf(error);
        useDocsStore.getState().setSaveStatus("error");
        notify.error(message);
      }
    });
    return saveSequence;
  };

  const flushSave = async () => {
    if (contentTimer) {
      clearTimeout(contentTimer);
      contentTimer = null;
    }
    const target = pendingSave;
    pendingSave = null;
    if (!target) {
      return;
    }
    await persistContent(target);
  };

  const flushTitleIfPending = (id: string) => {
    if (!pendingTitle || pendingTitle.id !== id) {
      return;
    }
    if (titleTimer) {
      clearTimeout(titleTimer);
      titleTimer = null;
    }
    const target = pendingTitle;
    pendingTitle = null;
    void model
      .renameNode({ id: target.id, title: target.title })
      .then((summary) => useDocsStore.getState().replaceSummary(summary))
      .catch((error) => notify.error(messageOf(error)));
  };

  const applyContentChange = (id: string, content: string) => {
    useDocsStore.getState().setDraftContent(content);
    pendingSave = { id, content };
    if (contentTimer) {
      clearTimeout(contentTimer);
    }
    contentTimer = setTimeout(() => {
      contentTimer = null;
      const target = pendingSave;
      pendingSave = null;
      if (target) {
        void persistContent(target);
      }
    }, CONTENT_SAVE_DELAY);
  };

  const applyTitleChange = (id: string, title: string) => {
    useDocsStore.getState().setDraftTitle(title);
    pendingTitle = { id, title };
    if (titleTimer) {
      clearTimeout(titleTimer);
    }
    titleTimer = setTimeout(() => {
      titleTimer = null;
      const target = pendingTitle;
      pendingTitle = null;
      if (!target) {
        return;
      }
      void model
        .renameNode({ id: target.id, title: target.title })
        .then((summary) => useDocsStore.getState().replaceSummary(summary))
        .catch((error) => notify.error(messageOf(error)));
    }, TITLE_SAVE_DELAY);
  };

  const createNode = async (kind: DocNodeKind, parentId: string | null) => {
    try {
      const node = await model.createNode({
        kind,
        parentId,
        clientRequestId: crypto.randomUUID(),
      });
      useDocsStore.getState().setTree([
        ...useDocsStore.getState().nodes,
        node,
      ]);
      return node;
    } catch (error) {
      notify.error(messageOf(error));
      return null;
    }
  };

  const renameNode = async (id: string, title: string) => {
    try {
      const summary = await model.renameNode({ id, title });
      useDocsStore.getState().replaceSummary(summary);
      notify.success("已重命名");
      return summary;
    } catch (error) {
      notify.error(messageOf(error));
      return null;
    }
  };

  const moveNode = async (id: string, parentId: string | null) => {
    try {
      const summary = await model.moveNode({ id, parentId });
      useDocsStore.getState().replaceSummary(summary);
      notify.success("已移动");
      return summary;
    } catch (error) {
      notify.error(messageOf(error));
      return null;
    }
  };

  const deleteNode = async (id: string) => {
    try {
      const result = await model.deleteNode(id);
      useDocsStore.getState().removeNodes(result.deletedIds);
      notify.success("已删除");
      return result.deletedIds;
    } catch (error) {
      notify.error(messageOf(error));
      return null;
    }
  };

  const search = (rawQuery: string) => {
    const query = rawQuery.trim();
    useDocsStore.getState().setSearchQuery(rawQuery);

    if (searchTimer) {
      clearTimeout(searchTimer);
      searchTimer = null;
    }
    if (!query) {
      searchSequence += 1;
      useDocsStore.getState().clearSearch();
      return;
    }

    searchTimer = setTimeout(() => {
      searchTimer = null;
      searchSequence += 1;
      const sequence = searchSequence;
      useDocsStore.getState().setSearchPhase("loading");
      void model
        .search({ q: query, limit: 20 })
        .then((page) => {
          if (sequence !== searchSequence) {
            return;
          }
          useDocsStore.getState().setSearchResults(page.items, page.nextCursor);
        })
        .catch((error) => {
          if (sequence !== searchSequence) {
            return;
          }
          useDocsStore.getState().setSearchError();
          notify.error(messageOf(error));
        });
    }, SEARCH_DELAY);
  };

  const flushSearch = async () => {
    if (searchTimer) {
      clearTimeout(searchTimer);
      searchTimer = null;
    }
  };

  const dispose = () => {
    void flushSave();
    void flushSearch();
  };

  return {
    loadTree,
    openNode,
    createNode,
    renameNode,
    moveNode,
    deleteNode,
    applyContentChange,
    applyTitleChange,
    flushSave,
    search,
    flushSearch,
    dispose,
  };
}

export function useDocsEffect(model: DocsModel = docsModel) {
  const effect = useMemo(() => createDocsEffect(model), [model]);

  useEffect(() => () => effect.dispose(), [effect]);

  return effect;
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm vitest run tests/e2e/unit/docs/useDocsEffect.test.ts`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add src/controllers/effects/useDocsEffect.ts tests/e2e/unit/docs/useDocsEffect.test.ts
git commit -m "feat: add document effect with debounced save and search"
```

---

## Task 12: 编辑器依赖与 Tiptap 封装

**Files:**
- Modify: `package.json`
- Create: `src/components/docs/docEditor.tsx`
- Create: `src/styles/docsProse.css`
- Modify: `src/styles/globals.css:3`
- Test: `tests/e2e/unit/docs/docEditor.test.tsx`

**Interfaces:**
- Consumes: `StarterKit`、`Markdown`、`Placeholder`、`EditorContent`、`useEditor`、`useEditorState`、`BubbleMenu`
- Produces: `DocEditor({ documentId, initialMarkdown, onChange, ariaLabel? })`

- [ ] **Step 1: 安装依赖**

Run:
```bash
pnpm add @tiptap/react@3.32.3 @tiptap/pm@3.32.3 @tiptap/starter-kit@3.32.3 @tiptap/markdown@3.32.3 @tiptap/extensions@3.32.3
```
Expected: `package.json` 的 `dependencies` 新增 5 个包，`pnpm-lock.yaml` 更新

- [ ] **Step 2: 写失败测试**

创建 `tests/e2e/unit/docs/docEditor.test.tsx`：

```tsx
import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DocEditor } from "@/components/docs/docEditor";

describe("DocEditor", () => {
  it("renders markdown as editable content", async () => {
    render(
      <DocEditor
        documentId="doc-1"
        initialMarkdown={"# 标题\n\n- 一\n- 二"}
        onChange={() => undefined}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "标题" }),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("一")).toBeInTheDocument();
  });

  it("emits markdown when the document changes", async () => {
    const onChange = vi.fn();
    render(
      <DocEditor
        documentId="doc-1"
        initialMarkdown="# 标题"
        onChange={onChange}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "标题" })).toBeInTheDocument();
    });

    const editable = screen.getByLabelText("文档正文");
    editable.textContent = "改写后的内容";
    editable.dispatchEvent(new Event("input", { bubbles: true }));

    await waitFor(() => {
      expect(onChange).toHaveBeenCalled();
    });
    expect(onChange.mock.calls.at(-1)?.[0]).toContain("改写后的内容");
  });
});
```

- [ ] **Step 3: 运行测试确认失败**

Run: `pnpm vitest run tests/e2e/unit/docs/docEditor.test.tsx`
Expected: FAIL，无法解析 `@/components/docs/docEditor`

- [ ] **Step 4: 写样式与编辑器**

创建 `src/styles/docsProse.css`：

```css
.doc-prose {
  min-height: 320px;
  outline: none;
  color: var(--textBody);
  font-size: 15px;
  line-height: 1.75;
}

.doc-prose > * + * {
  margin-top: 0.9em;
}

.doc-prose h1,
.doc-prose h2,
.doc-prose h3 {
  color: var(--textPrimary);
  font-weight: 600;
  line-height: 1.35;
}

.doc-prose h1 {
  font-size: 1.6em;
  margin-top: 1.6em;
}

.doc-prose h2 {
  font-size: 1.3em;
  margin-top: 1.4em;
}

.doc-prose h3 {
  font-size: 1.1em;
  margin-top: 1.2em;
}

.doc-prose strong {
  color: var(--textPrimary);
  font-weight: 600;
}

.doc-prose a {
  color: var(--brand);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.doc-prose code {
  border-radius: 4px;
  background: var(--surfaceSubtle);
  padding: 0.1em 0.35em;
  font-size: 0.9em;
}

.doc-prose pre {
  overflow-x: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surfaceSubtle);
  padding: 12px 14px;
}

.doc-prose pre code {
  background: transparent;
  padding: 0;
  font-size: 0.85em;
}

.doc-prose blockquote {
  border-left: 3px solid var(--border);
  color: var(--textSecondary);
  padding-left: 12px;
}

.doc-prose ul,
.doc-prose ol {
  padding-left: 1.4em;
}

.doc-prose ul {
  list-style: disc;
}

.doc-prose ol {
  list-style: decimal;
}

.doc-prose hr {
  border: none;
  border-top: 1px solid var(--border);
  margin: 1.5em 0;
}

.doc-prose p.is-editor-empty:first-child::before {
  color: var(--textSecondary);
  content: attr(data-placeholder);
  float: left;
  height: 0;
  pointer-events: none;
}
```

在 `src/styles/globals.css` 的 `@import "./tokens.css";` 之后加一行：

```css
@import "./docsProse.css";
```

创建 `src/components/docs/docEditor.tsx`：

```tsx
import { EditorContent, useEditor, useEditorState } from "@tiptap/react";
import { BubbleMenu } from "@tiptap/react/menus";
import { Placeholder } from "@tiptap/extensions";
import { Markdown } from "@tiptap/markdown";
import StarterKit from "@tiptap/starter-kit";
import { Bold, Code, Italic, Link2, List, ListOrdered, Quote, Strikethrough } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/components/utils";

type Props = {
  documentId: string;
  initialMarkdown: string;
  onChange: (markdown: string) => void;
  ariaLabel?: string;
};

export function DocEditor({
  documentId,
  initialMarkdown,
  onChange,
  ariaLabel = "文档正文",
}: Props) {
  const editor = useEditor(
    {
      extensions: [
        StarterKit,
        Markdown,
        Placeholder.configure({ placeholder: "开始写点什么…" }),
      ],
      content: initialMarkdown,
      contentType: "markdown",
      editorProps: {
        attributes: {
          class: "doc-prose",
          "aria-label": ariaLabel,
        },
      },
      onUpdate: ({ editor: instance }) => {
        onChange(instance.getMarkdown());
      },
    },
    [documentId],
  );

  const state = useEditorState({
    editor,
    selector: ({ editor: instance }) => {
      if (!instance) {
        return null;
      }
      return {
        bold: instance.isActive("bold"),
        italic: instance.isActive("italic"),
        strike: instance.isActive("strike"),
        code: instance.isActive("code"),
        link: instance.isActive("link"),
        bullet: instance.isActive("bulletList"),
        ordered: instance.isActive("orderedList"),
        quote: instance.isActive("blockquote"),
      };
    },
  });

  if (!editor) {
    return null;
  }

  const toggleLink = () => {
    if (editor.isActive("link")) {
      editor.chain().focus().unsetLink().run();
      return;
    }
    const href = window.prompt("输入链接地址", "https://");
    if (!href) {
      return;
    }
    editor.chain().focus().extendMarkRange("link").setLink({ href }).run();
  };

  const items = [
    {
      label: "加粗",
      icon: Bold,
      active: state?.bold,
      run: () => editor.chain().focus().toggleBold().run(),
    },
    {
      label: "斜体",
      icon: Italic,
      active: state?.italic,
      run: () => editor.chain().focus().toggleItalic().run(),
    },
    {
      label: "删除线",
      icon: Strikethrough,
      active: state?.strike,
      run: () => editor.chain().focus().toggleStrike().run(),
    },
    {
      label: "行内代码",
      icon: Code,
      active: state?.code,
      run: () => editor.chain().focus().toggleCode().run(),
    },
    {
      label: "链接",
      icon: Link2,
      active: state?.link,
      run: toggleLink,
    },
    {
      label: "无序列表",
      icon: List,
      active: state?.bullet,
      run: () => editor.chain().focus().toggleBulletList().run(),
    },
    {
      label: "有序列表",
      icon: ListOrdered,
      active: state?.ordered,
      run: () => editor.chain().focus().toggleOrderedList().run(),
    },
    {
      label: "引用",
      icon: Quote,
      active: state?.quote,
      run: () => editor.chain().focus().toggleBlockquote().run(),
    },
  ];

  return (
    <div className="relative">
      <BubbleMenu
        editor={editor}
        options={{ placement: "top" }}
        shouldShow={({ from, to }) => from !== to}
      >
        <div className="flex items-center gap-0.5 rounded-[10px] border border-border bg-surface p-1 shadow-lg">
          {items.map((item) => (
            <Button
              key={item.label}
              type="button"
              variant="ghost"
              size="icon"
              aria-label={item.label}
              title={item.label}
              aria-pressed={item.active}
              onClick={item.run}
              className={cn(
                "size-8 text-textSecondary hover:text-textPrimary",
                item.active && "bg-brandSoft text-brand",
              )}
            >
              <item.icon aria-hidden="true" size={15} />
            </Button>
          ))}
        </div>
      </BubbleMenu>
      <EditorContent editor={editor} />
    </div>
  );
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `pnpm vitest run tests/e2e/unit/docs/docEditor.test.tsx`
Expected: PASS

- [ ] **Step 6: 类型检查**

Run: `pnpm typecheck`
Expected: 无错误

- [ ] **Step 7: 提交**

```bash
git add package.json pnpm-lock.yaml src/components/docs/docEditor.tsx src/styles/docsProse.css src/styles/globals.css tests/e2e/unit/docs/docEditor.test.tsx
git commit -m "feat: add tiptap markdown editor"
```

---

## Task 13: 布局、路由与空状态页

**Files:**
- Create: `src/layouts/docsWorkspaceLayout.tsx`
- Create: `src/pages/docs/index.tsx`
- Modify: `src/router/index.tsx:1-30`
- Modify: `src/components/shell/navRail.tsx:1-45`
- Test: `tests/e2e/unit/app/appRouter.test.tsx`

**Interfaces:**
- Consumes: `DocSidebar`（Task 14 创建，本任务先以占位导出保证编译；Task 14 覆盖为完整实现）
- Produces: 路由 `/docs` 与 `/docs/:nodeId`；导航项 `文档`

- [ ] **Step 1: 扩展路由测试**

在 `tests/e2e/unit/app/appRouter.test.tsx` 末尾追加：

```tsx
  it("opens the documents workspace with its tree sidebar", () => {
    const router = createMemoryRouter(routes, { initialEntries: ["/docs"] });
    render(<RouterProvider router={router} />);

    expect(screen.getByRole("complementary", { name: "文档目录" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "文档" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByText("选择或新建一篇文档")).toBeInTheDocument();
  });
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run tests/e2e/unit/app/appRouter.test.tsx`
Expected: FAIL，找不到「文档目录」

- [ ] **Step 3: 写布局与占位侧栏**

创建 `src/layouts/docsWorkspaceLayout.tsx`：

```tsx
import { Outlet } from "react-router-dom";
import { DocSidebar } from "@/components/docs/docSidebar";

export function DocsWorkspaceLayout() {
  return (
    <div className="flex h-full min-w-0">
      <DocSidebar />
      <main className="min-h-0 min-w-0 flex-1 overflow-hidden">
        <Outlet />
      </main>
    </div>
  );
}
```

创建 `src/components/docs/docSidebar.tsx`（Task 14 会整体替换）：

```tsx
export function DocSidebar() {
  return <aside aria-label="文档目录" className="w-[88px] shrink-0 border-r border-border bg-surfaceSubtle" />;
}
```

创建 `src/pages/docs/index.tsx`：

```tsx
import { FileText } from "lucide-react";

export default function DocsEmptyPage() {
  return (
    <section className="grid h-full place-items-center bg-surface px-6">
      <div className="text-center">
        <div className="mx-auto grid size-12 place-items-center rounded-xl bg-brandSoft text-brand">
          <FileText aria-hidden="true" size={20} />
        </div>
        <h1 className="mt-4 text-base font-semibold text-textPrimary">
          选择或新建一篇文档
        </h1>
        <p className="mt-2 text-sm text-textSecondary">
          在左侧目录中新建文件夹或文档，开始写作。
        </p>
      </div>
    </section>
  );
}
```

- [ ] **Step 4: 注册路由与导航**

修改 `src/router/index.tsx`：加入两个 import 与一条路由。

```tsx
import { DocsWorkspaceLayout } from "@/layouts/docsWorkspaceLayout";
import DocsEmptyPage from "@/pages/docs";
```

在 `/agent` 路由之后加入：

```tsx
      {
        path: "/docs",
        element: <DocsWorkspaceLayout />,
        children: [
          { index: true, element: <DocsEmptyPage /> },
          { path: ":nodeId", element: <DocsEmptyPage /> },
        ],
      },
```

修改 `src/components/shell/navRail.tsx`：从 `lucide-react` 引入 `FileText`，在「Agent」链接之前加入：

```tsx
        <NavLink
          to="/docs"
          aria-label="文档"
          className={({ isActive }) =>
            cn(
              "grid size-9 place-items-center rounded-md text-textSecondary transition-colors hover:bg-surfaceSubtle hover:text-textPrimary",
              isActive && "bg-brandSoft text-brand",
            )
          }
        >
          <FileText aria-hidden="true" size={18} />
        </NavLink>
```

- [ ] **Step 5: 运行测试确认通过**

Run: `pnpm vitest run tests/e2e/unit/app/appRouter.test.tsx`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add src/layouts/docsWorkspaceLayout.tsx src/components/docs/docSidebar.tsx src/pages/docs/index.tsx src/router/index.tsx src/components/shell/navRail.tsx tests/e2e/unit/app/appRouter.test.tsx
git commit -m "feat: add documents workspace route"
```

---

## Task 14: 侧栏、目录树与节点操作

**Files:**
- Modify: `src/components/docs/docSidebar.tsx`
- Create: `src/components/docs/docTree.tsx`
- Create: `src/components/docs/docTreeItem.tsx`
- Create: `src/components/docs/docNodeDialog.tsx`
- Test: `tests/e2e/unit/docs/docSidebar.test.tsx`

**Interfaces:**
- Consumes: `useDocsStore`、`useDocsEffect`、`Dialog` 系列、`Input`、`Button`
- Produces:
  - `DocSidebar()`：`aria-label="文档目录"`，内含搜索框 `aria-label="搜索文档"`
  - `DocTree({ nodes, onSelect })`
  - `buildTree(nodes)`：把扁平列表转为 `{ roots, childrenOf }` 结构（导出以便测试）

- [ ] **Step 1: 写失败测试**

创建 `tests/e2e/unit/docs/docSidebar.test.tsx`：

```tsx
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { DocSidebar, buildTree } from "@/components/docs/docSidebar";
import { initialDocsState, useDocsStore } from "@/controllers/stores/docsStore";

const folder = {
  id: "folder-1",
  parentId: null,
  kind: "folder" as const,
  title: "工作",
  sortOrder: 0,
  createdAt: "2026-11-06T00:00:00Z",
  updatedAt: "2026-11-06T00:00:00Z",
};

const document = {
  ...folder,
  id: "doc-1",
  kind: "document" as const,
  title: "接口设计",
  parentId: "folder-1",
};

beforeEach(() => {
  useDocsStore.setState(initialDocsState, true);
});

describe("buildTree", () => {
  it("groups children under their folder", () => {
    const tree = buildTree([folder, document]);

    expect(tree.roots).toHaveLength(1);
    expect(tree.roots[0].id).toBe("folder-1");
    expect(tree.childrenOf["folder-1"].map((node) => node.id)).toEqual([
      "doc-1",
    ]);
  });
});

describe("DocSidebar", () => {
  it("renders folders and their documents", async () => {
    useDocsStore.getState().setTree([folder, document]);

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("complementary", { name: "文档目录" })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText("工作")).toBeInTheDocument();
    });
    expect(screen.getByText("接口设计")).toBeInTheDocument();
  });

  it("filters the tree by the search query", async () => {
    useDocsStore.getState().setTree([folder, document]);
    const user = userEvent.setup();

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    await user.type(screen.getByLabelText("搜索文档"), "接口");

    await waitFor(() => {
      expect(screen.getByText("接口设计")).toBeInTheDocument();
      expect(screen.queryByText("工作")).not.toBeInTheDocument();
    });
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run tests/e2e/unit/docs/docSidebar.test.tsx`
Expected: FAIL，`buildTree` 未导出

- [ ] **Step 3: 实现侧栏与目录树**

创建 `src/components/docs/docTree.tsx`：

```tsx
import { ChevronRight, FileText, Folder, FolderOpen } from "lucide-react";
import { useState } from "react";
import type { DocNodeSummary } from "@/models/docContracts";
import { cn } from "@/components/utils";

export type DocTreeIndex = {
  roots: DocNodeSummary[];
  childrenOf: Record<string, DocNodeSummary[]>;
};

export const buildTree = (nodes: DocNodeSummary[]): DocTreeIndex => {
  const known = new Set(nodes.map((node) => node.id));
  const childrenOf: Record<string, DocNodeSummary[]> = {};
  const roots: DocNodeSummary[] = [];

  for (const node of nodes) {
    const parentId = node.parentId && known.has(node.parentId) ? node.parentId : null;
    if (!parentId) {
      roots.push(node);
      continue;
    }
    childrenOf[parentId] = [...(childrenOf[parentId] ?? []), node];
  }

  const sortNodes = (items: DocNodeSummary[]) =>
    [...items].sort((left, right) => {
      if (left.kind !== right.kind) {
        return left.kind === "folder" ? -1 : 1;
      }
      if (left.sortOrder !== right.sortOrder) {
        return left.sortOrder - right.sortOrder;
      }
      return right.updatedAt.localeCompare(left.updatedAt);
    });

  return {
    roots: sortNodes(roots),
    childrenOf: Object.fromEntries(
      Object.entries(childrenOf).map(([key, value]) => [key, sortNodes(value)]),
    ),
  };
};

type Props = {
  nodes: DocNodeSummary[];
  activeNodeId: string | null;
  onSelect: (node: DocNodeSummary) => void;
  onMenu: (node: DocNodeSummary) => void;
};

export function DocTree({ nodes, activeNodeId, onSelect, onMenu }: Props) {
  const { roots, childrenOf } = buildTree(nodes);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  const toggle = (id: string) =>
    setCollapsed((current) => ({ ...current, [id]: !current[id] }));

  const renderRow = (node: DocNodeSummary, depth: number) => {
    const isFolder = node.kind === "folder";
    const isCollapsed = Boolean(collapsed[node.id]);
    const children = childrenOf[node.id] ?? [];
    const isActive = node.id === activeNodeId;

    return (
      <li key={node.id}>
        <div
          className={cn(
            "group flex items-center gap-1 rounded-md pr-1",
            isActive ? "bg-brandSoft text-brand" : "text-textBody hover:bg-surface",
          )}
          style={{ paddingLeft: depth === 0 ? 4 : 18 }}
        >
          {isFolder ? (
            <button
              type="button"
              aria-label={isCollapsed ? `展开 ${node.title}` : `折叠 ${node.title}`}
              onClick={() => toggle(node.id)}
              className="grid size-5 shrink-0 place-items-center text-textSecondary"
            >
              <ChevronRight
                aria-hidden="true"
                size={13}
                className={cn("transition-transform", !isCollapsed && "rotate-90")}
              />
            </button>
          ) : (
            <span className="size-5 shrink-0" />
          )}
          <button
            type="button"
            onClick={() => onSelect(node)}
            className="flex min-w-0 flex-1 items-center gap-1.5 py-1.5 text-left text-sm"
          >
            {isFolder ? (
              isCollapsed ? (
                <Folder aria-hidden="true" size={14} className="shrink-0 opacity-70" />
              ) : (
                <FolderOpen aria-hidden="true" size={14} className="shrink-0 opacity-70" />
              )
            ) : (
              <FileText aria-hidden="true" size={14} className="shrink-0 opacity-70" />
            )}
            <span className="truncate">{node.title}</span>
          </button>
          <button
            type="button"
            aria-label={`操作 ${node.title}`}
            onClick={() => onMenu(node)}
            className="hidden shrink-0 rounded px-1 text-xs text-textSecondary group-hover:block hover:text-textPrimary"
          >
            ⋯
          </button>
        </div>
        {isFolder && !isCollapsed && children.length > 0 && (
          <ul>{children.map((child) => renderRow(child, depth + 1))}</ul>
        )}
      </li>
    );
  };

  return <ul className="space-y-0.5">{roots.map((root) => renderRow(root, 0))}</ul>;
}
```

创建 `src/components/docs/docNodeDialog.tsx`：

```tsx
import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import type { DocNodeSummary } from "@/models/docContracts";

export type DocDialogState =
  | { type: "none" }
  | { type: "rename"; node: DocNodeSummary }
  | { type: "move"; node: DocNodeSummary }
  | { type: "delete"; node: DocNodeSummary };

type Props = {
  state: DocDialogState;
  nodes: DocNodeSummary[];
  onClose: () => void;
  onRename: (id: string, title: string) => Promise<unknown>;
  onMove: (id: string, parentId: string | null) => Promise<unknown>;
  onDelete: (id: string) => Promise<unknown>;
};

export function DocNodeDialog({
  state,
  nodes,
  onClose,
  onRename,
  onMove,
  onDelete,
}: Props) {
  const [title, setTitle] = useState("");
  const [parentId, setParentId] = useState<string>("");
  const [pending, setPending] = useState(false);
  const open = state.type !== "none";

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setTitle("");
      setParentId("");
      onClose();
    }
  };

  const folders = nodes.filter(
    (node) => node.kind === "folder" && node.id !== (state.type === "none" ? "" : state.node.id),
  );

  const submit = async () => {
    if (state.type === "none") {
      return;
    }
    setPending(true);
    try {
      if (state.type === "rename") {
        await onRename(state.node.id, title.trim() || state.node.title);
      }
      if (state.type === "move") {
        await onMove(state.node.id, parentId || null);
      }
      if (state.type === "delete") {
        await onDelete(state.node.id);
      }
      setTitle("");
      setParentId("");
      onClose();
    } finally {
      setPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {state.type === "rename" && "重命名"}
            {state.type === "move" && "移动到"}
            {state.type === "delete" && "删除确认"}
          </DialogTitle>
          <DialogDescription>
            {state.type === "rename" && "修改节点标题。"}
            {state.type === "move" && "选择目标文件夹，留空表示移到根目录。"}
            {state.type === "delete" &&
              "删除后不可在界面恢复，文件夹内的内容会一并删除。"}
          </DialogDescription>
        </DialogHeader>

        {state.type === "rename" && (
          <Input
            aria-label="节点标题"
            autoFocus
            defaultValue={state.node.title}
            onChange={(event) => setTitle(event.target.value)}
            className="mt-4"
          />
        )}

        {state.type === "move" && (
          <select
            aria-label="目标文件夹"
            value={parentId}
            onChange={(event) => setParentId(event.target.value)}
            className="mt-4 h-9 w-full rounded-md border border-border bg-surface px-3 text-sm text-textPrimary"
          >
            <option value="">根目录</option>
            {folders.map((folder) => (
              <option key={folder.id} value={folder.id}>
                {folder.title}
              </option>
            ))}
          </select>
        )}

        <DialogFooter className="mt-4">
          <Button type="button" variant="ghost" onClick={() => handleOpenChange(false)}>
            取消
          </Button>
          <Button
            type="button"
            variant={state.type === "delete" ? "destructive" : "default"}
            disabled={pending}
            onClick={() => void submit()}
          >
            {pending ? "处理中" : state.type === "delete" ? "删除" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

创建 `src/components/docs/docSidebar.tsx`（整体替换 Task 13 的占位实现，并重新导出 `buildTree`）：

```tsx
import { FilePlus2, FolderPlus, Search } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { DocNodeDialog, type DocDialogState } from "@/components/docs/docNodeDialog";
import { DocTree, buildTree } from "@/components/docs/docTree";
import { useDocsEffect } from "@/controllers/effects/useDocsEffect";
import { useDocsStore } from "@/controllers/stores/docsStore";
import type { DocNodeSummary } from "@/models/docContracts";

export { buildTree };

export function DocSidebar() {
  const navigate = useNavigate();
  const { nodeId } = useParams();
  const effect = useDocsEffect();
  const nodes = useDocsStore((state) => state.nodes);
  const searchQuery = useDocsStore((state) => state.searchQuery);
  const searchResults = useDocsStore((state) => state.searchResults);
  const [dialog, setDialog] = useState<DocDialogState>({ type: "none" });

  useEffect(() => {
    void effect.loadTree();
  }, [effect]);

  const create = async (kind: "folder" | "document") => {
    const node = await effect.createNode(kind, null);
    if (node) {
      setSearchQueryAndTree(node);
      navigate(`/docs/${node.id}`);
    }
  };

  const setSearchQueryAndTree = (node: DocNodeSummary) => {
    void node;
    void effect.loadTree();
  };

  const filtering = searchQuery.trim().length > 0;

  return (
    <aside
      aria-label="文档目录"
      className="flex h-full w-[88px] shrink-0 flex-col border-r border-border bg-surfaceSubtle px-2 py-4 min-[461px]:w-[104px] min-[768px]:w-[220px] min-[768px]:px-3 min-[1280px]:w-[260px]"
    >
      <div className="relative hidden min-[768px]:block">
        <Search
          aria-hidden="true"
          size={14}
          className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-textSecondary"
        />
        <Input
          aria-label="搜索文档"
          value={searchQuery}
          onChange={(event) => effect.search(event.target.value)}
          placeholder="搜索文档"
          className="h-8 pl-8 text-sm"
        />
      </div>

      <div className="mt-3 flex items-center gap-1 min-[768px]:justify-start">
        <Button
          type="button"
          variant="ghost"
          aria-label="新建文档"
          title="新建文档"
          onClick={() => void create("document")}
          className="h-8 flex-1 justify-center gap-1.5 px-1 text-xs text-textPrimary hover:bg-surface min-[768px]:justify-start min-[768px]:px-2"
        >
          <FilePlus2 aria-hidden="true" size={14} />
          <span className="hidden min-[768px]:inline">新建文档</span>
        </Button>
        <Button
          type="button"
          variant="ghost"
          aria-label="新建文件夹"
          title="新建文件夹"
          onClick={() => void create("folder")}
          className="h-8 flex-1 justify-center gap-1.5 px-1 text-xs text-textPrimary hover:bg-surface min-[768px]:justify-start min-[768px]:px-2"
        >
          <FolderPlus aria-hidden="true" size={14} />
          <span className="hidden min-[768px]:inline">新建文件夹</span>
        </Button>
      </div>

      <nav aria-label="文档树" className="mt-4 min-h-0 flex-1 overflow-y-auto">
        {filtering ? (
          <ul className="space-y-0.5" aria-label="搜索结果">
            {searchResults.map((hit) => (
              <li key={hit.id}>
                <button
                  type="button"
                  onClick={() => navigate(`/docs/${hit.id}`)}
                  className="flex w-full flex-col rounded-md px-2 py-1.5 text-left hover:bg-surface"
                >
                  <span className="truncate text-sm text-textPrimary">{hit.title}</span>
                  {hit.path.length > 0 && (
                    <span className="truncate text-[11px] text-textSecondary">
                      {hit.path.join(" / ")}
                    </span>
                  )}
                </button>
              </li>
            ))}
            {searchResults.length === 0 && (
              <li className="px-2 py-2 text-xs text-textSecondary">没有匹配的文档</li>
            )}
          </ul>
        ) : (
          <DocTree
            nodes={nodes}
            activeNodeId={nodeId ?? null}
            onSelect={(node) => navigate(`/docs/${node.id}`)}
            onMenu={(node) => setDialog({ type: "rename", node })}
          />
        )}
      </nav>

      <DocNodeDialog
        state={dialog}
        nodes={nodes}
        onClose={() => setDialog({ type: "none" })}
        onRename={effect.renameNode}
        onMove={effect.moveNode}
        onDelete={effect.deleteNode}
      />
    </aside>
  );
}
```

`DocTreeItem` 在实现中并入 `DocTree` 的行渲染，因此不单独创建文件；如后续需要独立行组件再拆分。

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm vitest run tests/e2e/unit/docs/docSidebar.test.tsx`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add src/components/docs/docSidebar.tsx src/components/docs/docTree.tsx src/components/docs/docNodeDialog.tsx tests/e2e/unit/docs/docSidebar.test.tsx
git commit -m "feat: add document tree sidebar"
```

---

## Task 15: 文档页、标题保存与导入导出

**Files:**
- Create: `src/components/docs/docTitleInput.tsx`
- Create: `src/components/docs/docFileTransfer.ts`
- Create: `src/pages/docs/docPage.tsx`
- Modify: `src/router/index.tsx:20`
- Test: `tests/e2e/unit/docs/docFileTransfer.test.ts`

**Interfaces:**
- Consumes: `DocEditor`、`useDocsEffect`、`useDocsStore`、`docsModel`
- Produces:
  - `documentTitleFromFilename(name: string): string`
  - `downloadMarkdown(title: string, content: string): void`
  - `DocTitleInput({ value, status, onChange })`
  - `DocPage()` 挂载在 `/docs/:nodeId`

- [ ] **Step 1: 写失败测试**

创建 `tests/e2e/unit/docs/docFileTransfer.test.ts`：

```ts
import { describe, expect, it } from "vitest";
import {
  documentTitleFromFilename,
  sanitizeFileName,
} from "@/components/docs/docFileTransfer";

describe("docFileTransfer", () => {
  it("derives a title from a markdown filename", () => {
    expect(documentTitleFromFilename("接口设计.md")).toBe("接口设计");
    expect(documentTitleFromFilename("notes.markdown")).toBe("notes");
    expect(documentTitleFromFilename("无扩展名")).toBe("无扩展名");
  });

  it("replaces characters that are illegal in filenames", () => {
    expect(sanitizeFileName('a/b\\c:d*e?f"g<h>i|j')).toBe("a-b-c-d-e-f-g-h-i-j");
  });

  it("falls back to a default title for empty names", () => {
    expect(documentTitleFromFilename(".md")).toBe("无标题文档");
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run tests/e2e/unit/docs/docFileTransfer.test.ts`
Expected: FAIL，无法解析 `@/components/docs/docFileTransfer`

- [ ] **Step 3: 实现文件处理与标题组件**

创建 `src/components/docs/docFileTransfer.ts`：

```ts
const DEFAULT_TITLE = "无标题文档";

export function documentTitleFromFilename(name: string): string {
  const trimmed = name.trim();
  const withoutExtension = trimmed.replace(/\.(md|markdown|txt)$/i, "");
  const title = withoutExtension.trim();
  return title || DEFAULT_TITLE;
}

export function sanitizeFileName(value: string): string {
  return value.replace(/[\\/:*?"<>|]/g, "-").trim() || DEFAULT_TITLE;
}

export function downloadMarkdown(title: string, content: string): void {
  const blob = new Blob([content], { type: "text/markdown;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `${sanitizeFileName(title)}.md`;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

export function readMarkdownFile(file: File): Promise<string> {
  return file.text();
}
```

创建 `src/components/docs/docTitleInput.tsx`：

```tsx
import type { SaveStatus } from "@/controllers/stores/docsStore";

type Props = {
  value: string;
  status: SaveStatus;
  onChange: (value: string) => void;
};

const statusText: Record<SaveStatus, string> = {
  idle: "",
  saving: "保存中…",
  saved: "已保存",
  error: "保存失败，点击重试",
};

export function DocTitleInput({ value, status, onChange }: Props) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <input
        aria-label="文档标题"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder="无标题文档"
        className="min-w-0 flex-1 border-none bg-transparent text-2xl font-semibold text-textPrimary outline-none placeholder:text-textSecondary"
      />
      <span
        role="status"
        className={
          status === "error"
            ? "text-xs text-destructive"
            : "text-xs text-textSecondary"
        }
      >
        {statusText[status]}
      </span>
    </div>
  );
}
```

- [ ] **Step 4: 实现文档页**

创建 `src/pages/docs/docPage.tsx`：

```tsx
import { Download, Upload } from "lucide-react";
import { useEffect, useRef } from "react";
import { useParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { DocEditor } from "@/components/docs/docEditor";
import {
  documentTitleFromFilename,
  downloadMarkdown,
  readMarkdownFile,
} from "@/components/docs/docFileTransfer";
import { DocTitleInput } from "@/components/docs/docTitleInput";
import { useDocsEffect } from "@/controllers/effects/useDocsEffect";
import { useDocsStore } from "@/controllers/stores/docsStore";

export default function DocPage() {
  const { nodeId } = useParams();
  const effect = useDocsEffect();
  const fileInput = useRef<HTMLInputElement>(null);

  const activeNode = useDocsStore((state) => state.activeNode);
  const draftTitle = useDocsStore((state) => state.draftTitle);
  const draftContent = useDocsStore((state) => state.draftContent);
  const saveStatus = useDocsStore((state) => state.saveStatus);
  const phase = useDocsStore((state) => state.phase);
  const errorMessage = useDocsStore((state) => state.errorMessage);

  useEffect(() => {
    if (nodeId) {
      void effect.openNode(nodeId);
    }
  }, [effect, nodeId]);

  useEffect(() => {
    const flush = () => {
      void effect.flushSave();
    };
    window.addEventListener("beforeunload", flush);
    return () => {
      window.removeEventListener("beforeunload", flush);
      flush();
    };
  }, [effect]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
        event.preventDefault();
        void effect.flushSave();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [effect]);

  const importFile = async (file: File) => {
    const content = await readMarkdownFile(file);
    const node = await effect.createNode("document", activeNode?.parentId ?? null);
    if (!node) {
      return;
    }
    await effect.renameNode(node.id, documentTitleFromFilename(file.name));
    effect.applyContentChange(node.id, content);
    await effect.flushSave();
  };

  if (phase === "loading" && !activeNode) {
    return (
      <div className="mx-auto w-full max-w-3xl px-6 py-10">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="mt-6 h-4 w-full" />
        <Skeleton className="mt-3 h-4 w-5/6" />
      </div>
    );
  }

  if (errorMessage && !activeNode) {
    return (
      <div className="mx-auto w-full max-w-3xl px-6 py-10">
        <p className="text-sm text-textPrimary">{errorMessage}</p>
        <Button
          className="mt-4"
          variant="outline"
          onClick={() => nodeId && void effect.openNode(nodeId)}
        >
          重试
        </Button>
      </div>
    );
  }

  if (!activeNode) {
    return null;
  }

  return (
    <section className="flex h-full min-w-0 flex-col bg-surface">
      <header className="flex flex-wrap items-center gap-2 border-b border-border px-6 py-4">
        <div className="min-w-0 flex-1">
          <DocTitleInput
            value={draftTitle}
            status={saveStatus}
            onChange={(title) => effect.applyTitleChange(activeNode.id, title)}
          />
        </div>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => fileInput.current?.click()}
          >
            <Upload aria-hidden="true" size={14} />
            导入
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => downloadMarkdown(draftTitle, draftContent)}
          >
            <Download aria-hidden="true" size={14} />
            导出
          </Button>
        </div>
        <input
          ref={fileInput}
          type="file"
          accept=".md,.markdown,.txt,text/markdown"
          aria-label="导入 Markdown"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (file) {
              void importFile(file);
            }
          }}
        />
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-6">
        <div className="mx-auto w-full max-w-3xl">
          <DocEditor
            documentId={activeNode.id}
            initialMarkdown={activeNode.content}
            onChange={(markdown) =>
              effect.applyContentChange(activeNode.id, markdown)
            }
          />
        </div>
      </div>
    </section>
  );
}
```

在 `src/router/index.tsx` 中把 `{ path: ":nodeId", element: <DocsEmptyPage /> }` 改为：

```tsx
          { path: ":nodeId", element: <DocPage /> },
```

并加入 `import DocPage from "@/pages/docs/docPage";`。

- [ ] **Step 5: 运行测试确认通过**

Run: `pnpm vitest run tests/e2e/unit/docs/docFileTransfer.test.ts`
Expected: PASS

- [ ] **Step 6: 类型检查与全量前端测试**

Run: `pnpm typecheck ; pnpm test`
Expected: 均通过

- [ ] **Step 7: 提交**

```bash
git add src/components/docs/docTitleInput.tsx src/components/docs/docFileTransfer.ts src/pages/docs/docPage.tsx src/router/index.tsx tests/e2e/unit/docs/docFileTransfer.test.ts
git commit -m "feat: add document page with autosave and markdown transfer"
```

---

## Task 16: 端到端验收

**Files:**
- Create: `tests/e2e/modules/docs/docs.e2e.ts`

**Interfaces:**
- Consumes: 运行中的 `pnpm dev`（Web 5173 + API 3001）
- Produces: 无

- [ ] **Step 1: 写 E2E 用例**

创建 `tests/e2e/modules/docs/docs.e2e.ts`：

```ts
import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.goto("/#/docs");
}); 

test("creates a document, edits it, and keeps content after reload", async ({
  page,
}) => {
  await page.getByRole("button", { name: "新建文档" }).click();
  await expect(page.getByLabel("文档标题")).toBeVisible();

  await page.getByLabel("文档标题").fill("接口设计");
  await page.getByLabel("文档正文").click();
  await page.keyboard.type("# 接口设计");
  await page.keyboard.press("Enter");
  await page.keyboard.type("自动保存验证");

  await expect(page.getByRole("status")).toHaveText("已保存");

  await page.reload();
  await expect(page.getByLabel("文档标题")).toHaveValue("接口设计");
  await expect(page.getByRole("heading", { name: "接口设计" })).toBeVisible();
  await expect(page.getByText("自动保存验证")).toBeVisible();
});

test("groups a document into a folder and searches it", async ({ page }) => {
  await page.getByRole("button", { name: "新建文件夹" }).click();
  await expect(page.getByText("未命名文件夹")).toBeVisible();

  await page.getByRole("button", { name: "操作 未命名文件夹" }).click({ force: true });
  await page.getByLabel("节点标题").fill("工作");
  await page.getByRole("button", { name: "保存" }).click();

  await expect(page.getByText("工作")).toBeVisible();

  await page.getByRole("button", { name: "新建文档" }).click();
  await page.getByLabel("文档标题").fill("接口说明");

  await page.getByLabel("搜索文档").fill("接口");
  await expect(page.getByRole("list", { name: "搜索结果" })).toBeVisible();
  await expect(page.getByText("接口说明")).toBeVisible();
});
```

- [ ] **Step 2: 启动服务并运行 E2E**

Run: `pnpm dev`
Expected: Web 5173 与 API 3001 均启动，无报错

在另一个终端运行：

Run: `pnpm test:e2e tests/e2e/modules/docs/docs.e2e.ts`
Expected: 两个用例 PASS

- [ ] **Step 3: 运行全量质量门禁**

Run: `pnpm typecheck ; pnpm test ; pnpm test:go`
Expected: 全部 PASS

- [ ] **Step 4: 提交**

```bash
git add tests/e2e/modules/docs/docs.e2e.ts
git commit -m "test: cover documents workspace end to end"
```

---

## Task 17: 文档与索引更新

**Files:**
- Modify: `docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md:5`
- Modify: `README.md`

**Interfaces:**
- Consumes: 全部实现结果
- Produces: 无

- [ ] **Step 1: 更新规范状态**

把设计规范的 `- **Status:** Draft for review` 改为：

```markdown
- **Status:** Implemented and verified on 2026-11-06
```

- [ ] **Step 2: 在 README 记录工作区入口**

在 `README.md` 的一级导航说明处补充一行（若已有导航列表，追加到同一列表末尾）：

```markdown
- 文档工作区：`/docs` 提供两级目录树、Markdown 编辑、自动保存与搜索，数据保存在本地 SQLite。
```

- [ ] **Step 3: 校验文档链接**

Run: `git --no-pager diff --check`
Expected: 无输出

- [ ] **Step 4: 提交**

```bash
git add docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md README.md
git commit -m "docs: record documents workspace implementation"
```

---

## 自审结论

**规范覆盖**

| 规范章节 | 对应任务 |
|---|---|
| 5 数据模型 | Task 1 |
| 7 API 契约（含 7.4 搜索） | Task 2、3、4、7、8 |
| 6 后端模块结构 | Task 2、4、5、6、7 |
| 8 前端 MVC 结构 | Task 9、10、11 |
| 9 编辑器选型 | Task 12 |
| 10 自动保存 | Task 11、15 |
| 11 导入导出 | Task 15 |
| 12 搜索交互 | Task 11、14 |
| 13 错误处理 | Task 7、11、15 |
| 14 测试与验收 | Task 5、6、8、10、11、15、16 |
| 15 安全与隐私 | Task 7（不记录正文）、Task 9（参数绑定由 store 实现） |
| 16 文档治理 | Task 17 |
| 4 界面结构 | Task 13、14、15 |

**已知需要在实施中遵守的约定**

1. `docs.SearchInput` 包含 `QueryHash string`，由 `Service.Search` 用原始查询词填充；`store` 只负责把它写进游标。
2. `service_test.go` 复用 `search_test.go` 的 `stringPointer`，不要重复声明。
3. `DocTree` 的行渲染已包含操作入口，不再单独创建 `docTreeItem.tsx`。
4. `DocSidebar` 中的 `setSearchQueryAndTree` 仅用于刷新树，实施时内联为 `void effect.loadTree()`。
