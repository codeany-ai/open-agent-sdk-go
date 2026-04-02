package tools

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/codeany-ai/open-agent-sdk-go/types"
	"github.com/google/uuid"
)

// WorktreeEntry holds metadata for an active git worktree.
type WorktreeEntry struct {
	ID          string
	Path        string
	Branch      string
	OriginalCwd string
}

// WorktreeStore manages active worktrees in memory.
type WorktreeStore struct {
	mu    sync.Mutex
	trees map[string]*WorktreeEntry
}

// NewWorktreeStore creates a new WorktreeStore.
func NewWorktreeStore() *WorktreeStore {
	return &WorktreeStore{trees: make(map[string]*WorktreeEntry)}
}

func (s *WorktreeStore) Add(entry *WorktreeEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trees[entry.ID] = entry
}

func (s *WorktreeStore) Get(id string) (*WorktreeEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.trees[id]
	return e, ok
}

func (s *WorktreeStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.trees, id)
}

func (s *WorktreeStore) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.trees))
	for id := range s.trees {
		ids = append(ids, id)
	}
	return ids
}

// EnterWorktreeTool creates an isolated git worktree for parallel work.
type EnterWorktreeTool struct{ Store *WorktreeStore }

// NewEnterWorktreeTool creates a new EnterWorktreeTool.
func NewEnterWorktreeTool(store *WorktreeStore) *EnterWorktreeTool {
	return &EnterWorktreeTool{Store: store}
}

func (t *EnterWorktreeTool) Name() string        { return "EnterWorktree" }
func (t *EnterWorktreeTool) Description() string { return "Create an isolated git worktree for parallel work." }
func (t *EnterWorktreeTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"branch": map[string]interface{}{"type": "string", "description": "Branch name for the worktree"},
			"path":   map[string]interface{}{"type": "string", "description": "Path for the worktree"},
		},
	}
}
func (t *EnterWorktreeTool) IsConcurrencySafe(_ map[string]interface{}) bool { return false }
func (t *EnterWorktreeTool) IsReadOnly(_ map[string]interface{}) bool         { return false }

func (t *EnterWorktreeTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	cwd := tCtx.WorkingDir

	if out, err := exec.CommandContext(ctx, "git", "-C", cwd, "rev-parse", "--git-dir").Output(); err != nil || len(out) == 0 {
		return errorResult("not a git repository"), nil
	}

	branch, _ := input["branch"].(string)
	if branch == "" {
		branch = fmt.Sprintf("worktree-%d", time.Now().UnixMilli())
	}

	wtPath, _ := input["path"].(string)
	if wtPath == "" {
		wtPath = filepath.Join(cwd, "..", ".worktree-"+branch)
	}

	// Create branch if it doesn't exist; ignore error if it already does.
	exec.CommandContext(ctx, "git", "-C", cwd, "branch", branch).Run()

	if out, err := exec.CommandContext(ctx, "git", "-C", cwd, "worktree", "add", wtPath, branch).CombinedOutput(); err != nil {
		return errorResult(fmt.Sprintf("git worktree add failed: %s", string(out))), nil
	}

	id := uuid.New().String()
	t.Store.Add(&WorktreeEntry{ID: id, Path: wtPath, Branch: branch, OriginalCwd: cwd})

	return textResult(fmt.Sprintf("Worktree created:\n  ID: %s\n  Path: %s\n  Branch: %s", id, wtPath, branch)), nil
}

// ExitWorktreeTool exits and optionally removes a git worktree.
type ExitWorktreeTool struct{ Store *WorktreeStore }

// NewExitWorktreeTool creates a new ExitWorktreeTool.
func NewExitWorktreeTool(store *WorktreeStore) *ExitWorktreeTool {
	return &ExitWorktreeTool{Store: store}
}

func (t *ExitWorktreeTool) Name() string        { return "ExitWorktree" }
func (t *ExitWorktreeTool) Description() string { return "Exit and optionally remove a git worktree." }
func (t *ExitWorktreeTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"id":     map[string]interface{}{"type": "string", "description": "Worktree ID"},
			"action": map[string]interface{}{"type": "string", "enum": []string{"keep", "remove"}, "description": "Whether to keep or remove the worktree (default: remove)"},
		},
		Required: []string{"id"},
	}
}
func (t *ExitWorktreeTool) IsConcurrencySafe(_ map[string]interface{}) bool { return false }
func (t *ExitWorktreeTool) IsReadOnly(_ map[string]interface{}) bool         { return false }

func (t *ExitWorktreeTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	id, _ := input["id"].(string)
	entry, ok := t.Store.Get(id)
	if !ok {
		return errorResult(fmt.Sprintf("worktree not found: %s", id)), nil
	}

	action, _ := input["action"].(string)
	if action == "" {
		action = "remove"
	}

	if action == "remove" {
		if out, err := exec.CommandContext(ctx, "git", "-C", entry.OriginalCwd, "worktree", "remove", "--force", entry.Path).CombinedOutput(); err != nil {
			return errorResult(fmt.Sprintf("git worktree remove failed: %s", string(out))), nil
		}
		// Delete the branch; ignore failure (e.g. branch checked out elsewhere).
		exec.CommandContext(ctx, "git", "-C", entry.OriginalCwd, "branch", "-D", entry.Branch).Run()
	}

	t.Store.Remove(id)
	return textResult(fmt.Sprintf("Worktree %sd: %s", action, entry.Path)), nil
}
