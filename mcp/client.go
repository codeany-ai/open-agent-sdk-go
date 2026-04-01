package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

// Client manages connections to MCP servers.
type Client struct {
	mu          sync.RWMutex
	connections map[string]*Connection
}

// Connection represents a live connection to an MCP server.
type Connection struct {
	Name    string
	Config  types.MCPServerConfig
	Status  types.MCPServerStatus
	Tools   []types.MCPToolDefinition
	Error   string
	cleanup func()

	// For stdio transport
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader

	// For HTTP transport
	httpClient *http.Client
	baseURL    string

	// JSON-RPC state
	nextID int
	mu     sync.Mutex
}

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC error.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewClient creates a new MCP client manager.
func NewClient() *Client {
	return &Client{
		connections: make(map[string]*Connection),
	}
}

// ConnectServer connects to an MCP server.
func (c *Client) ConnectServer(ctx context.Context, name string, config types.MCPServerConfig) (*Connection, error) {
	transportType := config.Type
	if transportType == "" {
		if config.Command != "" {
			transportType = types.MCPTransportStdio
		} else if config.URL != "" {
			transportType = types.MCPTransportHTTP
		}
	}

	var conn *Connection
	var err error

	switch transportType {
	case types.MCPTransportStdio:
		conn, err = c.connectStdio(ctx, name, config)
	case types.MCPTransportHTTP, types.MCPTransportSSE:
		conn, err = c.connectHTTP(ctx, name, config)
	default:
		return nil, fmt.Errorf("unsupported MCP transport type: %s", transportType)
	}

	if err != nil {
		return &Connection{
			Name:   name,
			Config: config,
			Status: types.MCPStatusError,
			Error:  err.Error(),
		}, err
	}

	// Fetch available tools
	tools, err := conn.ListTools(ctx)
	if err != nil {
		conn.Status = types.MCPStatusError
		conn.Error = fmt.Sprintf("failed to list tools: %v", err)
	} else {
		conn.Tools = tools
	}

	c.mu.Lock()
	c.connections[name] = conn
	c.mu.Unlock()

	return conn, nil
}

// connectStdio connects to an MCP server via stdio.
func (c *Client) connectStdio(ctx context.Context, name string, config types.MCPServerConfig) (*Connection, error) {
	cmd := exec.CommandContext(ctx, config.Command, config.Args...)

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range config.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	conn := &Connection{
		Name:   name,
		Config: config,
		Status: types.MCPStatusConnected,
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		reader: bufio.NewReader(stdout),
		cleanup: func() {
			stdin.Close()
			cmd.Process.Kill()
			cmd.Wait()
		},
	}

	// Send initialize request
	initResult, err := conn.sendRequest(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "open-agent-sdk-go",
			"version": "0.1.0",
		},
	})
	if err != nil {
		conn.cleanup()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	// Send initialized notification
	_ = conn.sendNotification("notifications/initialized", nil)

	_ = initResult
	return conn, nil
}

// connectHTTP connects to an MCP server via HTTP.
func (c *Client) connectHTTP(ctx context.Context, name string, config types.MCPServerConfig) (*Connection, error) {
	conn := &Connection{
		Name:       name,
		Config:     config,
		Status:     types.MCPStatusConnected,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    strings.TrimRight(config.URL, "/"),
		cleanup:    func() {},
	}

	return conn, nil
}

// GetConnection returns a connection by name.
func (c *Client) GetConnection(name string) *Connection {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connections[name]
}

// AllConnections returns all connections.
func (c *Client) AllConnections() []*Connection {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*Connection, 0, len(c.connections))
	for _, conn := range c.connections {
		result = append(result, conn)
	}
	return result
}

// AllTools returns tools from all connected servers, prefixed with server name.
func (c *Client) AllTools() []types.MCPToolDefinition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var tools []types.MCPToolDefinition
	for _, conn := range c.connections {
		if conn.Status != types.MCPStatusConnected {
			continue
		}
		for _, t := range conn.Tools {
			prefixed := t
			prefixed.Name = fmt.Sprintf("mcp__%s__%s", conn.Name, t.Name)
			tools = append(tools, prefixed)
		}
	}
	return tools
}

// Close disconnects all servers.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.connections {
		if conn.cleanup != nil {
			conn.cleanup()
		}
	}
	c.connections = make(map[string]*Connection)
}

// ListTools fetches the list of available tools from the server.
func (conn *Connection) ListTools(ctx context.Context) ([]types.MCPToolDefinition, error) {
	result, err := conn.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Tools []struct {
			Name        string                 `json:"name"`
			Description string                 `json:"description"`
			InputSchema map[string]interface{} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}

	tools := make([]types.MCPToolDefinition, len(resp.Tools))
	for i, t := range resp.Tools {
		schema := types.ToolInputSchema{
			Type: "object",
		}
		if props, ok := t.InputSchema["properties"].(map[string]interface{}); ok {
			schema.Properties = props
		}
		if req, ok := t.InputSchema["required"].([]interface{}); ok {
			for _, r := range req {
				if s, ok := r.(string); ok {
					schema.Required = append(schema.Required, s)
				}
			}
		}
		tools[i] = types.MCPToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		}
	}

	return tools, nil
}

// CallTool executes a tool on the MCP server.
func (conn *Connection) CallTool(ctx context.Context, toolName string, input map[string]interface{}) (*types.MCPToolCallResult, error) {
	result, err := conn.sendRequest(ctx, "tools/call", map[string]interface{}{
		"name":      toolName,
		"arguments": input,
	})
	if err != nil {
		return nil, err
	}

	var resp types.MCPToolCallResult
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal tool result: %w", err)
	}

	return &resp, nil
}

// sendRequest sends a JSON-RPC request and waits for a response.
func (conn *Connection) sendRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	conn.mu.Lock()
	conn.nextID++
	id := conn.nextID
	conn.mu.Unlock()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// Stdio transport
	if conn.stdin != nil {
		return conn.sendStdioRequest(ctx, req)
	}

	// HTTP transport
	if conn.httpClient != nil {
		return conn.sendHTTPRequest(ctx, req)
	}

	return nil, fmt.Errorf("no transport available")
}

func (conn *Connection) sendStdioRequest(ctx context.Context, req JSONRPCRequest) (json.RawMessage, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Write request
	if _, err := conn.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read response (blocking, with context cancellation via goroutine)
	type readResult struct {
		line []byte
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := conn.reader.ReadBytes('\n')
		ch <- readResult{line, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("read response: %w", r.err)
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(r.line, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}

		if resp.Error != nil {
			return nil, fmt.Errorf("JSON-RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}

		return resp.Result, nil
	}
}

func (conn *Connection) sendHTTPRequest(ctx context.Context, req JSONRPCRequest) (json.RawMessage, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, conn.baseURL, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range conn.Config.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := conn.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var jsonResp JSONRPCResponse
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if jsonResp.Error != nil {
		return nil, fmt.Errorf("JSON-RPC error %d: %s", jsonResp.Error.Code, jsonResp.Error.Message)
	}

	return jsonResp.Result, nil
}

// sendNotification sends a JSON-RPC notification (no response expected).
func (conn *Connection) sendNotification(method string, params interface{}) error {
	req := struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	if conn.stdin != nil {
		_, err = conn.stdin.Write(append(data, '\n'))
		return err
	}

	return nil
}
