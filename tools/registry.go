package tools

import (
	"sort"

	"github.com/codeany-ai/open-agent-sdk-go/types"
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

// All returns all registered tools, sorted by name. The order is stable
// across calls so callers (such as the agent request builder) produce a
// deterministic tools list, which keeps the cacheable request prefix from
// shifting between requests.
func (r *Registry) All() []types.Tool {
	result := make([]types.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result
}

// Names returns all registered tool names, sorted for a stable order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
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
	store := NewTaskStore()
	todoStore := NewTodoStore()
	configStore := NewConfigStore()
	mailbox := NewMailbox()
	teamStore := NewTeamStore()
	planState := NewPlanModeState()
	worktreeStore := NewWorktreeStore()
	cronStore := NewCronStore()

	return []types.Tool{
		NewBashTool(),
		NewFileReadTool(),
		NewFileWriteTool(),
		NewFileEditTool(),
		NewGlobTool(),
		NewGrepTool(),
		NewWebFetchTool(),
		NewWebSearchTool(),
		&TaskCreateTool{Store: store},
		&TaskGetTool{Store: store},
		&TaskListTool{Store: store},
		&TaskUpdateTool{Store: store},
		&TaskStopTool{Store: store},
		&TaskOutputTool{Store: store},
		NewTodoWriteTool(todoStore),
		NewConfigTool(configStore),
		NewSendMessageTool(mailbox, "agent"),
		NewTeamCreateTool(teamStore),
		NewTeamDeleteTool(teamStore),
		NewEnterPlanModeTool(planState),
		NewExitPlanModeTool(planState),
		NewEnterWorktreeTool(worktreeStore),
		NewExitWorktreeTool(worktreeStore),
		NewListMcpResourcesTool(nil),
		NewReadMcpResourceTool(nil),
		NewCronCreateTool(cronStore),
		NewCronDeleteTool(cronStore),
		NewCronListTool(cronStore),
		NewRemoteTriggerTool(),
		NewNotebookEditTool(),
		NewLSPTool(),
	}
}
