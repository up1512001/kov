package pipe

import (
	"encoding/json"
	"testing"
)

func TestResponse_JSON(t *testing.T) {
	resp := &Response{
		Success:     true,
		Content:     "Fixed the bug in auth.go",
		ToolCalls:   3,
		Cost:        0.0042,
		Model:       "claude-sonnet-4-20250514",
		FilesEdited: []string{"auth.go", "auth_test.go"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var parsed Response
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !parsed.Success {
		t.Error("expected success=true")
	}
	if parsed.ToolCalls != 3 {
		t.Errorf("expected 3 tool calls, got %d", parsed.ToolCalls)
	}
	if len(parsed.FilesEdited) != 2 {
		t.Errorf("expected 2 files, got %d", len(parsed.FilesEdited))
	}
}

func TestResponse_Error(t *testing.T) {
	resp := &Response{
		Success: false,
		Error:   "budget exceeded",
	}

	data, _ := json.Marshal(resp)
	var parsed Response
	json.Unmarshal(data, &parsed)

	if parsed.Success {
		t.Error("expected success=false")
	}
	if parsed.Error != "budget exceeded" {
		t.Errorf("expected 'budget exceeded', got %q", parsed.Error)
	}
}

func TestRequest_ParseJSON(t *testing.T) {
	input := `{"prompt":"fix auth","mode":"fast","model":"claude-haiku"}`
	var req Request
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if req.Prompt != "fix auth" {
		t.Errorf("expected 'fix auth', got %q", req.Prompt)
	}
	if req.Mode != "fast" {
		t.Errorf("expected 'fast', got %s", req.Mode)
	}
}
