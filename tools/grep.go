package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

const defaultGrepHeadLimit = 250

// GrepTool searches file contents using ripgrep.
type GrepTool struct{}

func NewGrepTool() *GrepTool { return &GrepTool{} }

func (t *GrepTool) Name() string { return "Grep" }

func (t *GrepTool) Description() string {
	return `Search tool built on ripgrep. Supports full regex syntax.
Output modes: "content" shows matching lines, "files_with_matches" shows only file paths (default), "count" shows match counts.`
}

func (t *GrepTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The regex pattern to search for",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File or directory to search in (defaults to working directory)",
			},
			"glob": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern to filter files (e.g. \"*.js\")",
			},
			"output_mode": map[string]interface{}{
				"type":        "string",
				"description": "Output mode: content, files_with_matches, or count",
				"enum":        []string{"content", "files_with_matches", "count"},
			},
			"-i": map[string]interface{}{
				"type":        "boolean",
				"description": "Case insensitive search",
			},
			"-n": map[string]interface{}{
				"type":        "boolean",
				"description": "Show line numbers (default true for content mode)",
			},
			"-A": map[string]interface{}{
				"type":        "number",
				"description": "Lines to show after each match",
			},
			"-B": map[string]interface{}{
				"type":        "number",
				"description": "Lines to show before each match",
			},
			"-C": map[string]interface{}{
				"type":        "number",
				"description": "Context lines (before and after)",
			},
			"head_limit": map[string]interface{}{
				"type":        "number",
				"description": "Limit output to first N entries (default 250)",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "File type filter (e.g. js, py, go)",
			},
			"multiline": map[string]interface{}{
				"type":        "boolean",
				"description": "Enable multiline matching",
			},
		},
		Required: []string{"pattern"},
	}
}

func (t *GrepTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *GrepTool) IsReadOnly(input map[string]interface{}) bool        { return true }

func (t *GrepTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	pattern, _ := input["pattern"].(string)
	if pattern == "" {
		return &types.ToolResult{IsError: true, Error: "pattern is required"}, nil
	}

	searchPath, _ := input["path"].(string)
	if searchPath == "" && tCtx != nil {
		searchPath = tCtx.WorkingDir
	}
	if searchPath == "" {
		searchPath = "."
	}

	outputMode, _ := input["output_mode"].(string)
	if outputMode == "" {
		outputMode = "files_with_matches"
	}

	headLimit := defaultGrepHeadLimit
	if hl, ok := input["head_limit"].(float64); ok {
		headLimit = int(hl)
	}

	// Build rg command
	args := []string{
		"--hidden",
		"--max-columns", "500",
	}

	// Output mode
	switch outputMode {
	case "files_with_matches":
		args = append(args, "--files-with-matches")
	case "count":
		args = append(args, "--count")
	default: // "content"
		showLineNumbers := true
		if n, ok := input["-n"].(bool); ok {
			showLineNumbers = n
		}
		if showLineNumbers {
			args = append(args, "-n")
		}
	}

	// Case insensitive
	if ci, ok := input["-i"].(bool); ok && ci {
		args = append(args, "-i")
	}

	// Context lines
	if c, ok := input["-C"].(float64); ok {
		args = append(args, "-C", strconv.Itoa(int(c)))
	}
	if a, ok := input["-A"].(float64); ok {
		args = append(args, "-A", strconv.Itoa(int(a)))
	}
	if b, ok := input["-B"].(float64); ok {
		args = append(args, "-B", strconv.Itoa(int(b)))
	}

	// Glob filter
	if g, ok := input["glob"].(string); ok && g != "" {
		args = append(args, "--glob", g)
	}

	// Type filter
	if tp, ok := input["type"].(string); ok && tp != "" {
		args = append(args, "--type", tp)
	}

	// Multiline
	if ml, ok := input["multiline"].(bool); ok && ml {
		args = append(args, "-U", "--multiline-dotall")
	}

	// Pattern and path
	args = append(args, pattern, searchPath)

	cmd := exec.CommandContext(ctx, "rg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()

	// rg returns exit code 1 for no matches, which is not an error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				// No matches
				return &types.ToolResult{
					Data: map[string]interface{}{
						"mode":     outputMode,
						"numFiles": 0,
					},
					Content: []types.ContentBlock{{
						Type: types.ContentBlockText,
						Text: "No matches found.",
					}},
				}, nil
			}
		}
		// Check if rg is not installed
		if strings.Contains(err.Error(), "executable file not found") {
			return &types.ToolResult{
				IsError: true,
				Error:   "ripgrep (rg) is not installed. Please install it: https://github.com/BurntSushi/ripgrep",
			}, nil
		}
		if stderr.Len() > 0 {
			return &types.ToolResult{IsError: true, Error: stderr.String()}, nil
		}
	}

	// Apply head limit
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	truncated := false
	if headLimit > 0 && len(lines) > headLimit {
		lines = lines[:headLimit]
		truncated = true
	}

	result := strings.Join(lines, "\n")
	if truncated {
		result += fmt.Sprintf("\n\n(results truncated to %d entries)", headLimit)
	}

	numFiles := 0
	if outputMode == "files_with_matches" {
		numFiles = len(lines)
		if lines[0] == "" {
			numFiles = 0
		}
	}

	return &types.ToolResult{
		Data: map[string]interface{}{
			"mode":     outputMode,
			"numFiles": numFiles,
		},
		Content: []types.ContentBlock{{
			Type: types.ContentBlockText,
			Text: result,
		}},
	}, nil
}
