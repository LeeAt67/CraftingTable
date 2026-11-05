// Package store 提供 CraftingTable 的基础设施持久化实现。
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/leeat/craftingtable/server/internal/today"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const selectItem = `
SELECT
  id,
  kind,
  title,
  description,
  status,
  category_id,
  start_at,
  end_at,
  completed_at,
  created_at,
  updated_at
FROM items`

// SQLiteStore 是面向本地优先版本的 SQLite Repository 实现。
type SQLiteStore struct {
	db *sql.DB
}

// OpenSQLite 打开数据库、执行初始化迁移并验证连接。
func OpenSQLite(ctx context.Context, filename string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	for _, statement := range []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("initialize sqlite: %w", err)
		}
	}
	if err := applyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func applyMigrations(ctx context.Context, db *sql.DB) error {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// Close 关闭数据库连接。
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ListToday 返回指定时间区间内创建或开始的条目。
func (s *SQLiteStore) ListToday(
	ctx context.Context,
	startAt string,
	endAt string,
) ([]today.Item, error) {
	rows, err := s.db.QueryContext(
		ctx,
		selectItem+`
WHERE (start_at >= ? AND start_at < ?)
   OR (start_at IS NULL AND created_at >= ? AND created_at < ?)
ORDER BY start_at IS NULL, start_at, created_at`,
		startAt,
		endAt,
		startAt,
		endAt,
	)
	if err != nil {
		return nil, fmt.Errorf("list today items: %w", err)
	}
	defer rows.Close()

	items := make([]today.Item, 0)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan today item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate today items: %w", err)
	}
	return items, nil
}

// ListCategories 返回按名称排序的分类。
func (s *SQLiteStore) ListCategories(ctx context.Context) ([]today.Category, error) {
	rows, err := s.db.QueryContext(
		ctx,
		"SELECT id, name, color FROM categories ORDER BY name",
	)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	categories := make([]today.Category, 0)
	for rows.Next() {
		var category today.Category
		if err := rows.Scan(&category.ID, &category.Name, &category.Color); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}
	return categories, nil
}

// CreateItem 创建一个任务；带开始时间时状态为 planned。
func (s *SQLiteStore) CreateItem(
	ctx context.Context,
	input today.CreateItemInput,
) (today.Item, error) {
	id, err := newID()
	if err != nil {
		return today.Item{}, fmt.Errorf("create item id: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO items (
		  id, kind, title, status, category_id, start_at, end_at, created_at, updated_at
		) VALUES (?, 'task', ?, ?, ?, ?, ?, ?, ?)`,
		id,
		input.Title,
		string(today.StatusForStart(input.StartAt)),
		nullableValue(input.CategoryID),
		nullableValue(input.StartAt),
		nullableValue(input.EndAt),
		now,
		now,
	)
	if err != nil {
		return today.Item{}, fmt.Errorf("insert item: %w", err)
	}
	return s.getItem(ctx, id)
}

// UpdateItem 更新可编辑字段并返回最新条目。
func (s *SQLiteStore) UpdateItem(
	ctx context.Context,
	input today.UpdateItemInput,
) (today.Item, error) {
	current, err := s.getItem(ctx, input.ID)
	if err != nil {
		return today.Item{}, err
	}

	title := current.Title
	if input.Title != nil {
		title = *input.Title
	}
	description := current.Description
	if input.Description != nil {
		description = input.Description
	}
	categoryID := current.CategoryID
	if input.CategoryID != nil {
		categoryID = input.CategoryID
	}
	startAt := current.StartAt
	if input.StartAt != nil {
		startAt = input.StartAt
	}
	endAt := current.EndAt
	if input.EndAt != nil {
		endAt = input.EndAt
	}

	_, err = s.db.ExecContext(
		ctx,
		`UPDATE items
		 SET title = ?, description = ?, category_id = ?, start_at = ?, end_at = ?, updated_at = ?
		 WHERE id = ?`,
		title,
		nullableValue(description),
		nullableValue(categoryID),
		nullableValue(startAt),
		nullableValue(endAt),
		time.Now().UTC().Format(time.RFC3339Nano),
		input.ID,
	)
	if err != nil {
		return today.Item{}, fmt.Errorf("update item: %w", err)
	}
	return s.getItem(ctx, input.ID)
}

// CompleteItem 将任务标记为完成。
func (s *SQLiteStore) CompleteItem(
	ctx context.Context,
	id string,
) (today.Item, error) {
	if _, err := s.getItem(ctx, id); err != nil {
		return today.Item{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE items
		 SET status = 'done', completed_at = ?, updated_at = ?
		 WHERE id = ?`,
		now,
		now,
		id,
	)
	if err != nil {
		return today.Item{}, fmt.Errorf("complete item: %w", err)
	}
	return s.getItem(ctx, id)
}

// UncompleteItem 恢复任务；有开始时间时为 planned，否则为 unplanned。
func (s *SQLiteStore) UncompleteItem(
	ctx context.Context,
	id string,
) (today.Item, error) {
	current, err := s.getItem(ctx, id)
	if err != nil {
		return today.Item{}, err
	}
	status := today.RestoredStatus(current.StartAt)
	_, err = s.db.ExecContext(
		ctx,
		`UPDATE items
		 SET status = ?, completed_at = NULL, updated_at = ?
		 WHERE id = ?`,
		string(status),
		time.Now().UTC().Format(time.RFC3339Nano),
		id,
	)
	if err != nil {
		return today.Item{}, fmt.Errorf("uncomplete item: %w", err)
	}
	return s.getItem(ctx, id)
}

func (s *SQLiteStore) getItem(ctx context.Context, id string) (today.Item, error) {
	item, err := scanItem(s.db.QueryRowContext(ctx, selectItem+" WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return today.Item{}, today.ErrItemNotFound
	}
	if err != nil {
		return today.Item{}, fmt.Errorf("get item: %w", err)
	}
	return item, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(scanner rowScanner) (today.Item, error) {
	var (
		item        today.Item
		kind        string
		status      string
		description sql.NullString
		categoryID  sql.NullString
		startAt     sql.NullString
		endAt       sql.NullString
		completedAt sql.NullString
	)
	err := scanner.Scan(
		&item.ID,
		&kind,
		&item.Title,
		&description,
		&status,
		&categoryID,
		&startAt,
		&endAt,
		&completedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return today.Item{}, err
	}
	item.Kind = today.ItemKind(kind)
	item.Status = today.ItemStatus(status)
	item.Description = stringPointer(description)
	item.CategoryID = stringPointer(categoryID)
	item.StartAt = stringPointer(startAt)
	item.EndAt = stringPointer(endAt)
	item.CompletedAt = stringPointer(completedAt)
	return item, nil
}

func nullableValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}
