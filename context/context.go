package agentcontext

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SystemContext holds system-level context for the agent.
type SystemContext struct {
	GitStatus string `json:"gitStatus,omitempty"`
}

// UserContext holds user-level context for the agent.
type UserContext struct {
	ClaudeMD    string `json:"claudeMd,omitempty"`
	CurrentDate string `json:"currentDate,omitempty"`
}

var (
	systemContextOnce sync.Once
	systemContext     *SystemContext

	userContextOnce sync.Once
	userContext      *UserContext
)

// GetSystemContext returns system context (memoized for the session).
func GetSystemContext(cwd string) *SystemContext {
	systemContextOnce.Do(func() {
		systemContext = &SystemContext{
			GitStatus: captureGitStatus(cwd),
		}
	})
	return systemContext
}

// GetUserContext returns user context (memoized for the session).
func GetUserContext(cwd string) *UserContext {
	userContextOnce.Do(func() {
		userContext = &UserContext{
			ClaudeMD:    loadClaudeMD(cwd),
			CurrentDate: fmt.Sprintf("Today's date is %s.", time.Now().Format("2006-01-02")),
		}
	})
	return userContext
}

// ResetContextCache clears the memoized contexts.
func ResetContextCache() {
	systemContextOnce = sync.Once{}
	userContextOnce = sync.Once{}
	systemContext = nil
	userContext = nil
}

// BuildSystemPromptBlocks returns the system prompt blocks for the API.
func BuildSystemPromptBlocks(systemPrompt string, sysCtx *SystemContext, userCtx *UserContext) []map[string]interface{} {
	blocks := []map[string]interface{}{
		{
			"type": "text",
			"text": systemPrompt,
		},
	}

	if sysCtx != nil && sysCtx.GitStatus != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "text",
			"text": sysCtx.GitStatus,
			"cache_control": map[string]string{"type": "ephemeral"},
		})
	}

	if userCtx != nil {
		var contextParts []string
		if userCtx.CurrentDate != "" {
			contextParts = append(contextParts, userCtx.CurrentDate)
		}
		if userCtx.ClaudeMD != "" {
			contextParts = append(contextParts, userCtx.ClaudeMD)
		}
		if len(contextParts) > 0 {
			blocks = append(blocks, map[string]interface{}{
				"type": "text",
				"text": strings.Join(contextParts, "\n\n"),
				"cache_control": map[string]string{"type": "ephemeral"},
			})
		}
	}

	return blocks
}

func captureGitStatus(cwd string) string {
	var parts []string

	// Current branch
	if branch := gitCmd(cwd, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" {
		parts = append(parts, "Current branch: "+branch)
	}

	// Git user
	if user := gitCmd(cwd, "config", "user.name"); user != "" {
		parts = append(parts, "Git user: "+user)
	}

	// Status
	if status := gitCmd(cwd, "status", "--short"); status != "" {
		parts = append(parts, "Status:\n"+status)
	} else {
		parts = append(parts, "Status: clean")
	}

	// Recent commits
	if log := gitCmd(cwd, "log", "--oneline", "-n", "5"); log != "" {
		parts = append(parts, "Recent commits:\n"+log)
	}

	if len(parts) == 0 {
		return ""
	}

	result := strings.Join(parts, "\n\n")
	// Truncate if too long
	if len(result) > 2000 {
		result = result[:2000] + "\n... (truncated)"
	}
	return result
}

func gitCmd(cwd string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func loadClaudeMD(cwd string) string {
	// Check for CLAUDE.md in the project root
	candidates := []string{
		filepath.Join(cwd, "CLAUDE.md"),
		filepath.Join(cwd, ".claude", "CLAUDE.md"),
	}

	// Also check home directory
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".claude", "CLAUDE.md"),
		)
	}

	var parts []string
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content != "" {
			parts = append(parts, content)
		}
	}

	return strings.Join(parts, "\n\n---\n\n")
}
