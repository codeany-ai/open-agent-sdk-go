package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/codeany-ai/open-agent-sdk-go/types"
)

func TestWorktreeTools(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	readme := filepath.Join(dir, "README.md")
	os.WriteFile(readme, []byte("hello"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()

	store := NewWorktreeStore()
	enter := NewEnterWorktreeTool(store)
	exit := NewExitWorktreeTool(store)
	ctx := context.Background()
	tCtx := &types.ToolUseContext{WorkingDir: dir}

	r, _ := enter.Call(ctx, map[string]interface{}{"branch": "test-wt"}, tCtx)
	if r.IsError {
		t.Fatalf("enter failed: %s", r.Content[0].Text)
	}

	ids := store.List()
	if len(ids) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(ids))
	}

	r, _ = exit.Call(ctx, map[string]interface{}{"id": ids[0], "action": "remove"}, tCtx)
	if r.IsError {
		t.Fatalf("exit failed: %s", r.Content[0].Text)
	}

	if len(store.List()) != 0 {
		t.Error("expected store to be empty after remove")
	}
}
