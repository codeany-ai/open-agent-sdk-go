package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

// MCPTool wraps an MCP server tool as a types.Tool.
type MCPTool struct {
	serverName   string
	toolName     string
	prefixedName string
	description  string
	inputSchema  types.ToolInputSchema
	conn         *Connection
}

// NewMCPTool creates a Tool wrapper for an MCP tool.
func NewMCPTool(serverName string, def types.MCPToolDefinition, conn *Connection) *MCPTool {
	return &MCPTool{
		serverName:   serverName,
		toolName:     def.Name,
		prefixedName: fmt.Sprintf("mcp__%s__%s", serverName, def.Name),
		description:  def.Description,
		inputSchema:  def.InputSchema,
		conn:         conn,
	}
}

func (t *MCPTool) Name() string        { return t.prefixedName }
func (t *MCPTool) Description() string  { return t.description }
func (t *MCPTool) InputSchema() types.ToolInputSchema { return t.inputSchema }
func (t *MCPTool) IsConcurrencySafe(input map[string]interface{}) bool { return false }
func (t *MCPTool) IsReadOnly(input map[string]interface{}) bool        { return false }

func (t *MCPTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	result, err := t.conn.CallTool(ctx, t.toolName, input)
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("MCP tool call failed: %v", err),
		}, nil
	}

	// Convert MCP result to ToolResult
	var contentBlocks []types.ContentBlock
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			contentBlocks = append(contentBlocks, types.ContentBlock{
				Type: types.ContentBlockText,
				Text: block.Text,
			})
		case "image":
			contentBlocks = append(contentBlocks, types.ContentBlock{
				Type: types.ContentBlockImage,
				Source: &types.ImageSource{
					Type:      "base64",
					MediaType: block.MimeType,
					Data:      block.Data,
				},
			})
		}
	}

	return &types.ToolResult{
		Content: contentBlocks,
		IsError: result.IsError,
	}, nil
}

// ParseMCPToolName splits "mcp__server__tool" into server and tool names.
func ParseMCPToolName(name string) (serverName, toolName string, ok bool) {
	if !strings.HasPrefix(name, "mcp__") {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(name, "mcp__"), "__", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// ToolsFromConnection creates Tool wrappers for all tools on a connection.
func ToolsFromConnection(conn *Connection) []types.Tool {
	tools := make([]types.Tool, len(conn.Tools))
	for i, def := range conn.Tools {
		tools[i] = NewMCPTool(conn.Name, def, conn)
	}
	return tools
}
