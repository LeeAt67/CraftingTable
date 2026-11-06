// Package store 提供 CraftingTable 的基础设施持久化实现。
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
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
	if err := verifyItemTimestamps(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// verifyItemTimestamps 确认迁移没有把任何时间列写成 NULL。
// strftime 对无法解析的输入返回 NULL，这里把它视为迁移失败而不是静默丢数据。
func verifyItemTimestamps(ctx context.Context, db *sql.DB) error {
	for _, column := range []string{
		"created_at",
		"updated_at",
		"start_at",
		"end_at",
		"completed_at",
	} {
		query := `SELECT COUNT(*) FROM items
			WHERE ` + column + ` IS NOT NULL AND ` + column + ` <> ''
			AND strftime('%Y-%m-%dT%H:%M:%fZ', ` + column + `) IS NULL`
		var lossy int
		if err := db.QueryRowContext(ctx, query).Scan(&lossy); err != nil {
			return fmt.Errorf("verify %s: %w", column, err)
		}
		if lossy != 0 {
			return fmt.Errorf(
				"migration would lose %d unparsable %s values",
				lossy,
				column,
			)
		}
	}
	return nil
}

func applyMigrations(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
		  name TEXT PRIMARY KEY,
		  checksum TEXT NOT NULL,
		  applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema migrations table: %w", err)
	}
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
		sum := sha256.Sum256(body)
		checksum := hex.EncodeToString(sum[:])
		var appliedChecksum string
		err = db.QueryRowContext(
			ctx,
			`SELECT checksum FROM schema_migrations WHERE name = ?`,
			entry.Name(),
		).Scan(&appliedChecksum)
		if err == nil {
			if appliedChecksum != checksum {
				return fmt.Errorf(
					"migration %s checksum mismatch: applied %s, embedded %s",
					entry.Name(),
					appliedChecksum,
					checksum,
				)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read migration %s ledger: %w", entry.Name(), err)
		}
		if err := applyMigration(ctx, db, entry.Name(), string(body), checksum); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(
	ctx context.Context,
	db *sql.DB,
	name string,
	body string,
	checksum string,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, body); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (name, checksum, applied_at)
		 VALUES (?, ?, ?)`,
		name,
		checksum,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}

// MigrationContent returns one embedded migration body for upgrade tests.
func MigrationContent(name string) (string, error) {
	body, err := migrationFiles.ReadFile("migrations/" + name)
	if err != nil {
		return "", fmt.Errorf("read migration %s: %w", name, err)
	}
	return string(body), nil
}

// MigrationChecksum returns the recorded SHA-256 of one embedded migration.
func MigrationChecksum(name string) (string, error) {
	body, err := MigrationContent(name)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:]), nil
}

// Close 关闭数据库连接。
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ListToday 返回指定 UTC 区间内开始或创建的条目，已删除条目不会返回。
func (s *SQLiteStore) ListToday(
	ctx context.Context,
	startAt string,
	endAt string,
) ([]today.Item, error) {
	start := normalizedBound(startAt)
	end := normalizedBound(endAt)
	rows, err := s.db.QueryContext(
		ctx,
		selectItem+`
WHERE deleted_at IS NULL
  AND ((start_at >= ? AND start_at < ?)
   OR (start_at IS NULL AND created_at >= ? AND created_at < ?))
ORDER BY start_at IS NULL, start_at, created_at`,
		start,
		end,
		start,
		end,
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

// GetItem returns one non-deleted item by ID for read-only Agent access.
func (s *SQLiteStore) GetItem(
	ctx context.Context,
	id string,
) (today.Item, error) {
	return s.getItem(ctx, id)
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
	startAt := normalizeTimestamp(input.StartAt)
	endAt := normalizeTimestamp(input.EndAt)
	if problem := today.TimeRangeProblem(startAt, endAt); problem != "" {
		return today.Item{}, today.ValidationError{Message: problem}
	}
	now := today.FormatTimestamp(time.Now())

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
// startAt 或 endAt 传入空字符串表示清空该时间。
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
	if problem := today.TimeRangeProblem(startAt, endAt); problem != "" {
		return today.Item{}, today.ValidationError{Message: problem}
	}
	normalizedStart := normalizeTimestamp(startAt)
	normalizedEnd := normalizeTimestamp(endAt)

	// 已完成条目改期后仍保持 done，其他条目按开始时间重算。
	status := current.Status
	if status != today.ItemStatusDone {
		status = today.StatusForStart(normalizedStart)
	}

	_, err = s.db.ExecContext(
		ctx,
		`UPDATE items
		 SET title = ?, description = ?, status = ?, category_id = ?, start_at = ?, end_at = ?, updated_at = ?
		 WHERE id = ? AND deleted_at IS NULL`,
		title,
		nullableValue(description),
		string(status),
		nullableValue(categoryID),
		nullableTimestamp(normalizedStart),
		nullableTimestamp(normalizedEnd),
		today.FormatTimestamp(time.Now()),
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
	now := today.FormatTimestamp(time.Now())
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE items
		 SET status = 'done', completed_at = ?, updated_at = ?
		 WHERE id = ? AND deleted_at IS NULL`,
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
		 WHERE id = ? AND deleted_at IS NULL`,
		string(status),
		today.FormatTimestamp(time.Now()),
		id,
	)
	if err != nil {
		return today.Item{}, fmt.Errorf("uncomplete item: %w", err)
	}
	return s.getItem(ctx, id)
}

// DeleteItem 为条目写入删除墓碑；重复删除返回首次的 DeletedAt。
func (s *SQLiteStore) DeleteItem(
	ctx context.Context,
	id string,
) (today.DeletedItem, error) {
	deletedAt := today.FormatTimestamp(time.Now())
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE items
		 SET deleted_at = ?, updated_at = ?
		 WHERE id = ? AND deleted_at IS NULL`,
		deletedAt,
		deletedAt,
		id,
	)
	if err != nil {
		return today.DeletedItem{}, fmt.Errorf("delete item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return today.DeletedItem{}, fmt.Errorf("delete item rows: %w", err)
	}
	if affected == 1 {
		return today.DeletedItem{ID: id, DeletedAt: deletedAt}, nil
	}

	// 条件更新没有命中：要么已删除（幂等返回首次墓碑），要么从未存在。
	var existing string
	err = s.db.QueryRowContext(
		ctx,
		`SELECT deleted_at FROM items WHERE id = ? AND deleted_at IS NOT NULL`,
		id,
	).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return today.DeletedItem{}, today.ErrItemNotFound
	}
	if err != nil {
		return today.DeletedItem{}, fmt.Errorf("read item tombstone: %w", err)
	}
	return today.DeletedItem{ID: id, DeletedAt: existing}, nil
}

// RestoreItem 清除删除墓碑；未删除的条目原样返回。
func (s *SQLiteStore) RestoreItem(
	ctx context.Context,
	id string,
) (today.Item, error) {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE items
		 SET deleted_at = NULL, updated_at = ?
		 WHERE id = ? AND deleted_at IS NOT NULL`,
		today.FormatTimestamp(time.Now()),
		id,
	)
	if err != nil {
		return today.Item{}, fmt.Errorf("restore item: %w", err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return today.Item{}, fmt.Errorf("restore item rows: %w", err)
	}
	return s.getItem(ctx, id)
}

func (s *SQLiteStore) getItem(ctx context.Context, id string) (today.Item, error) {
	item, err := scanItem(s.db.QueryRowContext(
		ctx,
		selectItem+" WHERE id = ? AND deleted_at IS NULL",
		id,
	))
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

// nullableValue 把 nil 写成 SQL NULL。
func nullableValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

// normalizeTimestamp 返回固定宽度 UTC 时间；nil 与空白表示没有时间。
func normalizeTimestamp(value *string) *string {
	normalized, _ := today.NormalizeTimestamp(value)
	return normalized
}

// normalizedBound 把查询边界规范化为与列相同的固定宽度格式。
func normalizedBound(value string) string {
	normalized := value
	if result, message := today.NormalizeTimestamp(&value); message == "" {
		if result != nil {
			normalized = *result
		} else {
			normalized = ""
		}
	}
	return normalized
}

// nullableTimestamp 把空时间写成 SQL NULL，而不是空字符串。
func nullableTimestamp(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
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
