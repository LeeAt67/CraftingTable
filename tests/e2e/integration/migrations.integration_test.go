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
	for _, name := range []string{
		"001_initial.sql",
		"002_agent.sql",
		"003_agent_reasoning.sql",
	} {
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
