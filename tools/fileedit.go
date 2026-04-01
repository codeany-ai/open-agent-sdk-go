package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

// FileEditTool performs string replacements in files.
type FileEditTool struct{}

func NewFileEditTool() *FileEditTool { return &FileEditTool{} }

func (t *FileEditTool) Name() string { return "Edit" }

func (t *FileEditTool) Description() string {
	return `Performs exact string replacements in files. The old_string must be unique in the file
unless replace_all is set to true. Use this tool for making targeted changes to existing files.`
}

func (t *FileEditTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path to the file to modify",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "The text to find and replace",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "The replacement text",
			},
			"replace_all": map[string]interface{}{
				"type":        "boolean",
				"description": "Replace all occurrences (default false)",
			},
		},
		Required: []string{"file_path", "old_string", "new_string"},
	}
}

func (t *FileEditTool) IsConcurrencySafe(input map[string]interface{}) bool { return false }
func (t *FileEditTool) IsReadOnly(input map[string]interface{}) bool        { return false }

func (t *FileEditTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	filePath, _ := input["file_path"].(string)
	oldString, _ := input["old_string"].(string)
	newString, _ := input["new_string"].(string)
	replaceAll, _ := input["replace_all"].(bool)

	if filePath == "" {
		return &types.ToolResult{IsError: true, Error: "file_path is required"}, nil
	}
	if oldString == "" {
		return &types.ToolResult{IsError: true, Error: "old_string is required"}, nil
	}
	if oldString == newString {
		return &types.ToolResult{IsError: true, Error: "old_string and new_string must be different"}, nil
	}

	// Resolve relative paths
	if !filepath.IsAbs(filePath) && tCtx != nil && tCtx.WorkingDir != "" {
		filePath = filepath.Join(tCtx.WorkingDir, filePath)
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &types.ToolResult{
				IsError: true,
				Error:   fmt.Sprintf("File does not exist: %s", filePath),
			}, nil
		}
		return &types.ToolResult{IsError: true, Error: err.Error()}, nil
	}

	content := string(data)

	// Count occurrences
	count := strings.Count(content, oldString)
	if count == 0 {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("old_string not found in %s. Make sure the string matches exactly, including whitespace and indentation.", filePath),
		}, nil
	}

	if count > 1 && !replaceAll {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("old_string found %d times in %s. Use replace_all=true to replace all occurrences, or provide more context to make the match unique.", count, filePath),
		}, nil
	}

	// Perform replacement
	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(content, oldString, newString)
	} else {
		newContent = strings.Replace(content, oldString, newString, 1)
	}

	// Write back
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("Failed to write file: %v", err),
		}, nil
	}

	replacements := 1
	if replaceAll {
		replacements = count
	}

	return &types.ToolResult{
		Data: map[string]interface{}{
			"filePath":     filePath,
			"replacements": replacements,
		},
		Content: []types.ContentBlock{{
			Type: types.ContentBlockText,
			Text: fmt.Sprintf("Successfully edited %s (%d replacement(s))", filePath, replacements),
		}},
	}, nil
}
