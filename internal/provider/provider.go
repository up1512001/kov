// Package provider implements the LLM provider abstraction layer.
// All providers implement the same interface, enabling seamless failover
// between cloud and local models.
package provider

import (
	"context"
	"fmt"
	"time"
)

// Provider is the core interface that all LLM providers implement.
// The router uses this to manage failover chains and health monitoring.
type Provider interface {
	// ID returns the unique provider identifier (e.g., "anthropic", "openai").
	ID() string

	// Name returns the human-friendly provider name.
	Name() string

	// Models returns the list of available models for this provider.
	Models() []Model

	// Chat sends a non-streaming chat request and returns the full response.
	Chat(ctx context.Context, params ChatParams) (*ChatResponse, error)

	// Stream sends a streaming chat request and returns a channel of events.
	// The channel is closed when the stream ends. Callers must read from
	// the channel until it closes or cancel the context.
	Stream(ctx context.Context, params ChatParams) (<-chan StreamEvent, error)

	// IsAvailable checks if the provider is reachable (health check).
	IsAvailable(ctx context.Context) bool

	// SupportsThinking returns true if the provider supports extended thinking.
	SupportsThinking() bool
}

// Model describes an LLM model's capabilities and pricing.
type Model struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Provider        string  `json:"provider"`
	ContextWindow   int     `json:"contextWindow"`
	MaxOutput       int     `json:"maxOutput"`
	InputCostPer1M  float64 `json:"inputCostPer1M"`  // USD per 1M input tokens
	OutputCostPer1M float64 `json:"outputCostPer1M"` // USD per 1M output tokens
	SupportsThinking bool   `json:"supportsThinking"`
	SupportsTools    bool   `json:"supportsTools"`
}

// CalculateCost returns the cost in USD for a given token count.
func (m Model) CalculateCost(inputTokens, outputTokens int) float64 {
	return (float64(inputTokens) * m.InputCostPer1M / 1_000_000) +
		(float64(outputTokens) * m.OutputCostPer1M / 1_000_000)
}

// ChatParams holds parameters for a chat/stream request.
type ChatParams struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	MaxTokens   int       `json:"maxTokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	TopP        float64   `json:"topP,omitempty"`
	Stop        []string  `json:"stop,omitempty"`

	// Thinking mode settings
	ThinkingEnabled bool   `json:"thinkingEnabled,omitempty"`
	ThinkingBudget  int    `json:"thinkingBudget,omitempty"` // max thinking tokens
}

// Message is a conversation message.
type Message struct {
	Role       string        `json:"role"`       // system, user, assistant, tool
	Content    string        `json:"content"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"` // for tool results
	Name       string        `json:"name,omitempty"`
}

// Tool defines a tool available to the LLM.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"` // JSON Schema
}

// ToolCall represents the LLM requesting a tool invocation.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ChatResponse is the full response from a non-streaming chat request.
type ChatResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"toolCalls,omitempty"`
	Thinking     string     `json:"thinking,omitempty"`
	InputTokens  int        `json:"inputTokens"`
	OutputTokens int        `json:"outputTokens"`
	Model        string     `json:"model"`
	StopReason   string     `json:"stopReason"`
}

// StreamEvent represents a single event in a streaming response.
type StreamEvent struct {
	Type         StreamEventType
	Content      string     // for EventContent
	Thinking     string     // for EventThinking
	ToolCall     *ToolCall  // for EventToolCall
	InputTokens  int        // for EventDone
	OutputTokens int        // for EventDone
	Error        error      // for EventError
}

// StreamEventType identifies the type of streaming event.
type StreamEventType int

const (
	EventContent   StreamEventType = iota // Token content delta
	EventThinking                         // Thinking content delta
	EventToolCall                         // Tool call request
	EventToolDone                         // Tool call fully received
	EventDone                             // Stream ended with usage
	EventError                            // Error occurred
)

// ProviderError wraps an error with provider-specific metadata.
type ProviderError struct {
	Provider   string
	StatusCode int
	Message    string
	Err        error
	RetryAfter time.Duration // from Retry-After header
}

func (e *ProviderError) Error() string {
	if e.StatusCode > 0 {
		return e.Provider + ": HTTP " + fmt.Sprint(e.StatusCode) + ": " + e.Message
	}
	return e.Provider + ": " + e.Message
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}
