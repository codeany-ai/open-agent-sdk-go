package tools

import (
	"github.com/shipany-ai/open-agent-sdk-go/types"
)

// Registry manages available tools.
type Registry struct {
	tools map[string]types.Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]types.Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool types.Tool) {
	r.tools[tool.Name()] = tool
}

// Get returns a tool by name, or nil if not found.
func (r *Registry) Get(name string) types.Tool {
	return r.tools[name]
}

// All returns all registered tools.
func (r *Registry) All() []types.Tool {
	result := make([]types.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// Names returns all registered tool names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Filter returns tools matching a filter function.
func (r *Registry) Filter(fn func(types.Tool) bool) []types.Tool {
	var result []types.Tool
	for _, t := range r.tools {
		if fn(t) {
			result = append(result, t)
		}
	}
	return result
}

// DefaultRegistry returns a registry with all built-in tools.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	for _, t := range GetAllBaseTools() {
		r.Register(t)
	}
	return r
}

// GetAllBaseTools returns all built-in tool implementations.
func GetAllBaseTools() []types.Tool {
	return []types.Tool{
		NewBashTool(),
		NewFileReadTool(),
		NewFileWriteTool(),
		NewFileEditTool(),
		NewGlobTool(),
		NewGrepTool(),
		NewWebFetchTool(),
		NewWebSearchTool(),
	}
}
