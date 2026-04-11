package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/up1512001/kov/internal/bus"
)

// Router manages multiple providers in a failover chain.
// It handles automatic retry, failover, and health monitoring.
type Router struct {
	chain    []Provider
	health   map[string]*providerHealth
	mu       sync.RWMutex
	bus      *bus.Bus
	logger   *slog.Logger
	config   RouterConfig
	stopHealth chan struct{}
}

// RouterConfig controls retry and failover behavior.
type RouterConfig struct {
	MaxRetries      int           // per-provider retry count (default: 3)
	BackoffInitial  time.Duration // default: 2s
	BackoffMax      time.Duration // default: 60s
	HealthInterval  time.Duration // default: 60s
}

// DefaultRouterConfig returns sensible default router configuration.
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		MaxRetries:     3,
		BackoffInitial: 2 * time.Second,
		BackoffMax:     60 * time.Second,
		HealthInterval: 60 * time.Second,
	}
}

type providerHealth struct {
	available    bool
	lastCheck    time.Time
	failureCount int
}

// NewRouter creates a router with the given failover chain.
// Providers are tried in order; the first available one handles the request.
func NewRouter(providers []Provider, eventBus *bus.Bus, logger *slog.Logger, config RouterConfig) *Router {
	health := make(map[string]*providerHealth)
	for _, p := range providers {
		health[p.ID()] = &providerHealth{available: true}
	}

	r := &Router{
		chain:      providers,
		health:     health,
		bus:        eventBus,
		logger:     logger,
		config:     config,
		stopHealth: make(chan struct{}),
	}

	// Start background health checks
	if config.HealthInterval > 0 {
		go r.healthLoop()
	}

	return r
}

// Chat sends a non-streaming request, trying each provider in the chain.
func (r *Router) Chat(ctx context.Context, params ChatParams) (*ChatResponse, error) {
	var lastErr error

	for _, provider := range r.chain {
		if !r.isHealthy(provider.ID()) {
			r.logger.Debug("skipping unhealthy provider", slog.String("provider", provider.ID()))
			continue
		}

		resp, err := r.tryWithRetry(ctx, provider, params)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		action, _ := ClassifyError(err, r.config.MaxRetries)

		switch action {
		case ActionRetry:
			// Already retried in tryWithRetry
			r.logger.Warn("provider exhausted retries, trying next",
				slog.String("provider", provider.ID()),
				slog.String("error", err.Error()))
			continue

		case ActionFailover:
			r.markUnhealthy(provider.ID())
			if r.bus != nil {
				r.bus.Publish(bus.ProviderFailover{
					From: provider.ID(),
				})
			}
			r.logger.Warn("failing over to next provider",
				slog.String("from", provider.ID()),
				slog.String("error", err.Error()))
			continue

		case ActionPause:
			// Don't failover on auth errors — the user needs to fix credentials
			return nil, err

		case ActionCompact:
			// Context too large — caller needs to compact and retry
			return nil, err

		case ActionAbort:
			return nil, err
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed: %w", lastErr)
	}
	return nil, fmt.Errorf("no providers configured")
}

// Stream sends a streaming request, trying each provider in the chain.
func (r *Router) Stream(ctx context.Context, params ChatParams) (<-chan StreamEvent, error) {
	var lastErr error

	for _, provider := range r.chain {
		if !r.isHealthy(provider.ID()) {
			continue
		}

		ch, err := provider.Stream(ctx, params)
		if err == nil {
			return ch, nil
		}

		lastErr = err
		action, delay := ClassifyError(err, 0)

		switch action {
		case ActionRetry:
			// For streaming, retry once then failover
			time.Sleep(delay)
			ch, err = provider.Stream(ctx, params)
			if err == nil {
				return ch, nil
			}
			r.markUnhealthy(provider.ID())
			continue

		case ActionFailover:
			r.markUnhealthy(provider.ID())
			if r.bus != nil {
				r.bus.Publish(bus.ProviderFailover{From: provider.ID()})
			}
			continue

		case ActionPause, ActionCompact, ActionAbort:
			return nil, err
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed: %w", lastErr)
	}
	return nil, fmt.Errorf("no providers configured")
}

// tryWithRetry attempts a Chat request with retry logic.
func (r *Router) tryWithRetry(ctx context.Context, provider Provider, params ChatParams) (*ChatResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		resp, err := provider.Chat(ctx, params)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		action, delay := ClassifyError(err, attempt)

		if action != ActionRetry {
			return nil, err
		}

		r.logger.Info("retrying request",
			slog.String("provider", provider.ID()),
			slog.Int("attempt", attempt+1),
			slog.Duration("delay", delay))

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
}

// isHealthy checks if a provider is currently considered healthy.
func (r *Router) isHealthy(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.health[id]
	if !ok {
		return false
	}
	return h.available
}

// markUnhealthy marks a provider as temporarily unavailable.
func (r *Router) markUnhealthy(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.health[id]; ok {
		h.available = false
		h.failureCount++
		h.lastCheck = time.Now()
		r.logger.Warn("provider marked unhealthy",
			slog.String("provider", id),
			slog.Int("failures", h.failureCount))
	}
}

// healthLoop periodically checks provider health and restores healthy ones.
func (r *Router) healthLoop() {
	ticker := time.NewTicker(r.config.HealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.checkAllHealth()
		case <-r.stopHealth:
			return
		}
	}
}

// checkAllHealth probes each unhealthy provider.
func (r *Router) checkAllHealth() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, provider := range r.chain {
		if r.isHealthy(provider.ID()) {
			continue // Already healthy, skip
		}

		if provider.IsAvailable(ctx) {
			r.mu.Lock()
			if h, ok := r.health[provider.ID()]; ok {
				h.available = true
				h.failureCount = 0
				h.lastCheck = time.Now()
				r.logger.Info("provider restored to healthy",
					slog.String("provider", provider.ID()))
			}
			r.mu.Unlock()
		}
	}
}

// ActiveProvider returns the first healthy provider in the chain.
func (r *Router) ActiveProvider() Provider {
	for _, p := range r.chain {
		if r.isHealthy(p.ID()) {
			return p
		}
	}
	if len(r.chain) > 0 {
		return r.chain[0] // Fallback to first even if unhealthy
	}
	return nil
}

// GetProvider returns a specific provider by ID.
func (r *Router) GetProvider(id string) Provider {
	for _, p := range r.chain {
		if p.ID() == id {
			return p
		}
	}
	return nil
}

// Providers returns all configured providers.
func (r *Router) Providers() []Provider {
	return r.chain
}

// HealthStatus returns the health status of all providers.
func (r *Router) HealthStatus() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status := make(map[string]bool, len(r.health))
	for id, h := range r.health {
		status[id] = h.available
	}
	return status
}

// Close stops the health monitoring goroutine.
func (r *Router) Close() {
	close(r.stopHealth)
}
