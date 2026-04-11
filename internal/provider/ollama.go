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

const ollamaDefaultURL = "http://localhost:11434"

// OllamaProvider implements the Provider interface for local Ollama models.
// This is the emergency fallback — always available when cloud providers fail.
type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

var _ Provider = (*OllamaProvider)(nil)

func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = ollamaDefaultURL
	}
	if model == "" {
		model = "qwen3:8b"
	}
	return &OllamaProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 10 * time.Minute}, // Local models can be slow
	}
}

func (p *OllamaProvider) ID() string   { return "ollama" }
func (p *OllamaProvider) Name() string { return "Ollama (local)" }
func (p *OllamaProvider) SupportsThinking() bool { return false }

func (p *OllamaProvider) Models() []Model {
	return []Model{
		{
			ID: p.model, Name: p.model, Provider: "ollama",
			ContextWindow: 32768, MaxOutput: 8192,
			InputCostPer1M: 0, OutputCostPer1M: 0, // Free — local
			SupportsThinking: false, SupportsTools: true,
		},
	}
}

func (p *OllamaProvider) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// Chat sends a non-streaming request using Ollama's OpenAI-compatible endpoint.
func (p *OllamaProvider) Chat(ctx context.Context, params ChatParams) (*ChatResponse, error) {
	if params.Model == "" {
		params.Model = p.model
	}

	body := p.buildRequest(params, false)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("ollama: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &ProviderError{Provider: "ollama", Message: err.Error(), Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{
			Provider:   "ollama",
			StatusCode: resp.StatusCode,
			Message:    string(bodyBytes),
		}
	}

	// Ollama uses OpenAI-compatible response format
	var result openaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama: decoding response: %w", err)
	}

	cr := &ChatResponse{
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		Model:        result.Model,
	}
	if len(result.Choices) > 0 {
		cr.Content = result.Choices[0].Message.Content
		cr.StopReason = result.Choices[0].FinishReason
		for _, tc := range result.Choices[0].Message.ToolCalls {
			cr.ToolCalls = append(cr.ToolCalls, ToolCall{
				ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
			})
		}
	}
	return cr, nil
}

// Stream sends a streaming request using Ollama's OpenAI-compatible endpoint.
func (p *OllamaProvider) Stream(ctx context.Context, params ChatParams) (<-chan StreamEvent, error) {
	if params.Model == "" {
		params.Model = p.model
	}

	body := p.buildRequest(params, true)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("ollama: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &ProviderError{Provider: "ollama", Message: err.Error(), Err: err}
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{Provider: "ollama", StatusCode: resp.StatusCode, Message: string(bodyBytes)}
	}

	ch := make(chan StreamEvent, 8)
	go p.readStream(ctx, resp.Body, ch)
	return ch, nil
}

func (p *OllamaProvider) buildRequest(params ChatParams, stream bool) map[string]interface{} {
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
					"id": tc.ID, "type": "function",
					"function": map[string]interface{}{
						"name": tc.Name, "arguments": tc.Arguments,
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
		"stream":   stream,
	}
	if params.MaxTokens > 0 {
		req["max_tokens"] = params.MaxTokens
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
					"name": t.Name, "description": t.Description, "parameters": t.InputSchema,
				},
			}
		}
		req["tools"] = tools
	}
	return req
}

func (p *OllamaProvider) readStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	parser := NewSSEParser(body)
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

		if chunk.Usage.TotalTokens > 0 {
			ch <- StreamEvent{
				Type: EventDone,
				InputTokens: chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
			continue
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				ch <- StreamEvent{Type: EventContent, Content: choice.Delta.Content}
			}
			for _, tc := range choice.Delta.ToolCalls {
				existing, ok := toolCalls[tc.Index]
				if !ok {
					existing = &ToolCall{ID: tc.ID, Name: tc.Function.Name}
					toolCalls[tc.Index] = existing
				}
				existing.Arguments += tc.Function.Arguments
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
