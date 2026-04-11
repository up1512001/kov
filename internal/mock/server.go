// Package mock provides a mock LLM server for integration testing.
// It simulates provider responses including streaming, tool calls,
// and various error scenarios.
package mock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// Server is a mock LLM API server for testing.
type Server struct {
	server    *httptest.Server
	mu        sync.Mutex
	responses []Response
	calls     []Call
	errorMode ErrorMode
	delay     time.Duration
}

// Response configures what the mock server returns.
type Response struct {
	Content   string
	ToolCalls []ToolCall
	Tokens    TokenUsage
	Error     *ErrorResponse
}

// ToolCall represents a tool call in the response.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// TokenUsage tracks token counts.
type TokenUsage struct {
	Input  int `json:"input_tokens"`
	Output int `json:"output_tokens"`
}

// ErrorResponse simulates an API error.
type ErrorResponse struct {
	StatusCode int
	Message    string
	Type       string
}

// ErrorMode controls error simulation.
type ErrorMode int

const (
	ErrorNone        ErrorMode = iota
	Error429RateLimit          // 429 Too Many Requests
	Error401Auth               // 401 Unauthorized
	Error500Server             // 500 Internal Server Error
	ErrorTimeout               // Connection timeout
	ErrorPartialStream         // Stream cuts off mid-response
)

// Call records a request to the mock server.
type Call struct {
	Method    string
	Path      string
	Body      string
	Timestamp time.Time
}

// NewServer creates a new mock LLM server.
func NewServer() *Server {
	s := &Server{}
	s.server = httptest.NewServer(http.HandlerFunc(s.handler))
	return s
}

// URL returns the server's URL.
func (s *Server) URL() string {
	return s.server.URL
}

// Close shuts down the server.
func (s *Server) Close() {
	s.server.Close()
}

// SetResponse configures the next response.
func (s *Server) SetResponse(resp Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = append(s.responses, resp)
}

// SetError sets the error mode.
func (s *Server) SetError(mode ErrorMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorMode = mode
}

// SetDelay adds artificial latency.
func (s *Server) SetDelay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delay = d
}

// Calls returns all recorded calls.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call{}, s.calls...)
}

// Reset clears all state.
func (s *Server) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = nil
	s.calls = nil
	s.errorMode = ErrorNone
	s.delay = 0
}

func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	// Record the call
	body := ""
	if r.Body != nil {
		buf := make([]byte, 1024*64)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
		r.Body.Close()
	}

	s.mu.Lock()
	s.calls = append(s.calls, Call{
		Method:    r.Method,
		Path:      r.URL.Path,
		Body:      body,
		Timestamp: time.Now(),
	})

	delay := s.delay
	errMode := s.errorMode

	var resp *Response
	if len(s.responses) > 0 {
		resp = &s.responses[0]
		s.responses = s.responses[1:]
	}
	s.mu.Unlock()

	// Apply delay
	if delay > 0 {
		time.Sleep(delay)
	}

	// Handle error modes
	switch errMode {
	case Error429RateLimit:
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"error":{"type":"rate_limit_error","message":"Rate limited"}}`, 429)
		return
	case Error401Auth:
		http.Error(w, `{"error":{"type":"authentication_error","message":"Invalid API key"}}`, 401)
		return
	case Error500Server:
		http.Error(w, `{"error":{"type":"server_error","message":"Internal server error"}}`, 500)
		return
	case ErrorTimeout:
		time.Sleep(30 * time.Second) // Will be cancelled by client timeout
		return
	case ErrorPartialStream:
		// Send partial SSE then disconnect
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"partial\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		return // Connection closes, no done event
	}

	// Default response
	if resp == nil {
		resp = &Response{
			Content: "I'll help you with that.",
			Tokens:  TokenUsage{Input: 100, Output: 50},
		}
	}

	// Stream SSE response (Anthropic format)
	if strings.Contains(r.URL.Path, "messages") || strings.Contains(r.URL.Path, "chat") {
		s.streamAnthropicResponse(w, resp)
		return
	}

	// Plain JSON fallback
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) streamAnthropicResponse(w http.ResponseWriter, resp *Response) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)

	// Message start
	fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_mock\",\"role\":\"assistant\",\"model\":\"mock\",\"usage\":{\"input_tokens\":%d}}}\n\n", resp.Tokens.Input)
	if ok {
		flusher.Flush()
	}

	// Content
	if resp.Content != "" {
		fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		if ok {
			flusher.Flush()
		}

		// Stream tokens
		words := strings.Fields(resp.Content)
		for i, word := range words {
			sep := " "
			if i == 0 {
				sep = ""
			}
			fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"%s%s\"}}\n\n", sep, word)
			if ok {
				flusher.Flush()
			}
		}

		fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		if ok {
			flusher.Flush()
		}
	}

	// Tool calls
	for i, tc := range resp.ToolCalls {
		idx := i + 1
		fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":%d,\"content_block\":{\"type\":\"tool_use\",\"id\":\"%s\",\"name\":\"%s\",\"input\":{}}}\n\n", idx, tc.ID, tc.Name)
		fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":%d,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"%s\"}}\n\n", idx, escapeJSON(tc.Arguments))
		fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", idx)
		if ok {
			flusher.Flush()
		}
	}

	// Message delta (usage)
	fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":%d}}\n\n", resp.Tokens.Output)

	// Done
	fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	if ok {
		flusher.Flush()
	}
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
