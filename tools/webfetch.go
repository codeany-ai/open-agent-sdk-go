package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

const (
	webFetchTimeout     = 30 * time.Second
	webFetchMaxBodySize = 512 * 1024 // 512KB
)

// WebFetchTool fetches content from URLs.
type WebFetchTool struct{}

func NewWebFetchTool() *WebFetchTool { return &WebFetchTool{} }

func (t *WebFetchTool) Name() string { return "WebFetch" }

func (t *WebFetchTool) Description() string {
	return `Fetches content from a URL. Returns the response body as text.
Useful for reading web pages, API responses, documentation, etc.`
}

func (t *WebFetchTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to fetch",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "Optional HTTP headers",
			},
		},
		Required: []string{"url"},
	}
}

func (t *WebFetchTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *WebFetchTool) IsReadOnly(input map[string]interface{}) bool        { return true }

func (t *WebFetchTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	url, _ := input["url"].(string)
	if url == "" {
		return &types.ToolResult{IsError: true, Error: "url is required"}, nil
	}

	// Validate URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return &types.ToolResult{IsError: true, Error: "URL must start with http:// or https://"}, nil
	}

	client := &http.Client{Timeout: webFetchTimeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &types.ToolResult{IsError: true, Error: fmt.Sprintf("Invalid URL: %v", err)}, nil
	}

	req.Header.Set("User-Agent", "open-agent-sdk-go/0.1.0")

	// Apply custom headers
	if headers, ok := input["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("Fetch failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	// Read body with size limit
	body, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBodySize))
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("Read body failed: %v", err),
		}, nil
	}

	content := string(body)
	if len(body) >= webFetchMaxBodySize {
		content += "\n\n... (response truncated)"
	}

	if resp.StatusCode >= 400 {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("HTTP %d: %s", resp.StatusCode, content),
		}, nil
	}

	return &types.ToolResult{
		Data: map[string]interface{}{
			"status":      resp.StatusCode,
			"contentType": resp.Header.Get("Content-Type"),
		},
		Content: []types.ContentBlock{{
			Type: types.ContentBlockText,
			Text: content,
		}},
	}, nil
}
