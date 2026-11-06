// Package tools adapts CraftingTable domain reads into read-only Agent tools.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/leeat/craftingtable/server/internal/agent/core"
	"github.com/leeat/craftingtable/server/internal/today"
)

const (
	todayToolVersion  = "1.0.0"
	todayToolTimeout  = 5 * time.Second
	maxTodayInput     = 16 * 1024
	maxTodayResult    = 256 * 1024
	defaultSearchSize = 20
	maxSearchSize     = 50
)

// TodayReader is the read-only Today port exposed to Agent tools.
type TodayReader interface {
	ListToday(context.Context, string, string) ([]today.Item, error)
	GetItem(context.Context, string) (today.Item, error)
	ListCategories(context.Context) ([]today.Category, error)
}

// NewTodayTools builds the three approved read-only Today tools.
func NewTodayTools(
	reader TodayReader,
	now func() time.Time,
) ([]core.Tool, error) {
	if reader == nil {
		return nil, errors.New("today reader is required")
	}
	if now == nil {
		now = time.Now
	}
	return []core.Tool{
		{
			Definition: core.ToolDefinition{
				Name:        "today_search",
				Namespace:   "today",
				Version:     todayToolVersion,
				Title:       "Search today items",
				Description: "Search today's tasks and events by date, text, status, and category.",
				InputSchema: json.RawMessage(`{
					"type":"object",
					"properties":{
						"date":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"},
						"timezone":{"type":"string","minLength":1},
						"query":{"type":"string"},
						"status":{"type":"string","enum":["unplanned","planned","done"]},
						"categoryId":{"type":"string","minLength":1},
						"limit":{"type":"integer","minimum":1,"maximum":50}
					},
					"additionalProperties":false
				}`),
				ReadOnly:       true,
				Idempotent:     true,
				Timeout:        todayToolTimeout,
				MaxInputBytes:  maxTodayInput,
				MaxResultBytes: maxTodayResult,
			},
			Handler: func(
				ctx context.Context,
				arguments json.RawMessage,
			) (core.ToolResult, error) {
				return executeTodaySearch(ctx, reader, now(), arguments)
			},
		},
		{
			Definition: core.ToolDefinition{
				Name:        "today_get_item",
				Namespace:   "today",
				Version:     todayToolVersion,
				Title:       "Get a today item",
				Description: "Get one task or event by ID, including its full description.",
				InputSchema: json.RawMessage(`{
					"type":"object",
					"properties":{
						"id":{"type":"string","minLength":1}
					},
					"required":["id"],
					"additionalProperties":false
				}`),
				ReadOnly:       true,
				Idempotent:     true,
				Timeout:        todayToolTimeout,
				MaxInputBytes:  maxTodayInput,
				MaxResultBytes: maxTodayResult,
			},
			Handler: func(
				ctx context.Context,
				arguments json.RawMessage,
			) (core.ToolResult, error) {
				return executeTodayGetItem(ctx, reader, arguments)
			},
		},
		{
			Definition: core.ToolDefinition{
				Name:        "today_summary",
				Namespace:   "today",
				Version:     todayToolVersion,
				Title:       "Summarize a day",
				Description: "Summarize today's status totals, completion rate, schedule, and categories.",
				InputSchema: json.RawMessage(`{
					"type":"object",
					"properties":{
						"date":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"},
						"timezone":{"type":"string","minLength":1}
					},
					"additionalProperties":false
				}`),
				ReadOnly:       true,
				Idempotent:     true,
				Timeout:        todayToolTimeout,
				MaxInputBytes:  maxTodayInput,
				MaxResultBytes: maxTodayResult,
			},
			Handler: func(
				ctx context.Context,
				arguments json.RawMessage,
			) (core.ToolResult, error) {
				return executeTodaySummary(ctx, reader, now(), arguments)
			},
		},
	}, nil
}

type searchArguments struct {
	Date       string           `json:"date"`
	Timezone   string           `json:"timezone"`
	Query      string           `json:"query"`
	Status     today.ItemStatus `json:"status"`
	CategoryID string           `json:"categoryId"`
	Limit      int              `json:"limit"`
}

type summaryArguments struct {
	Date     string `json:"date"`
	Timezone string `json:"timezone"`
}

type itemArguments struct {
	ID string `json:"id"`
}

func executeTodaySearch(
	ctx context.Context,
	reader TodayReader,
	now time.Time,
	arguments json.RawMessage,
) (core.ToolResult, error) {
	var input searchArguments
	if err := json.Unmarshal(arguments, &input); err != nil {
		return core.ToolResult{}, invalidArgumentsError()
	}
	dayRange, message := today.ResolveDayRange(input.Date, input.Timezone, now)
	if message != "" {
		return core.ToolResult{}, validationError(message)
	}
	items, err := reader.ListToday(ctx, dayRange.Start, dayRange.End)
	if err != nil {
		return core.ToolResult{}, internalError()
	}
	return jsonToolResult(today.SearchItems(items, today.SearchInput{
		Query:      input.Query,
		Status:     input.Status,
		CategoryID: input.CategoryID,
		Limit:      input.Limit,
	})), nil
}

func executeTodayGetItem(
	ctx context.Context,
	reader TodayReader,
	arguments json.RawMessage,
) (core.ToolResult, error) {
	var input itemArguments
	if err := json.Unmarshal(arguments, &input); err != nil {
		return core.ToolResult{}, invalidArgumentsError()
	}
	item, err := reader.GetItem(ctx, input.ID)
	if errors.Is(err, today.ErrItemNotFound) {
		return core.ToolResult{}, core.ToolError{
			Code:      "TOOL_ITEM_NOT_FOUND",
			Kind:      "not_found",
			Message:   "事务条目不存在",
			Retryable: false,
		}
	}
	if err != nil {
		return core.ToolResult{}, internalError()
	}
	return jsonToolResult(item), nil
}

func executeTodaySummary(
	ctx context.Context,
	reader TodayReader,
	now time.Time,
	arguments json.RawMessage,
) (core.ToolResult, error) {
	var input summaryArguments
	if err := json.Unmarshal(arguments, &input); err != nil {
		return core.ToolResult{}, invalidArgumentsError()
	}
	dayRange, message := today.ResolveDayRange(input.Date, input.Timezone, now)
	if message != "" {
		return core.ToolResult{}, validationError(message)
	}
	items, err := reader.ListToday(ctx, dayRange.Start, dayRange.End)
	if err != nil {
		return core.ToolResult{}, internalError()
	}
	categories, err := reader.ListCategories(ctx)
	if err != nil {
		return core.ToolResult{}, internalError()
	}
	return jsonToolResult(today.SummarizeItems(items, categories)), nil
}

func jsonToolResult(value any) core.ToolResult {
	return core.ToolResult{
		Content: []core.ToolResultContent{{
			Type: "json",
			JSON: value,
		}},
	}
}

func invalidArgumentsError() core.ToolError {
	return validationError("工具参数无效")
}

func validationError(message string) core.ToolError {
	return core.ToolError{
		Code:      "TOOL_ARGUMENTS_INVALID",
		Kind:      "validation",
		Message:   message,
		Retryable: false,
	}
}

func internalError() core.ToolError {
	return core.ToolError{
		Code:      "TOOL_INTERNAL_ERROR",
		Kind:      "internal",
		Message:   "工具执行失败",
		Retryable: false,
	}
}
