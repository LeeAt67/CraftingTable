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

// DecodeCursor 解析游标并校验其与查询词一致。空串不是合法游标，
// 首屏查询由调用方跳过解码直接以 nil 游标发起。
func DecodeCursor(raw string, query string) (*SearchCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrInvalidCursor
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
