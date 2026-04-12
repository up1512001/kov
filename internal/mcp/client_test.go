package mcp

import (
	"encoding/json"
	"testing"
)

func TestServerConfig_Fields(t *testing.T) {
	cfg := ServerConfig{
		Name:    "test",
		Command: "npx",
		Args:    []string{"-y", "mcp-server-test"},
		Env:     map[string]string{"KEY": "value"},
	}

	if cfg.Name != "test" {
		t.Errorf("expected name test, got %s", cfg.Name)
	}
	if cfg.Command != "npx" {
		t.Errorf("expected command npx, got %s", cfg.Command)
	}
	if len(cfg.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(cfg.Args))
	}
}

func TestToolDef_JSON(t *testing.T) {
	td := ToolDef{
		Name:        "read_file",
		Description: "Read a file",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
	}

	data, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed ToolDef
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Name != "read_file" {
		t.Errorf("expected read_file, got %s", parsed.Name)
	}
	if parsed.Description != "Read a file" {
		t.Errorf("expected 'Read a file', got %s", parsed.Description)
	}
}

func TestRequest_JSON(t *testing.T) {
	req := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed Request
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.JSONRPC != "2.0" {
		t.Errorf("expected 2.0, got %s", parsed.JSONRPC)
	}
	if parsed.ID != 1 {
		t.Errorf("expected id 1, got %d", parsed.ID)
	}
	if parsed.Method != "tools/list" {
		t.Errorf("expected tools/list, got %s", parsed.Method)
	}
}

func TestResponse_WithResult(t *testing.T) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(`{"tools":[]}`),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed Response
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Error != nil {
		t.Error("expected no error")
	}
	if string(parsed.Result) != `{"tools":[]}` {
		t.Errorf("unexpected result: %s", string(parsed.Result))
	}
}

func TestResponse_WithError(t *testing.T) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      1,
		Error: &RPCError{
			Code:    -32600,
			Message: "Invalid Request",
		},
	}

	if resp.Error.Error() != "MCP error -32600: Invalid Request" {
		t.Errorf("unexpected error string: %s", resp.Error.Error())
	}
}

func TestRPCError_Error(t *testing.T) {
	e := &RPCError{Code: -32601, Message: "Method not found"}
	if e.Error() != "MCP error -32601: Method not found" {
		t.Errorf("unexpected: %s", e.Error())
	}
}

func TestClient_Fields(t *testing.T) {
	// Test that Client struct compiles and has expected methods
	c := &Client{
		name:    "test",
		pending: make(map[int64]chan *Response),
	}

	if c.Name() != "test" {
		t.Errorf("expected name test, got %s", c.Name())
	}
	if c.Tools() != nil {
		t.Error("expected nil tools before initialization")
	}
}
