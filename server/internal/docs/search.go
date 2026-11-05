// 本文件在 Task 3 中补全游标编解码与入参清洗。

package docs

const (
	// DefaultSearchLimit 是搜索默认返回条数。
	DefaultSearchLimit = 20
	// MaxSearchLimit 是搜索允许的最大返回条数。
	MaxSearchLimit = 50
	// MaxQueryRunes 是搜索关键词最大字符数。
	MaxQueryRunes = 100
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
