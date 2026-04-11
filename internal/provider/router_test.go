package provider

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/utsavkovy/kov/internal/bus"
)

// mockProvider is a test provider that can simulate failures.
type mockProvider struct {
	id         string
	available  bool
	chatErr    error
	chatResp   *ChatResponse
	callCount  int
}

func (m *mockProvider) ID() string   { return m.id }
func (m *mockProvider) Name() string { return m.id }
func (m *mockProvider) Models() []Model { return nil }
func (m *mockProvider) SupportsThinking() bool { return false }
func (m *mockProvider) IsAvailable(ctx context.Context) bool { return m.available }

func (m *mockProvider) Chat(ctx context.Context, params ChatParams) (*ChatResponse, error) {
	m.callCount++
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	return m.chatResp, nil
}

func (m *mockProvider) Stream(ctx context.Context, params ChatParams) (<-chan StreamEvent, error) {
	m.callCount++
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Type: EventDone}
	close(ch)
	return ch, nil
}

func TestRouter_HealthyProvider(t *testing.T) {
	b := bus.New()
	defer b.Close()

	primary := &mockProvider{
		id:        "primary",
		available: true,
		chatResp:  &ChatResponse{Content: "hello from primary"},
	}

	r := NewRouter([]Provider{primary}, b, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), RouterConfig{MaxRetries: 3, HealthInterval: 0})
	defer r.Close()

	resp, err := r.Chat(context.Background(), ChatParams{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello from primary" {
		t.Errorf("expected 'hello from primary', got %q", resp.Content)
	}
}

func TestRouter_FailoverOnNetworkError(t *testing.T) {
	b := bus.New()
	defer b.Close()

	primary := &mockProvider{
		id:        "primary",
		available: true,
		chatErr:   fmt.Errorf("connection refused"),
	}
	fallback := &mockProvider{
		id:        "fallback",
		available: true,
		chatResp:  &ChatResponse{Content: "hello from fallback"},
	}

	r := NewRouter([]Provider{primary, fallback}, b, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), RouterConfig{MaxRetries: 0, HealthInterval: 0})
	defer r.Close()

	resp, err := r.Chat(context.Background(), ChatParams{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello from fallback" {
		t.Errorf("expected 'hello from fallback', got %q", resp.Content)
	}
}

func TestRouter_FailoverOn429(t *testing.T) {
	b := bus.New()
	defer b.Close()

	primary := &mockProvider{
		id:        "primary",
		available: true,
		chatErr:   &ProviderError{Provider: "primary", StatusCode: 429, Message: "rate limited"},
	}
	fallback := &mockProvider{
		id:        "fallback",
		available: true,
		chatResp:  &ChatResponse{Content: "hello from fallback"},
	}

	r := NewRouter([]Provider{primary, fallback}, b, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), RouterConfig{MaxRetries: 0, HealthInterval: 0})
	defer r.Close()

	resp, err := r.Chat(context.Background(), ChatParams{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello from fallback" {
		t.Errorf("expected fallback response")
	}
}

func TestRouter_PauseOnAuthError(t *testing.T) {
	b := bus.New()
	defer b.Close()

	primary := &mockProvider{
		id:        "primary",
		available: true,
		chatErr:   &ProviderError{Provider: "primary", StatusCode: 401, Message: "unauthorized"},
	}
	fallback := &mockProvider{
		id:        "fallback",
		available: true,
		chatResp:  &ChatResponse{Content: "should not reach"},
	}

	r := NewRouter([]Provider{primary, fallback}, b, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), RouterConfig{MaxRetries: 0, HealthInterval: 0})
	defer r.Close()

	_, err := r.Chat(context.Background(), ChatParams{Model: "test"})
	if err == nil {
		t.Fatal("expected error on auth failure")
	}
	// Should NOT failover — fallback should not be called
	if fallback.callCount > 0 {
		t.Error("fallback should not be called on auth error (user needs to fix creds)")
	}
}

func TestRouter_AllProvidersFail(t *testing.T) {
	b := bus.New()
	defer b.Close()

	p1 := &mockProvider{id: "p1", available: true, chatErr: fmt.Errorf("connection refused")}
	p2 := &mockProvider{id: "p2", available: true, chatErr: fmt.Errorf("connection refused")}

	r := NewRouter([]Provider{p1, p2}, b, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), RouterConfig{MaxRetries: 0, HealthInterval: 0})
	defer r.Close()

	_, err := r.Chat(context.Background(), ChatParams{Model: "test"})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestRouter_SkipUnhealthyProvider(t *testing.T) {
	b := bus.New()
	defer b.Close()

	primary := &mockProvider{
		id:        "primary",
		available: true,
		chatResp:  &ChatResponse{Content: "primary"},
	}
	fallback := &mockProvider{
		id:        "fallback",
		available: true,
		chatResp:  &ChatResponse{Content: "fallback"},
	}

	r := NewRouter([]Provider{primary, fallback}, b, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), RouterConfig{MaxRetries: 0, HealthInterval: 0})
	defer r.Close()

	// Mark primary as unhealthy
	r.markUnhealthy("primary")

	resp, err := r.Chat(context.Background(), ChatParams{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "fallback" {
		t.Errorf("expected fallback response, got %q", resp.Content)
	}
	if primary.callCount > 0 {
		t.Error("unhealthy primary should not have been called")
	}
}

func TestRouter_HealthStatus(t *testing.T) {
	b := bus.New()
	defer b.Close()

	p1 := &mockProvider{id: "p1", available: true}
	p2 := &mockProvider{id: "p2", available: true}

	r := NewRouter([]Provider{p1, p2}, b, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), RouterConfig{HealthInterval: 0})
	defer r.Close()

	status := r.HealthStatus()
	if !status["p1"] || !status["p2"] {
		t.Error("all providers should start healthy")
	}

	r.markUnhealthy("p1")
	status = r.HealthStatus()
	if status["p1"] {
		t.Error("p1 should be unhealthy")
	}
	if !status["p2"] {
		t.Error("p2 should still be healthy")
	}
}

func TestRouter_RetryWithBackoff(t *testing.T) {
	b := bus.New()
	defer b.Close()

	callCount := 0
	primary := &mockProvider{
		id:        "primary",
		available: true,
	}
	// Override Chat to fail first then succeed
	originalChat := primary.Chat
	_ = originalChat
	primary.chatErr = &ProviderError{Provider: "primary", StatusCode: 500, Message: "server error"}

	r := NewRouter([]Provider{primary}, b, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), RouterConfig{MaxRetries: 2, BackoffInitial: 10 * time.Millisecond, HealthInterval: 0})
	defer r.Close()

	start := time.Now()
	_, err := r.Chat(context.Background(), ChatParams{Model: "test"})
	elapsed := time.Since(start)
	_ = callCount

	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	// Should have taken time due to backoff
	if elapsed < 10*time.Millisecond {
		t.Errorf("expected backoff delay, elapsed: %v", elapsed)
	}
	// Should have tried multiple times
	if primary.callCount < 2 {
		t.Errorf("expected at least 2 attempts, got %d", primary.callCount)
	}
}
