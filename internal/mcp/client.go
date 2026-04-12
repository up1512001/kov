// Package mcp implements Model Context Protocol (MCP) client support.
// MCP allows kov to connect to external tool servers, extending the
// agent's capabilities with project-specific or third-party tools.
//
// Spec: https://modelcontextprotocol.io
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Client manages a connection to an MCP server process.
type Client struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	mu      sync.Mutex
	nextID  atomic.Int64
	pending map[int64]chan *Response
	logger  *slog.Logger
	tools   []ToolDef
	closed  bool
}

// ServerConfig describes how to launch an MCP server.
type ServerConfig struct {
	Name    string            `json:"name" yaml:"name"`
	Command string            `json:"command" yaml:"command"`
	Args    []string          `json:"args" yaml:"args"`
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

// ToolDef is an MCP tool definition returned by the server.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Request is a JSON-RPC 2.0 request to the MCP server.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response from the MCP server.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// NewClient creates and starts an MCP server process.
func NewClient(cfg ServerConfig, logger *slog.Logger) (*Client, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)

	// Set environment variables
	if len(cfg.Env) > 0 {
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting MCP server %q: %w", cfg.Name, err)
	}

	c := &Client{
		name:    cfg.Name,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		pending: make(map[int64]chan *Response),
		logger:  logger,
	}

	// Start reading responses
	go c.readLoop()

	return c, nil
}

// Initialize sends the MCP initialize handshake.
func (c *Client) Initialize(ctx context.Context) error {
	resp, err := c.call(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":   map[string]interface{}{},
		"clientInfo": map[string]string{
			"name":    "kov",
			"version": "0.1.0",
		},
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	c.logger.Info("MCP server initialized",
		slog.String("server", c.name),
		slog.String("result", string(resp)))

	// Send initialized notification
	return c.notify("notifications/initialized", nil)
}

// ListTools fetches available tools from the server.
func (c *Client) ListTools(ctx context.Context) ([]ToolDef, error) {
	resp, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parsing tools list: %w", err)
	}

	c.tools = result.Tools
	c.logger.Info("MCP tools loaded",
		slog.String("server", c.name),
		slog.Int("count", len(result.Tools)))

	return result.Tools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	resp, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError,omitempty"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parsing tool result: %w", err)
	}

	// Collect text content
	var output string
	for _, c := range result.Content {
		if c.Type == "text" {
			output += c.Text
		}
	}

	if result.IsError {
		return output, fmt.Errorf("tool %s returned error: %s", name, output)
	}

	return output, nil
}

// Tools returns the cached tool definitions.
func (c *Client) Tools() []ToolDef {
	return c.tools
}

// Name returns the server name.
func (c *Client) Name() string {
	return c.name
}

// Close shuts down the MCP server process.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	c.stdin.Close()
	return c.cmd.Process.Kill()
}

// call sends a JSON-RPC request and waits for the response.
func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// Create response channel
	ch := make(chan *Response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	// Send request
	data, _ := json.Marshal(req)
	c.mu.Lock()
	_, err := fmt.Fprintf(c.stdin, "%s\n", data)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("writing request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(method string, params interface{}) error {
	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, _ := json.Marshal(req)
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := fmt.Fprintf(c.stdin, "%s\n", data)
	return err
}

// readLoop continuously reads JSON-RPC responses from stdout.
func (c *Client) readLoop() {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			if !c.closed {
				c.logger.Error("MCP read error",
					slog.String("server", c.name),
					slog.String("error", err.Error()))
			}
			return
		}

		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			c.logger.Warn("MCP invalid response",
				slog.String("server", c.name),
				slog.String("line", string(line)))
			continue
		}

		// Dispatch to pending request
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		c.mu.Unlock()
		if ok {
			ch <- &resp
		}
	}
}
