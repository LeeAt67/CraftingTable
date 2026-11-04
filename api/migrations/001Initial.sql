CREATE TABLE IF NOT EXISTS categories (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  color TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS items (
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

CREATE INDEX IF NOT EXISTS idx_items_start_at ON items(start_at);
CREATE INDEX IF NOT EXISTS idx_items_created_at ON items(created_at);

INSERT OR IGNORE INTO categories (id, name, color) VALUES
  ('work', '工作', '#5468FF'),
  ('study', '学习', '#00B42A'),
  ('life', '生活', '#F59E0B');
