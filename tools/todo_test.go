package tools

import (
	"context"
	"strings"
	"testing"
)

func TestTodoWrite(t *testing.T) {
	store := NewTodoStore()
	tool := NewTodoWriteTool(store)
	ctx := context.Background()
	tCtx := testToolCtx(t)

	// add
	r, err := tool.Call(ctx, map[string]interface{}{"action": "add", "text": "buy milk", "priority": "high"}, tCtx)
	if err != nil || r.IsError {
		t.Fatalf("add failed: err=%v, isError=%v", err, r.IsError)
	}

	// list
	r, _ = tool.Call(ctx, map[string]interface{}{"action": "list"}, tCtx)
	if !strings.Contains(r.Content[0].Text, "buy milk") {
		t.Errorf("expected 'buy milk' in list, got: %s", r.Content[0].Text)
	}

	// toggle
	r, _ = tool.Call(ctx, map[string]interface{}{"action": "toggle", "id": float64(1)}, tCtx)
	if r.IsError {
		t.Fatal(r.Error)
	}

	// remove
	r, _ = tool.Call(ctx, map[string]interface{}{"action": "remove", "id": float64(1)}, tCtx)
	if r.IsError {
		t.Fatal(r.Error)
	}

	// clear
	r, _ = tool.Call(ctx, map[string]interface{}{"action": "clear"}, tCtx)
	if r.IsError {
		t.Fatal(r.Error)
	}

	// list after clear
	r, _ = tool.Call(ctx, map[string]interface{}{"action": "list"}, tCtx)
	if strings.Contains(r.Content[0].Text, "buy milk") {
		t.Error("expected empty list after clear")
	}
}
