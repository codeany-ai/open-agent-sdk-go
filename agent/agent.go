package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/codeany-ai/open-agent-sdk-go/api"
	"github.com/codeany-ai/open-agent-sdk-go/costtracker"
	"github.com/codeany-ai/open-agent-sdk-go/hooks"
	"github.com/codeany-ai/open-agent-sdk-go/mcp"
	"github.com/codeany-ai/open-agent-sdk-go/permissions"
	"github.com/codeany-ai/open-agent-sdk-go/tools"
	"github.com/codeany-ai/open-agent-sdk-go/types"
)

const (
	defaultMaxTurns = 100
)

// ThinkingConfig configures extended thinking.
type ThinkingConfig struct {
	// Type must be "enabled" to activate thinking.
	Type string `json:"type"`
	// BudgetTokens is the max number of thinking tokens.
	BudgetTokens int `json:"budget_tokens"`
}

// Options configures an Agent.
type Options struct {
	// Model ID (e.g. "sonnet-4-6")
	Model string

	// API key
	APIKey string

	// API base URL override
	BaseURL string

	// Working directory for tools
	CWD string

	// System prompt override
	SystemPrompt string

	// Append to default system prompt
	AppendSystemPrompt string

	// Maximum agentic turns per query
	MaxTurns int

	// Maximum USD budget per query
	MaxBudgetUSD float64

	// Permission mode
	PermissionMode types.PermissionMode

	// Tool names to pre-approve
	AllowedTools []string

	// Permission handler callback
	CanUseTool types.CanUseToolFn

	// MCP server configurations
	MCPServers map[string]types.MCPServerConfig

	// Custom tools to add
	CustomTools []types.Tool

	// Hook configuration
	Hooks hooks.HookConfig

	// Environment variables (for API key, model, etc.)
	Env map[string]string

	// Extended thinking configuration
	Thinking *ThinkingConfig

	// Structured output JSON schema name and schema
	JSONSchema map[string]interface{}

	// Custom HTTP headers
	CustomHeaders map[string]string

	// Proxy URL for API requests
	ProxyURL string

	// API timeout in milliseconds
	TimeoutMs int

	// Subagent definitions
	Agents map[string]AgentDefinition
}

// AgentDefinition defines a subagent configuration.
type AgentDefinition struct {
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Tools        []string `json:"tools,omitempty"`
	Model        string   `json:"model,omitempty"`
}

// Agent is the main agent that runs the agentic loop.
type Agent struct {
	opts         Options
	apiClient    *api.Client
	toolRegistry *tools.Registry
	mcpClient    *mcp.Client
	costTracker  *costtracker.Tracker
	hookManager  *hooks.Manager
	canUseTool   types.CanUseToolFn
	messages     []types.Message
	sessionID    string
}

// New creates a new Agent.
func New(opts Options) *Agent {
	resolveEnvOptions(&opts)

	sessionID := uuid.New().String()

	apiClient := api.NewClient(api.ClientConfig{
		APIKey:        opts.APIKey,
		BaseURL:       opts.BaseURL,
		Model:         opts.Model,
		CustomHeaders: opts.CustomHeaders,
		ProxyURL:      opts.ProxyURL,
		TimeoutMs:     opts.TimeoutMs,
	})

	registry := tools.DefaultRegistry()
	for _, t := range opts.CustomTools {
		registry.Register(t)
	}

	permConfig := &permissions.Config{Mode: opts.PermissionMode}
	if permConfig.Mode == "" {
		permConfig.Mode = types.PermissionModeBypassPermissions
	}
	canUseTool := opts.CanUseTool
	if canUseTool == nil {
		canUseTool = permissions.NewCanUseToolFn(permConfig, opts.AllowedTools)
	}

	hookManager := hooks.NewManager(opts.Hooks)

	a := &Agent{
		opts:         opts,
		apiClient:    apiClient,
		toolRegistry: registry,
		mcpClient:    mcp.NewClient(),
		costTracker:  costtracker.NewTracker(sessionID),
		hookManager:  hookManager,
		canUseTool:   canUseTool,
		sessionID:    sessionID,
	}

	// Register AgentTool with subagent spawner if definitions provided
	if len(opts.Agents) > 0 {
		defs := make(map[string]tools.SubagentDefinition, len(opts.Agents))
		for name, def := range opts.Agents {
			defs[name] = tools.SubagentDefinition{
				Description:  def.Description,
				Instructions: def.Instructions,
				Tools:        def.Tools,
				Model:        def.Model,
			}
		}
		agentTool := tools.NewAgentTool(defs, a.spawnSubagent)
		registry.Register(agentTool)
	} else {
		// Register with default agent types even if none configured
		defaultDefs := map[string]tools.SubagentDefinition{
			"general-purpose": {Description: "General-purpose agent for complex multi-step tasks"},
			"explore":         {Description: "Fast agent for codebase exploration and search"},
			"plan":            {Description: "Planning agent for designing implementation strategies"},
		}
		agentTool := tools.NewAgentTool(defaultDefs, a.spawnSubagent)
		registry.Register(agentTool)
	}

	return a
}

// Init performs async initialization (MCP connections, etc.)
func (a *Agent) Init(ctx context.Context) error {
	if a.opts.MCPServers == nil {
		return nil
	}

	for name, config := range a.opts.MCPServers {
		conn, err := a.mcpClient.ConnectServer(ctx, name, config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[MCP] Failed to connect to %q: %v\n", name, err)
			continue
		}

		mcpTools := mcp.ToolsFromConnection(conn)
		for _, t := range mcpTools {
			a.toolRegistry.Register(t)
		}
	}

	return nil
}

// QueryResult is the final result of a query.
type QueryResult struct {
	Text     string          `json:"text"`
	Usage    types.Usage     `json:"usage"`
	NumTurns int             `json:"num_turns"`
	Duration time.Duration   `json:"duration"`
	Messages []types.Message `json:"messages"`
	Cost     float64         `json:"cost"`
}

// Query runs the agentic loop with streaming events.
func (a *Agent) Query(ctx context.Context, prompt string) (<-chan types.SDKMessage, <-chan error) {
	eventCh := make(chan types.SDKMessage, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		err := a.runLoop(ctx, prompt, eventCh)
		if err != nil {
			errCh <- err
		}
	}()

	return eventCh, errCh
}

// Prompt runs a query and returns the final result (blocking).
func (a *Agent) Prompt(ctx context.Context, prompt string) (*QueryResult, error) {
	start := time.Now()
	eventCh, errCh := a.Query(ctx, prompt)

	var result QueryResult
	var lastAssistantText string

	for event := range eventCh {
		switch event.Type {
		case types.MessageTypeAssistant:
			if event.Message != nil {
				if text := types.ExtractText(event.Message); text != "" {
					lastAssistantText = text
				}
			}
		case types.MessageTypeResult:
			if event.Usage != nil {
				result.Usage = *event.Usage
			}
			result.NumTurns = event.NumTurns
			result.Cost = event.Cost
		}
	}

	select {
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	default:
	}

	result.Text = lastAssistantText
	result.Duration = time.Since(start)
	result.Messages = append([]types.Message{}, a.messages...)

	return &result, nil
}

// GetMessages returns conversation history.
func (a *Agent) GetMessages() []types.Message {
	return append([]types.Message{}, a.messages...)
}

// Clear resets conversation history.
func (a *Agent) Clear() {
	a.messages = nil
}

// Close cleans up resources.
func (a *Agent) Close() {
	a.mcpClient.Close()
}

// spawnSubagent creates a child agent and runs a prompt synchronously.
func (a *Agent) spawnSubagent(ctx context.Context, config tools.SubagentConfig) (string, error) {
	model := config.Model
	if model == "" {
		model = a.opts.Model
	}

	childOpts := Options{
		Model:              model,
		APIKey:             a.opts.APIKey,
		BaseURL:            a.opts.BaseURL,
		CWD:                config.CWD,
		MaxTurns:           30,
		PermissionMode:     a.opts.PermissionMode,
		SystemPrompt:       config.SystemPrompt,
		CustomHeaders:      a.opts.CustomHeaders,
		ProxyURL:           a.opts.ProxyURL,
		TimeoutMs:          a.opts.TimeoutMs,
	}

	if childOpts.CWD == "" {
		childOpts.CWD = a.opts.CWD
	}

	// Use parent's permission callback if available
	if a.opts.CanUseTool != nil {
		childOpts.CanUseTool = a.opts.CanUseTool
	} else {
		childOpts.PermissionMode = a.opts.PermissionMode
	}

	child := New(childOpts)
	defer child.Close()

	if err := child.Init(ctx); err != nil {
		return "", fmt.Errorf("subagent init failed: %w", err)
	}

	result, err := child.Prompt(ctx, config.Prompt)
	if err != nil {
		return "", err
	}

	// Merge cost into parent tracker
	if child.costTracker != nil && a.costTracker != nil {
		childIn, childOut := child.costTracker.TotalTokens()
		a.costTracker.AddUsage(model, &types.Usage{
			InputTokens:  childIn,
			OutputTokens: childOut,
		})
	}

	return result.Text, nil
}

// SessionID returns the current session ID.
func (a *Agent) SessionID() string {
	return a.sessionID
}

// CostTracker returns the cost tracker.
func (a *Agent) CostTracker() *costtracker.Tracker {
	return a.costTracker
}

// MCPClient returns the MCP client for managing MCP server connections.
func (a *Agent) MCPClient() *mcp.Client {
	return a.mcpClient
}

// envFirst returns the first non-empty value from the env map or os env,
// trying CODEANY_ prefix first, then ANTHROPIC_ for compatibility.
func envFirst(env map[string]string, keys ...string) string {
	for _, key := range keys {
		if env != nil {
			if v := env[key]; v != "" {
				return v
			}
		}
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func resolveEnvOptions(opts *Options) {
	env := opts.Env

	if opts.APIKey == "" {
		opts.APIKey = envFirst(env, "CODEANY_API_KEY", "ANTHROPIC_API_KEY", "CODEANY_AUTH_TOKEN", "ANTHROPIC_AUTH_TOKEN")
	}

	if opts.BaseURL == "" {
		opts.BaseURL = envFirst(env, "CODEANY_BASE_URL", "ANTHROPIC_BASE_URL")
	}

	if opts.Model == "" {
		opts.Model = envFirst(env, "CODEANY_MODEL", "ANTHROPIC_MODEL")
		if opts.Model == "" {
			opts.Model = "sonnet-4-6"
		}
	}

	if opts.CWD == "" {
		opts.CWD, _ = os.Getwd()
	}

	if opts.MaxTurns == 0 {
		opts.MaxTurns = defaultMaxTurns
	}
}
