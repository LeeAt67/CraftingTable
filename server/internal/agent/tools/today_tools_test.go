package tools

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeat/craftingtable/server/internal/agent/core"
	"github.com/leeat/craftingtable/server/internal/today"
)

type todayReaderStub struct {
	items         []today.Item
	categories    []today.Category
	item          today.Item
	itemErr       error
	listCalls     atomic.Int32
	listStart     string
	listEnd       string
	categoryCalls atomic.Int32
	getItemCalls  atomic.Int32
}

func (r *todayReaderStub) ListToday(
	_ context.Context,
	startAt string,
	endAt string,
) ([]today.Item, error) {
	r.listCalls.Add(1)
	r.listStart = startAt
	r.listEnd = endAt
	return r.items, nil
}

func (r *todayReaderStub) GetItem(context.Context, string) (today.Item, error) {
	r.getItemCalls.Add(1)
	return r.item, r.itemErr
}

func (r *todayReaderStub) ListCategories(
	context.Context,
) ([]today.Category, error) {
	r.categoryCalls.Add(1)
	return r.categories, nil
}

func TestTodayToolsResolveDateAndTimezoneOnceForSearch(t *testing.T) {
	t.Parallel()

	reader := &todayReaderStub{
		items: []today.Item{{
			ID:     "item-1",
			Title:  "Daily report",
			Status: today.ItemStatusPlanned,
		}},
	}
	var nowCalls atomic.Int32
	now := func() time.Time {
		nowCalls.Add(1)
		return time.Date(
			2026,
			time.November,
			6,
			16,
			30,
			0,
			0,
			time.UTC,
		)
	}
	tools, err := NewTodayTools(reader, now)
	if err != nil {
		t.Fatalf("NewTodayTools() error = %v", err)
	}
	search := toolByName(t, tools, "today_search")
	_, err = search.Handler(
		context.Background(),
		json.RawMessage(`{"date":"2026-11-06","timezone":"Asia/Shanghai"}`),
	)
	if err != nil {
		t.Fatalf("today_search error = %v", err)
	}
	if nowCalls.Load() != 1 {
		t.Fatalf("clock calls = %d, want 1", nowCalls.Load())
	}
	if reader.listCalls.Load() != 1 {
		t.Fatalf("ListToday calls = %d, want 1", reader.listCalls.Load())
	}
	if reader.listStart != "2026-11-05T16:00:00.000Z" ||
		reader.listEnd != "2026-11-06T16:00:00.000Z" {
		t.Fatalf(
			"resolved day = %s..%s",
			reader.listStart,
			reader.listEnd,
		)
	}
}

func TestTodayGetItemMapsDeletedItemToNotFound(t *testing.T) {
	t.Parallel()

	reader := &todayReaderStub{itemErr: today.ErrItemNotFound}
	tools, err := NewTodayTools(reader, nil)
	if err != nil {
		t.Fatalf("NewTodayTools() error = %v", err)
	}
	getItem := toolByName(t, tools, "today_get_item")
	_, err = getItem.Handler(
		context.Background(),
		json.RawMessage(`{"id":"deleted"}`),
	)
	var toolError core.ToolError
	if !errors.As(err, &toolError) {
		t.Fatalf("today_get_item error = %v, want ToolError", err)
	}
	if toolError.Kind != "not_found" {
		t.Fatalf("tool error = %#v, want not_found", toolError)
	}
}

func TestTodaySummaryReturnsStableJSONContent(t *testing.T) {
	t.Parallel()

	reader := &todayReaderStub{
		items: []today.Item{
			{ID: "done", Status: today.ItemStatusDone},
			{ID: "planned", Status: today.ItemStatusPlanned},
		},
	}
	tools, err := NewTodayTools(reader, nil)
	if err != nil {
		t.Fatalf("NewTodayTools() error = %v", err)
	}
	summary := toolByName(t, tools, "today_summary")
	result, err := summary.Handler(
		context.Background(),
		json.RawMessage(`{"date":"2026-11-06","timezone":"Asia/Shanghai"}`),
	)
	if err != nil {
		t.Fatalf("today_summary error = %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "json" {
		t.Fatalf("tool result = %#v", result)
	}
	body, err := json.Marshal(result.Content[0].JSON)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	var summaryBody struct {
		Total int `json:"total"`
		Done  int `json:"done"`
	}
	if err := json.Unmarshal(body, &summaryBody); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summaryBody.Total != 2 || summaryBody.Done != 1 {
		t.Fatalf("summary = %#v", summaryBody)
	}
}

func TestTodayToolsHaveStableDefinitions(t *testing.T) {
	t.Parallel()

	tools, err := NewTodayTools(&todayReaderStub{}, nil)
	if err != nil {
		t.Fatalf("NewTodayTools() error = %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("tools length = %d, want 3", len(tools))
	}
	for _, tool := range tools {
		if tool.Definition.Version != "1.0.0" ||
			tool.Definition.Namespace != "today" ||
			!tool.Definition.ReadOnly ||
			!tool.Definition.Idempotent {
			t.Fatalf("tool definition = %#v", tool.Definition)
		}
	}
}

func toolByName(t *testing.T, tools []core.Tool, name string) core.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Definition.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return core.Tool{}
}
