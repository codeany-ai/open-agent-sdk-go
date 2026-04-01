package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/codeany-ai/open-agent-sdk-go/types"
)

// TaskStatus represents the status of a task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusDeleted    TaskStatus = "deleted"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// Task represents a tracked task.
type Task struct {
	ID          string     `json:"id"`
	Subject     string     `json:"subject"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	ActiveForm  string     `json:"activeForm,omitempty"`
	Owner       string     `json:"owner,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	BlockedBy   []string   `json:"blockedBy,omitempty"`
	Blocks      []string   `json:"blocks,omitempty"`
	Output      string     `json:"output,omitempty"`
}

// TaskStore manages tasks in memory.
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

// NewTaskStore creates a new task store.
func NewTaskStore() *TaskStore {
	return &TaskStore{tasks: make(map[string]*Task)}
}

func (s *TaskStore) Create(subject, description, activeForm string) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("%d", len(s.tasks)+1)
	task := &Task{
		ID:          id,
		Subject:     subject,
		Description: description,
		Status:      TaskStatusPending,
		ActiveForm:  activeForm,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	s.tasks[id] = task
	return task
}

func (s *TaskStore) Get(id string) *Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if t, ok := s.tasks[id]; ok {
		cp := *t
		return &cp
	}
	return nil
}

func (s *TaskStore) List() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Task
	for _, t := range s.tasks {
		cp := *t
		result = append(result, &cp)
	}
	return result
}

func (s *TaskStore) Update(id string, updates map[string]interface{}) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return nil
	}

	if v, ok := updates["status"].(string); ok {
		t.Status = TaskStatus(v)
	}
	if v, ok := updates["subject"].(string); ok {
		t.Subject = v
	}
	if v, ok := updates["description"].(string); ok {
		t.Description = v
	}
	if v, ok := updates["activeForm"].(string); ok {
		t.ActiveForm = v
	}
	if v, ok := updates["owner"].(string); ok {
		t.Owner = v
	}
	t.UpdatedAt = time.Now()

	if t.Status == TaskStatusDeleted {
		delete(s.tasks, id)
		return t
	}

	cp := *t
	return &cp
}

// --- Task Tools ---

// TaskCreateTool creates tasks.
type TaskCreateTool struct{ Store *TaskStore }

func (t *TaskCreateTool) Name() string { return "TaskCreate" }
func (t *TaskCreateTool) Description() string {
	return "Create a structured task for tracking progress on complex work."
}
func (t *TaskCreateTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"subject":     map[string]interface{}{"type": "string", "description": "Brief title for the task"},
			"description": map[string]interface{}{"type": "string", "description": "What needs to be done"},
			"activeForm":  map[string]interface{}{"type": "string", "description": "Present continuous form for spinner"},
		},
		Required: []string{"subject", "description"},
	}
}
func (t *TaskCreateTool) IsConcurrencySafe(input map[string]interface{}) bool { return false }
func (t *TaskCreateTool) IsReadOnly(input map[string]interface{}) bool        { return false }
func (t *TaskCreateTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	subject, _ := input["subject"].(string)
	description, _ := input["description"].(string)
	activeForm, _ := input["activeForm"].(string)
	task := t.Store.Create(subject, description, activeForm)
	return &types.ToolResult{
		Content: []types.ContentBlock{{Type: types.ContentBlockText, Text: fmt.Sprintf("Task #%s created: %s", task.ID, subject)}},
		Data:    task,
	}, nil
}

// TaskGetTool retrieves a task.
type TaskGetTool struct{ Store *TaskStore }

func (t *TaskGetTool) Name() string        { return "TaskGet" }
func (t *TaskGetTool) Description() string { return "Get details of a specific task by ID." }
func (t *TaskGetTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"taskId": map[string]interface{}{"type": "string", "description": "The task ID"},
		},
		Required: []string{"taskId"},
	}
}
func (t *TaskGetTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *TaskGetTool) IsReadOnly(input map[string]interface{}) bool        { return true }
func (t *TaskGetTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	id, _ := input["taskId"].(string)
	task := t.Store.Get(id)
	if task == nil {
		return errorResult(fmt.Sprintf("Task %s not found", id)), nil
	}
	data, _ := json.Marshal(task)
	return &types.ToolResult{
		Content: []types.ContentBlock{{Type: types.ContentBlockText, Text: string(data)}},
		Data:    task,
	}, nil
}

// TaskListTool lists all tasks.
type TaskListTool struct{ Store *TaskStore }

func (t *TaskListTool) Name() string        { return "TaskList" }
func (t *TaskListTool) Description() string { return "List all tasks and their status." }
func (t *TaskListTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{Type: "object", Properties: map[string]interface{}{}}
}
func (t *TaskListTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *TaskListTool) IsReadOnly(input map[string]interface{}) bool        { return true }
func (t *TaskListTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	tasks := t.Store.List()
	data, _ := json.Marshal(tasks)
	return &types.ToolResult{
		Content: []types.ContentBlock{{Type: types.ContentBlockText, Text: string(data)}},
		Data:    tasks,
	}, nil
}

// TaskUpdateTool updates a task.
type TaskUpdateTool struct{ Store *TaskStore }

func (t *TaskUpdateTool) Name() string        { return "TaskUpdate" }
func (t *TaskUpdateTool) Description() string { return "Update a task's status, subject, or description." }
func (t *TaskUpdateTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"taskId":      map[string]interface{}{"type": "string", "description": "The task ID"},
			"status":      map[string]interface{}{"type": "string", "description": "New status: pending, in_progress, completed, or deleted"},
			"subject":     map[string]interface{}{"type": "string", "description": "New subject"},
			"description": map[string]interface{}{"type": "string", "description": "New description"},
			"activeForm":  map[string]interface{}{"type": "string", "description": "New active form"},
			"owner":       map[string]interface{}{"type": "string", "description": "New owner"},
		},
		Required: []string{"taskId"},
	}
}
func (t *TaskUpdateTool) IsConcurrencySafe(input map[string]interface{}) bool { return false }
func (t *TaskUpdateTool) IsReadOnly(input map[string]interface{}) bool        { return false }
func (t *TaskUpdateTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	id, _ := input["taskId"].(string)
	task := t.Store.Update(id, input)
	if task == nil {
		return errorResult(fmt.Sprintf("Task %s not found", id)), nil
	}
	return &types.ToolResult{
		Content: []types.ContentBlock{{Type: types.ContentBlockText, Text: fmt.Sprintf("Task #%s updated (status: %s)", task.ID, task.Status)}},
		Data:    task,
	}, nil
}

func (s *TaskStore) SetOutput(id, output string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return false
	}
	t.Output = output
	t.UpdatedAt = time.Now()
	return true
}

// TaskStopTool stops/cancels a running task.
type TaskStopTool struct{ Store *TaskStore }

func (t *TaskStopTool) Name() string        { return "TaskStop" }
func (t *TaskStopTool) Description() string { return "Stop/cancel a running task." }
func (t *TaskStopTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"taskId": map[string]interface{}{"type": "string", "description": "Task ID to stop"},
			"reason": map[string]interface{}{"type": "string", "description": "Reason for stopping"},
		},
		Required: []string{"taskId"},
	}
}
func (t *TaskStopTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *TaskStopTool) IsReadOnly(input map[string]interface{}) bool        { return false }
func (t *TaskStopTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	id, _ := input["taskId"].(string)
	reason, _ := input["reason"].(string)
	task := t.Store.Get(id)
	if task == nil {
		return errorResult(fmt.Sprintf("Task %s not found", id)), nil
	}
	t.Store.Update(id, map[string]interface{}{"status": string(TaskStatusCancelled)})
	if reason != "" {
		t.Store.SetOutput(id, "Stopped: "+reason)
	}
	return textResult(fmt.Sprintf("Task stopped: %s", id)), nil
}

// TaskOutputTool gets the output/result of a task.
type TaskOutputTool struct{ Store *TaskStore }

func (t *TaskOutputTool) Name() string        { return "TaskOutput" }
func (t *TaskOutputTool) Description() string { return "Get the output/result of a task." }
func (t *TaskOutputTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"taskId": map[string]interface{}{"type": "string", "description": "Task ID"},
		},
		Required: []string{"taskId"},
	}
}
func (t *TaskOutputTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *TaskOutputTool) IsReadOnly(input map[string]interface{}) bool        { return true }
func (t *TaskOutputTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	id, _ := input["taskId"].(string)
	task := t.Store.Get(id)
	if task == nil {
		return errorResult(fmt.Sprintf("Task %s not found", id)), nil
	}
	out := task.Output
	if out == "" {
		out = "(no output yet)"
	}
	return textResult(out), nil
}

// Ensure uuid is used
var _ = uuid.New
