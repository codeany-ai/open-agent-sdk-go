package permissions

import (
	"strings"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

// Rule represents a permission rule.
type Rule struct {
	// ToolName is the tool to match (e.g., "Bash", "Edit")
	ToolName string `json:"tool_name"`
	// Pattern is an optional pattern (e.g., "git *" for Bash)
	Pattern string `json:"pattern,omitempty"`
}

// Config holds all permission configuration.
type Config struct {
	Mode       types.PermissionMode `json:"mode"`
	AllowRules []Rule               `json:"allow_rules,omitempty"`
	DenyRules  []Rule               `json:"deny_rules,omitempty"`
}

// DefaultConfig returns the default permission configuration.
func DefaultConfig() *Config {
	return &Config{
		Mode: types.PermissionModeBypassPermissions,
	}
}

// NewCanUseToolFn creates a CanUseToolFn from a permission config.
func NewCanUseToolFn(config *Config, allowedTools []string) types.CanUseToolFn {
	allowedSet := make(map[string]bool, len(allowedTools))
	for _, t := range allowedTools {
		allowedSet[t] = true
	}

	return func(tool types.Tool, input map[string]interface{}) (*types.PermissionDecision, error) {
		toolName := tool.Name()

		// Check deny rules first
		for _, rule := range config.DenyRules {
			if matchesRule(rule, toolName, input) {
				return &types.PermissionDecision{
					Behavior: types.PermissionDeny,
					Reason:   "Denied by rule: " + rule.ToolName,
				}, nil
			}
		}

		// Check allow rules
		for _, rule := range config.AllowRules {
			if matchesRule(rule, toolName, input) {
				return &types.PermissionDecision{
					Behavior: types.PermissionAllow,
				}, nil
			}
		}

		// Check allowedTools set
		if len(allowedSet) > 0 {
			if !allowedSet[toolName] {
				if config.Mode == types.PermissionModeBypassPermissions {
					return &types.PermissionDecision{Behavior: types.PermissionAllow}, nil
				}
				return &types.PermissionDecision{
					Behavior: types.PermissionDeny,
					Reason:   "Tool not in allowed list",
				}, nil
			}
		}

		// Apply permission mode
		switch config.Mode {
		case types.PermissionModeBypassPermissions:
			return &types.PermissionDecision{Behavior: types.PermissionAllow}, nil
		case types.PermissionModeAcceptEdits:
			if tool.IsReadOnly(input) || isFileEditTool(toolName) {
				return &types.PermissionDecision{Behavior: types.PermissionAllow}, nil
			}
			return &types.PermissionDecision{Behavior: types.PermissionAllow}, nil
		case types.PermissionModePlan:
			return &types.PermissionDecision{Behavior: types.PermissionAllow}, nil
		default:
			return &types.PermissionDecision{Behavior: types.PermissionAllow}, nil
		}
	}
}

// matchesRule checks if a rule matches the tool and input.
func matchesRule(rule Rule, toolName string, input map[string]interface{}) bool {
	if rule.ToolName != toolName {
		// Check for MCP prefix matching
		if !strings.HasPrefix(toolName, rule.ToolName) {
			return false
		}
	}

	if rule.Pattern == "" {
		return true
	}

	// Match pattern against relevant input
	var value string
	switch toolName {
	case "Bash":
		value, _ = input["command"].(string)
	case "Edit", "Write", "Read":
		value, _ = input["file_path"].(string)
	case "Glob":
		value, _ = input["pattern"].(string)
	case "Grep":
		value, _ = input["pattern"].(string)
	default:
		return true
	}

	return simpleWildcardMatch(rule.Pattern, value)
}

// simpleWildcardMatch performs simple wildcard matching with *.
func simpleWildcardMatch(pattern, value string) bool {
	if pattern == "*" {
		return true
	}

	// Simple prefix/suffix matching
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(value, strings.TrimPrefix(pattern, "*"))
	}

	return pattern == value
}

func isFileEditTool(name string) bool {
	return name == "Edit" || name == "Write" || name == "Read" || name == "Glob" || name == "Grep"
}
