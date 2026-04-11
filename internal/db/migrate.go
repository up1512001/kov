package db

import "fmt"

// migrate runs all database migrations in order.
// Migrations are idempotent (IF NOT EXISTS) and run inside a transaction.
func (db *DB) migrate() error {
	for i, migration := range migrations {
		tx, err := db.writeDB.Begin()
		if err != nil {
			return fmt.Errorf("migration %d: begin: %w", i, err)
		}
		if _, err := tx.Exec(migration); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: exec: %w", i, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %d: commit: %w", i, err)
		}
	}
	return nil
}

// migrations is the ordered list of SQL migration scripts.
// Each migration must be idempotent using IF NOT EXISTS / IF EXISTS.
var migrations = []string{
	// Migration 0: Core tables
	`
	-- Sessions table: tracks each kov usage session
	CREATE TABLE IF NOT EXISTS sessions (
		id          TEXT PRIMARY KEY,
		project_dir TEXT NOT NULL,
		mode        TEXT NOT NULL DEFAULT 'code',
		provider    TEXT NOT NULL DEFAULT '',
		model       TEXT NOT NULL DEFAULT '',
		state       TEXT NOT NULL DEFAULT 'idle',
		prompt      TEXT NOT NULL DEFAULT '',
		cost        REAL NOT NULL DEFAULT 0.0,
		created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	);

	-- Messages: conversation history for each session (lives in DB, NOT RAM)
	CREATE TABLE IF NOT EXISTS messages (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		role        TEXT NOT NULL,
		content     TEXT NOT NULL,
		tool_name   TEXT NOT NULL DEFAULT '',
		tool_call_id TEXT NOT NULL DEFAULT '',
		tool_args   TEXT NOT NULL DEFAULT '',
		token_count INTEGER NOT NULL DEFAULT 0,
		cost        REAL NOT NULL DEFAULT 0.0,
		created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	);
	CREATE INDEX IF NOT EXISTS idx_messages_session
		ON messages(session_id, created_at);

	-- Tasks: the resilience backbone — atomic work units in a queue
	CREATE TABLE IF NOT EXISTS tasks (
		id           TEXT PRIMARY KEY,
		session_id   TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		seq          INTEGER NOT NULL,
		description  TEXT NOT NULL,
		status       TEXT NOT NULL DEFAULT 'pending',
		result       TEXT NOT NULL DEFAULT '',
		git_hash     TEXT NOT NULL DEFAULT '',
		fix_retries  INTEGER NOT NULL DEFAULT 0,
		error_msg    TEXT NOT NULL DEFAULT '',
		created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		completed_at TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_tasks_session
		ON tasks(session_id, seq);

	-- Checkpoints: survive kill -9 via WAL
	CREATE TABLE IF NOT EXISTS checkpoints (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		state       TEXT NOT NULL,
		task_id     TEXT NOT NULL DEFAULT '',
		context_summary TEXT NOT NULL DEFAULT '',
		provider_id TEXT NOT NULL DEFAULT '',
		model       TEXT NOT NULL DEFAULT '',
		retry_count INTEGER NOT NULL DEFAULT 0,
		cost        REAL NOT NULL DEFAULT 0.0,
		created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	);
	CREATE INDEX IF NOT EXISTS idx_checkpoints_session
		ON checkpoints(session_id, created_at DESC);

	-- Cost events: granular cost tracking per API call
	CREATE TABLE IF NOT EXISTS cost_events (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id    TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		provider      TEXT NOT NULL,
		model         TEXT NOT NULL,
		input_tokens  INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		cost          REAL NOT NULL DEFAULT 0.0,
		created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	);
	CREATE INDEX IF NOT EXISTS idx_cost_session
		ON cost_events(session_id);
	`,
}
