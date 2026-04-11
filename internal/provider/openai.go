package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const openaiBaseURL = "https://api.openai.com"

// OpenAIProvider implements the Provider interface for OpenAI's models.
// Also serves as the base for OpenAI-compatible endpoints.
type OpenAIProvider struct {
	apiKey   string
	baseURL  string
	client   *http.Client
	id       string
	name     string
	models   []Model
}

// Compile-time interface check
var _ Provider = (*OpenAIProvider)(nil)

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = openaiBaseURL
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Minute},
		id:      "openai",
		name:    "OpenAI",
		models: []Model{
			KnownModels["gpt-4.1"],
			KnownModels["gpt-4.1-mini"],
			KnownModels["o3"],
			KnownModels["o4-mini"],
		},
	}
}

// NewOpenAICompatibleProvider creates a provider for any OpenAI-compatible API.
func NewOpenAICompatibleProvider(id, name, apiKey, baseURL string, models []Model) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Minute},
		id:      id,
		name:    name,
		models:  models,
	}
}

func (p *OpenAIProvider) ID() string   { return p.id }
func (p *OpenAIProvider) Name() string { return p.name }
func (p *OpenAIProvider) Models() []Model { return p.models }

func (p *OpenAIProvider) SupportsThinking() bool {
	for _, m := range p.models {
		if m.SupportsThinking {
			return true
		}
	}
	return false
}

func (p *OpenAIProvider) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// Chat sends a non-streaming chat completion request.
func (p *OpenAIProvider) Chat(ctx context.Context, params ChatParams) (*ChatResponse, error) {
	body := p.buildRequest(params, false)
	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var result openaiChatResponse
	if err := json.NewDecoder(respBody).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai: decoding response: %w", err)
	}

	return p.parseResponse(&result), nil
}

// Stream sends a streaming chat completion request.
func (p *OpenAIProvider) Stream(ctx context.Context, params ChatParams) (<-chan StreamEvent, error) {
	body := p.buildRequest(params, true)
	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamEvent, 8)
	go p.readStream(ctx, respBody, ch)
	return ch, nil
}

// buildRequest constructs the OpenAI Chat Completions API request.
func (p *OpenAIProvider) buildRequest(params ChatParams, stream bool) map[string]interface{} {
	messages := make([]map[string]interface{}, 0, len(params.Messages))

	for _, m := range params.Messages {
		msg := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}

		if m.Role == "tool" {
			msg["tool_call_id"] = m.ToolCallID
		}

		if len(m.ToolCalls) > 0 {
			toolCalls := make([]map[string]interface{}, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				toolCalls[i] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				}
			}
			msg["tool_calls"] = toolCalls
		}

		messages = append(messages, msg)
	}

	req := map[string]interface{}{
		"model":    params.Model,
		"messages": messages,
	}

	if params.MaxTokens > 0 {
		req["max_tokens"] = params.MaxTokens
	}
	if stream {
		req["stream"] = true
		req["stream_options"] = map[string]interface{}{
			"include_usage": true,
		}
	}
	if params.Temperature > 0 {
		req["temperature"] = params.Temperature
	}
	if len(params.Tools) > 0 {
		tools := make([]map[string]interface{}, len(params.Tools))
		for i, t := range params.Tools {
			tools[i] = map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.InputSchema,
				},
			}
		}
		req["tools"] = tools
	}

	return req
}

// doRequest makes an HTTP request to the OpenAI API.
func (p *OpenAIProvider) doRequest(ctx context.Context, body map[string]interface{}) (io.ReadCloser, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: marshaling request: %w", p.id, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("%s: creating request: %w", p.id, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.id,
			Message:  err.Error(),
			Err:      err,
		}
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &ProviderError{
			Provider:   p.id,
			StatusCode: resp.StatusCode,
			Message:    string(bodyBytes),
			RetryAfter: retryAfter,
		}
	}

	return resp.Body, nil
}

// readStream reads SSE events from an OpenAI streaming response.
func (p *OpenAIProvider) readStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	parser := NewSSEParser(body)

	// Track partial tool calls across chunks
	toolCalls := map[int]*ToolCall{}

	for {
		sse, err := parser.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- StreamEvent{Type: EventError, Error: err}
			return
		}

		if sse.Data == "[DONE]" {
			break
		}

		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(sse.Data), &chunk); err != nil {
			continue
		}

		// Handle usage in the final chunk
		if chunk.Usage.TotalTokens > 0 {
			ch <- StreamEvent{
				Type:         EventDone,
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
			continue
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			// Text content
			if delta.Content != "" {
				ch <- StreamEvent{Type: EventContent, Content: delta.Content}
			}

			// Reasoning (thinking)
			if delta.Reasoning != "" {
				ch <- StreamEvent{Type: EventThinking, Thinking: delta.Reasoning}
			}

			// Tool calls
			for _, tc := range delta.ToolCalls {
				existing, ok := toolCalls[tc.Index]
				if !ok {
					existing = &ToolCall{
						ID:   tc.ID,
						Name: tc.Function.Name,
					}
					toolCalls[tc.Index] = existing
				}
				existing.Arguments += tc.Function.Arguments

				// When finish_reason is "tool_calls", emit them
			}

			if choice.FinishReason == "tool_calls" || choice.FinishReason == "stop" {
				for _, tc := range toolCalls {
					if tc.Name != "" {
						ch <- StreamEvent{Type: EventToolCall, ToolCall: tc}
						ch <- StreamEvent{Type: EventToolDone}
					}
				}
				toolCalls = map[int]*ToolCall{}
			}
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (p *OpenAIProvider) parseResponse(resp *openaiChatResponse) *ChatResponse {
	cr := &ChatResponse{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		Model:        resp.Model,
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		cr.Content = choice.Message.Content
		cr.StopReason = choice.FinishReason

		for _, tc := range choice.Message.ToolCalls {
			cr.ToolCalls = append(cr.ToolCalls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}

	return cr
}

// OpenAI API types
type openaiChatResponse struct {
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
	Model   string         `json:"model"`
}

type openaiChoice struct {
	Message      openaiMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openaiMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openaiStreamChunk struct {
	Choices []openaiStreamChoice `json:"choices"`
	Usage   openaiUsage          `json:"usage,omitempty"`
}

type openaiStreamChoice struct {
	Delta        openaiStreamDelta `json:"delta"`
	FinishReason string            `json:"finish_reason"`
}

type openaiStreamDelta struct {
	Content   string                   `json:"content,omitempty"`
	Reasoning string                   `json:"reasoning,omitempty"`
	ToolCalls []openaiStreamToolCall   `json:"tool_calls,omitempty"`
}

type openaiStreamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}
