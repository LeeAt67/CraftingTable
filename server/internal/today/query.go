package today

import (
	"strings"
	"time"
)

// SearchInput describes the Agent-facing Today search filters.
type SearchInput struct {
	Query      string
	Status     ItemStatus
	CategoryID string
	Limit      int
}

// SearchResult is the reduced item shape returned to the Agent.
type SearchResult struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Status      ItemStatus `json:"status"`
	CategoryID  *string    `json:"categoryId,omitempty"`
	StartAt     *string    `json:"startAt,omitempty"`
	EndAt       *string    `json:"endAt,omitempty"`
	CompletedAt *string    `json:"completedAt,omitempty"`
}

// CategorySummary aggregates items in one category.
type CategorySummary struct {
	CategoryID string `json:"categoryId"`
	Name       string `json:"name"`
	Total      int    `json:"total"`
	Done       int    `json:"done"`
}

// Summary is the Agent-facing Today aggregate.
type Summary struct {
	Total            int               `json:"total"`
	Done             int               `json:"done"`
	Planned          int               `json:"planned"`
	Unplanned        int               `json:"unplanned"`
	CompletionRate   float64           `json:"completionRate"`
	ScheduledSeconds int64             `json:"scheduledSeconds"`
	Categories       []CategorySummary `json:"categories"`
}

// SearchItems applies the approved filters and reduces result size.
func SearchItems(items []Item, input SearchInput) []SearchResult {
	query := strings.ToLower(strings.TrimSpace(input.Query))
	status := input.Status
	categoryID := strings.TrimSpace(input.CategoryID)
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	results := make([]SearchResult, 0, min(limit, len(items)))
	for _, item := range items {
		if status != "" && item.Status != status {
			continue
		}
		if categoryID != "" &&
			(item.CategoryID == nil || *item.CategoryID != categoryID) {
			continue
		}
		if query != "" && !searchTextContains(item, query) {
			continue
		}
		results = append(results, SearchResult{
			ID:          item.ID,
			Title:       item.Title,
			Status:      item.Status,
			CategoryID:  cloneStringPointer(item.CategoryID),
			StartAt:     cloneStringPointer(item.StartAt),
			EndAt:       cloneStringPointer(item.EndAt),
			CompletedAt: cloneStringPointer(item.CompletedAt),
		})
		if len(results) == limit {
			break
		}
	}
	return results
}

func searchTextContains(item Item, query string) bool {
	if strings.Contains(strings.ToLower(item.Title), query) {
		return true
	}
	return item.Description != nil &&
		strings.Contains(strings.ToLower(*item.Description), query)
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// SummarizeItems calculates status, schedule, and category aggregates.
func SummarizeItems(items []Item, categories []Category) Summary {
	summary := Summary{
		Total:      len(items),
		Categories: make([]CategorySummary, len(categories)),
	}
	categoryIndex := make(map[string]int, len(categories))
	for index, category := range categories {
		summary.Categories[index] = CategorySummary{
			CategoryID: category.ID,
			Name:       category.Name,
		}
		categoryIndex[category.ID] = index
	}

	for _, item := range items {
		switch item.Status {
		case ItemStatusDone:
			summary.Done++
		case ItemStatusPlanned:
			summary.Planned++
		case ItemStatusUnplanned:
			summary.Unplanned++
		}
		if item.CategoryID != nil {
			if index, exists := categoryIndex[*item.CategoryID]; exists {
				summary.Categories[index].Total++
				if item.Status == ItemStatusDone {
					summary.Categories[index].Done++
				}
			}
		}
		summary.ScheduledSeconds += scheduledSeconds(item)
	}
	if summary.Total > 0 {
		summary.CompletionRate = float64(summary.Done) / float64(summary.Total)
	}
	return summary
}

func scheduledSeconds(item Item) int64 {
	if item.StartAt == nil || item.EndAt == nil {
		return 0
	}
	start, startErr := time.Parse(time.RFC3339, *item.StartAt)
	end, endErr := time.Parse(time.RFC3339, *item.EndAt)
	if startErr != nil || endErr != nil || !end.After(start) {
		return 0
	}
	return int64(end.Sub(start) / time.Second)
}
