package tools

import (
	"context"
	"testing"
)

func TestSendMessageTool(t *testing.T) {
	mb := NewMailbox()
	mb.Register("agent-b")
	mb.Register("agent-c")
	tool := NewSendMessageTool(mb, "agent-a")
	ctx := context.Background()
	tCtx := testToolCtx(t)

	// send to specific agent
	r, err := tool.Call(ctx, map[string]interface{}{
		"to": "agent-b", "content": "hello",
	}, tCtx)
	if err != nil || r.IsError {
		t.Fatalf("send failed: %v", err)
	}

	msgs := mb.Read("agent-b")
	if len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Errorf("expected 1 message with 'hello', got %v", msgs)
	}

	// broadcast
	tool.Call(ctx, map[string]interface{}{"to": "*", "content": "broadcast"}, tCtx)
	bMsgs := mb.Read("agent-b")
	cMsgs := mb.Read("agent-c")
	if len(bMsgs) != 1 || len(cMsgs) != 1 {
		t.Errorf("broadcast failed: b=%d c=%d", len(bMsgs), len(cMsgs))
	}
}
