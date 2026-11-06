package docs

import (
	"context"
	"errors"
	"testing"
)

type stubRepository struct {
	nodes       map[string]Node
	ancestors   map[string][]string
	created     []CreateNodeInput
	saved       []SaveContentInput
	deleted     []string
	searchInput SearchInput
	searchPage  SearchPage
}

func newStubRepository() *stubRepository {
	return &stubRepository{
		nodes:     map[string]Node{},
		ancestors: map[string][]string{},
	}
}

func (s *stubRepository) ListTree(context.Context) ([]NodeSummary, error) {
	result := make([]NodeSummary, 0, len(s.nodes))
	for _, node := range s.nodes {
		result = append(result, node.NodeSummary)
	}
	return result, nil
}

func (s *stubRepository) GetNode(_ context.Context, id string) (Node, error) {
	node, ok := s.nodes[id]
	if !ok {
		return Node{}, ErrNodeNotFound
	}
	return node, nil
}

func (s *stubRepository) CreateNode(_ context.Context, input CreateNodeInput) (Node, error) {
	s.created = append(s.created, input)
	node := Node{
		NodeSummary: NodeSummary{
			ID:       "created-id",
			ParentID: input.ParentID,
			Kind:     input.Kind,
			Title:    input.Title,
		},
	}
	s.nodes["created-id"] = node
	return node, nil
}

func (s *stubRepository) RenameNode(_ context.Context, input RenameNodeInput) (NodeSummary, error) {
	node, ok := s.nodes[input.ID]
	if !ok {
		return NodeSummary{}, ErrNodeNotFound
	}
	node.Title = input.Title
	s.nodes[input.ID] = node
	return node.NodeSummary, nil
}

func (s *stubRepository) MoveNode(ctx context.Context, input MoveNodeInput) (NodeSummary, error) {
	return s.RenameNode(ctx, RenameNodeInput{ID: input.ID, Title: s.nodes[input.ID].Title})
}

func (s *stubRepository) SaveContent(_ context.Context, input SaveContentInput) (string, error) {
	s.saved = append(s.saved, input)
	return "2026-11-06T00:00:00Z", nil
}

func (s *stubRepository) SoftDelete(_ context.Context, id string, _ string) ([]string, error) {
	if _, ok := s.nodes[id]; !ok {
		return nil, ErrNodeNotFound
	}
	s.deleted = append(s.deleted, id)
	return []string{id}, nil
}

func (s *stubRepository) AncestorIDs(_ context.Context, id string) ([]string, error) {
	return s.ancestors[id], nil
}

func (s *stubRepository) Search(_ context.Context, input SearchInput) (SearchPage, error) {
	s.searchInput = input
	return s.searchPage, nil
}

func folderNode(id string, parentID *string) Node {
	return Node{NodeSummary: NodeSummary{ID: id, Kind: NodeKindFolder, ParentID: parentID}}
}

func documentNode(id string, parentID *string) Node {
	return Node{NodeSummary: NodeSummary{ID: id, Kind: NodeKindDocument, ParentID: parentID}}
}

func TestServiceCreateNodeValidatesParent(t *testing.T) {
	t.Parallel()

	repository := newStubRepository()
	repository.nodes["doc"] = documentNode("doc", nil)
	service := NewService(repository)

	if _, err := service.CreateNode(context.Background(), CreateNodeInput{
		Kind:     NodeKindDocument,
		ParentID: stringPointer("doc"),
		Title:    "子文档",
	}); !errors.Is(err, ErrParentNotFolder) {
		t.Fatalf("error = %v, want ErrParentNotFolder", err)
	}

	if _, err := service.CreateNode(context.Background(), CreateNodeInput{
		Kind:  NodeKindDocument,
		Title: "   ",
	}); err != nil {
		t.Fatalf("create error = %v", err)
	}
	if repository.created[0].Title != DefaultDocumentTitle {
		t.Fatalf("title = %q, want %q", repository.created[0].Title, DefaultDocumentTitle)
	}
}

func TestServiceCreateNodeRejectsInvalidKind(t *testing.T) {
	t.Parallel()

	service := NewService(newStubRepository())
	if _, err := service.CreateNode(context.Background(), CreateNodeInput{
		Kind:  NodeKind("page"),
		Title: "错类型",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestServiceMoveNodeRejectsSelfAndDescendant(t *testing.T) {
	t.Parallel()

	repository := newStubRepository()
	repository.nodes["a"] = folderNode("a", nil)
	repository.nodes["b"] = folderNode("b", stringPointer("a"))
	repository.ancestors["b"] = []string{"a"}
	service := NewService(repository)

	if _, err := service.MoveNode(context.Background(), MoveNodeInput{
		ID:       "a",
		ParentID: stringPointer("a"),
	}); !errors.Is(err, ErrInvalidMove) {
		t.Fatalf("self move error = %v, want ErrInvalidMove", err)
	}

	if _, err := service.MoveNode(context.Background(), MoveNodeInput{
		ID:       "a",
		ParentID: stringPointer("b"),
	}); !errors.Is(err, ErrInvalidMove) {
		t.Fatalf("descendant move error = %v, want ErrInvalidMove", err)
	}

	if _, err := service.MoveNode(context.Background(), MoveNodeInput{
		ID:       "b",
		ParentID: nil,
	}); err != nil {
		t.Fatalf("move to root error = %v", err)
	}
}

func TestServiceSaveContentValidatesKindAndLength(t *testing.T) {
	t.Parallel()

	repository := newStubRepository()
	repository.nodes["folder"] = folderNode("folder", nil)
	repository.nodes["doc"] = documentNode("doc", nil)
	service := NewService(repository)

	if _, err := service.SaveContent(context.Background(), SaveContentInput{
		ID:      "folder",
		Content: "内容",
	}); !errors.Is(err, ErrFolderHasContent) {
		t.Fatalf("folder error = %v, want ErrFolderHasContent", err)
	}

	long := make([]rune, MaxContentRunes+1)
	for index := range long {
		long[index] = 'a'
	}
	if _, err := service.SaveContent(context.Background(), SaveContentInput{
		ID:      "doc",
		Content: string(long),
	}); !errors.Is(err, ErrContentTooLong) {
		t.Fatalf("long content error = %v, want ErrContentTooLong", err)
	}

	if _, err := service.SaveContent(context.Background(), SaveContentInput{
		ID:      "doc",
		Content: "# 标题",
	}); err != nil {
		t.Fatalf("save error = %v", err)
	}
	if repository.saved[0].Content != "# 标题" {
		t.Fatalf("saved = %#v", repository.saved[0])
	}
}

func TestServiceSearchNormalizesInput(t *testing.T) {
	t.Parallel()

	repository := newStubRepository()
	repository.searchPage = SearchPage{Items: make([]SearchHit, 0)}
	service := NewService(repository)

	page, err := service.Search(context.Background(), "   ", 0, "")
	if err != nil {
		t.Fatalf("blank search error = %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("blank search page = %#v", page)
	}

	if _, err := service.Search(context.Background(), "接口", 500, ""); err != nil {
		t.Fatalf("search error = %v", err)
	}
	if repository.searchInput.Limit != MaxSearchLimit {
		t.Fatalf("limit = %d, want %d", repository.searchInput.Limit, MaxSearchLimit)
	}

	raw := EncodeCursor(SearchCursor{
		Rank:      0,
		UpdatedAt: "2026-11-06T00:00:00Z",
		ID:        "6f1b0d5e-3f4a-4f7b-8c9d-0a1b2c3d4e5f",
		QueryHash: QueryHash("接口"),
	})
	if _, err := service.Search(context.Background(), "接口", 20, raw); err != nil {
		t.Fatalf("cursor search error = %v", err)
	}
	if repository.searchInput.Cursor == nil {
		t.Fatal("cursor was not passed to repository")
	}

	if _, err := service.Search(context.Background(), "接口", 20, "bad-cursor"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("bad cursor error = %v, want ErrInvalidCursor", err)
	}
}

func stringPointer(value string) *string {
	return &value
}
