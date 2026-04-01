package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

const (
	defaultReadLimit = 2000
	maxReadLimit     = 100000
)

// FileReadTool reads files from the filesystem.
type FileReadTool struct{}

func NewFileReadTool() *FileReadTool { return &FileReadTool{} }

func (t *FileReadTool) Name() string { return "Read" }

func (t *FileReadTool) Description() string {
	return `Reads a file from the local filesystem. Returns the file content with line numbers.
The file_path must be an absolute path. By default reads up to 2000 lines.
Use offset and limit to read specific parts of large files.`
}

func (t *FileReadTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path to the file to read",
			},
			"offset": map[string]interface{}{
				"type":        "number",
				"description": "Line number to start reading from (0-based)",
			},
			"limit": map[string]interface{}{
				"type":        "number",
				"description": "Maximum number of lines to read",
			},
		},
		Required: []string{"file_path"},
	}
}

func (t *FileReadTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *FileReadTool) IsReadOnly(input map[string]interface{}) bool        { return true }

func (t *FileReadTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	filePath, _ := input["file_path"].(string)
	if filePath == "" {
		return &types.ToolResult{IsError: true, Error: "file_path is required"}, nil
	}

	// Resolve relative paths
	if !filepath.IsAbs(filePath) && tCtx != nil && tCtx.WorkingDir != "" {
		filePath = filepath.Join(tCtx.WorkingDir, filePath)
	}

	// Check file exists
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &types.ToolResult{
				IsError: true,
				Error:   fmt.Sprintf("File does not exist: %s", filePath),
			}, nil
		}
		return &types.ToolResult{IsError: true, Error: err.Error()}, nil
	}

	if info.IsDir() {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("%s is a directory, not a file. Use Bash with 'ls' to list directory contents.", filePath),
		}, nil
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return &types.ToolResult{IsError: true, Error: err.Error()}, nil
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	// Apply offset and limit
	offset := 0
	if o, ok := input["offset"].(float64); ok {
		offset = int(o)
	}
	limit := defaultReadLimit
	if l, ok := input["limit"].(float64); ok {
		limit = int(l)
		if limit > maxReadLimit {
			limit = maxReadLimit
		}
	}

	if offset >= totalLines {
		return &types.ToolResult{
			IsError: true,
			Error:   fmt.Sprintf("Offset %d is beyond end of file (%d lines)", offset, totalLines),
		}, nil
	}

	end := offset + limit
	if end > totalLines {
		end = totalLines
	}

	// Format with line numbers (cat -n style)
	var sb strings.Builder
	for i := offset; i < end; i++ {
		sb.WriteString(strconv.Itoa(i+1) + "\t" + lines[i] + "\n")
	}

	// Track file state for staleness detection
	if tCtx != nil && tCtx.ReadFileState != nil {
		tCtx.ReadFileState[filePath] = &types.FileReadState{
			Content:   content,
			Timestamp: time.Now().UnixMilli(),
			Offset:    offset,
			Limit:     limit,
		}
	}

	result := sb.String()
	if end < totalLines {
		result += fmt.Sprintf("\n(showing lines %d-%d of %d total)", offset+1, end, totalLines)
	}

	return &types.ToolResult{
		Data: map[string]interface{}{
			"filePath":   filePath,
			"content":    content,
			"numLines":   end - offset,
			"startLine":  offset,
			"totalLines": totalLines,
		},
		Content: []types.ContentBlock{{
			Type: types.ContentBlockText,
			Text: result,
		}},
	}, nil
}
