package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

const (
	bashDefaultTimeout = 120 * time.Second
	bashMaxTimeout     = 600 * time.Second
	bashMaxOutputSize  = 1024 * 1024 // 1MB
)

// BashTool executes shell commands.
type BashTool struct{}

func NewBashTool() *BashTool { return &BashTool{} }

func (t *BashTool) Name() string { return "Bash" }

func (t *BashTool) Description() string {
	return `Executes a given bash command and returns its output.
The working directory persists between commands, but shell state does not.
Use this tool for running shell commands, git operations, build tasks, etc.`
}

func (t *BashTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The bash command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Optional timeout in milliseconds (max 600000)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Clear description of what this command does",
			},
		},
		Required: []string{"command"},
	}
}

func (t *BashTool) IsConcurrencySafe(input map[string]interface{}) bool { return false }
func (t *BashTool) IsReadOnly(input map[string]interface{}) bool        { return false }

func (t *BashTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	command, _ := input["command"].(string)
	if command == "" {
		return &types.ToolResult{
			IsError: true,
			Error:   "command is required",
		}, nil
	}

	// Parse timeout
	timeout := bashDefaultTimeout
	if timeoutMs, ok := input["timeout"].(float64); ok {
		timeout = time.Duration(timeoutMs) * time.Millisecond
		if timeout > bashMaxTimeout {
			timeout = bashMaxTimeout
		}
	}

	// Determine working directory
	workDir := "."
	if tCtx != nil && tCtx.WorkingDir != "" {
		workDir = tCtx.WorkingDir
	}

	// Create command with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", command)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Build output
	out := stdout.String()
	errOut := stderr.String()

	// Truncate if too large
	if len(out) > bashMaxOutputSize {
		out = out[:bashMaxOutputSize] + "\n... (output truncated)"
	}

	var result string
	if out != "" {
		result = out
	}
	if errOut != "" {
		if result != "" {
			result += "\n"
		}
		result += errOut
	}

	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return &types.ToolResult{
				IsError: true,
				Error:   fmt.Sprintf("Command timed out after %v", timeout),
				Content: []types.ContentBlock{{
					Type: types.ContentBlockText,
					Text: result,
				}},
			}, nil
		}

		// Command exited with non-zero status, still return output
		if result == "" {
			result = err.Error()
		}
	}

	if result == "" {
		result = "(no output)"
	}

	return &types.ToolResult{
		Data: map[string]interface{}{
			"stdout":      stdout.String(),
			"stderr":      stderr.String(),
			"interrupted": cmdCtx.Err() == context.DeadlineExceeded,
		},
		Content: []types.ContentBlock{{
			Type: types.ContentBlockText,
			Text: strings.TrimRight(result, "\n"),
		}},
	}, nil
}

// isReadCommand returns true if the command is read-only.
func isReadCommand(cmd string) bool {
	readPrefixes := []string{
		"cat ", "head ", "tail ", "less ", "more ",
		"ls ", "dir ", "find ", "locate ",
		"grep ", "rg ", "ag ", "ack ",
		"git log", "git show", "git diff", "git status", "git branch",
		"echo ", "printf ",
		"wc ", "du ", "df ",
		"which ", "whereis ", "type ",
		"env", "printenv",
	}
	trimmed := strings.TrimSpace(cmd)
	for _, prefix := range readPrefixes {
		if strings.HasPrefix(trimmed, prefix) || trimmed == strings.TrimSpace(prefix) {
			return true
		}
	}
	return false
}
