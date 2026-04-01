package hooks

import (
	"context"
	"fmt"
	"strings"
)

// HookEvent represents when a hook fires.
type HookEvent string

const (
	HookPreToolUse   HookEvent = "PreToolUse"
	HookPostToolUse  HookEvent = "PostToolUse"
	HookPostSampling HookEvent = "PostSampling"
	HookStop         HookEvent = "Stop"
)

// HookFn is a function that runs as a hook.
// Returns an error message to block the action, or empty string to allow.
type HookFn func(ctx context.Context, toolName string, input map[string]interface{}) (string, error)

// HookRule defines when a hook should fire.
type HookRule struct {
	// Matcher is a tool name pattern (e.g., "Bash", "Edit|Write", "*")
	Matcher string `json:"matcher"`
	// Hooks are the functions to run
	Hooks []HookFn `json:"-"`
}

// HookConfig holds all hook definitions.
type HookConfig struct {
	PreToolUse   []HookRule `json:"PreToolUse,omitempty"`
	PostToolUse  []HookRule `json:"PostToolUse,omitempty"`
	PostSampling []HookRule `json:"PostSampling,omitempty"`
	Stop         []HookRule `json:"Stop,omitempty"`
}

// Manager handles hook execution.
type Manager struct {
	config HookConfig
}

// NewManager creates a new hook manager.
func NewManager(config HookConfig) *Manager {
	return &Manager{config: config}
}

// RunPreToolUse runs pre-tool-use hooks. Returns error message if blocked.
func (m *Manager) RunPreToolUse(ctx context.Context, toolName string, input map[string]interface{}) (string, error) {
	return m.runHooks(ctx, m.config.PreToolUse, toolName, input)
}

// RunPostToolUse runs post-tool-use hooks.
func (m *Manager) RunPostToolUse(ctx context.Context, toolName string, input map[string]interface{}) (string, error) {
	return m.runHooks(ctx, m.config.PostToolUse, toolName, input)
}

// RunPostSampling runs post-sampling hooks.
func (m *Manager) RunPostSampling(ctx context.Context, toolName string, input map[string]interface{}) (string, error) {
	return m.runHooks(ctx, m.config.PostSampling, toolName, input)
}

// RunStop runs stop hooks.
func (m *Manager) RunStop(ctx context.Context, toolName string, input map[string]interface{}) (string, error) {
	return m.runHooks(ctx, m.config.Stop, toolName, input)
}

func (m *Manager) runHooks(ctx context.Context, rules []HookRule, toolName string, input map[string]interface{}) (string, error) {
	for _, rule := range rules {
		if !matchesTool(rule.Matcher, toolName) {
			continue
		}
		for _, hook := range rule.Hooks {
			msg, err := hook(ctx, toolName, input)
			if err != nil {
				return "", fmt.Errorf("hook error: %w", err)
			}
			if msg != "" {
				return msg, nil
			}
		}
	}
	return "", nil
}

// matchesTool checks if a matcher pattern matches a tool name.
// Supports: exact match, pipe-separated alternatives, and "*" wildcard.
func matchesTool(matcher, toolName string) bool {
	if matcher == "*" {
		return true
	}

	// Check pipe-separated alternatives
	parts := strings.Split(matcher, "|")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == toolName {
			return true
		}
		// Simple wildcard
		if strings.Contains(part, "*") {
			if strings.HasPrefix(part, "*") && strings.HasSuffix(toolName, strings.TrimPrefix(part, "*")) {
				return true
			}
			if strings.HasSuffix(part, "*") && strings.HasPrefix(toolName, strings.TrimSuffix(part, "*")) {
				return true
			}
		}
	}

	return false
}
