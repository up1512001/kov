package resilience

import (
	"sync"
)

// BudgetMonitor tracks cumulative cost and enforces spending limits.
type BudgetMonitor struct {
	budget   float64 // max USD per session (0 = unlimited)
	warnAt   float64 // USD threshold for warning
	current  float64
	warned   bool
	mu       sync.RWMutex
}

// NewBudgetMonitor creates a new budget monitor.
func NewBudgetMonitor(budget, warnAt float64) *BudgetMonitor {
	return &BudgetMonitor{
		budget: budget,
		warnAt: warnAt,
	}
}

// Add adds cost to the running total.
func (b *BudgetMonitor) Add(cost float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.current += cost
}

// Set sets the current cost (used for recovery from checkpoint).
func (b *BudgetMonitor) Set(cost float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.current = cost
}

// Current returns the current accumulated cost.
func (b *BudgetMonitor) Current() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.current
}

// IsExceeded returns true if the budget has been exceeded.
func (b *BudgetMonitor) IsExceeded() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.budget <= 0 {
		return false // unlimited
	}
	return b.current >= b.budget
}

// ShouldWarn returns true the first time the cost exceeds warnAt.
func (b *BudgetMonitor) ShouldWarn() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.warnAt <= 0 || b.warned {
		return false
	}
	if b.current >= b.warnAt {
		b.warned = true
		return true
	}
	return false
}

// Remaining returns the remaining budget. Returns -1 for unlimited.
func (b *BudgetMonitor) Remaining() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.budget <= 0 {
		return -1
	}
	remaining := b.budget - b.current
	if remaining < 0 {
		return 0
	}
	return remaining
}
