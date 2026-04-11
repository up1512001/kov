package provider

import (
	"fmt"
	"testing"
	"time"
)

func TestClassifyError_RateLimit(t *testing.T) {
	err := &ProviderError{Provider: "anthropic", StatusCode: 429, Message: "rate limited"}

	// First retry: should retry
	action, _ := ClassifyError(err, 0)
	if action != ActionRetry {
		t.Errorf("retry 0: expected ActionRetry, got %v", action)
	}

	// After 3 retries: should failover
	action, _ = ClassifyError(err, 3)
	if action != ActionFailover {
		t.Errorf("retry 3: expected ActionFailover, got %v", action)
	}
}

func TestClassifyError_RateLimitWithRetryAfter(t *testing.T) {
	err := &ProviderError{
		Provider:   "anthropic",
		StatusCode: 429,
		Message:    "rate limited",
		RetryAfter: 30 * time.Second,
	}

	action, delay := ClassifyError(err, 0)
	if action != ActionRetry {
		t.Errorf("expected ActionRetry, got %v", action)
	}
	if delay != 30*time.Second {
		t.Errorf("expected 30s delay, got %v", delay)
	}
}

func TestClassifyError_AuthError(t *testing.T) {
	for _, status := range []int{401, 403} {
		err := &ProviderError{Provider: "openai", StatusCode: status, Message: "unauthorized"}
		action, _ := ClassifyError(err, 0)
		if action != ActionPause {
			t.Errorf("status %d: expected ActionPause, got %v", status, action)
		}
	}
}

func TestClassifyError_ServerError(t *testing.T) {
	err := &ProviderError{Provider: "google", StatusCode: 500, Message: "internal error"}

	action, _ := ClassifyError(err, 0)
	if action != ActionRetry {
		t.Errorf("retry 0: expected ActionRetry, got %v", action)
	}

	action, _ = ClassifyError(err, 2)
	if action != ActionFailover {
		t.Errorf("retry 2: expected ActionFailover, got %v", action)
	}
}

func TestClassifyError_ContextTooLong(t *testing.T) {
	err := &ProviderError{
		Provider:   "anthropic",
		StatusCode: 400,
		Message:    "This request exceeds the maximum context length",
	}
	action, _ := ClassifyError(err, 0)
	if action != ActionCompact {
		t.Errorf("expected ActionCompact, got %v", action)
	}
}

func TestClassifyError_NetworkError(t *testing.T) {
	err := fmt.Errorf("dial tcp: connection refused")
	action, _ := ClassifyError(err, 0)
	if action != ActionFailover {
		t.Errorf("expected ActionFailover, got %v", action)
	}
}

func TestClassifyError_Timeout(t *testing.T) {
	err := fmt.Errorf("request timeout exceeded deadline")

	action, _ := ClassifyError(err, 0)
	if action != ActionRetry {
		t.Errorf("retry 0: expected ActionRetry, got %v", action)
	}

	action, _ = ClassifyError(err, 1)
	if action != ActionFailover {
		t.Errorf("retry 1: expected ActionFailover, got %v", action)
	}
}

func TestClassifyError_PaymentRequired(t *testing.T) {
	err := &ProviderError{Provider: "openai", StatusCode: 402, Message: "payment required"}
	action, _ := ClassifyError(err, 0)
	if action != ActionPause {
		t.Errorf("expected ActionPause, got %v", action)
	}
}

func TestClassifyError_UnknownError(t *testing.T) {
	err := fmt.Errorf("something completely unexpected")
	action, _ := ClassifyError(err, 0)
	if action != ActionAbort {
		t.Errorf("expected ActionAbort, got %v", action)
	}
}

func TestModel_CalculateCost(t *testing.T) {
	model := KnownModels["claude-sonnet-4-20250514"]
	// 1000 input tokens at $3/1M = $0.003
	// 500 output tokens at $15/1M = $0.0075
	cost := model.CalculateCost(1000, 500)
	expected := 0.003 + 0.0075
	if cost < expected-0.0001 || cost > expected+0.0001 {
		t.Errorf("expected cost ~%f, got %f", expected, cost)
	}
}

func TestBackoff(t *testing.T) {
	d0 := backoff(0)
	if d0 != 2*time.Second {
		t.Errorf("expected 2s, got %v", d0)
	}
	d1 := backoff(1)
	if d1 != 4*time.Second {
		t.Errorf("expected 4s, got %v", d1)
	}
	d2 := backoff(2)
	if d2 != 8*time.Second {
		t.Errorf("expected 8s, got %v", d2)
	}
	// Should cap at 60s
	d10 := backoff(10)
	if d10 != 60*time.Second {
		t.Errorf("expected 60s cap, got %v", d10)
	}
}
