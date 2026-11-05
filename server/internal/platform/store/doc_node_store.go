package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leeat/craftingtable/server/internal/docs"
)

const selectDocNode = `
SELECT
  id,
  parent_id,
  kind,
  title,
  sort_order,
  created_at,
  updated_at`

const selectDocNodeWithContent = selectDocNode + `,
  content`

// ListTree 返回全量未删除节点摘要，文件夹优先。
func (s *SQLiteStore) ListTree(ctx context.Context) ([]docs.NodeSummary, error) {
	rows, err := s.db.QueryContext(
		ctx,
		selectDocNode+`
FROM doc_nodes
WHERE deleted_at IS NULL
ORDER BY CASE kind WHEN 'folder' THEN 0 ELSE 1 END, sort_order, updated_at DESC, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list doc tree: %w", err)
	}
	defer rows.Close()

	nodes := make([]docs.NodeSummary, 0)
	for rows.Next() {
		node, err := scanNodeSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("scan doc node: %w", err)
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate doc nodes: %w", err)
	}
	return nodes, nil
}

// GetNode 返回单个未删除节点。
func (s *SQLiteStore) GetNode(ctx context.Context, id string) (docs.Node, error) {
	row := s.db.QueryRowContext(
		ctx,
		selectDocNodeWithContent+`
FROM doc_nodes
WHERE id = ? AND deleted_at IS NULL`,
		id,
	)
	node, err := scanNode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return docs.Node{}, docs.ErrNodeNotFound
	}
	if err != nil {
		return docs.Node{}, fmt.Errorf("get doc node: %w", err)
	}
	return node, nil
}

// CreateNode 创建节点；相同 clientRequestId 返回已存在节点。
func (s *SQLiteStore) CreateNode(
	ctx context.Context,
	input docs.CreateNodeInput,
) (docs.Node, error) {
	if input.ClientRequestID != nil {
		existing, err := s.docNodeByRequestID(ctx, *input.ClientRequestID)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, docs.ErrNodeNotFound) {
			return docs.Node{}, err
		}
	}

	id, err := newID()
	if err != nil {
		return docs.Node{}, fmt.Errorf("create doc node id: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO doc_nodes (
		  id, parent_id, kind, title, content, sort_order,
		  client_request_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, '', 0, ?, ?, ?)`,
		id,
		nullableValue(input.ParentID),
		string(input.Kind),
		input.Title,
		nullableValue(input.ClientRequestID),
		now,
		now,
	)
	if err != nil {
		if input.ClientRequestID != nil {
			if existing, lookupErr := s.docNodeByRequestID(ctx, *input.ClientRequestID); lookupErr == nil {
				return existing, nil
			}
		}
		return docs.Node{}, fmt.Errorf("insert doc node: %w", err)
	}
	return s.GetNode(ctx, id)
}

// RenameNode 更新标题并返回节点摘要。
func (s *SQLiteStore) RenameNode(
	ctx context.Context,
	input docs.RenameNodeInput,
) (docs.NodeSummary, error) {
	return s.updateDocNodeSummary(
		ctx,
		"UPDATE doc_nodes SET title = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL",
		input.Title,
		input.ID,
	)
}

// MoveNode 更新父节点并返回节点摘要。
func (s *SQLiteStore) MoveNode(
	ctx context.Context,
	input docs.MoveNodeInput,
) (docs.NodeSummary, error) {
	return s.updateDocNodeSummary(
		ctx,
		"UPDATE doc_nodes SET parent_id = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL",
		nullableValue(input.ParentID),
		input.ID,
	)
}

// SaveContent 保存正文并返回新的更新时间。
func (s *SQLiteStore) SaveContent(
	ctx context.Context,
	input docs.SaveContentInput,
) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE doc_nodes
		 SET content = ?, updated_at = ?
		 WHERE id = ? AND kind = 'document' AND deleted_at IS NULL`,
		input.Content,
		now,
		input.ID,
	)
	if err != nil {
		return "", fmt.Errorf("save doc content: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("save doc content rows: %w", err)
	}
	if affected == 0 {
		return "", docs.ErrNodeNotFound
	}
	return now, nil
}

// SoftDelete 在单个事务内递归软删除节点与子孙。
func (s *SQLiteStore) SoftDelete(
	ctx context.Context,
	id string,
	deletedAt string,
) ([]string, error) {
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin doc delete: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()

	rows, err := transaction.QueryContext(
		ctx,
		`WITH RECURSIVE subtree(id) AS (
		   SELECT id FROM doc_nodes WHERE id = ? AND deleted_at IS NULL
		   UNION ALL
		   SELECT node.id
		   FROM doc_nodes node
		   JOIN subtree parent ON node.parent_id = parent.id
		   WHERE node.deleted_at IS NULL
		 )
		 SELECT id FROM subtree`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("collect doc subtree: %w", err)
	}
	identifiers := make([]string, 0)
	for rows.Next() {
		var identifier string
		if err := rows.Scan(&identifier); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan doc subtree: %w", err)
		}
		identifiers = append(identifiers, identifier)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate doc subtree: %w", err)
	}
	_ = rows.Close()

	if len(identifiers) == 0 {
		return nil, docs.ErrNodeNotFound
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(identifiers)), ",")
	arguments := make([]any, 0, len(identifiers)+2)
	arguments = append(arguments, deletedAt, deletedAt)
	for _, identifier := range identifiers {
		arguments = append(arguments, identifier)
	}
	if _, err := transaction.ExecContext(
		ctx,
		"UPDATE doc_nodes SET deleted_at = ?, updated_at = ? WHERE id IN ("+placeholders+")",
		arguments...,
	); err != nil {
		return nil, fmt.Errorf("soft delete doc subtree: %w", err)
	}

	if err := transaction.Commit(); err != nil {
		return nil, fmt.Errorf("commit doc delete: %w", err)
	}
	return identifiers, nil
}

// AncestorIDs 返回从直接父节点到根的 id 列表。
func (s *SQLiteStore) AncestorIDs(
	ctx context.Context,
	id string,
) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`WITH RECURSIVE chain(id, parent_id, depth) AS (
		   SELECT id, parent_id, 0 FROM doc_nodes WHERE id = ?
		   UNION ALL
		   SELECT node.id, node.parent_id, chain.depth + 1
		   FROM doc_nodes node
		   JOIN chain ON node.id = chain.parent_id
		   WHERE chain.depth < 32
		 )
		 SELECT id FROM chain WHERE depth > 0`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("collect doc ancestors: %w", err)
	}
	defer rows.Close()

	ancestors := make([]string, 0)
	for rows.Next() {
		var ancestor string
		if err := rows.Scan(&ancestor); err != nil {
			return nil, fmt.Errorf("scan doc ancestor: %w", err)
		}
		ancestors = append(ancestors, ancestor)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate doc ancestors: %w", err)
	}
	return ancestors, nil
}

func (s *SQLiteStore) docNodeByRequestID(
	ctx context.Context,
	requestID string,
) (docs.Node, error) {
	row := s.db.QueryRowContext(
		ctx,
		selectDocNodeWithContent+`
FROM doc_nodes
WHERE client_request_id = ? AND deleted_at IS NULL`,
		requestID,
	)
	node, err := scanNode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return docs.Node{}, docs.ErrNodeNotFound
	}
	if err != nil {
		return docs.Node{}, fmt.Errorf("find doc node by request: %w", err)
	}
	return node, nil
}

func (s *SQLiteStore) updateDocNodeSummary(
	ctx context.Context,
	statement string,
	value any,
	id string,
) (docs.NodeSummary, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, statement, value, now, id)
	if err != nil {
		return docs.NodeSummary{}, fmt.Errorf("update doc node: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return docs.NodeSummary{}, fmt.Errorf("update doc node rows: %w", err)
	}
	if affected == 0 {
		return docs.NodeSummary{}, docs.ErrNodeNotFound
	}
	node, err := s.GetNode(ctx, id)
	if err != nil {
		return docs.NodeSummary{}, err
	}
	return node.NodeSummary, nil
}

func scanNodeSummary(scanner rowScanner) (docs.NodeSummary, error) {
	var (
		summary  docs.NodeSummary
		kind     string
		parentID sql.NullString
	)
	if err := scanner.Scan(
		&summary.ID,
		&parentID,
		&kind,
		&summary.Title,
		&summary.SortOrder,
		&summary.CreatedAt,
		&summary.UpdatedAt,
	); err != nil {
		return docs.NodeSummary{}, err
	}
	summary.Kind = docs.NodeKind(kind)
	summary.ParentID = stringPointer(parentID)
	return summary, nil
}

func scanNode(scanner rowScanner) (docs.Node, error) {
	var (
		node     docs.Node
		kind     string
		parentID sql.NullString
	)
	if err := scanner.Scan(
		&node.ID,
		&parentID,
		&kind,
		&node.Title,
		&node.SortOrder,
		&node.CreatedAt,
		&node.UpdatedAt,
		&node.Content,
	); err != nil {
		return docs.Node{}, err
	}
	node.Kind = docs.NodeKind(kind)
	node.ParentID = stringPointer(parentID)
	return node, nil
}
