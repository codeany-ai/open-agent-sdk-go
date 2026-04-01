package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/codeany-ai/open-agent-sdk-go/types"
)

// TodoItem represents a single todo entry.
type TodoItem struct {
	ID       int    `json:"id"`
	Text     string `json:"text"`
	Done     bool   `json:"done"`
	Priority string `json:"priority,omitempty"` // high|medium|low
}

// TodoStore manages session-scoped todos.
type TodoStore struct {
	mu      sync.Mutex
	items   []*TodoItem
	counter atomic.Int64
}

// NewTodoStore creates a new TodoStore.
func NewTodoStore() *TodoStore { return &TodoStore{} }

func (s *TodoStore) Add(text, priority string) *TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := &TodoItem{ID: int(s.counter.Add(1)), Text: text, Priority: priority}
	s.items = append(s.items, item)
	return item
}

func (s *TodoStore) Toggle(id int) (*TodoItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.ID == id {
			item.Done = !item.Done
			cp := *item
			return &cp, true
		}
	}
	return nil, false
}

func (s *TodoStore) Remove(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.items {
		if item.ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return true
		}
	}
	return false
}

func (s *TodoStore) List() []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]TodoItem, len(s.items))
	for i, item := range s.items {
		result[i] = *item
	}
	return result
}

func (s *TodoStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = nil
}

// TodoWriteTool manages a session todo/checklist.
type TodoWriteTool struct{ Store *TodoStore }

// NewTodoWriteTool creates a new TodoWriteTool.
func NewTodoWriteTool(store *TodoStore) *TodoWriteTool { return &TodoWriteTool{Store: store} }

func (t *TodoWriteTool) Name() string { return "TodoWrite" }
func (t *TodoWriteTool) Description() string {
	return "Manage a session todo/checklist. Supports add, toggle, remove, list, and clear operations."
}
func (t *TodoWriteTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"action":   map[string]interface{}{"type": "string", "enum": []string{"add", "toggle", "remove", "list", "clear"}, "description": "Operation to perform"},
			"text":     map[string]interface{}{"type": "string", "description": "Todo item text (for add)"},
			"id":       map[string]interface{}{"type": "number", "description": "Todo item ID (for toggle/remove)"},
			"priority": map[string]interface{}{"type": "string", "enum": []string{"high", "medium", "low"}, "description": "Priority level (for add)"},
		},
		Required: []string{"action"},
	}
}
func (t *TodoWriteTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *TodoWriteTool) IsReadOnly(input map[string]interface{}) bool        { return false }

func (t *TodoWriteTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	action, _ := input["action"].(string)
	switch action {
	case "add":
		text, _ := input["text"].(string)
		if text == "" {
			return errorResult("text required for add"), nil
		}
		priority, _ := input["priority"].(string)
		item := t.Store.Add(text, priority)
		return textResult(fmt.Sprintf("Todo added: #%d \"%s\"", item.ID, item.Text)), nil

	case "toggle":
		id := toInt(input["id"])
		item, ok := t.Store.Toggle(id)
		if !ok {
			return errorResult(fmt.Sprintf("Todo #%d not found", id)), nil
		}
		state := "reopened"
		if item.Done {
			state = "completed"
		}
		return textResult(fmt.Sprintf("Todo #%d %s", id, state)), nil

	case "remove":
		id := toInt(input["id"])
		if !t.Store.Remove(id) {
			return errorResult(fmt.Sprintf("Todo #%d not found", id)), nil
		}
		return textResult(fmt.Sprintf("Todo #%d removed", id)), nil

	case "list":
		items := t.Store.List()
		if len(items) == 0 {
			return textResult("No todos."), nil
		}
		var lines []string
		for _, item := range items {
			check := "[ ]"
			if item.Done {
				check = "[x]"
			}
			pri := ""
			if item.Priority != "" {
				pri = fmt.Sprintf(" (%s)", item.Priority)
			}
			lines = append(lines, fmt.Sprintf("%s #%d %s%s", check, item.ID, item.Text, pri))
		}
		return textResult(strings.Join(lines, "\n")), nil

	case "clear":
		t.Store.Clear()
		return textResult("All todos cleared."), nil

	default:
		return errorResult(fmt.Sprintf("Unknown action: %s", action)), nil
	}
}

// toInt converts interface{} to int (handles float64 from JSON).
func toInt(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return 0
}
