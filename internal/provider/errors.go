package provider

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// FailureAction determines what the resilience engine should do with an error.
type FailureAction int

const (
	// ActionRetry means backoff and retry the same provider.
	ActionRetry FailureAction = iota
	// ActionFailover means switch to the next provider in the chain.
	ActionFailover
	// ActionPause means checkpoint state and wait for user intervention.
	ActionPause
	// ActionAbort means the error is non-recoverable.
	ActionAbort
	// ActionCompact means the context is too large, compact and retry.
	ActionCompact
)

// String returns a human-readable name for the action.
func (a FailureAction) String() string {
	switch a {
	case ActionRetry:
		return "retry"
	case ActionFailover:
		return "failover"
	case ActionPause:
		return "pause"
	case ActionAbort:
		return "abort"
	case ActionCompact:
		return "compact"
	default:
		return fmt.Sprintf("unknown(%d)", int(a))
	}
}

// ClassifyError determines the appropriate resilience action for an error.
// This is the brain of the resilience engine — every user-reported failure
// scenario maps to a specific recovery path.
func ClassifyError(err error, retryCount int) (FailureAction, time.Duration) {
	if err == nil {
		return ActionAbort, 0 // should not happen
	}

	// Check for ProviderError with HTTP status
	var pe *ProviderError
	if asProviderError(err, &pe) {
		return classifyHTTPError(pe, retryCount)
	}

	// Network errors → immediate failover
	if isNetworkError(err) {
		return ActionFailover, 0
	}

	// Timeout → retry once, then failover
	if isTimeout(err) {
		if retryCount < 1 {
			return ActionRetry, 5 * time.Second
		}
		return ActionFailover, 0
	}

	// Unknown error → abort
	return ActionAbort, 0
}

// classifyHTTPError handles ProviderError with HTTP status codes.
func classifyHTTPError(pe *ProviderError, retryCount int) (FailureAction, time.Duration) {
	switch {
	// 429: Rate limited — exponential backoff then failover
	case pe.StatusCode == 429:
		if pe.RetryAfter > 0 {
			if retryCount < 3 {
				return ActionRetry, pe.RetryAfter
			}
		} else {
			if retryCount < 3 {
				return ActionRetry, backoff(retryCount)
			}
		}
		return ActionFailover, 0

	// 401/403: Auth error — pause for re-authentication
	case pe.StatusCode == 401, pe.StatusCode == 403:
		return ActionPause, 0

	// 400: Bad request — check for context too long
	case pe.StatusCode == 400:
		msg := strings.ToLower(pe.Message)
		if strings.Contains(msg, "context") || strings.Contains(msg, "token") ||
			strings.Contains(msg, "too long") || strings.Contains(msg, "max_tokens") {
			return ActionCompact, 0
		}
		return ActionAbort, 0

	// 402: Payment required — pause
	case pe.StatusCode == 402:
		return ActionPause, 0

	// 5xx: Server error — retry then failover
	case pe.StatusCode >= 500:
		if retryCount < 2 {
			return ActionRetry, backoff(retryCount)
		}
		return ActionFailover, 0

	// 408: Timeout
	case pe.StatusCode == 408:
		if retryCount < 1 {
			return ActionRetry, 5 * time.Second
		}
		return ActionFailover, 0

	default:
		return ActionAbort, 0
	}
}

// backoff returns exponential backoff duration with jitter.
func backoff(retryCount int) time.Duration {
	base := 2 * time.Second
	for i := 0; i < retryCount; i++ {
		base *= 2
	}
	if base > 60*time.Second {
		base = 60 * time.Second
	}
	return base
}

// isNetworkError checks if the error is a network-level failure.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var netErr *net.OpError
	if asNetOpError(err, &netErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "connection reset")
}

// isTimeout checks if the error is a timeout.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	type timeouter interface{ Timeout() bool }
	if t, ok := err.(timeouter); ok {
		return t.Timeout()
	}
	msg := err.Error()
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded")
}

// Helper functions using errors.As patterns
func asProviderError(err error, target **ProviderError) bool {
	for err != nil {
		if pe, ok := err.(*ProviderError); ok {
			*target = pe
			return true
		}
		if u, ok := err.(interface{ Unwrap() error }); ok {
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}

func asNetOpError(err error, target **net.OpError) bool {
	for err != nil {
		if ne, ok := err.(*net.OpError); ok {
			*target = ne
			return true
		}
		if u, ok := err.(interface{ Unwrap() error }); ok {
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}
