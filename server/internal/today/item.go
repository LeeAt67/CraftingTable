// Package today 实现今日事务与分类领域。
package today

import (
	"context"
	"errors"
	"strings"
)

// ItemKind 是事务条目的类型。
type ItemKind string

const (
	// ItemKindTask 表示待办任务。
	ItemKindTask ItemKind = "task"
	// ItemKindEvent 表示日程事件。
	ItemKindEvent ItemKind = "event"
)

// ItemStatus 是事务条目的生命周期状态。
type ItemStatus string

const (
	// ItemStatusUnplanned 表示尚未安排时间。
	ItemStatusUnplanned ItemStatus = "unplanned"
	// ItemStatusPlanned 表示已经安排时间。
	ItemStatusPlanned ItemStatus = "planned"
	// ItemStatusDone 表示已经完成。
	ItemStatusDone ItemStatus = "done"
)

var (
	// ErrItemNotFound 表示指定事务条目不存在。
	ErrItemNotFound = errors.New("item not found")
	// ErrInvalidItemInput 表示事务条目输入不满足领域约束。
	ErrInvalidItemInput = errors.New("invalid item input")
)

// Item 是工作台展示的事务条目。
type Item struct {
	ID          string     `json:"id"`
	Kind        ItemKind   `json:"kind"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Status      ItemStatus `json:"status"`
	CategoryID  *string    `json:"categoryId,omitempty"`
	StartAt     *string    `json:"startAt,omitempty"`
	EndAt       *string    `json:"endAt,omitempty"`
	CompletedAt *string    `json:"completedAt,omitempty"`
	CreatedAt   string     `json:"createdAt"`
	UpdatedAt   string     `json:"updatedAt"`
}

// Category 是事务条目的分类。
type Category struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// CreateItemInput 描述创建事务条目的允许字段。
type CreateItemInput struct {
	Title      string
	CategoryID *string
	StartAt    *string
	EndAt      *string
}

// UpdateItemInput 描述更新事务条目的允许字段。
type UpdateItemInput struct {
	ID          string
	Title       *string
	Description *string
	CategoryID  *string
	StartAt     *string
	EndAt       *string
}

// Repository 是今日模块拥有的持久化端口。
type Repository interface {
	ListToday(ctx context.Context, startAt, endAt string) ([]Item, error)
	ListCategories(ctx context.Context) ([]Category, error)
	CreateItem(ctx context.Context, input CreateItemInput) (Item, error)
	UpdateItem(ctx context.Context, input UpdateItemInput) (Item, error)
	CompleteItem(ctx context.Context, id string) (Item, error)
	UncompleteItem(ctx context.Context, id string) (Item, error)
}

// StatusForStart 根据开始时间决定新建任务状态。
func StatusForStart(startAt *string) ItemStatus {
	if startAt != nil && strings.TrimSpace(*startAt) != "" {
		return ItemStatusPlanned
	}
	return ItemStatusUnplanned
}

// RestoredStatus 根据是否存在开始时间决定取消完成后的状态。
func RestoredStatus(startAt *string) ItemStatus {
	return StatusForStart(startAt)
}
