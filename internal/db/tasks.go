package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Task represents an atomic work unit in the resilience queue.
type Task struct {
	ID          string
	SessionID   string
	Seq         int
	Description string
	Status      string // pending, running, done, failed, skipped
	Result      string
	GitHash     string
	FixRetries  int
	ErrorMsg    string
	CreatedAt   time.Time
	CompletedAt time.Time
}

// Task status constants.
const (
	TaskStatusPending = "pending"
	TaskStatusRunning = "running"
	TaskStatusDone    = "done"
	TaskStatusFailed  = "failed"
	TaskStatusSkipped = "skipped"
)

// CreateTask inserts a new task into the queue.
func (db *DB) CreateTask(ctx context.Context, t *Task) (string, error) {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	err := db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (id, session_id, seq, description, status, git_hash)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			t.ID, t.SessionID, t.Seq, t.Description, t.Status, t.GitHash,
		)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("creating task: %w", err)
	}
	return t.ID, nil
}

// CreateTasks inserts multiple tasks in a single transaction.
func (db *DB) CreateTasks(ctx context.Context, tasks []*Task) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO tasks (id, session_id, seq, description, status, git_hash)
			 VALUES (?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, t := range tasks {
			if t.ID == "" {
				t.ID = uuid.New().String()
			}
			if _, err := stmt.ExecContext(ctx, t.ID, t.SessionID, t.Seq, t.Description, t.Status, t.GitHash); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetTasks retrieves all tasks for a session, ordered by sequence.
func (db *DB) GetTasks(ctx context.Context, sessionID string) ([]*Task, error) {
	rows, err := db.readDB.QueryContext(ctx,
		`SELECT id, session_id, seq, description, status, result, git_hash, fix_retries, error_msg, created_at, completed_at
		 FROM tasks WHERE session_id = ? ORDER BY seq ASC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// GetPendingTasks returns tasks that haven't been completed yet.
func (db *DB) GetPendingTasks(ctx context.Context, sessionID string) ([]*Task, error) {
	rows, err := db.readDB.QueryContext(ctx,
		`SELECT id, session_id, seq, description, status, result, git_hash, fix_retries, error_msg, created_at, completed_at
		 FROM tasks WHERE session_id = ? AND status IN ('pending', 'running', 'failed')
		 ORDER BY seq ASC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting pending tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// UpdateTaskStatus updates a task's status and result.
func (db *DB) UpdateTaskStatus(ctx context.Context, id string, status string, result string, errorMsg string) error {
	completedAt := ""
	if status == TaskStatusDone || status == TaskStatusFailed || status == TaskStatusSkipped {
		completedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE tasks SET status = ?, result = ?, error_msg = ?, completed_at = ? WHERE id = ?`,
			status, result, errorMsg, completedAt, id,
		)
		return err
	})
}

// IncrementTaskRetries increments the fix_retries counter and sets status to running.
func (db *DB) IncrementTaskRetries(ctx context.Context, id string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE tasks SET fix_retries = fix_retries + 1, status = 'running' WHERE id = ?`, id,
		)
		return err
	})
}

// GetTaskStats returns task completion stats for a session.
func (db *DB) GetTaskStats(ctx context.Context, sessionID string) (total int, done int, failed int, err error) {
	err = db.readDB.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0)
		 FROM tasks WHERE session_id = ?`, sessionID,
	).Scan(&total, &done, &failed)
	return
}

func scanTasks(rows *sql.Rows) ([]*Task, error) {
	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		var createdAt, completedAt string
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Seq, &t.Description, &t.Status, &t.Result, &t.GitHash, &t.FixRetries, &t.ErrorMsg, &createdAt, &completedAt); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if completedAt != "" {
			t.CompletedAt, _ = time.Parse(time.RFC3339Nano, completedAt)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
