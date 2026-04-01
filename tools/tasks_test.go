package tools

import (
	"context"
	"testing"

	"github.com/codeany-ai/open-agent-sdk-go/types"
)

func testToolCtx(t *testing.T) *types.ToolUseContext {
	return &types.ToolUseContext{WorkingDir: t.TempDir()}
}

func TestTaskStop(t *testing.T) {
	store := NewTaskStore()
	task := store.Create("test task", "desc", "")
	tool := &TaskStopTool{Store: store}

	result, err := tool.Call(context.Background(), map[string]interface{}{
		"taskId": task.ID,
		"reason": "test cancel",
	}, testToolCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	stopped := store.Get(task.ID)
	if stopped == nil {
		t.Fatal("task not found after stop")
	}
	if stopped.Status != TaskStatusCancelled {
		t.Errorf("expected cancelled, got %s", stopped.Status)
	}
}

func TestTaskOutput(t *testing.T) {
	store := NewTaskStore()
	task := store.Create("test task", "desc", "")
	store.SetOutput(task.ID, "hello output")
	tool := &TaskOutputTool{Store: store}

	result, err := tool.Call(context.Background(), map[string]interface{}{
		"taskId": task.ID,
	}, testToolCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Content[0].Text != "hello output" {
		t.Errorf("got %q, want %q", result.Content[0].Text, "hello output")
	}
}
