package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

// FileWriteTool writes content to files.
type FileWriteTool struct{}

func NewFileWriteTool() *FileWriteTool { return &FileWriteTool{} }

func (t *FileWriteTool) Name() string { return "Write" }

func (t *FileWriteTool) Description() string {
	return `Writes content to a file, creating it if it doesn't exist or overwriting if it does.
The file_path must be an absolute path. Parent directories are created automatically.`
}

func (t *FileWriteTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path to the file to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write to the file",
			},
		},
		Required: []string{"file_path", "content"},
	}
}

func (t *FileWriteTool) IsConcurrencySafe(input map[string]interface{}) bool { return false }
func (t *FileWriteTool) IsReadOnly(input map[string]interface{}) bool        { return false }

func (t *FileWriteTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	filePath, _ := input["file_path"].(string)
	content, _ := input["content"].(string)

	if filePath == "" {
		return &types.ToolResult{IsError: true, Error: "file_path is required"}, nil
	}

	// Resolve relative paths
	if !filepath.IsAbs(filePath) && tCtx != nil && tCtx.WorkingDir != "" {
		filePath = filepath.Join(tCtx.WorkingDir, filePath)
	}

	// Check if file exists for type detection
	_, err := os.Stat(filePath)
	isCreate := os.IsNotExist(err)

	// Ensure parent directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("Failed to create directory %s: %v", dir, err),
		}, nil
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("Failed to write file: %v", err),
		}, nil
	}

	writeType := "update"
	if isCreate {
		writeType = "create"
	}

	return &types.ToolResult{
		Data: map[string]interface{}{
			"type":     writeType,
			"filePath": filePath,
		},
		Content: []types.ContentBlock{{
			Type: types.ContentBlockText,
			Text: fmt.Sprintf("Successfully wrote to %s (%s)", filePath, writeType),
		}},
	}, nil
}
