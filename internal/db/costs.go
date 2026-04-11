package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CostEvent records the cost of a single LLM API call.
type CostEvent struct {
	ID           int64
	SessionID    string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	Cost         float64
	CreatedAt    time.Time
}

// RecordCost inserts a cost event and updates the session's total cost.
func (db *DB) RecordCost(ctx context.Context, c *CostEvent) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		// Insert cost event
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO cost_events (session_id, provider, model, input_tokens, output_tokens, cost)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			c.SessionID, c.Provider, c.Model, c.InputTokens, c.OutputTokens, c.Cost,
		); err != nil {
			return err
		}
		// Update session total cost
		_, err := tx.ExecContext(ctx,
			`UPDATE sessions SET cost = cost + ? WHERE id = ?`,
			c.Cost, c.SessionID,
		)
		return err
	})
}

// GetSessionCost returns the total cost for a session.
func (db *DB) GetSessionCost(ctx context.Context, sessionID string) (float64, error) {
	var cost float64
	err := db.readDB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost), 0) FROM cost_events WHERE session_id = ?`, sessionID,
	).Scan(&cost)
	return cost, err
}

// GetCostEvents returns all cost events for a session.
func (db *DB) GetCostEvents(ctx context.Context, sessionID string) ([]*CostEvent, error) {
	rows, err := db.readDB.QueryContext(ctx,
		`SELECT id, session_id, provider, model, input_tokens, output_tokens, cost, created_at
		 FROM cost_events WHERE session_id = ? ORDER BY created_at ASC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting cost events: %w", err)
	}
	defer rows.Close()

	var events []*CostEvent
	for rows.Next() {
		c := &CostEvent{}
		var createdAt string
		if err := rows.Scan(&c.ID, &c.SessionID, &c.Provider, &c.Model, &c.InputTokens, &c.OutputTokens, &c.Cost, &createdAt); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		events = append(events, c)
	}
	return events, rows.Err()
}
