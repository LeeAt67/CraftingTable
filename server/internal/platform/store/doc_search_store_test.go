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
