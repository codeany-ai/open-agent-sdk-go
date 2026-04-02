package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/codeany-ai/open-agent-sdk-go/types"
)

// LSPTool provides Language Server Protocol operations via grep-based fallback.
type LSPTool struct{}

// NewLSPTool creates a new LSPTool.
func NewLSPTool() *LSPTool { return &LSPTool{} }

func (t *LSPTool) Name() string { return "LSP" }
func (t *LSPTool) Description() string {
	return "Language Server Protocol operations: goToDefinition, findReferences, hover, documentSymbol, workspaceSymbol."
}
func (t *LSPTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"operation": map[string]interface{}{
				"type": "string",
				"enum": []string{"goToDefinition", "findReferences", "hover", "documentSymbol", "workspaceSymbol"},
			},
			"file_path": map[string]interface{}{"type": "string", "description": "File path for the operation"},
			"line":      map[string]interface{}{"type": "number", "description": "Line number (0-based)"},
			"character": map[string]interface{}{"type": "number", "description": "Character position (0-based)"},
			"query":     map[string]interface{}{"type": "string", "description": "Symbol name (for workspaceSymbol)"},
		},
		Required: []string{"operation"},
	}
}
func (t *LSPTool) IsConcurrencySafe(_ map[string]interface{}) bool { return true }
func (t *LSPTool) IsReadOnly(_ map[string]interface{}) bool        { return true }

func (t *LSPTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	operation, _ := input["operation"].(string)
	filePath, _ := input["file_path"].(string)
	line := toInt(input["line"])
	character := toInt(input["character"])
	query, _ := input["query"].(string)
	cwd := tCtx.WorkingDir

	switch operation {
	case "goToDefinition":
		if filePath == "" {
			return errorResult("file_path required"), nil
		}
		symbol, err := getSymbolAtPosition(ctx, filePath, line, character)
		if err != nil || symbol == "" {
			return textResult("Could not identify symbol at position"), nil
		}
		out, _ := runRg(ctx, cwd, fmt.Sprintf(`\b(func|type|var|const|class|interface)\s+%s\b`, symbol), "")
		if out == "" {
			out = fmt.Sprintf("No definition found for %q", symbol)
		}
		return textResult(out), nil

	case "findReferences":
		if filePath == "" {
			return errorResult("file_path required"), nil
		}
		symbol, err := getSymbolAtPosition(ctx, filePath, line, character)
		if err != nil || symbol == "" {
			return textResult("Could not identify symbol at position"), nil
		}
		out, _ := runRg(ctx, cwd, `\b`+symbol+`\b`, "")
		if out == "" {
			out = fmt.Sprintf("No references found for %q", symbol)
		}
		return textResult(out), nil

	case "hover":
		return textResult("Hover information requires a running language server. Use Read tool to examine the file content."), nil

	case "documentSymbol":
		if filePath == "" {
			return errorResult("file_path required"), nil
		}
		out, _ := runRg(ctx, cwd, `^\s*(export\s+)?(func|type|var|const|class|interface|struct|enum)\s+\w+`, filePath)
		if out == "" {
			out = "No symbols found"
		}
		return textResult(out), nil

	case "workspaceSymbol":
		if query == "" {
			return errorResult("query required"), nil
		}
		out, _ := runRg(ctx, cwd, fmt.Sprintf(`\b(func|type|var|const|class|interface)\s+%s\b`, query), "")
		if out == "" {
			out = fmt.Sprintf("No symbols found for %q", query)
		}
		return textResult(out), nil

	default:
		return errorResult(fmt.Sprintf("Unknown operation: %s", operation)), nil
	}
}

func getSymbolAtPosition(_ context.Context, filePath string, line, character int) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if line >= len(lines) {
		return "", nil
	}
	lineStr := lines[line]
	if character >= len(lineStr) {
		character = len(lineStr) - 1
	}
	if character < 0 {
		return "", nil
	}
	isWord := func(c byte) bool {
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
	}
	start, end := character, character
	for start > 0 && isWord(lineStr[start-1]) {
		start--
	}
	for end < len(lineStr) && isWord(lineStr[end]) {
		end++
	}
	return lineStr[start:end], nil
}

func runRg(ctx context.Context, cwd, pattern, file string) (string, error) {
	args := []string{"-n", "--no-heading", pattern}
	if file != "" {
		args = append(args, file)
	} else {
		args = append(args, cwd)
	}
	cmd := exec.CommandContext(ctx, "rg", args...)
	out, _ := cmd.Output()
	result := strings.TrimSpace(string(out))
	if len(result) > 5000 {
		lines := strings.Split(result, "\n")
		if len(lines) > 50 {
			lines = lines[:50]
		}
		result = strings.Join(lines, "\n")
	}
	return result, nil
}
