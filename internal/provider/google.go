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

const googleBaseURL = "https://generativelanguage.googleapis.com"

// GoogleProvider implements the Provider interface for Google's Gemini models.
type GoogleProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

var _ Provider = (*GoogleProvider)(nil)

func NewGoogleProvider(apiKey, baseURL string) *GoogleProvider {
	if baseURL == "" {
		baseURL = googleBaseURL
	}
	return &GoogleProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

func (p *GoogleProvider) ID() string   { return "google" }
func (p *GoogleProvider) Name() string { return "Google" }
func (p *GoogleProvider) SupportsThinking() bool { return true }

func (p *GoogleProvider) Models() []Model {
	return []Model{
		KnownModels["gemini-2.5-pro"],
		KnownModels["gemini-2.5-flash"],
	}
}

func (p *GoogleProvider) IsAvailable(ctx context.Context) bool {
	url := fmt.Sprintf("%s/v1beta/models?key=%s", p.baseURL, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

func (p *GoogleProvider) Chat(ctx context.Context, params ChatParams) (*ChatResponse, error) {
	body := p.buildRequest(params)
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", p.baseURL, params.Model, p.apiKey)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("google: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("google: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &ProviderError{Provider: "google", Message: err.Error(), Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{
			Provider:   "google",
			StatusCode: resp.StatusCode,
			Message:    string(bodyBytes),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}

	var result geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("google: decoding response: %w", err)
	}

	return p.parseResponse(&result), nil
}

func (p *GoogleProvider) Stream(ctx context.Context, params ChatParams) (<-chan StreamEvent, error) {
	body := p.buildRequest(params)
	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", p.baseURL, params.Model, p.apiKey)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("google: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("google: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &ProviderError{Provider: "google", Message: err.Error(), Err: err}
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{
			Provider:   "google",
			StatusCode: resp.StatusCode,
			Message:    string(bodyBytes),
		}
	}

	ch := make(chan StreamEvent, 8)
	go p.readStream(ctx, resp.Body, ch)
	return ch, nil
}

func (p *GoogleProvider) buildRequest(params ChatParams) map[string]interface{} {
	var contents []map[string]interface{}
	var systemInstruction map[string]interface{}

	for _, m := range params.Messages {
		if m.Role == "system" {
			systemInstruction = map[string]interface{}{
				"parts": []map[string]interface{}{
					{"text": m.Content},
				},
			}
			continue
		}

		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		if role == "tool" {
			// Tool results in Gemini format
			var result interface{}
			json.Unmarshal([]byte(m.Content), &result)
			if result == nil {
				result = map[string]interface{}{"result": m.Content}
			}
			contents = append(contents, map[string]interface{}{
				"role": "function",
				"parts": []map[string]interface{}{
					{
						"functionResponse": map[string]interface{}{
							"name":     m.Name,
							"response": result,
						},
					},
				},
			})
			continue
		}

		parts := []map[string]interface{}{}
		if m.Content != "" {
			parts = append(parts, map[string]interface{}{"text": m.Content})
		}
		for _, tc := range m.ToolCalls {
			var args interface{}
			json.Unmarshal([]byte(tc.Arguments), &args)
			parts = append(parts, map[string]interface{}{
				"functionCall": map[string]interface{}{
					"name": tc.Name,
					"args": args,
				},
			})
		}

		contents = append(contents, map[string]interface{}{
			"role":  role,
			"parts": parts,
		})
	}

	req := map[string]interface{}{
		"contents": contents,
	}

	if systemInstruction != nil {
		req["systemInstruction"] = systemInstruction
	}

	// Generation config
	genConfig := map[string]interface{}{}
	if params.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = params.MaxTokens
	}
	if params.Temperature > 0 {
		genConfig["temperature"] = params.Temperature
	}
	if params.ThinkingEnabled {
		genConfig["thinkingConfig"] = map[string]interface{}{
			"thinkingBudget": params.ThinkingBudget,
		}
	}
	if len(genConfig) > 0 {
		req["generationConfig"] = genConfig
	}

	// Tools
	if len(params.Tools) > 0 {
		funcs := make([]map[string]interface{}, len(params.Tools))
		for i, t := range params.Tools {
			funcs[i] = map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			}
		}
		req["tools"] = []map[string]interface{}{
			{"functionDeclarations": funcs},
		}
	}

	return req
}

func (p *GoogleProvider) readStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	parser := NewSSEParser(body)

	for {
		sse, err := parser.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- StreamEvent{Type: EventError, Error: err}
			return
		}

		var chunk geminiResponse
		if err := json.Unmarshal([]byte(sse.Data), &chunk); err != nil {
			continue
		}

		for _, candidate := range chunk.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					ch <- StreamEvent{Type: EventContent, Content: part.Text}
				}
				if part.Thought != "" {
					ch <- StreamEvent{Type: EventThinking, Thinking: part.Thought}
				}
				if part.FunctionCall != nil {
					args, _ := json.Marshal(part.FunctionCall.Args)
					ch <- StreamEvent{
						Type: EventToolCall,
						ToolCall: &ToolCall{
							ID:        part.FunctionCall.Name,
							Name:      part.FunctionCall.Name,
							Arguments: string(args),
						},
					}
					ch <- StreamEvent{Type: EventToolDone}
				}
			}
		}

		if chunk.UsageMetadata.TotalTokenCount > 0 {
			ch <- StreamEvent{
				Type:         EventDone,
				InputTokens:  chunk.UsageMetadata.PromptTokenCount,
				OutputTokens: chunk.UsageMetadata.CandidatesTokenCount,
			}
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (p *GoogleProvider) parseResponse(resp *geminiResponse) *ChatResponse {
	cr := &ChatResponse{
		InputTokens:  resp.UsageMetadata.PromptTokenCount,
		OutputTokens: resp.UsageMetadata.CandidatesTokenCount,
	}

	for _, candidate := range resp.Candidates {
		cr.StopReason = candidate.FinishReason
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				cr.Content += part.Text
			}
			if part.Thought != "" {
				cr.Thinking += part.Thought
			}
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				cr.ToolCalls = append(cr.ToolCalls, ToolCall{
					ID:        part.FunctionCall.Name,
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				})
			}
		}
	}

	return cr
}

// Gemini API types
type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata geminiUsage       `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role"`
}

type geminiPart struct {
	Text         string             `json:"text,omitempty"`
	Thought      string             `json:"thought,omitempty"`
	FunctionCall *geminiFunctionCall `json:"functionCall,omitempty"`
}

type geminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}
