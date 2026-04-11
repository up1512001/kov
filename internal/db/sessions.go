package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Session represents a kov coding session.
type Session struct {
	ID         string
	ProjectDir string
	Mode       string
	Provider   string
	Model      string
	State      string // idle, planning, executing, verifying, error_wait, done
	Prompt     string
	Cost       float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateSession creates a new session and returns its ID.
func (db *DB) CreateSession(ctx context.Context, s *Session) (string, error) {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	err := db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO sessions (id, project_dir, mode, provider, model, state, prompt, cost, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, s.ProjectDir, s.Mode, s.Provider, s.Model, s.State, s.Prompt, s.Cost, now, now,
		)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}
	return s.ID, nil
}

// GetSession retrieves a session by ID.
func (db *DB) GetSession(ctx context.Context, id string) (*Session, error) {
	s := &Session{}
	var createdAt, updatedAt string
	err := db.readDB.QueryRowContext(ctx,
		`SELECT id, project_dir, mode, provider, model, state, prompt, cost, created_at, updated_at
		 FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.ProjectDir, &s.Mode, &s.Provider, &s.Model, &s.State, &s.Prompt, &s.Cost, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return s, nil
}

// UpdateSessionState updates the state and cost of a session.
func (db *DB) UpdateSessionState(ctx context.Context, id string, state string, cost float64) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE sessions SET state = ?, cost = ?, updated_at = ? WHERE id = ?`,
			state, cost, now, id,
		)
		return err
	})
}

// ListSessions lists recent sessions, most recent first.
func (db *DB) ListSessions(ctx context.Context, limit int) ([]*Session, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.readDB.QueryContext(ctx,
		`SELECT id, project_dir, mode, provider, model, state, prompt, cost, created_at, updated_at
		 FROM sessions ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s := &Session{}
		var createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.ProjectDir, &s.Mode, &s.Provider, &s.Model, &s.State, &s.Prompt, &s.Cost, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// GetLastInterruptedSession returns the most recent session that was interrupted.
func (db *DB) GetLastInterruptedSession(ctx context.Context, projectDir string) (*Session, error) {
	s := &Session{}
	var createdAt, updatedAt string
	err := db.readDB.QueryRowContext(ctx,
		`SELECT id, project_dir, mode, provider, model, state, prompt, cost, created_at, updated_at
		 FROM sessions
		 WHERE project_dir = ? AND state IN ('executing', 'error_wait', 'planning', 'verifying')
		 ORDER BY updated_at DESC LIMIT 1`, projectDir,
	).Scan(&s.ID, &s.ProjectDir, &s.Mode, &s.Provider, &s.Model, &s.State, &s.Prompt, &s.Cost, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting interrupted session: %w", err)
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return s, nil
}

// DeleteSession removes a session and all associated data (cascading).
func (db *DB) DeleteSession(ctx context.Context, id string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
		return err
	})
}
