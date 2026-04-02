package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/codeany-ai/open-agent-sdk-go/types"
)

func TestLSPToolDocumentSymbol(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	os.WriteFile(goFile, []byte("package main\n\nfunc Hello() {}\ntype Foo struct{}\n"), 0644)

	tool := NewLSPTool()
	r, _ := tool.Call(context.Background(), map[string]interface{}{
		"operation": "documentSymbol",
		"file_path": goFile,
	}, &types.ToolUseContext{WorkingDir: dir})
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Content[0].Text)
	}
}

func TestLSPToolHover(t *testing.T) {
	tool := NewLSPTool()
	r, _ := tool.Call(context.Background(), map[string]interface{}{
		"operation": "hover",
	}, &types.ToolUseContext{})
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Content[0].Text)
	}
}

func TestLSPToolUnknownOperation(t *testing.T) {
	tool := NewLSPTool()
	r, _ := tool.Call(context.Background(), map[string]interface{}{
		"operation": "unknown",
	}, &types.ToolUseContext{})
	if !r.IsError {
		t.Error("expected error for unknown operation")
	}
}
