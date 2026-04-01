package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/codeany-ai/open-agent-sdk-go/types"
)

// ConfigStore is an in-memory key-value config store.
type ConfigStore struct {
	mu    sync.RWMutex
	store map[string]interface{}
}

// NewConfigStore creates a new ConfigStore.
func NewConfigStore() *ConfigStore { return &ConfigStore{store: make(map[string]interface{})} }

func (s *ConfigStore) Get(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.store[key]
	return v, ok
}

func (s *ConfigStore) Set(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[key] = value
}

func (s *ConfigStore) List() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]interface{}, len(s.store))
	for k, v := range s.store {
		cp[k] = v
	}
	return cp
}

// ConfigTool provides get/set/list for session-scoped config.
type ConfigTool struct{ Store *ConfigStore }

// NewConfigTool creates a new ConfigTool.
func NewConfigTool(store *ConfigStore) *ConfigTool { return &ConfigTool{Store: store} }

func (t *ConfigTool) Name() string { return "Config" }
func (t *ConfigTool) Description() string {
	return "Get or set configuration values. Supports session-scoped settings."
}
func (t *ConfigTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"action": map[string]interface{}{"type": "string", "enum": []string{"get", "set", "list"}},
			"key":    map[string]interface{}{"type": "string", "description": "Config key"},
			"value":  map[string]interface{}{"description": "Config value (for set)"},
		},
		Required: []string{"action"},
	}
}
func (t *ConfigTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *ConfigTool) IsReadOnly(input map[string]interface{}) bool        { return false }

func (t *ConfigTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	action, _ := input["action"].(string)
	switch action {
	case "get":
		key, _ := input["key"].(string)
		if key == "" {
			return errorResult("key required for get"), nil
		}
		v, ok := t.Store.Get(key)
		if !ok {
			return textResult(fmt.Sprintf("Config key %q not found", key)), nil
		}
		b, _ := json.Marshal(v)
		return textResult(string(b)), nil

	case "set":
		key, _ := input["key"].(string)
		if key == "" {
			return errorResult("key required for set"), nil
		}
		t.Store.Set(key, input["value"])
		b, _ := json.Marshal(input["value"])
		return textResult(fmt.Sprintf("Config set: %s = %s", key, string(b))), nil

	case "list":
		entries := t.Store.List()
		if len(entries) == 0 {
			return textResult("No config values set."), nil
		}
		var lines []string
		for k, v := range entries {
			b, _ := json.Marshal(v)
			lines = append(lines, fmt.Sprintf("%s = %s", k, string(b)))
		}
		return textResult(strings.Join(lines, "\n")), nil

	default:
		return errorResult(fmt.Sprintf("Unknown action: %s", action)), nil
	}
}
