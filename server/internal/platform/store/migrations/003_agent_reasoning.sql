CREATE TABLE agent_message_parts_with_reasoning (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL REFERENCES agent_messages(id) ON DELETE CASCADE,
  run_id TEXT REFERENCES agent_runs(id),
  part_index INTEGER NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('text', 'reasoning', 'error')),
  status TEXT NOT NULL CHECK (
    status IN ('streaming', 'complete', 'failed', 'cancelled', 'interrupted')
  ),
  text_content TEXT NOT NULL DEFAULT '',
  error_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

INSERT INTO agent_message_parts_with_reasoning (
  id, message_id, run_id, part_index, type, status, text_content,
  error_json, created_at, updated_at
)
SELECT id, message_id, run_id, part_index, type, status, text_content,
       error_json, created_at, updated_at
FROM agent_message_parts;

DROP TABLE agent_message_parts;

ALTER TABLE agent_message_parts_with_reasoning RENAME TO agent_message_parts;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_message_parts
  ON agent_message_parts(message_id, part_index) WHERE run_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_assistant_run_parts
  ON agent_message_parts(message_id, run_id, part_index) WHERE run_id IS NOT NULL;
