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
	// ErrContentTooLong 表示文档正文超过 MaxContentRunes。
	ErrContentTooLong = errors.New("doc content too long")
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
