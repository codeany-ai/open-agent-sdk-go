package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/codeany-ai/open-agent-sdk-go/types"
)

// AgentMessage is a message sent between agents.
type AgentMessage struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
}

// Mailbox manages inter-agent message queues.
type Mailbox struct {
	mu    sync.Mutex
	boxes map[string][]AgentMessage
}

// NewMailbox creates a new Mailbox.
func NewMailbox() *Mailbox { return &Mailbox{boxes: make(map[string][]AgentMessage)} }

// Register adds an agent mailbox (idempotent).
func (m *Mailbox) Register(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.boxes[name]; !ok {
		m.boxes[name] = nil
	}
}

// Send delivers a message to the target mailbox.
func (m *Mailbox) Send(msg AgentMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.boxes[msg.To] = append(m.boxes[msg.To], msg)
}

// Read drains and returns all messages for the named agent.
func (m *Mailbox) Read(name string) []AgentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs := m.boxes[name]
	m.boxes[name] = nil
	return msgs
}

// AllNames returns all registered agent names.
func (m *Mailbox) AllNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.boxes))
	for name := range m.boxes {
		names = append(names, name)
	}
	return names
}

// SendMessageTool sends messages to other agents.
type SendMessageTool struct {
	Mailbox *Mailbox
	From    string
}

// NewSendMessageTool creates a new SendMessageTool.
func NewSendMessageTool(mb *Mailbox, from string) *SendMessageTool {
	return &SendMessageTool{Mailbox: mb, From: from}
}

func (t *SendMessageTool) Name() string { return "SendMessage" }
func (t *SendMessageTool) Description() string {
	return "Send a message to another agent or teammate. Use \"*\" for broadcast."
}
func (t *SendMessageTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"to":      map[string]interface{}{"type": "string", "description": "Recipient agent name. Use \"*\" for broadcast."},
			"content": map[string]interface{}{"type": "string", "description": "Message content"},
			"type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"text", "shutdown_request", "shutdown_response", "plan_approval_response"},
				"description": "Message type (default: text)",
			},
		},
		Required: []string{"to", "content"},
	}
}
func (t *SendMessageTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *SendMessageTool) IsReadOnly(input map[string]interface{}) bool        { return false }

func (t *SendMessageTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	to, _ := input["to"].(string)
	content, _ := input["content"].(string)
	msgType, _ := input["type"].(string)
	if msgType == "" {
		msgType = "text"
	}

	msg := AgentMessage{
		From:      t.From,
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
		Type:      msgType,
	}

	if to == "*" {
		for _, name := range t.Mailbox.AllNames() {
			m := msg
			m.To = name
			t.Mailbox.Send(m)
		}
		return textResult("Message broadcast to all agents"), nil
	}

	msg.To = to
	t.Mailbox.Send(msg)
	return textResult(fmt.Sprintf("Message sent to %s", to)), nil
}
