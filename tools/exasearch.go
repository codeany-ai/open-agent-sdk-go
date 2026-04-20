package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/codeany-ai/open-agent-sdk-go/types"
)

const (
	exaDefaultEndpoint = "https://api.exa.ai/search"
	exaDefaultTimeout  = 30 * time.Second
	exaIntegrationID   = "open-agent-sdk-go"
)

// ExaSearchTool performs web searches using the Exa AI-powered search API
// (https://exa.ai). Requires the EXA_API_KEY environment variable to be set
// when the tool is invoked.
type ExaSearchTool struct {
	// APIKey is the Exa API key. If empty, the value of EXA_API_KEY at call
	// time is used.
	APIKey string
	// HTTPClient is the HTTP client used for requests. Defaults to a client
	// with a 30s timeout.
	HTTPClient *http.Client
	// Endpoint overrides the Exa search endpoint. Defaults to the public API.
	Endpoint string
}

// NewExaSearchTool creates a new Exa search tool. The API key is read from
// EXA_API_KEY lazily on each call, so tests and config flows can set the
// variable after construction.
func NewExaSearchTool() *ExaSearchTool {
	return &ExaSearchTool{}
}

func (t *ExaSearchTool) Name() string { return "ExaSearch" }

func (t *ExaSearchTool) Description() string {
	return `Performs a web search using Exa (https://exa.ai) and returns results.

Exa is an AI-powered search engine that understands natural language queries
and returns high-quality web pages, research papers, company profiles, news
articles, and more. Unlike a keyword search engine, Exa supports neural
retrieval, category filtering, domain filtering, published-date ranges, and
rich content modes (text, highlights, summaries).

Use this when you need current, high-quality information from the web.
Requires the EXA_API_KEY environment variable to be set.`
}

func (t *ExaSearchTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query.",
			},
			"num_results": map[string]interface{}{
				"type":        "number",
				"description": "Maximum number of results (default 5, max 100).",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "Search type: 'auto' (default), 'neural', 'fast', 'deep-lite', 'deep', 'deep-reasoning', or 'instant'.",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Category filter: 'company', 'research paper', 'news', 'personal site', 'financial report', or 'people'.",
			},
			"include_domains": map[string]interface{}{
				"type":        "array",
				"description": "Only include results from these domains.",
				"items":       map[string]interface{}{"type": "string"},
			},
			"exclude_domains": map[string]interface{}{
				"type":        "array",
				"description": "Exclude results from these domains.",
				"items":       map[string]interface{}{"type": "string"},
			},
			"start_published_date": map[string]interface{}{
				"type":        "string",
				"description": "Only include results published on or after this ISO 8601 date.",
			},
			"end_published_date": map[string]interface{}{
				"type":        "string",
				"description": "Only include results published on or before this ISO 8601 date.",
			},
			"user_location": map[string]interface{}{
				"type":        "string",
				"description": "Two-letter ISO country code for localized results.",
			},
			"include_text": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, return full page text for each result (default false).",
			},
			"include_highlights": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, return AI-generated highlights for each result (default true).",
			},
			"include_summary": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, return an AI-generated summary for each result (default false).",
			},
		},
		Required: []string{"query"},
	}
}

func (t *ExaSearchTool) IsConcurrencySafe(_ map[string]interface{}) bool { return true }
func (t *ExaSearchTool) IsReadOnly(_ map[string]interface{}) bool        { return true }

// exaContents mirrors the Exa /search `contents` field. Each content mode
// (text, highlights, summary) is independent and may be requested together.
type exaContents struct {
	Text       bool            `json:"text,omitempty"`
	Highlights bool            `json:"highlights,omitempty"`
	Summary    *exaSummaryOpts `json:"summary,omitempty"`
}

type exaSummaryOpts struct{}

// exaRequest is the POST /search request body.
type exaRequest struct {
	Query              string       `json:"query"`
	Type               string       `json:"type,omitempty"`
	NumResults         int          `json:"numResults,omitempty"`
	Category           string       `json:"category,omitempty"`
	IncludeDomains     []string     `json:"includeDomains,omitempty"`
	ExcludeDomains     []string     `json:"excludeDomains,omitempty"`
	StartPublishedDate string       `json:"startPublishedDate,omitempty"`
	EndPublishedDate   string       `json:"endPublishedDate,omitempty"`
	UserLocation       string       `json:"userLocation,omitempty"`
	Contents           *exaContents `json:"contents,omitempty"`
}

// ExaResult is a single search result returned by the Exa API.
type ExaResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	ID            string   `json:"id"`
	Score         float64  `json:"score"`
	PublishedDate string   `json:"publishedDate,omitempty"`
	Author        string   `json:"author,omitempty"`
	Text          string   `json:"text,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	Highlights    []string `json:"highlights,omitempty"`
}

// ExaResponse is the POST /search response body.
type ExaResponse struct {
	Results          []ExaResult `json:"results"`
	AutopromptString string      `json:"autopromptString,omitempty"`
}

func (t *ExaSearchTool) Call(ctx context.Context, input map[string]interface{}, _ *types.ToolUseContext) (*types.ToolResult, error) {
	apiKey := t.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("EXA_API_KEY")
	}
	if apiKey == "" {
		return &types.ToolResult{
			IsError: true,
			Error:   "EXA_API_KEY environment variable is not set. Get a key at https://exa.ai/",
		}, nil
	}

	query, _ := input["query"].(string)
	if query == "" {
		return &types.ToolResult{IsError: true, Error: "query is required"}, nil
	}

	req := exaRequest{
		Query:      query,
		Type:       "auto",
		NumResults: 5,
	}
	if v, ok := input["num_results"].(float64); ok && v > 0 {
		req.NumResults = int(v)
	}
	if v, ok := input["type"].(string); ok && v != "" {
		req.Type = v
	}
	if v, ok := input["category"].(string); ok && v != "" {
		req.Category = v
	}
	if v, ok := input["start_published_date"].(string); ok {
		req.StartPublishedDate = v
	}
	if v, ok := input["end_published_date"].(string); ok {
		req.EndPublishedDate = v
	}
	if v, ok := input["user_location"].(string); ok {
		req.UserLocation = v
	}
	req.IncludeDomains = toStringSlice(input["include_domains"])
	req.ExcludeDomains = toStringSlice(input["exclude_domains"])

	// Default to highlights on; caller can opt into full text or summary and
	// can turn highlights off.
	contents := &exaContents{Highlights: true}
	if v, ok := input["include_highlights"].(bool); ok {
		contents.Highlights = v
	}
	if v, ok := input["include_text"].(bool); ok {
		contents.Text = v
	}
	if v, ok := input["include_summary"].(bool); ok && v {
		contents.Summary = &exaSummaryOpts{}
	}
	req.Contents = contents

	body, err := json.Marshal(req)
	if err != nil {
		return &types.ToolResult{IsError: true, Error: fmt.Sprintf("marshal request: %v", err)}, nil
	}

	endpoint := t.Endpoint
	if endpoint == "" {
		endpoint = exaDefaultEndpoint
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return &types.ToolResult{IsError: true, Error: fmt.Sprintf("build request: %v", err)}, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("x-exa-integration", exaIntegrationID)

	client := t.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: exaDefaultTimeout}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return &types.ToolResult{IsError: true, Error: fmt.Sprintf("Exa request failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &types.ToolResult{IsError: true, Error: fmt.Sprintf("read Exa response: %v", err)}, nil
	}

	if resp.StatusCode >= 400 {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("Exa API error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))),
		}, nil
	}

	var parsed ExaResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return &types.ToolResult{IsError: true, Error: fmt.Sprintf("parse Exa response: %v", err)}, nil
	}

	text := formatExaResults(parsed.Results)
	if text == "" {
		text = "No results found."
	}

	return &types.ToolResult{
		Data: map[string]interface{}{
			"numResults":       len(parsed.Results),
			"autopromptString": parsed.AutopromptString,
		},
		Content: []types.ContentBlock{{
			Type: types.ContentBlockText,
			Text: text,
		}},
	}, nil
}

// exaSnippet returns the best available short content for a result. The API
// may return any combination of highlights, summary, and text, so we cascade
// through them rather than assuming one is present.
func exaSnippet(r ExaResult) string {
	if len(r.Highlights) > 0 {
		parts := make([]string, 0, len(r.Highlights))
		for _, h := range r.Highlights {
			if s := strings.TrimSpace(h); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " … ")
		}
	}
	if s := strings.TrimSpace(r.Summary); s != "" {
		return s
	}
	if s := strings.TrimSpace(r.Text); s != "" {
		if len(s) > 300 {
			return s[:300] + "…"
		}
		return s
	}
	return ""
}

func formatExaResults(results []ExaResult) string {
	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "%d. **%s**\n   %s\n", i+1, r.Title, r.URL)
		if r.PublishedDate != "" {
			fmt.Fprintf(&b, "   Published: %s\n", r.PublishedDate)
		}
		if snippet := exaSnippet(r); snippet != "" {
			fmt.Fprintf(&b, "   %s\n", snippet)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func toStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
