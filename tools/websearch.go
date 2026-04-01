package tools

import (
	"context"
	"fmt"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

// WebSearchTool performs web searches.
// Note: This is a placeholder — actual implementation requires a search API
// (e.g., Brave Search, Google Custom Search, or Anthropic's built-in search).
type WebSearchTool struct {
	// SearchFn is a pluggable search implementation.
	// If nil, the tool returns an error asking to configure a search provider.
	SearchFn func(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
}

// SearchResult represents a single search result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func NewWebSearchTool() *WebSearchTool { return &WebSearchTool{} }

func (t *WebSearchTool) Name() string { return "WebSearch" }

func (t *WebSearchTool) Description() string {
	return `Performs a web search and returns results.
Use this to find current information, documentation, or answers to questions.`
}

func (t *WebSearchTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query",
			},
			"max_results": map[string]interface{}{
				"type":        "number",
				"description": "Maximum number of results (default 5)",
			},
		},
		Required: []string{"query"},
	}
}

func (t *WebSearchTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *WebSearchTool) IsReadOnly(input map[string]interface{}) bool        { return true }

func (t *WebSearchTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return &types.ToolResult{IsError: true, Error: "query is required"}, nil
	}

	maxResults := 5
	if mr, ok := input["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	if t.SearchFn == nil {
		return &types.ToolResult{
			IsError: true,
			Error:   "WebSearch is not configured. Set WebSearchTool.SearchFn to a search provider implementation.",
		}, nil
	}

	results, err := t.SearchFn(ctx, query, maxResults)
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("Search failed: %v", err),
		}, nil
	}

	// Format results
	var text string
	for i, r := range results {
		text += fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}

	if text == "" {
		text = "No results found."
	}

	return &types.ToolResult{
		Data: map[string]interface{}{
			"numResults": len(results),
		},
		Content: []types.ContentBlock{{
			Type: types.ContentBlockText,
			Text: text,
		}},
	}, nil
}
