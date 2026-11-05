package integration

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeat/craftingtable/server/testkit"
)

func TestSQLiteMigrationsAreRecordedAndChecksumGuarded(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "migration-ledger.sqlite")
	first, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows, err := db.QueryContext(
		ctx,
		`SELECT name, checksum FROM schema_migrations ORDER BY name`,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	applied := make(map[string]string)
	for rows.Next() {
		var name string
		var checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			t.Fatal(err)
		}
		applied[name] = checksum
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_initial.sql", "002_agent.sql"} {
		if len(applied[name]) != 64 {
			t.Fatalf("migration %s checksum = %q", name, applied[name])
		}
	}
	if _, err := db.ExecContext(
		ctx,
		`UPDATE schema_migrations SET checksum = 'tampered'
		 WHERE name = '001_initial.sql'`,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = testkit.OpenSQLite(ctx, path)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("reopen error = %v, want checksum mismatch", err)
	}
}

func TestSQLiteMigrationUpgradesLegacyDatabaseWithoutLosingTodayData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		CREATE TABLE categories (
		  id TEXT PRIMARY KEY,
		  name TEXT NOT NULL,
		  color TEXT NOT NULL
		);
		CREATE TABLE items (
		  id TEXT PRIMARY KEY,
		  kind TEXT NOT NULL CHECK (kind IN ('task', 'event')),
		  title TEXT NOT NULL,
		  description TEXT,
		  status TEXT NOT NULL CHECK (status IN ('unplanned', 'planned', 'done')),
		  category_id TEXT REFERENCES categories(id),
		  start_at TEXT,
		  end_at TEXT,
		  completed_at TEXT,
		  created_at TEXT NOT NULL,
		  updated_at TEXT NOT NULL
		);
		INSERT INTO categories (id, name, color)
		VALUES ('legacy-category', '旧分类', '#123456');
		INSERT INTO items (
		  id, kind, title, status, category_id, created_at, updated_at
		) VALUES (
		  'legacy-item', 'task', '保留的任务', 'unplanned',
		  'legacy-category', '2026-11-05T00:00:00Z',
		  '2026-11-05T00:00:00Z'
		);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	items, err := store.ListToday(
		ctx,
		"2026-11-05T00:00:00Z",
		"2026-11-06T00:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "legacy-item" {
		t.Fatalf("items = %#v", items)
	}
}

func TestSQLiteMigrationAddsDeletionTombstoneAndNormalizesTimestamps(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "normalized.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		CREATE TABLE categories (
		  id TEXT PRIMARY KEY,
		  name TEXT NOT NULL,
		  color TEXT NOT NULL
		);
		CREATE TABLE items (
		  id TEXT PRIMARY KEY,
		  kind TEXT NOT NULL,
		  title TEXT NOT NULL,
		  description TEXT,
		  status TEXT NOT NULL,
		  category_id TEXT,
		  start_at TEXT,
		  end_at TEXT,
		  completed_at TEXT,
		  created_at TEXT NOT NULL,
		  updated_at TEXT NOT NULL
		);
		INSERT INTO categories (id, name, color)
		VALUES ('legacy-category', '旧分类', '#123456');
		INSERT INTO items (
		  id, kind, title, status, category_id, start_at, end_at, completed_at,
		  created_at, updated_at
		) VALUES
		  ('nano-item', 'event', '纳秒格式', 'done', 'legacy-category',
		   '2026-11-05T09:00:00.123456789Z', '2026-11-05T10:00:00Z',
		   '2026-11-05T10:01:00.5Z',
		   '2026-11-05T08:00:00Z', '2026-11-05T10:01:00Z'),
		  ('plain-item', 'task', '无小数位', 'planned', NULL,
		   '2026-11-05T11:00:00+08:00', NULL, NULL,
		   '2026-11-05T00:00:00Z', '2026-11-05T00:00:00Z');
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := testkit.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	verification, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer verification.Close()

	var deletedAtColumns int
	if err := verification.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM pragma_table_info('items') WHERE name = 'deleted_at'`,
	).Scan(&deletedAtColumns); err != nil {
		t.Fatal(err)
	}
	if deletedAtColumns != 1 {
		t.Fatalf("deleted_at column count = %d, want 1", deletedAtColumns)
	}

	var rowCount int
	if err := verification.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM items`,
	).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 2 {
		t.Fatalf("item count after migration = %d, want 2", rowCount)
	}

	fixedWidthGlob := "[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]" +
		"T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9]Z"
	for _, column := range []string{
		"created_at",
		"updated_at",
		"start_at",
		"end_at",
		"completed_at",
	} {
		var mismatched int
		query := `SELECT COUNT(*) FROM items
			WHERE ` + column + ` IS NOT NULL AND ` + column + ` <> ''
			AND ` + column + ` NOT GLOB ?`
		if err := verification.QueryRowContext(ctx, query, fixedWidthGlob).Scan(&mismatched); err != nil {
			t.Fatalf("verify %s: %v", column, err)
		}
		if mismatched != 0 {
			t.Fatalf("column %s has %d values outside fixed width format", column, mismatched)
		}
	}

	items, err := store.ListToday(
		ctx,
		"2026-11-05T00:00:00.000Z",
		"2026-11-06T00:00:00.000Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].StartAt == nil || *items[0].StartAt != "2026-11-05T03:00:00.000Z" {
		t.Fatalf("plain-item startAt = %v", items[0].StartAt)
	}
}
