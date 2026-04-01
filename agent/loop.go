package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/shipany-ai/open-agent-sdk-go/api"
	agentcontext "github.com/shipany-ai/open-agent-sdk-go/context"
	"github.com/shipany-ai/open-agent-sdk-go/tools"
	"github.com/shipany-ai/open-agent-sdk-go/types"
)

const defaultSystemPrompt = `You are an AI assistant with access to tools. Use the tools available to you to help the user with their request. Be concise and direct in your responses.`

// runLoop is the main agentic loop.
func (a *Agent) runLoop(ctx context.Context, prompt string, eventCh chan<- types.SDKMessage) error {
	startTime := time.Now()

	// Build system prompt
	systemPrompt := a.opts.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}
	if a.opts.AppendSystemPrompt != "" {
		systemPrompt += "\n\n" + a.opts.AppendSystemPrompt
	}

	// Get context
	sysCtx := agentcontext.GetSystemContext(a.opts.CWD)
	userCtx := agentcontext.GetUserContext(a.opts.CWD)
	systemBlocks := agentcontext.BuildSystemPromptBlocks(systemPrompt, sysCtx, userCtx)

	// Convert system blocks to API format
	apiSystemBlocks := make([]api.SystemBlock, len(systemBlocks))
	for i, b := range systemBlocks {
		block := api.SystemBlock{
			Type: "text",
			Text: b["text"].(string),
		}
		if cc, ok := b["cache_control"]; ok {
			if ccMap, ok := cc.(map[string]string); ok {
				block.CacheControl = &api.CacheControl{Type: ccMap["type"]}
			}
		}
		apiSystemBlocks[i] = block
	}

	// Add user message
	userMsg := types.Message{
		Type: types.MessageTypeUser,
		Role: "user",
		Content: []types.ContentBlock{{
			Type: types.ContentBlockText,
			Text: prompt,
		}},
		UUID:      uuid.New().String(),
		Timestamp: time.Now(),
	}
	a.messages = append(a.messages, userMsg)

	// Build tool params
	allTools := a.toolRegistry.All()
	apiTools := make([]api.APIToolParam, len(allTools))
	for i, t := range allTools {
		apiTools[i] = api.ToolToAPIParam(t)
	}

	// Create tool context
	toolCtx := &types.ToolUseContext{
		WorkingDir:    a.opts.CWD,
		AbortCtx:      ctx,
		ReadFileState: make(map[string]*types.FileReadState),
	}

	// Create tool executor
	executor := tools.NewExecutor(a.toolRegistry, a.canUseTool, toolCtx)

	var totalUsage types.Usage
	turn := 0

	// Main loop
	for turn < a.opts.MaxTurns {
		turn++

		// Check budget
		if a.opts.MaxBudgetUSD > 0 && a.costTracker.TotalCost() >= a.opts.MaxBudgetUSD {
			eventCh <- types.SDKMessage{
				Type: types.MessageTypeSystem,
				Text: fmt.Sprintf("Budget limit reached ($%.2f)", a.opts.MaxBudgetUSD),
			}
			break
		}

		// Build API messages from conversation history
		apiMessages := a.buildAPIMessages()

		// Call the API
		req := api.MessagesRequest{
			System:   apiSystemBlocks,
			Messages: apiMessages,
			Tools:    apiTools,
		}

		streamEvents, streamErr := a.apiClient.CreateMessageStream(ctx, req)

		// Accumulate the assistant response
		assistantMsg := &types.Message{
			Type:      types.MessageTypeAssistant,
			Role:      "assistant",
			UUID:      uuid.New().String(),
			Timestamp: time.Now(),
		}

		var toolUseBlocks []types.ToolUseBlock

		// Process stream
	streamLoop:
		for {
			select {
			case event, ok := <-streamEvents:
				if !ok {
					break streamLoop
				}
				a.processStreamEvent(event, assistantMsg, &toolUseBlocks)

			case err := <-streamErr:
				if err != nil {
					return fmt.Errorf("API stream error: %w", err)
				}
				break streamLoop

			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Update usage
		if assistantMsg.Usage != nil {
			totalUsage.InputTokens += assistantMsg.Usage.InputTokens
			totalUsage.OutputTokens += assistantMsg.Usage.OutputTokens
			totalUsage.CacheReadInputTokens += assistantMsg.Usage.CacheReadInputTokens
			totalUsage.CacheCreationInputTokens += assistantMsg.Usage.CacheCreationInputTokens
			a.costTracker.AddUsage(a.opts.Model, assistantMsg.Usage)
		}

		// Store assistant message
		a.messages = append(a.messages, *assistantMsg)

		// Emit assistant event
		eventCh <- types.SDKMessage{
			Type:    types.MessageTypeAssistant,
			Message: assistantMsg,
		}

		// Check if we need to run tools
		if len(toolUseBlocks) == 0 {
			// No tool calls — end of turn
			break
		}

		// Check stop reason
		if assistantMsg.StopReason == "end_turn" && len(toolUseBlocks) == 0 {
			break
		}

		// Execute tools
		toolCalls := make([]tools.ToolCallRequest, len(toolUseBlocks))
		for i, tb := range toolUseBlocks {
			toolCalls[i] = tools.ToolCallRequest{
				ToolUseID: tb.ID,
				ToolName:  tb.Name,
				Input:     tb.Input,
			}
		}

		results := executor.RunTools(ctx, toolCalls)

		// Build tool result message
		var toolResultContent []types.ContentBlock
		for _, result := range results {
			content := result.Result.Content
			if len(content) == 0 {
				text := "(no output)"
				if result.Result.Error != "" {
					text = result.Result.Error
				}
				content = []types.ContentBlock{{
					Type: types.ContentBlockText,
					Text: text,
				}}
			}

			toolResultContent = append(toolResultContent, types.ContentBlock{
				Type:      types.ContentBlockToolResult,
				ToolUseID: result.ToolUseID,
				Content:   content,
				IsError:   result.Result.IsError,
			})
		}

		toolResultMsg := types.Message{
			Type:      types.MessageTypeUser,
			Role:      "user",
			Content:   toolResultContent,
			UUID:      uuid.New().String(),
			Timestamp: time.Now(),
		}
		a.messages = append(a.messages, toolResultMsg)
	}

	// Emit result
	eventCh <- types.SDKMessage{
		Type:     types.MessageTypeResult,
		Text:     types.ExtractText(&a.messages[len(a.messages)-1]),
		Usage:    &totalUsage,
		NumTurns: turn,
		Duration: time.Since(startTime).Milliseconds(),
		Cost:     a.costTracker.TotalCost(),
	}

	return nil
}

// processStreamEvent accumulates streaming data into the assistant message.
func (a *Agent) processStreamEvent(event api.StreamEvent, msg *types.Message, toolUseBlocks *[]types.ToolUseBlock) {
	switch event.Type {
	case "message_start":
		if event.Message != nil {
			msg.Model = event.Message.Model
			if event.Message.Usage != nil {
				msg.Usage = event.Message.Usage
			}
		}

	case "content_block_start":
		if event.ContentBlock != nil {
			msg.Content = append(msg.Content, *event.ContentBlock)

			// Track tool use blocks
			if event.ContentBlock.Type == types.ContentBlockToolUse {
				*toolUseBlocks = append(*toolUseBlocks, types.ToolUseBlock{
					ID:    event.ContentBlock.ID,
					Name:  event.ContentBlock.Name,
					Input: event.ContentBlock.Input,
				})
			}
		}

	case "content_block_delta":
		if event.Delta == nil || len(msg.Content) == 0 {
			return
		}
		idx := event.Index
		if idx >= len(msg.Content) {
			return
		}

		delta := event.Delta
		switch delta["type"] {
		case "text_delta":
			if text, ok := delta["text"].(string); ok {
				msg.Content[idx].Text += text
			}
		case "input_json_delta":
			if partialJSON, ok := delta["partial_json"].(string); ok {
				// Accumulate JSON for tool input
				// We'll parse the full input when the block stops
				msg.Content[idx].Text += partialJSON
			}
		case "thinking_delta":
			if thinking, ok := delta["thinking"].(string); ok {
				msg.Content[idx].Thinking += thinking
			}
		}

	case "content_block_stop":
		idx := event.Index
		if idx >= len(msg.Content) {
			return
		}
		block := &msg.Content[idx]

		// For tool_use blocks, parse accumulated JSON input
		if block.Type == types.ContentBlockToolUse && block.Text != "" {
			var input map[string]interface{}
			if err := parseJSON(block.Text, &input); err == nil {
				block.Input = input
				block.Text = ""

				// Update the tool use block's input
				for i, tb := range *toolUseBlocks {
					if tb.ID == block.ID {
						(*toolUseBlocks)[i].Input = input
						break
					}
				}
			}
		}

	case "message_delta":
		if event.Delta != nil {
			if sr, ok := event.Delta["stop_reason"].(string); ok {
				msg.StopReason = sr
			}
		}
		if event.Usage != nil {
			if msg.Usage == nil {
				msg.Usage = event.Usage
			} else {
				msg.Usage.OutputTokens += event.Usage.OutputTokens
			}
		}
	}
}

// buildAPIMessages converts internal messages to API format.
func (a *Agent) buildAPIMessages() []api.APIMessage {
	var apiMsgs []api.APIMessage

	for _, msg := range a.messages {
		apiMsg := api.APIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
		apiMsgs = append(apiMsgs, apiMsg)
	}

	return apiMsgs
}

// parseJSON safely parses JSON, handling the streaming accumulation pattern.
func parseJSON(data string, v interface{}) error {
	// The streamed JSON might have been accumulated from partial chunks
	return jsonUnmarshal([]byte(data), v)
}

// jsonUnmarshal is a wrapper for json.Unmarshal to handle edge cases.
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
