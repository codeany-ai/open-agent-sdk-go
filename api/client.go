package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

const (
	defaultBaseURL   = "https://api.anthropic.com"
	defaultModel     = "claude-sonnet-4-6"
	apiVersion       = "2023-06-01"
	defaultMaxTokens = 16384
)

// ClientConfig configures the Anthropic API client.
type ClientConfig struct {
	APIKey     string
	BaseURL    string
	Model      string
	MaxTokens  int
	HTTPClient *http.Client
}

// Client communicates with the Anthropic Messages API.
type Client struct {
	config ClientConfig
}

// NewClient creates a new Anthropic API client.
func NewClient(config ClientConfig) *Client {
	if config.APIKey == "" {
		config.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if config.BaseURL == "" {
		config.BaseURL = os.Getenv("ANTHROPIC_BASE_URL")
		if config.BaseURL == "" {
			config.BaseURL = defaultBaseURL
		}
	}
	if config.Model == "" {
		config.Model = os.Getenv("ANTHROPIC_MODEL")
		if config.Model == "" {
			config.Model = defaultModel
		}
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = defaultMaxTokens
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 10 * time.Minute}
	}
	return &Client{config: config}
}

// APIMessage is a message sent to the API.
type APIMessage struct {
	Role    string              `json:"role"`
	Content []types.ContentBlock `json:"content"`
}

// APIToolParam is a tool definition for the API.
type APIToolParam struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// MessagesRequest is the request body for the Messages API.
type MessagesRequest struct {
	Model     string                   `json:"model"`
	MaxTokens int                      `json:"max_tokens"`
	System    []SystemBlock            `json:"system,omitempty"`
	Messages  []APIMessage             `json:"messages"`
	Tools     []APIToolParam           `json:"tools,omitempty"`
	Stream    bool                     `json:"stream"`
	Metadata  map[string]interface{}   `json:"metadata,omitempty"`
	StopSequences []string             `json:"stop_sequences,omitempty"`
	Temperature   *float64             `json:"temperature,omitempty"`

	// Extended thinking
	Thinking *ThinkingConfig `json:"thinking,omitempty"`
}

// SystemBlock is a system prompt block.
type SystemBlock struct {
	Type         string `json:"type"`
	Text         string `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// CacheControl configures prompt caching.
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// ThinkingConfig configures extended thinking.
type ThinkingConfig struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"` // Max thinking tokens
}

// StreamEvent represents a server-sent event from the streaming API.
type StreamEvent struct {
	Type string `json:"type"`

	// message_start
	Message *StreamMessage `json:"message,omitempty"`

	// content_block_start, content_block_delta
	Index        int                    `json:"index,omitempty"`
	ContentBlock *types.ContentBlock    `json:"content_block,omitempty"`
	Delta        map[string]interface{} `json:"delta,omitempty"`

	// message_delta
	Usage *types.Usage `json:"usage,omitempty"`
}

// StreamMessage is the message object in a message_start event.
type StreamMessage struct {
	ID         string              `json:"id"`
	Type       string              `json:"type"`
	Role       string              `json:"role"`
	Content    []types.ContentBlock `json:"content"`
	Model      string              `json:"model"`
	StopReason string              `json:"stop_reason"`
	Usage      *types.Usage        `json:"usage"`
}

// StreamCallback is called for each streaming event.
type StreamCallback func(event StreamEvent) error

// CreateMessageStream sends a streaming messages request and calls the callback for each event.
func (c *Client) CreateMessageStream(ctx context.Context, req MessagesRequest) (<-chan StreamEvent, <-chan error) {
	eventCh := make(chan StreamEvent, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		req.Stream = true
		if req.Model == "" {
			req.Model = c.config.Model
		}
		if req.MaxTokens == 0 {
			req.MaxTokens = c.config.MaxTokens
		}

		body, err := json.Marshal(req)
		if err != nil {
			errCh <- fmt.Errorf("marshal request: %w", err)
			return
		}

		url := strings.TrimRight(c.config.BaseURL, "/") + "/v1/messages"
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			errCh <- fmt.Errorf("create request: %w", err)
			return
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-API-Key", c.config.APIKey)
		httpReq.Header.Set("Anthropic-Version", apiVersion)
		httpReq.Header.Set("Anthropic-Beta", "prompt-caching-2024-07-31")
		httpReq.Header.Set("Accept", "text/event-stream")

		resp, err := c.config.HTTPClient.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("send request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer

		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var event StreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue // Skip malformed events
			}

			select {
			case eventCh <- event:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("read stream: %w", err)
		}
	}()

	return eventCh, errCh
}

// CreateMessage sends a non-streaming messages request.
func (c *Client) CreateMessage(ctx context.Context, req MessagesRequest) (*StreamMessage, error) {
	req.Stream = false
	if req.Model == "" {
		req.Model = c.config.Model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = c.config.MaxTokens
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.config.APIKey)
	httpReq.Header.Set("Anthropic-Version", apiVersion)

	resp, err := c.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var msg StreamMessage
	if err := json.Unmarshal(bodyBytes, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &msg, nil
}

// ToolToAPIParam converts a Tool to an API tool parameter.
func ToolToAPIParam(t types.Tool) APIToolParam {
	schema := t.InputSchema()
	schemaMap := map[string]interface{}{
		"type": schema.Type,
	}
	if schema.Properties != nil {
		schemaMap["properties"] = schema.Properties
	}
	if schema.Required != nil {
		schemaMap["required"] = schema.Required
	}

	return APIToolParam{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: schemaMap,
	}
}
