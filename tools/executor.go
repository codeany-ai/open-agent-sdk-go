package tools

import (
	"context"
	"sync"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

// ToolCallRequest represents a pending tool call.
type ToolCallRequest struct {
	ToolUseID string
	ToolName  string
	Input     map[string]interface{}
}

// ToolCallResponse is the result of a tool call execution.
type ToolCallResponse struct {
	ToolUseID string
	Result    *types.ToolResult
	Error     error
}

// Executor runs tool calls with concurrency management.
type Executor struct {
	registry   *Registry
	canUseTool types.CanUseToolFn
	toolCtx    *types.ToolUseContext
}

// NewExecutor creates a new tool executor.
func NewExecutor(registry *Registry, canUseTool types.CanUseToolFn, toolCtx *types.ToolUseContext) *Executor {
	return &Executor{
		registry:   registry,
		canUseTool: canUseTool,
		toolCtx:    toolCtx,
	}
}

// RunTools executes a batch of tool calls, partitioning into concurrent
// and sequential groups based on tool properties.
func (e *Executor) RunTools(ctx context.Context, calls []ToolCallRequest) []ToolCallResponse {
	if len(calls) == 0 {
		return nil
	}

	// Partition into concurrent-safe and sequential groups
	var concurrent []ToolCallRequest
	var sequential []ToolCallRequest

	for _, call := range calls {
		tool := e.registry.Get(call.ToolName)
		if tool == nil {
			sequential = append(sequential, call)
			continue
		}
		if tool.IsConcurrencySafe(call.Input) {
			concurrent = append(concurrent, call)
		} else {
			sequential = append(sequential, call)
		}
	}

	var results []ToolCallResponse

	// Run concurrent tools in parallel
	if len(concurrent) > 0 {
		results = append(results, e.runConcurrent(ctx, concurrent)...)
	}

	// Run sequential tools one at a time
	for _, call := range sequential {
		result := e.runSingle(ctx, call)
		results = append(results, result)
	}

	return results
}

func (e *Executor) runConcurrent(ctx context.Context, calls []ToolCallRequest) []ToolCallResponse {
	results := make([]ToolCallResponse, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c ToolCallRequest) {
			defer wg.Done()
			results[idx] = e.runSingle(ctx, c)
		}(i, call)
	}

	wg.Wait()
	return results
}

func (e *Executor) runSingle(ctx context.Context, call ToolCallRequest) ToolCallResponse {
	tool := e.registry.Get(call.ToolName)
	if tool == nil {
		return ToolCallResponse{
			ToolUseID: call.ToolUseID,
			Result: &types.ToolResult{
				IsError: true,
				Error:   "Unknown tool: " + call.ToolName,
				Content: []types.ContentBlock{{
					Type: types.ContentBlockText,
					Text: "Error: tool '" + call.ToolName + "' not found",
				}},
			},
		}
	}

	// Check permissions
	if e.canUseTool != nil {
		decision, err := e.canUseTool(tool, call.Input)
		if err != nil {
			return ToolCallResponse{
				ToolUseID: call.ToolUseID,
				Result: &types.ToolResult{
					IsError: true,
					Error:   "Permission check failed: " + err.Error(),
				},
			}
		}
		if decision.Behavior == types.PermissionDeny {
			reason := decision.Reason
			if reason == "" {
				reason = "Permission denied"
			}
			return ToolCallResponse{
				ToolUseID: call.ToolUseID,
				Result: &types.ToolResult{
					IsError: true,
					Error:   reason,
					Content: []types.ContentBlock{{
						Type: types.ContentBlockText,
						Text: "Error: " + reason,
					}},
				},
			}
		}
		// Apply updated input if permission handler modified it
		if decision.UpdatedInput != nil {
			call.Input = decision.UpdatedInput
		}
	}

	// Execute tool
	result, err := tool.Call(ctx, call.Input, e.toolCtx)
	if err != nil {
		return ToolCallResponse{
			ToolUseID: call.ToolUseID,
			Result: &types.ToolResult{
				IsError: true,
				Error:   err.Error(),
				Content: []types.ContentBlock{{
					Type: types.ContentBlockText,
					Text: "Error: " + err.Error(),
				}},
			},
		}
	}

	return ToolCallResponse{
		ToolUseID: call.ToolUseID,
		Result:    result,
	}
}
