package bus

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// Bus is a lightweight, typed, in-process publish/subscribe event bus.
// It uses Go channels for delivery, ensuring zero external dependencies.
//
// Design decisions:
//   - Typed events via reflect.Type (not string keys) for compile-time safety
//   - Bounded subscriber channels for backpressure (default 64 buffer)
//   - Thread-safe via RWMutex for subscriber map, atomic for cancellation
//   - Non-blocking publish (drops events on full channels)
type Bus struct {
	mu          sync.RWMutex
	subscribers map[reflect.Type][]*subscriber
	closed      bool
}

type subscriber struct {
	ch       chan Event
	canceled atomic.Bool
}

// New creates a new event bus.
func New() *Bus {
	return &Bus{
		subscribers: make(map[reflect.Type][]*subscriber),
	}
}

// Subscribe registers a listener for a specific event type.
// Returns a channel that will receive events of the specified type,
// and a cancel function to unsubscribe.
//
// Usage:
//
//	ch, cancel := bus.Subscribe(bus.TokenReceived{})
//	defer cancel()
//	for event := range ch {
//	    token := event.(bus.TokenReceived)
//	    // handle token
//	}
func (b *Bus) Subscribe(eventType Event, bufSize ...int) (<-chan Event, func()) {
	size := 64
	if len(bufSize) > 0 && bufSize[0] > 0 {
		size = bufSize[0]
	}

	sub := &subscriber{
		ch: make(chan Event, size),
	}

	t := reflect.TypeOf(eventType)

	b.mu.Lock()
	b.subscribers[t] = append(b.subscribers[t], sub)
	b.mu.Unlock()

	// Cancel marks the subscriber as canceled and removes it from the list.
	// We do NOT close the channel here — the channel will be drained by the
	// subscriber's reading goroutine and will be garbage collected. This
	// prevents send-on-closed-channel panics in concurrent Publish calls.
	cancelOnce := sync.Once{}
	cancelFn := func() {
		cancelOnce.Do(func() {
			sub.canceled.Store(true)

			// Remove from subscriber list under write lock
			b.mu.Lock()
			subs := b.subscribers[t]
			for i, s := range subs {
				if s == sub {
					b.subscribers[t] = append(subs[:i], subs[i+1:]...)
					break
				}
			}
			b.mu.Unlock()
		})
	}

	return sub.ch, cancelFn
}

// Publish sends an event to all subscribers of its type.
// Non-blocking: if a subscriber's channel is full, the event is dropped
// for that subscriber (prevents publisher from stalling).
func (b *Bus) Publish(event Event) {
	t := reflect.TypeOf(event)

	b.mu.RLock()
	// Copy the pointer slice under read lock so we iterate a stable snapshot.
	subs := make([]*subscriber, len(b.subscribers[t]))
	copy(subs, b.subscribers[t])
	b.mu.RUnlock()

	for _, sub := range subs {
		if sub.canceled.Load() {
			continue
		}
		// Use trySend to safely handle edge case where cancel
		// could theoretically happen between the check above
		// and the send below (the channel is never closed by cancel,
		// so this is actually safe, but we protect anyway).
		trySend(sub.ch, event)
	}
}

// trySend attempts a non-blocking send. If the channel is full, the
// event is dropped. Recovers from panics for defensive safety.
func trySend(ch chan Event, event Event) {
	defer func() { recover() }() //nolint:errcheck
	select {
	case ch <- event:
	default:
	}
}

// Close shuts down the bus and drains/closes all subscriber channels.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true

	for _, subs := range b.subscribers {
		for _, sub := range subs {
			sub.canceled.Store(true)
			// Safe to close here — we hold the write lock, and Publish
			// uses trySend with panic recovery.
			close(sub.ch)
		}
	}
	b.subscribers = nil
}

