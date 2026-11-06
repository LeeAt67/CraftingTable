ALTER TABLE agent_steps
ADD COLUMN kind TEXT NOT NULL DEFAULT 'model'
CHECK (kind IN ('model', 'tool'));

ALTER TABLE agent_steps
ADD COLUMN parent_step_id TEXT REFERENCES agent_steps(id);

ALTER TABLE agent_steps
ADD COLUMN error_code TEXT;

CREATE TABLE agent_tool_invocations (
  id TEXT PRIMARY KEY,
  step_id TEXT NOT NULL UNIQUE REFERENCES agent_steps(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  provider_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  tool_version TEXT NOT NULL,
  arguments_json TEXT NOT NULL,
  arguments_hash TEXT NOT NULL,
  result_json TEXT NOT NULL DEFAULT '',
  result_is_error INTEGER NOT NULL DEFAULT 0
    CHECK (result_is_error IN (0, 1)),
  result_truncated INTEGER NOT NULL DEFAULT 0
    CHECK (result_truncated IN (0, 1)),
  encoded_bytes INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(run_id, provider_call_id)
);

CREATE INDEX idx_agent_tool_invocations_run
  ON agent_tool_invocations(run_id, created_at);
