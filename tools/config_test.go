package tools

import (
	"context"
	"testing"
)

func TestConfigTool(t *testing.T) {
	store := NewConfigStore()
	tool := NewConfigTool(store)
	ctx := context.Background()
	tCtx := testToolCtx(t)

	// set
	r, err := tool.Call(ctx, map[string]interface{}{"action": "set", "key": "model", "value": "claude-opus"}, tCtx)
	if err != nil || r.IsError {
		t.Fatalf("set failed: %v", err)
	}

	// get
	r, _ = tool.Call(ctx, map[string]interface{}{"action": "get", "key": "model"}, tCtx)
	if r.IsError {
		t.Fatal(r.Error)
	}
	if r.Content[0].Text != `"claude-opus"` {
		t.Errorf("got %q, want %q", r.Content[0].Text, `"claude-opus"`)
	}

	// list
	r, _ = tool.Call(ctx, map[string]interface{}{"action": "list"}, tCtx)
	if r.IsError {
		t.Fatal(r.Error)
	}

	// get nonexistent
	r, _ = tool.Call(ctx, map[string]interface{}{"action": "get", "key": "missing"}, tCtx)
	if r.IsError {
		t.Fatal("expected non-error for missing key")
	}
}
