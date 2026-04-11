package resilience

import (
	"sync"
)

// LoopDetector detects when the agent is stuck in a repetitive loop.
// It tracks the last N actions and fires when the same action appears
// threshold times consecutively or within a small window.
type LoopDetector struct {
	threshold int
	history   []string
	mu        sync.Mutex
}

// NewLoopDetector creates a detector with the given threshold.
func NewLoopDetector(threshold int) *LoopDetector {
	if threshold < 2 {
		threshold = 3
	}
	return &LoopDetector{
		threshold: threshold,
		history:   make([]string, 0, threshold*2),
	}
}

// Record adds an action and returns true if a loop is detected.
func (d *LoopDetector) Record(action string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.history = append(d.history, action)

	// Keep only last threshold*2 entries
	if len(d.history) > d.threshold*2 {
		d.history = d.history[len(d.history)-d.threshold*2:]
	}

	return d.isLoop()
}

// isLoop checks if the recent actions show a repeating pattern.
func (d *LoopDetector) isLoop() bool {
	if len(d.history) < d.threshold {
		return false
	}

	// Check for same action repeated threshold times
	last := d.history[len(d.history)-1]
	count := 0
	for i := len(d.history) - 1; i >= 0 && i >= len(d.history)-d.threshold; i-- {
		if d.history[i] == last {
			count++
		}
	}

	return count >= d.threshold
}

// Reset clears the action history.
func (d *LoopDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.history = d.history[:0]
}

// Count returns the number of tracked actions.
func (d *LoopDetector) Count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.history)
}
