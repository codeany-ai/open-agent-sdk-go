package types

// MCPServerStatus represents the connection status of an MCP server.
type MCPServerStatus string

const (
	MCPStatusConnected    MCPServerStatus = "connected"
	MCPStatusDisconnected MCPServerStatus = "disconnected"
	MCPStatusError        MCPServerStatus = "error"
	MCPStatusPending      MCPServerStatus = "pending"
)

// MCPTransportType is the type of MCP transport.
type MCPTransportType string

const (
	MCPTransportStdio     MCPTransportType = "stdio"
	MCPTransportSSE       MCPTransportType = "sse"
	MCPTransportHTTP      MCPTransportType = "http"
	MCPTransportWebSocket MCPTransportType = "websocket"
)

// MCPServerConfig is the configuration for an MCP server.
type MCPServerConfig struct {
	// Transport type (stdio, sse, http, websocket)
	Type MCPTransportType `json:"type,omitempty"`

	// For stdio transport
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// For SSE/HTTP/WebSocket transport
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// MCPToolDefinition describes a tool exposed by an MCP server.
type MCPToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema ToolInputSchema `json:"inputSchema"`
}

// MCPServerConnection represents a connected MCP server.
type MCPServerConnection struct {
	Name         string            `json:"name"`
	Status       MCPServerStatus   `json:"status"`
	Config       MCPServerConfig   `json:"config"`
	Tools        []MCPToolDefinition `json:"tools,omitempty"`
	ServerInfo   *MCPServerInfo    `json:"server_info,omitempty"`
	Instructions string            `json:"instructions,omitempty"`
	Error        string            `json:"error,omitempty"`
}

// MCPServerInfo holds server metadata.
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPToolCallResult is the result from calling an MCP tool.
type MCPToolCallResult struct {
	Content []MCPContentBlock `json:"content"`
	IsError bool              `json:"isError,omitempty"`
}

// MCPContentBlock represents content from an MCP tool result.
type MCPContentBlock struct {
	Type     string `json:"type"` // "text", "image", "resource"
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}
