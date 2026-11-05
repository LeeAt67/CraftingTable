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
