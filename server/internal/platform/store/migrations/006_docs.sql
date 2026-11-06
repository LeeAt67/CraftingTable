CREATE TABLE IF NOT EXISTS doc_nodes (
  id                TEXT PRIMARY KEY,
  parent_id         TEXT REFERENCES doc_nodes(id),
  kind              TEXT NOT NULL CHECK (kind IN ('folder', 'document')),
  title             TEXT NOT NULL,
  content           TEXT NOT NULL DEFAULT '',
  sort_order        INTEGER NOT NULL DEFAULT 0,
  client_request_id TEXT,
  deleted_at        TEXT,
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_doc_nodes_request
  ON doc_nodes(client_request_id) WHERE client_request_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_doc_nodes_parent ON doc_nodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_doc_nodes_updated ON doc_nodes(updated_at DESC);
