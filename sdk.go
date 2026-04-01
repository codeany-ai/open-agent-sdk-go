// Package openagentsdk provides an open-source Go SDK for building AI agents
// powered by Claude. It implements the core agent loop, tool system, MCP client,
// and permission management without any CLI or UI dependencies.
//
// Usage:
//
//	a := agent.New(agent.Options{
//	    Model:  "claude-sonnet-4-6",
//	    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
//	})
//	defer a.Close()
//
//	// Streaming
//	events, errs := a.Query(ctx, "Analyze this codebase")
//	for event := range events {
//	    if event.Type == types.MessageTypeAssistant {
//	        fmt.Println(types.ExtractText(event.Message))
//	    }
//	}
//
//	// Simple (blocking)
//	result, err := a.Prompt(ctx, "What does this code do?")
//	fmt.Println(result.Text)
package openagentsdk

import (
	"context"

	"github.com/shipany-ai/open-agent-sdk-go/agent"
	"github.com/shipany-ai/open-agent-sdk-go/types"
)

// CreateAgent creates a new Agent with the given options.
func CreateAgent(opts agent.Options) *agent.Agent {
	return agent.New(opts)
}

// Query runs a one-shot agent query with streaming events.
func Query(ctx context.Context, prompt string, opts agent.Options) (<-chan types.SDKMessage, <-chan error) {
	a := agent.New(opts)
	return a.Query(ctx, prompt)
}

// Prompt runs a one-shot agent query and returns the final result.
func Prompt(ctx context.Context, prompt string, opts agent.Options) (*agent.QueryResult, error) {
	a := agent.New(opts)
	defer a.Close()
	return a.Prompt(ctx, prompt)
}
