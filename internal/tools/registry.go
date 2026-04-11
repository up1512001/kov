// Package tools implements the agent's tool system.
// Each tool has a name, description, input schema, and an Execute method.
// Tools are registered in a Registry and resolved by name during the agent loop.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Tool is the interface that all agent tools implement.
type Tool interface {
	// Name returns the unique tool identifier.
	Name() string

	// Description returns a human-readable description for the LLM.
	Description() string

	// InputSchema returns the JSON Schema for tool arguments.
	InputSchema() interface{}

	// Execute runs the tool with the given JSON arguments.
	// Returns the result as a string (which gets sent back to the LLM).
	Execute(ctx context.Context, args json.RawMessage) (string, error)

	// NeedsPermission returns true if this tool requires user confirmation.
	NeedsPermission() bool

	// Category returns the tool category for permission grouping.
	Category() ToolCategory
}

// ToolCategory groups tools by risk level.
type ToolCategory int

const (
	CategoryRead    ToolCategory = iota // file_read, search — safe
	CategoryWrite                       // file_write, file_edit — modifies files
	CategoryExecute                     // shell — runs commands
)

func (c ToolCategory) String() string {
	switch c {
	case CategoryRead:
		return "read"
	case CategoryWrite:
		return "write"
	case CategoryExecute:
		return "execute"
	default:
		return "unknown"
	}
}

// Registry holds all registered tools.
type Registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// All returns all registered tools.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// ForProvider converts all tools to provider.Tool format for the LLM.
func (r *Registry) ForProvider() []ProviderTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ProviderTool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, ProviderTool{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	return result
}

// ProviderTool matches the provider.Tool format.
type ProviderTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

// Execute runs a tool by name with JSON arguments.
func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	tool, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}
	return tool.Execute(ctx, args)
}

// DefaultRegistry creates a registry with all built-in tools.
func DefaultRegistry(projectDir string) *Registry {
	reg := NewRegistry()
	reg.Register(NewFileRead(projectDir))
	reg.Register(NewFileWrite(projectDir))
	reg.Register(NewFileEdit(projectDir))
	reg.Register(NewShellExec(projectDir))
	reg.Register(NewGrepSearch(projectDir))
	reg.Register(NewGlobSearch(projectDir))
	reg.Register(NewListDir(projectDir))
	return reg
}
