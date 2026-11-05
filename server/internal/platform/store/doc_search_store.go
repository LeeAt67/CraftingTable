package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/leeat/craftingtable/server/internal/docs"
)

// Search 返回标题命中的节点，并附带祖先路径。
func (s *SQLiteStore) Search(
	ctx context.Context,
	input docs.SearchInput,
) (docs.SearchPage, error) {
	rank := 0
	updatedAt := ""
	id := ""
	hasCursor := 1
	if input.Cursor != nil {
		rank = input.Cursor.Rank
		updatedAt = input.Cursor.UpdatedAt
		id = input.Cursor.ID
		hasCursor = 0
	}

	rows, err := s.db.QueryContext(
		ctx,
		`WITH ranked AS (
		   SELECT id, parent_id, kind, title, updated_at,
		          CASE WHEN lower(title) LIKE lower(?) || '%' ESCAPE '\' THEN 0 ELSE 1 END AS rank
		   FROM doc_nodes
		   WHERE deleted_at IS NULL
		     AND lower(title) LIKE '%' || lower(?) || '%' ESCAPE '\'
		 ),
		 page AS (
		   SELECT * FROM ranked
		   WHERE ? = 1
		      OR rank > ?
		      OR (rank = ? AND updated_at < ?)
		      OR (rank = ? AND updated_at = ? AND id < ?)
		   ORDER BY rank, updated_at DESC, id DESC
		   LIMIT ?
		 )
		 SELECT id, parent_id, kind, title, updated_at, rank
		 FROM page
		 ORDER BY rank, updated_at DESC, id DESC`,
		input.Query,
		input.Query,
		hasCursor,
		rank,
		rank,
		updatedAt,
		rank,
		updatedAt,
		id,
		input.Limit+1,
	)
	if err != nil {
		return docs.SearchPage{}, fmt.Errorf("search doc nodes: %w", err)
	}
	defer rows.Close()

	hits := make([]docs.SearchHit, 0)
	ranks := make([]int, 0)
	for rows.Next() {
		var (
			hit      docs.SearchHit
			kind     string
			parentID sql.NullString
			hitRank  int
		)
		if err := rows.Scan(
			&hit.ID,
			&parentID,
			&kind,
			&hit.Title,
			&hit.UpdatedAt,
			&hitRank,
		); err != nil {
			return docs.SearchPage{}, fmt.Errorf("scan doc search hit: %w", err)
		}
		hit.Kind = docs.NodeKind(kind)
		hit.ParentID = stringPointer(parentID)
		hit.Path = make([]string, 0)
		hits = append(hits, hit)
		ranks = append(ranks, hitRank)
	}
	if err := rows.Err(); err != nil {
		return docs.SearchPage{}, fmt.Errorf("iterate doc search hits: %w", err)
	}

	page := docs.SearchPage{Items: hits}
	if len(hits) > input.Limit {
		page.Items = hits[:input.Limit]
		last := page.Items[len(page.Items)-1]
		cursor := docs.EncodeCursor(docs.SearchCursor{
			Rank:      ranks[input.Limit-1],
			UpdatedAt: last.UpdatedAt,
			ID:        last.ID,
			QueryHash: input.QueryHash,
		})
		page.NextCursor = &cursor
	}

	identifiers := make([]string, 0, len(page.Items))
	for _, hit := range page.Items {
		identifiers = append(identifiers, hit.ID)
	}
	paths, err := s.docAncestorPaths(ctx, identifiers)
	if err != nil {
		return docs.SearchPage{}, err
	}
	for index := range page.Items {
		if path, ok := paths[page.Items[index].ID]; ok {
			page.Items[index].Path = path
		}
	}
	return page, nil
}

func (s *SQLiteStore) docAncestorPaths(
	ctx context.Context,
	identifiers []string,
) (map[string][]string, error) {
	paths := make(map[string][]string, len(identifiers))
	if len(identifiers) == 0 {
		return paths, nil
	}

	values := ""
	arguments := make([]any, 0, len(identifiers))
	for index, identifier := range identifiers {
		if index > 0 {
			values += ","
		}
		values += "(?)"
		arguments = append(arguments, identifier)
	}

	rows, err := s.db.QueryContext(
		ctx,
		`WITH RECURSIVE seed(node_id) AS (VALUES `+values+`),
		 chain(node_id, id, title, parent_id, depth) AS (
		   SELECT seed.node_id, parent.id, parent.title, parent.parent_id, 1
		   FROM seed
		   JOIN doc_nodes node ON node.id = seed.node_id
		   JOIN doc_nodes parent ON parent.id = node.parent_id
		   UNION ALL
		   SELECT chain.node_id, parent.id, parent.title, parent.parent_id, chain.depth + 1
		   FROM doc_nodes parent
		   JOIN chain ON parent.id = chain.parent_id
		   WHERE chain.depth < 8
		 )
		 SELECT node_id, title, depth FROM chain ORDER BY node_id, depth DESC`,
		arguments...,
	)
	if err != nil {
		return nil, fmt.Errorf("collect doc search paths: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			nodeID string
			title  string
			depth  int
		)
		if err := rows.Scan(&nodeID, &title, &depth); err != nil {
			return nil, fmt.Errorf("scan doc search path: %w", err)
		}
		paths[nodeID] = append(paths[nodeID], title)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate doc search paths: %w", err)
	}
	return paths, nil
}
