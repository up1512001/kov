package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	anthropicBaseURL   = "https://api.anthropic.com"
	anthropicAPIVersion = "2023-06-01"
)

// AnthropicProvider implements the Provider interface for Anthropic's Claude models.
type AnthropicProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// Compile-time interface check
var _ Provider = (*AnthropicProvider)(nil)

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey, baseURL string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = anthropicBaseURL
	}
	return &AnthropicProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for thinking models
		},
	}
}

func (p *AnthropicProvider) ID() string   { return "anthropic" }
func (p *AnthropicProvider) Name() string { return "Anthropic" }

func (p *AnthropicProvider) Models() []Model {
	return []Model{
		KnownModels["claude-sonnet-4-20250514"],
		KnownModels["claude-opus-4-20250514"],
		KnownModels["claude-3-5-haiku-20241022"],
	}
}

func (p *AnthropicProvider) SupportsThinking() bool { return true }

func (p *AnthropicProvider) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// Chat sends a non-streaming request to the Anthropic Messages API.
func (p *AnthropicProvider) Chat(ctx context.Context, params ChatParams) (*ChatResponse, error) {
	body := p.buildRequest(params, false)
	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var result anthropicResponse
	if err := json.NewDecoder(respBody).Decode(&result); err != nil {
		return nil, fmt.Errorf("anthropic: decoding response: %w", err)
	}

	return p.parseResponse(&result), nil
}

// Stream sends a streaming request and returns a channel of events.
func (p *AnthropicProvider) Stream(ctx context.Context, params ChatParams) (<-chan StreamEvent, error) {
	body := p.buildRequest(params, true)
	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamEvent, 8)
	go p.readStream(ctx, respBody, ch)
	return ch, nil
}

// buildRequest constructs the Anthropic Messages API request body.
func (p *AnthropicProvider) buildRequest(params ChatParams, stream bool) map[string]interface{} {
	// Separate system message from conversation messages
	var systemPrompt string
	var messages []map[string]interface{}

	for _, m := range params.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}

		msg := map[string]interface{}{
			"role": m.Role,
		}

		// Handle tool results
		if m.Role == "tool" {
			msg["role"] = "user"
			msg["content"] = []map[string]interface{}{
				{
					"type":         "tool_result",
					"tool_use_id":  m.ToolCallID,
					"content":      m.Content,
				},
			}
		} else if len(m.ToolCalls) > 0 {
			// Assistant message with tool calls
			content := []map[string]interface{}{}
			if m.Content != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				var args interface{}
				json.Unmarshal([]byte(tc.Arguments), &args)
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": args,
				})
			}
			msg["content"] = content
		} else {
			msg["content"] = m.Content
		}

		messages = append(messages, msg)
	}

	req := map[string]interface{}{
		"model":      params.Model,
		"messages":   messages,
		"max_tokens": maxTokensOrDefault(params.MaxTokens, 16384),
	}

	if systemPrompt != "" {
		req["system"] = systemPrompt
	}
	if stream {
		req["stream"] = true
	}
	if params.Temperature > 0 {
		req["temperature"] = params.Temperature
	}
	if len(params.Tools) > 0 {
		req["tools"] = convertTools(params.Tools)
	}
	if params.ThinkingEnabled {
		req["thinking"] = map[string]interface{}{
			"type":         "enabled",
			"budget_tokens": params.ThinkingBudget,
		}
		// Remove max_tokens when thinking is enabled (Anthropic requires this)
		delete(req, "max_tokens")
	}

	return req
}

// doRequest makes an HTTP request to the Anthropic API.
func (p *AnthropicProvider) doRequest(ctx context.Context, body map[string]interface{}) (io.ReadCloser, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("anthropic: creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &ProviderError{
			Provider: "anthropic",
			Message:  err.Error(),
			Err:      err,
		}
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &ProviderError{
			Provider:   "anthropic",
			StatusCode: resp.StatusCode,
			Message:    string(body),
			RetryAfter: retryAfter,
		}
	}

	return resp.Body, nil
}

// readStream reads SSE events from an Anthropic streaming response.
func (p *AnthropicProvider) readStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	parser := NewSSEParser(body)
	var currentToolCall *ToolCall

	for {
		sse, err := parser.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- StreamEvent{Type: EventError, Error: err}
			return
		}

		// Skip [DONE] sentinel
		if sse.Data == "[DONE]" {
			break
		}

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(sse.Data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta.Type == "text_delta" {
				ch <- StreamEvent{Type: EventContent, Content: event.Delta.Text}
			} else if event.Delta.Type == "thinking_delta" {
				ch <- StreamEvent{Type: EventThinking, Thinking: event.Delta.Thinking}
			} else if event.Delta.Type == "input_json_delta" && currentToolCall != nil {
				currentToolCall.Arguments += event.Delta.PartialJSON
			}

		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				currentToolCall = &ToolCall{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				}
			}

		case "content_block_stop":
			if currentToolCall != nil {
				ch <- StreamEvent{Type: EventToolCall, ToolCall: currentToolCall}
				ch <- StreamEvent{Type: EventToolDone}
				currentToolCall = nil
			}

		case "message_delta":
			// End of stream with usage
			ch <- StreamEvent{
				Type:         EventDone,
				OutputTokens: event.Usage.OutputTokens,
			}

		case "message_start":
			// Contains input tokens
			if event.Message.Usage.InputTokens > 0 {
				ch <- StreamEvent{
					Type:        EventDone,
					InputTokens: event.Message.Usage.InputTokens,
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// Anthropic API types
type anthropicResponse struct {
	Content  []anthropicContent `json:"content"`
	Usage    anthropicUsage     `json:"usage"`
	Model    string             `json:"model"`
	StopReason string           `json:"stop_reason"`
}

type anthropicContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Thinking string `json:"thinking,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicStreamEvent struct {
	Type         string                  `json:"type"`
	Delta        anthropicDelta          `json:"delta,omitempty"`
	ContentBlock anthropicContent        `json:"content_block,omitempty"`
	Usage        anthropicUsage          `json:"usage,omitempty"`
	Message      struct {
		Usage anthropicUsage `json:"usage"`
	} `json:"message,omitempty"`
}

type anthropicDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

func (p *AnthropicProvider) parseResponse(resp *anthropicResponse) *ChatResponse {
	cr := &ChatResponse{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		Model:        resp.Model,
		StopReason:   resp.StopReason,
	}

	for _, c := range resp.Content {
		switch c.Type {
		case "text":
			cr.Content += c.Text
		case "thinking":
			cr.Thinking += c.Thinking
		case "tool_use":
			cr.ToolCalls = append(cr.ToolCalls, ToolCall{
				ID:        c.ID,
				Name:      c.Name,
				Arguments: string(c.Input),
			})
		}
	}

	return cr
}

// Helper functions

func convertTools(tools []Tool) []map[string]interface{} {
	result := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		result[i] = map[string]interface{}{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.InputSchema,
		}
	}
	return result
}

func maxTokensOrDefault(maxTokens, defaultMax int) int {
	if maxTokens > 0 {
		return maxTokens
	}
	return defaultMax
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	seconds, err := strconv.Atoi(header)
	if err != nil {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
