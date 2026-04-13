// Package tools — MCP tool adapter.
// Wraps MCP server tools as native kov tools so they appear in the agent's
// tool registry alongside built-in tools.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/up1512001/kov/internal/mcp"
)

// MCPTool wraps an MCP server tool definition as a native kov tool.
type MCPTool struct {
	client   *mcp.Client
	def      mcp.ToolDef
	category ToolCategory
}

// NewMCPTool creates a kov tool that delegates to an MCP server.
func NewMCPTool(client *mcp.Client, def mcp.ToolDef) *MCPTool {
	return &MCPTool{
		client:   client,
		def:      def,
		category: CategoryExecute, // MCP tools are treated as execute-level (need permission)
	}
}

func (t *MCPTool) Name() string        { return fmt.Sprintf("mcp_%s_%s", t.client.Name(), t.def.Name) }
func (t *MCPTool) Description() string  { return t.def.Description }
func (t *MCPTool) NeedsPermission() bool { return true }
func (t *MCPTool) Category() ToolCategory { return t.category }

func (t *MCPTool) InputSchema() interface{} {
	var schema interface{}
	if err := json.Unmarshal(t.def.InputSchema, &schema); err != nil {
		return map[string]interface{}{"type": "object"}
	}
	return schema
}

func (t *MCPTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.client.CallTool(ctx, t.def.Name, args)
}

// RegisterMCPTools connects to configured MCP servers and registers their
// tools into the given registry. Returns the clients for cleanup.
func RegisterMCPTools(registry *Registry, servers map[string]mcp.ServerConfig, logger interface{ Info(string, ...interface{}) }) []*mcp.Client {
	var clients []*mcp.Client

	for _, serverCfg := range servers {
		client, err := mcp.NewClient(serverCfg, nil)
		if err != nil {
			continue
		}

		ctx := context.Background()
		if err := client.Initialize(ctx); err != nil {
			client.Close()
			continue
		}

		tools, err := client.ListTools(ctx)
		if err != nil {
			client.Close()
			continue
		}

		for _, toolDef := range tools {
			registry.Register(NewMCPTool(client, toolDef))
		}

		clients = append(clients, client)
	}

	return clients
}
