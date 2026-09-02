-- P0 session hardening. Existing sessions remain valid, with last_seen_at
-- initialized from created_at so idle timeout semantics are deterministic after
-- upgrade. Forward-only: do not edit previously applied migrations.

ALTER TABLE sessions ADD COLUMN last_seen_at TEXT;
ALTER TABLE sessions ADD COLUMN client_ip TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';

UPDATE sessions
SET last_seen_at = created_at
WHERE last_seen_at IS NULL OR last_seen_at = '';

CREATE INDEX IF NOT EXISTS idx_sessions_last_seen ON sessions(last_seen_at);
