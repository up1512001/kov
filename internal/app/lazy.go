// Package app provides the top-level application orchestrator for kov.
package app

import "sync"

// Lazy is a generic lazy initializer. It ensures the init function
// is called exactly once and caches the result. This is critical for
// achieving <50ms cold start — subsystems (DB, providers, git) are
// only initialized when first accessed.
type Lazy[T any] struct {
	once sync.Once
	val  T
	err  error
	init func() (T, error)
}

// NewLazy creates a new lazy value with the given init function.
func NewLazy[T any](init func() (T, error)) *Lazy[T] {
	return &Lazy[T]{init: init}
}

// Get returns the lazily-initialized value, calling init on first access.
// Subsequent calls return the cached value. Thread-safe via sync.Once.
func (l *Lazy[T]) Get() (T, error) {
	l.once.Do(func() {
		l.val, l.err = l.init()
	})
	return l.val, l.err
}

// MustGet is like Get but panics on error. Use only for subsystems
// where failure is truly unrecoverable (e.g., event bus).
func (l *Lazy[T]) MustGet() T {
	v, err := l.Get()
	if err != nil {
		panic("kov: lazy init failed: " + err.Error())
	}
	return v
}
