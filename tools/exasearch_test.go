package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExaSearch_MissingAPIKey(t *testing.T) {
	t.Setenv("EXA_API_KEY", "")
	tool := NewExaSearchTool()

	r, err := tool.Call(context.Background(), map[string]interface{}{
		"query": "test",
	}, testToolCtx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError when EXA_API_KEY is unset")
	}
	if !strings.Contains(r.Error, "EXA_API_KEY") {
		t.Errorf("expected error mentioning EXA_API_KEY, got: %s", r.Error)
	}
}

func TestExaSearch_MissingQuery(t *testing.T) {
	tool := &ExaSearchTool{APIKey: "test-key"}
	r, err := tool.Call(context.Background(), map[string]interface{}{}, testToolCtx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.IsError || !strings.Contains(r.Error, "query is required") {
		t.Errorf("expected 'query is required' error, got: %+v", r)
	}
}

func TestExaSearch_SuccessfulResponse(t *testing.T) {
	var gotBody exaRequest
	var gotAuthHeader, gotIntegration string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("x-api-key")
		gotIntegration = r.Header.Get("x-exa-integration")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{
					"title": "Result A",
					"url": "https://a.example.com",
					"id": "a",
					"score": 0.9,
					"publishedDate": "2026-01-02T00:00:00Z",
					"highlights": ["highlight one", "highlight two"]
				},
				{
					"title": "Result B",
					"url": "https://b.example.com",
					"id": "b",
					"score": 0.8,
					"summary": "B summary"
				}
			],
			"autopromptString": "refined: test query"
		}`))
	}))
	defer server.Close()

	tool := &ExaSearchTool{
		APIKey:   "secret-key",
		Endpoint: server.URL,
	}

	r, err := tool.Call(context.Background(), map[string]interface{}{
		"query":              "test query",
		"num_results":        float64(3),
		"type":               "neural",
		"category":           "news",
		"include_domains":    []interface{}{"example.com"},
		"include_summary":    true,
		"include_highlights": true,
	}, testToolCtx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected tool error: %s", r.Error)
	}

	if gotAuthHeader != "secret-key" {
		t.Errorf("expected x-api-key=secret-key, got %q", gotAuthHeader)
	}
	if gotIntegration != "open-agent-sdk-go" {
		t.Errorf("expected x-exa-integration=open-agent-sdk-go, got %q", gotIntegration)
	}

	if gotBody.Query != "test query" {
		t.Errorf("query not passed through: %q", gotBody.Query)
	}
	if gotBody.NumResults != 3 {
		t.Errorf("num_results not passed through: %d", gotBody.NumResults)
	}
	if gotBody.Type != "neural" {
		t.Errorf("type not passed through: %q", gotBody.Type)
	}
	if gotBody.Category != "news" {
		t.Errorf("category not passed through: %q", gotBody.Category)
	}
	if len(gotBody.IncludeDomains) != 1 || gotBody.IncludeDomains[0] != "example.com" {
		t.Errorf("include_domains not passed through: %+v", gotBody.IncludeDomains)
	}
	if gotBody.Contents == nil || !gotBody.Contents.Highlights || gotBody.Contents.Summary == nil {
		t.Errorf("expected contents.highlights=true and contents.summary set: %+v", gotBody.Contents)
	}

	data, _ := r.Data.(map[string]interface{})
	if data["numResults"].(int) != 2 {
		t.Errorf("expected numResults=2, got %v", data["numResults"])
	}
	if data["autopromptString"].(string) != "refined: test query" {
		t.Errorf("autoprompt missing: %v", data["autopromptString"])
	}

	text := r.Content[0].Text
	if !strings.Contains(text, "Result A") || !strings.Contains(text, "https://a.example.com") {
		t.Errorf("formatted result missing title/url: %s", text)
	}
	if !strings.Contains(text, "highlight one") {
		t.Errorf("expected highlight text, got: %s", text)
	}
	if !strings.Contains(text, "B summary") {
		t.Errorf("expected summary fallback for result B, got: %s", text)
	}
}

func TestExaSearch_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer server.Close()

	tool := &ExaSearchTool{APIKey: "bad-key", Endpoint: server.URL}
	r, err := tool.Call(context.Background(), map[string]interface{}{"query": "x"}, testToolCtx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError for 401 response")
	}
	if !strings.Contains(r.Error, "401") {
		t.Errorf("expected status code in error, got: %s", r.Error)
	}
}

func TestExaSnippet_Fallbacks(t *testing.T) {
	cases := []struct {
		name   string
		result ExaResult
		want   string
	}{
		{
			name:   "highlights preferred over summary and text",
			result: ExaResult{Highlights: []string{"h1", "h2"}, Summary: "summary", Text: "text"},
			want:   "h1 … h2",
		},
		{
			name:   "summary used when highlights missing",
			result: ExaResult{Summary: "just summary", Text: "ignored"},
			want:   "just summary",
		},
		{
			name:   "text used when highlights and summary missing",
			result: ExaResult{Text: "plain text"},
			want:   "plain text",
		},
		{
			name:   "text truncated beyond 300 chars",
			result: ExaResult{Text: strings.Repeat("a", 400)},
			want:   strings.Repeat("a", 300) + "…",
		},
		{
			name:   "empty when nothing available",
			result: ExaResult{},
			want:   "",
		},
		{
			name:   "empty highlight strings skipped",
			result: ExaResult{Highlights: []string{"   ", ""}, Summary: "fallback"},
			want:   "fallback",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exaSnippet(tc.result); got != tc.want {
				t.Errorf("exaSnippet = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExaSearch_RegisteredInDefaultRegistry(t *testing.T) {
	reg := DefaultRegistry()
	if reg.Get("ExaSearch") == nil {
		t.Fatal("ExaSearch tool not registered in DefaultRegistry")
	}
}
