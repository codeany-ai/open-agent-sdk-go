package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codeany-ai/open-agent-sdk-go/api"
	"github.com/codeany-ai/open-agent-sdk-go/types"
)

func isolate(t *testing.T) {
	t.Helper()
	for _, key := range []string{"CODEANY_API_KEY", "ANTHROPIC_API_KEY", "CODEANY_AUTH_TOKEN", "ANTHROPIC_AUTH_TOKEN", "CODEANY_BASE_URL", "ANTHROPIC_BASE_URL", "CODEANY_MODEL", "ANTHROPIC_MODEL", "CODEANY_CUSTOM_HEADERS", "ANTHROPIC_CUSTOM_HEADERS", "PARALLEL_API_KEY", "API_TIMEOUT_MS", "HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY"} {
		t.Setenv(key, "")
	}
}

type observation struct {
	Method      string          `json:"method"`
	UserAgent   string          `json:"user_agent"`
	AuthPresent bool            `json:"auth_present"`
	Body        json.RawMessage `json:"body"`
	Status      int             `json:"status"`
	Response    json.RawMessage `json:"response"`
}

// The model fixture requests search then fetch and quotes the tool results in
// its final answer. It verifies the whole agent path without a paid model call.
func exerciseResearch(t *testing.T, live bool) {
	t.Helper()
	upstreamProxy := os.Getenv("HTTPS_PROXY")
	isolate(t)
	opts := researchOptions()
	if opts.MCPServers["parallel"].URL != parallelURL {
		t.Fatal("unexpected endpoint")
	}
	var mu sync.Mutex
	var observed []observation
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if live && upstreamProxy != "" {
		proxyURL, err := url.Parse(upstreamProxy)
		if err != nil {
			t.Fatal("invalid network proxy")
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	client := &http.Client{Transport: transport, Timeout: 60 * time.Second}
	defer transport.CloseIdleConnections()
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if r.Header.Get("User-Agent") != "open-agent-sdk-go/0.1.0" {
			t.Error("missing project User-Agent")
		}
		auth := r.Header.Get("Authorization") != "" || r.Header.Get("x-api-key") != ""
		if auth {
			t.Error("anonymous MCP request carried authentication")
		}
		var response []byte
		status := 200
		if live {
			upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, parallelURL, strings.NewReader(string(body)))
			if err != nil {
				t.Error(err)
				w.WriteHeader(502)
				return
			}
			upstream.Header = r.Header.Clone()
			// Let the upstream client negotiate and decode response compression.
			upstream.Header.Del("Accept-Encoding")
			resp, err := client.Do(upstream)
			if err != nil {
				t.Error(err)
				w.WriteHeader(502)
				return
			}
			response, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Error(err)
			}
			status = resp.StatusCode
		} else if req.Method == "tools/list" {
			response, _ = json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": map[string]interface{}{"tools": []map[string]interface{}{
				{"name": "web_search", "description": "Search the web", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"objective": map[string]string{"type": "string"}, "search_queries": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}}}, "required": []string{"objective", "search_queries"}}},
				{"name": "web_fetch", "description": "Fetch excerpts", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"urls": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}}}, "required": []string{"urls"}}},
			}}})
		} else if req.Method == "tools/call" {
			if req.Params.Name != "web_search" && req.Params.Name != "web_fetch" {
				t.Errorf("unexpected tool %s", req.Params.Name)
			}
			if _, ok := req.Params.Arguments["session_id"].(string); !ok {
				t.Error("no conversation identifier")
			}
			response, _ = json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "Go 1.23 supports range over iterator functions. Source: https://go.dev/doc/go1.23"}}}})
		} else {
			t.Errorf("unexpected method %s", req.Method)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		observed = append(observed, observation{req.Method, r.Header.Get("User-Agent"), auth, body, status, response})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(response)
	}))
	defer mcpServer.Close()
	config := opts.MCPServers["parallel"]
	config.URL = mcpServer.URL
	opts.MCPServers["parallel"] = config
	opts.CWD = t.TempDir()
	var turns int
	var session string
	var answers []string
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.MessagesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected API path %s", r.URL.Path)
		}
		if len(req.Tools) != 2 {
			t.Errorf("got %d advertised tools, want 2", len(req.Tools))
		}
		names := map[string]bool{}
		for _, tool := range req.Tools {
			names[tool.Name] = true
			if len(tool.InputSchema) == 0 {
				t.Error("missing loaded schema")
			}
		}
		if !names["mcp__parallel__web_search"] || !names["mcp__parallel__web_fetch"] {
			t.Error("missing selected MCP tools")
		}
		if turns == 0 {
			for _, block := range req.System {
				session = regexp.MustCompile(`[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}`).FindString(block.Text)
				if session != "" {
					break
				}
			}
			if session == "" {
				t.Error("missing stable session instruction")
			}
		} else {
			last := req.Messages[len(req.Messages)-1]
			for _, block := range last.Content {
				if block.Type != types.ContentBlockToolResult || block.IsError {
					t.Errorf("unsuccessful tool result: %+v", block)
					continue
				}
				var text string
				for _, content := range block.Content {
					text += content.Text
				}
				if !strings.Contains(text, "go.dev") {
					t.Error("tool result did not contain official source")
				}
				answers = append(answers, text)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(event interface{}) {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
		}
		var block types.ContentBlock
		if turns == 0 {
			block = types.ContentBlock{Type: types.ContentBlockToolUse, ID: "search-1", Name: "mcp__parallel__web_search", Input: map[string]interface{}{"objective": "Find Go 1.23 iterator changes in official release notes", "search_queries": []string{"Go 1.23 iterator release notes"}, "session_id": session}}
		} else if turns == 1 {
			block = types.ContentBlock{Type: types.ContentBlockToolUse, ID: "fetch-1", Name: "mcp__parallel__web_fetch", Input: map[string]interface{}{"urls": []string{"https://go.dev/doc/go1.23"}, "objective": "Go 1.23 iterator changes", "session_id": session}}
		} else {
			block = types.ContentBlock{Type: types.ContentBlockText, Text: "Official Go 1.23 sources and excerpts:\n" + strings.Join(answers, "\n")}
		}
		turns++
		send(map[string]interface{}{"type": "content_block_start", "index": 0, "content_block": block})
		send(map[string]interface{}{"type": "content_block_stop", "index": 0})
		send(map[string]interface{}{"type": "message_stop"})
		// Mimic a streaming server keeping the connection open briefly after its
		// last event, so the SDK consumes the events before stream EOF.
		time.Sleep(20 * time.Millisecond)
	}))
	defer modelServer.Close()
	opts.BaseURL = modelServer.URL
	opts.APIKey = "local-fixture"
	opts.Provider = "anthropic"
	opts.Model = "fixture"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := research(ctx, opts, "What changed in Go 1.23? Cite official sources.")
	if err != nil {
		t.Fatal(err)
	}
	if result.NumTurns != 3 || !strings.Contains(result.Text, "go.dev") || !strings.Contains(strings.ToLower(result.Text), "iterator") {
		t.Fatalf("unexpected final answer: %+v", result)
	}
	if len(observed) != 3 {
		t.Fatalf("got %d MCP requests, want discovery/search/fetch", len(observed))
	}
	for _, call := range observed[1:] {
		var request struct {
			Params struct {
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"params"`
		}
		if err := json.Unmarshal(call.Body, &request); err != nil {
			t.Fatal(err)
		}
		if request.Params.Arguments["session_id"] != session {
			t.Error("session changed between calls")
		}
	}
	if live {
		t.Logf("Real anonymous MCP discovery/search/fetch succeeded; agent completed %d turns", result.NumTurns)
		if path := os.Getenv("PARALLEL_MCP_EVIDENCE_FILE"); path != "" {
			data, err := json.MarshalIndent(map[string]interface{}{"endpoint": parallelURL, "model_api": "deterministic local fixture", "requests": observed, "final_answer": result.Text}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestResearch(t *testing.T) { exerciseResearch(t, false) }
func TestResearchLive(t *testing.T) {
	if os.Getenv("PARALLEL_MCP_LIVE_TEST") != "1" {
		t.Skip("set PARALLEL_MCP_LIVE_TEST=1 for anonymous network validation")
	}
	exerciseResearch(t, true)
}

func TestDiscoveryFailure(t *testing.T) {
	isolate(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	opts := researchOptions()
	opts.CWD = t.TempDir()
	config := opts.MCPServers["parallel"]
	config.URL = server.URL
	opts.MCPServers["parallel"] = config
	_, err := research(context.Background(), opts, "research")
	if err == nil || !strings.Contains(err.Error(), "discovery failed") {
		t.Fatalf("expected discovery error, got %v", err)
	}
}

func TestCanceledDiscovery(t *testing.T) {
	isolate(t)
	opts := researchOptions()
	opts.CWD = t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := research(ctx, opts, "research")
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
