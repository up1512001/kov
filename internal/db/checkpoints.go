package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Checkpoint holds the state needed to resume a session after failure.
type Checkpoint struct {
	ID             int64
	SessionID      string
	State          string // FSM state
	TaskID         string // current/last task
	ContextSummary string // compressed conversation context
	ProviderID     string
	Model          string
	RetryCount     int
	Cost           float64
	CreatedAt      time.Time
}

// SaveCheckpoint persists the current session state for crash recovery.
func (db *DB) SaveCheckpoint(ctx context.Context, cp *Checkpoint) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO checkpoints (session_id, state, task_id, context_summary, provider_id, model, retry_count, cost)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			cp.SessionID, cp.State, cp.TaskID, cp.ContextSummary, cp.ProviderID, cp.Model, cp.RetryCount, cp.Cost,
		)
		return err
	})
}

// GetLatestCheckpoint returns the most recent checkpoint for a session.
func (db *DB) GetLatestCheckpoint(ctx context.Context, sessionID string) (*Checkpoint, error) {
	cp := &Checkpoint{}
	var createdAt string
	err := db.readDB.QueryRowContext(ctx,
		`SELECT id, session_id, state, task_id, context_summary, provider_id, model, retry_count, cost, created_at
		 FROM checkpoints WHERE session_id = ? ORDER BY created_at DESC LIMIT 1`, sessionID,
	).Scan(&cp.ID, &cp.SessionID, &cp.State, &cp.TaskID, &cp.ContextSummary, &cp.ProviderID, &cp.Model, &cp.RetryCount, &cp.Cost, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("getting checkpoint: %w", err)
	}
	cp.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return cp, nil
}

// CleanOldCheckpoints removes old checkpoints, keeping only the most recent N.
func (db *DB) CleanOldCheckpoints(ctx context.Context, sessionID string, keepCount int) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`DELETE FROM checkpoints WHERE session_id = ? AND id NOT IN (
				SELECT id FROM checkpoints WHERE session_id = ? ORDER BY created_at DESC LIMIT ?
			)`, sessionID, sessionID, keepCount,
		)
		return err
	})
}
