// Example 12: Opt-in web research using Parallel Search MCP over HTTP.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/codeany-ai/open-agent-sdk-go/agent"
	"github.com/codeany-ai/open-agent-sdk-go/types"
	"github.com/google/uuid"
)

const parallelURL = "https://search.parallel.ai/mcp"

// researchOptions uses the existing MCP configuration surface. Model API
// credentials and provider selection retain the SDK's usual environment setup.
func researchOptions() agent.Options {
	return agent.Options{
		MaxTurns:       8,
		PermissionMode: types.PermissionModeDontAsk,
		AllowedTools:   []string{"mcp__parallel__web_search", "mcp__parallel__web_fetch"},
		SystemPrompt: "Research the user's question with the Parallel MCP tools, then answer with source URLs. " +
			"Use web_search first and web_fetch when you need more detail. " +
			"Use this same session_id for every search and fetch in this conversation: " + uuid.New().String(),
		MCPServers: map[string]types.MCPServerConfig{
			"parallel": {
				Type:    types.MCPTransportHTTP,
				URL:     parallelURL,
				Headers: map[string]string{"User-Agent": "open-agent-sdk-go/0.1.0"},
			},
		},
	}
}

func research(ctx context.Context, opts agent.Options, question string) (*agent.QueryResult, error) {
	a := agent.New(opts)
	defer a.Close()
	if err := a.Init(ctx); err != nil {
		return nil, err
	}
	// Init currently logs connection failures and can return nil. Check discovery
	// before asking the model to research with tools that may be unavailable.
	conn := a.MCPClient().GetConnection("parallel")
	if conn == nil {
		return nil, fmt.Errorf("Parallel MCP connection unavailable")
	}
	if conn.Status != types.MCPStatusConnected {
		return nil, fmt.Errorf("Parallel MCP discovery failed: %s", conn.Error)
	}
	names := make(map[string]bool)
	for _, tool := range conn.Tools {
		names[tool.Name] = true
	}
	if !names["web_search"] || !names["web_fetch"] {
		return nil, fmt.Errorf("Parallel MCP did not expose web_search and web_fetch")
	}
	return a.Prompt(ctx, question)
}

func main() {
	question := strings.Join(os.Args[1:], " ")
	if question == "" {
		question = "What changed in Go 1.23? Use official sources and cite their URLs."
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := research(ctx, researchOptions(), question)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result.Text)
}
