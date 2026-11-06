package today

import "testing"

func TestSearchItemsFiltersTitleDescriptionAndLimit(t *testing.T) {
	t.Parallel()

	work := "work"
	life := "life"
	agentDescription := "Research the AGENT runtime"
	items := []Item{
		{
			ID:          "planned-report",
			Title:       "Write report",
			Description: &agentDescription,
			Status:      ItemStatusPlanned,
			CategoryID:  &work,
			StartAt:     stringPointer("2026-11-06T01:00:00.000Z"),
			EndAt:       stringPointer("2026-11-06T02:00:00.000Z"),
		},
		{
			ID:         "done-gym",
			Title:      "Agent gym",
			Status:     ItemStatusDone,
			CategoryID: &life,
		},
		{
			ID:     "unrelated",
			Title:  "Buy milk",
			Status: ItemStatusUnplanned,
		},
	}

	results := SearchItems(items, SearchInput{
		Query:      "agent",
		Status:     ItemStatusPlanned,
		CategoryID: "work",
		Limit:      1,
	})
	if len(results) != 1 {
		t.Fatalf("SearchItems() length = %d, want 1", len(results))
	}
	if results[0].ID != "planned-report" ||
		results[0].Title != "Write report" ||
		results[0].CategoryID == nil ||
		*results[0].CategoryID != "work" {
		t.Fatalf("SearchItems()[0] = %#v", results[0])
	}
}

func TestSearchItemsUsesDefaultAndMaximumLimit(t *testing.T) {
	t.Parallel()

	items := make([]Item, 75)
	for index := range items {
		items[index] = Item{
			ID:     string(rune('a' + index%26)),
			Title:  "item",
			Status: ItemStatusUnplanned,
		}
	}
	if got := len(SearchItems(items, SearchInput{})); got != 20 {
		t.Fatalf("default search length = %d, want 20", got)
	}
	if got := len(SearchItems(items, SearchInput{Limit: 1000})); got != 50 {
		t.Fatalf("maximum search length = %d, want 50", got)
	}
}

func TestSummarizeItemsCountsStatusesCategoriesAndScheduledDuration(t *testing.T) {
	t.Parallel()

	work := "work"
	life := "life"
	items := []Item{
		{
			ID:         "planned-work",
			Status:     ItemStatusPlanned,
			CategoryID: &work,
			StartAt:    stringPointer("2026-11-06T01:00:00.000Z"),
			EndAt:      stringPointer("2026-11-06T02:00:00.000Z"),
		},
		{
			ID:         "done-work",
			Status:     ItemStatusDone,
			CategoryID: &work,
			StartAt:    stringPointer("2026-11-06T03:00:00.000Z"),
			EndAt:      stringPointer("2026-11-06T03:30:00.000Z"),
		},
		{
			ID:         "unplanned-life",
			Status:     ItemStatusUnplanned,
			CategoryID: &life,
		},
	}
	categories := []Category{
		{ID: "work", Name: "工作"},
		{ID: "life", Name: "生活"},
	}

	summary := SummarizeItems(items, categories)
	if summary.Total != 3 ||
		summary.Planned != 1 ||
		summary.Done != 1 ||
		summary.Unplanned != 1 {
		t.Fatalf("summary status counts = %#v", summary)
	}
	if summary.CompletionRate != 1.0/3.0 {
		t.Fatalf("completion rate = %v, want %v", summary.CompletionRate, 1.0/3.0)
	}
	if summary.ScheduledSeconds != 5_400 {
		t.Fatalf("scheduled seconds = %d, want 5400", summary.ScheduledSeconds)
	}
	if len(summary.Categories) != 2 {
		t.Fatalf("category summaries = %#v", summary.Categories)
	}
	if summary.Categories[0].CategoryID != "work" ||
		summary.Categories[0].Total != 2 ||
		summary.Categories[0].Done != 1 {
		t.Fatalf("work summary = %#v", summary.Categories[0])
	}
	if summary.Categories[1].CategoryID != "life" ||
		summary.Categories[1].Total != 1 ||
		summary.Categories[1].Done != 0 {
		t.Fatalf("life summary = %#v", summary.Categories[1])
	}
}
