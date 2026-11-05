CREATE TABLE IF NOT EXISTS agent_sessions (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  client_request_id TEXT NOT NULL UNIQUE,
  activity_seq INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_messages (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
  status TEXT NOT NULL CHECK (
    status IN ('streaming', 'complete', 'failed', 'cancelled', 'interrupted')
  ),
  active_run_id TEXT REFERENCES agent_runs(id),
  user_message_id TEXT REFERENCES agent_messages(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(session_id, seq)
);

CREATE TABLE IF NOT EXISTS agent_runs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
  user_message_id TEXT NOT NULL REFERENCES agent_messages(id),
  assistant_message_id TEXT NOT NULL REFERENCES agent_messages(id),
  status TEXT NOT NULL CHECK (
    status IN ('running', 'completed', 'failed', 'cancelled', 'interrupted')
  ),
  client_request_id TEXT NOT NULL,
  error_json TEXT,
  next_event_sequence INTEGER NOT NULL DEFAULT 1,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_message_parts (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL REFERENCES agent_messages(id) ON DELETE CASCADE,
  run_id TEXT REFERENCES agent_runs(id),
  part_index INTEGER NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('text', 'error')),
  status TEXT NOT NULL CHECK (
    status IN ('streaming', 'complete', 'failed', 'cancelled', 'interrupted')
  ),
  text_content TEXT NOT NULL DEFAULT '',
  error_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_steps (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  step_index INTEGER NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  finished_at TEXT,
  UNIQUE(run_id, step_index)
);

CREATE TABLE IF NOT EXISTS agent_run_events (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  sequence INTEGER NOT NULL,
  event_type TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(run_id, sequence)
);

CREATE TABLE IF NOT EXISTS agent_idempotency_keys (
  operation TEXT NOT NULL,
  client_request_id TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  session_id TEXT,
  run_id TEXT,
  created_at TEXT NOT NULL,
  PRIMARY KEY(operation, client_request_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_one_running_agent_run_per_session
  ON agent_runs(session_id) WHERE status = 'running';
CREATE UNIQUE INDEX IF NOT EXISTS idx_one_active_run_per_message
  ON agent_messages(active_run_id) WHERE active_run_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_message_parts
  ON agent_message_parts(message_id, part_index) WHERE run_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_assistant_run_parts
  ON agent_message_parts(message_id, run_id, part_index) WHERE run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_agent_messages_session_seq
  ON agent_messages(session_id, seq);
CREATE INDEX IF NOT EXISTS idx_agent_runs_session_status
  ON agent_runs(session_id, status);
CREATE INDEX IF NOT EXISTS idx_agent_run_events_run_sequence
  ON agent_run_events(run_id, sequence);
