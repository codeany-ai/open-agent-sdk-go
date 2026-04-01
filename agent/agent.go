package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/shipany-ai/open-agent-sdk-go/api"
	"github.com/shipany-ai/open-agent-sdk-go/costtracker"
	"github.com/shipany-ai/open-agent-sdk-go/hooks"
	"github.com/shipany-ai/open-agent-sdk-go/mcp"
	"github.com/shipany-ai/open-agent-sdk-go/permissions"
	"github.com/shipany-ai/open-agent-sdk-go/tools"
	"github.com/shipany-ai/open-agent-sdk-go/types"
)

const (
	defaultMaxTurns = 100
)

// Options configures an Agent.
type Options struct {
	// Model ID (e.g. "claude-sonnet-4-6")
	Model string

	// Anthropic API key
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
	// Resolve from env map
	resolveEnvOptions(&opts)

	sessionID := uuid.New().String()

	// Create API client
	apiClient := api.NewClient(api.ClientConfig{
		APIKey:  opts.APIKey,
		BaseURL: opts.BaseURL,
		Model:   opts.Model,
	})

	// Create tool registry
	registry := tools.DefaultRegistry()

	// Add custom tools
	for _, t := range opts.CustomTools {
		registry.Register(t)
	}

	// Create permission handler
	permConfig := &permissions.Config{Mode: opts.PermissionMode}
	if permConfig.Mode == "" {
		permConfig.Mode = types.PermissionModeBypassPermissions
	}
	canUseTool := opts.CanUseTool
	if canUseTool == nil {
		canUseTool = permissions.NewCanUseToolFn(permConfig, opts.AllowedTools)
	}

	// Create hook manager
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

		// Register MCP tools
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
				lastAssistantText = types.ExtractText(event.Message)
			}
		case types.MessageTypeResult:
			if event.Usage != nil {
				result.Usage = *event.Usage
			}
			result.NumTurns = event.NumTurns
			result.Cost = event.Cost
		}
	}

	// Check for errors
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

// resolveEnvOptions resolves options from env map and process environment.
func resolveEnvOptions(opts *Options) {
	env := opts.Env

	if opts.APIKey == "" {
		if env != nil {
			if v := env["ANTHROPIC_API_KEY"]; v != "" {
				opts.APIKey = v
			} else if v := env["ANTHROPIC_AUTH_TOKEN"]; v != "" {
				opts.APIKey = v
			}
		}
		if opts.APIKey == "" {
			opts.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}

	if opts.BaseURL == "" {
		if env != nil {
			if v := env["ANTHROPIC_BASE_URL"]; v != "" {
				opts.BaseURL = v
			}
		}
		if opts.BaseURL == "" {
			opts.BaseURL = os.Getenv("ANTHROPIC_BASE_URL")
		}
	}

	if opts.Model == "" {
		if env != nil {
			if v := env["ANTHROPIC_MODEL"]; v != "" {
				opts.Model = v
			}
		}
		if opts.Model == "" {
			opts.Model = os.Getenv("ANTHROPIC_MODEL")
		}
		if opts.Model == "" {
			opts.Model = "claude-sonnet-4-6"
		}
	}

	if opts.CWD == "" {
		opts.CWD, _ = os.Getwd()
	}

	if opts.MaxTurns == 0 {
		opts.MaxTurns = defaultMaxTurns
	}
}
