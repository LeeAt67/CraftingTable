package docs

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

// Service 承担文档领域校验并编排持久化端口。
type Service struct {
	repository Repository
}

// NewService 创建文档领域服务。
func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// ListTree 返回全量未删除节点摘要。
func (s *Service) ListTree(ctx context.Context) ([]NodeSummary, error) {
	return s.repository.ListTree(ctx)
}

// GetNode 返回单个节点及其正文。
func (s *Service) GetNode(ctx context.Context, id string) (Node, error) {
	if strings.TrimSpace(id) == "" {
		return Node{}, ErrNodeNotFound
	}
	return s.repository.GetNode(ctx, strings.TrimSpace(id))
}

// CreateNode 校验并创建目录节点。
func (s *Service) CreateNode(ctx context.Context, input CreateNodeInput) (Node, error) {
	if !IsValidKind(input.Kind) {
		return Node{}, ErrInvalidInput
	}
	title := NormalizeTitle(input.Kind, input.Title)
	if utf8.RuneCountInString(title) > MaxTitleRunes {
		return Node{}, ErrInvalidInput
	}

	parentID := trimmedPointer(input.ParentID)
	if parentID != nil {
		parent, err := s.repository.GetNode(ctx, *parentID)
		if err != nil {
			return Node{}, err
		}
		if parent.Kind != NodeKindFolder {
			return Node{}, ErrParentNotFolder
		}
	}

	input.ParentID = parentID
	input.Title = title
	input.ClientRequestID = trimmedPointer(input.ClientRequestID)
	return s.repository.CreateNode(ctx, input)
}

// RenameNode 校验并重命名节点。
func (s *Service) RenameNode(ctx context.Context, input RenameNodeInput) (NodeSummary, error) {
	current, err := s.GetNode(ctx, input.ID)
	if err != nil {
		return NodeSummary{}, err
	}
	title := NormalizeTitle(current.Kind, input.Title)
	if utf8.RuneCountInString(title) > MaxTitleRunes {
		return NodeSummary{}, ErrInvalidInput
	}
	return s.repository.RenameNode(ctx, RenameNodeInput{
		ID:    current.ID,
		Title: title,
	})
}

// MoveNode 校验目标父节点并移动节点。
func (s *Service) MoveNode(ctx context.Context, input MoveNodeInput) (NodeSummary, error) {
	current, err := s.GetNode(ctx, input.ID)
	if err != nil {
		return NodeSummary{}, err
	}

	parentID := trimmedPointer(input.ParentID)
	if parentID != nil {
		if *parentID == current.ID {
			return NodeSummary{}, ErrInvalidMove
		}
		parent, err := s.repository.GetNode(ctx, *parentID)
		if err != nil {
			return NodeSummary{}, err
		}
		if parent.Kind != NodeKindFolder {
			return NodeSummary{}, ErrParentNotFolder
		}
		ancestors, err := s.repository.AncestorIDs(ctx, *parentID)
		if err != nil {
			return NodeSummary{}, err
		}
		for _, ancestor := range ancestors {
			if ancestor == current.ID {
				return NodeSummary{}, ErrInvalidMove
			}
		}
	}

	return s.repository.MoveNode(ctx, MoveNodeInput{
		ID:       current.ID,
		ParentID: parentID,
	})
}

// SaveContent 校验并保存文档正文，返回新的更新时间。
func (s *Service) SaveContent(ctx context.Context, input SaveContentInput) (string, error) {
	current, err := s.GetNode(ctx, input.ID)
	if err != nil {
		return "", err
	}
	if current.Kind != NodeKindDocument {
		return "", ErrFolderHasContent
	}
	if utf8.RuneCountInString(input.Content) > MaxContentRunes {
		return "", ErrContentTooLong
	}
	return s.repository.SaveContent(ctx, SaveContentInput{
		ID:      current.ID,
		Content: input.Content,
	})
}

// DeleteNode 软删除节点及其子孙，返回全部受影响 id。
func (s *Service) DeleteNode(ctx context.Context, id string) ([]string, error) {
	current, err := s.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.repository.SoftDelete(ctx, current.ID, now)
}

// Search 校验搜索入参并返回一页结果。
func (s *Service) Search(
	ctx context.Context,
	rawQuery string,
	rawLimit int,
	rawCursor string,
) (SearchPage, error) {
	query, err := NormalizeQuery(rawQuery)
	if err != nil {
		return SearchPage{}, err
	}
	if query == "" {
		return SearchPage{Items: make([]SearchHit, 0)}, nil
	}

	cursor, err := decodeOptionalCursor(rawCursor, query)
	if err != nil {
		return SearchPage{}, err
	}

	return s.repository.Search(ctx, SearchInput{
		Query:     LikePattern(query),
		Limit:     NormalizeLimit(rawLimit),
		Cursor:    cursor,
		QueryHash: QueryHash(query),
	})
}

// decodeOptionalCursor 只在游标非空时解码；首屏查询不带游标。
func decodeOptionalCursor(raw string, query string) (*SearchCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	return DecodeCursor(raw, query)
}

func trimmedPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
