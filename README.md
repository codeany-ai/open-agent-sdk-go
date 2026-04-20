# Open Agent SDK (Go)

A lightweight, open-source Go SDK for building AI agents. Run the full agent loop in-process — no CLI or subprocess required. Deploy anywhere: cloud, serverless, Docker, CI/CD.

Also available in [TypeScript](https://github.com/codeany-ai/open-agent-sdk-typescript).

## Features

- **Agent Loop** — Streaming agentic loop with tool execution, multi-turn conversations, and cost tracking
- **Multi-Provider** — Native support for both Anthropic and OpenAI-compatible APIs (auto-detected)
- **Built-in Tools** — Bash, Read, Write, Edit, Glob, Grep, WebFetch, WebSearch, ExaSearch, Agent (subagents), SendMessage, Tasks, Todo, Config, Cron, PlanMode, Worktree, LSP, NotebookEdit, MCP Resources, and more
- **MCP Support** — Connect to MCP servers via stdio, HTTP, SSE transports, plus in-process SDK server
- **Permission System** — Configurable tool approval with allow/deny rules, runtime mode changes, filesystem path validation, and directory allowlisting
- **Hook System** — 11 hook events: PreToolUse, PostToolUse, PostToolUseFailure, UserPromptSubmit, Stop, SubagentStop, SubagentStart, PreCompact, Notification, PermissionRequest, PostSampling
- **Extended Thinking** — Three modes (adaptive, enabled, disabled) with effort levels (low/medium/high/max)
- **Session Management** — List, get, rename, tag, delete, and fork sessions
- **Rate Limiting** — Parse API rate limit headers, track utilization, detect rejections
- **Context Usage** — Track token distribution across messages, tools, and context window percentage
- **File Checkpointing** — Snapshot and rewind file state to any checkpoint
- **Sandbox** — Command, file, and network access control
- **Plugins** — Local plugin loading from manifest files
- **Cost Tracking** — Per-model token usage, API/tool duration, code change stats
- **Fallback Model** — Automatic retry with a fallback model on API failure
- **Subagent System** — Enhanced agent definitions with skills, memory, effort, maxTurns, background mode, per-agent permissions and MCP servers
- **Custom Tools** — Implement the `Tool` interface to add your own tools

## Quick Start

```bash
go get github.com/codeany-ai/open-agent-sdk-go
```

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/codeany-ai/open-agent-sdk-go/agent"
    "github.com/codeany-ai/open-agent-sdk-go/types"
)

func main() {
    a := agent.New(agent.Options{
        Model:  "sonnet-4-6",
        APIKey: os.Getenv("CODEANY_API_KEY"),
    })
    defer a.Close()

    ctx := context.Background()

    // Streaming
    events, errs := a.Query(ctx, "What files are in this directory?")
    for event := range events {
        if event.Type == types.MessageTypeAssistant && event.Message != nil {
            fmt.Print(types.ExtractText(event.Message))
        }
    }
    if err := <-errs; err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
    }

    // Or use the blocking API
    result, _ := a.Prompt(ctx, "Count lines in go.mod")
    fmt.Println(result.Text)
}
```

## Multi-Provider Support

The SDK supports both **Anthropic** and **OpenAI-compatible** APIs. The provider is auto-detected based on the base URL, API key prefix, or model name:

```go
// Anthropic (default)
a := agent.New(agent.Options{
    Model:  "sonnet-4-6",
    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
})

// OpenAI
a := agent.New(agent.Options{
    Model:   "gpt-4o",
    APIKey:  os.Getenv("OPENAI_API_KEY"),
    BaseURL: "https://api.openai.com",
})

// OpenRouter, DeepSeek, Ollama, etc.
a := agent.New(agent.Options{
    Model:   "anthropic/claude-sonnet-4",
    APIKey:  os.Getenv("OPENROUTER_API_KEY"),
    BaseURL: "https://openrouter.ai/api",
})
```

## Extended Thinking & Effort

```go
// Explicit thinking config
a := agent.New(agent.Options{
    Thinking: &agent.ThinkingConfig{
        Type:         agent.ThinkingEnabled,
        BudgetTokens: 10000,
    },
})

// Or use effort levels (auto-configures thinking)
a := agent.New(agent.Options{
    Effort: agent.EffortHigh, // low, medium, high, max
})
```

## Fallback Model

```go
a := agent.New(agent.Options{
    Model:         "opus-4-6",
    FallbackModel: "sonnet-4-6", // Auto-retry on failure
})
```

## Subagents

```go
a := agent.New(agent.Options{
    Agents: map[string]agent.AgentDefinition{
        "researcher": {
            Description:  "Research agent for deep analysis",
            Instructions: "You are a research specialist...",
            Model:        "opus-4-6",
            Tools:        []string{"Read", "Glob", "Grep", "WebSearch"},
            MaxTurns:     20,
            Effort:       agent.EffortHigh,
        },
        "coder": {
            Description:    "Coding agent for implementation",
            Instructions:   "You are a coding specialist...",
            DisallowedTools: []string{"WebSearch", "WebFetch"},
            PermissionMode: types.PermissionModeAcceptEdits,
        },
    },
})
```

## Session Management

```go
import "github.com/codeany-ai/open-agent-sdk-go/session"

mgr := session.NewManager("")  // default ~/.claude/projects/

sessions, _ := mgr.ListSessions("my-project")
messages, _ := mgr.GetSessionMessages(sessions[0].SessionID)

mgr.RenameSession(sessions[0].SessionID, "New Title")
mgr.TagSession(sessions[0].SessionID, strPtr("important"))

fork, _ := mgr.ForkSession(sessions[0].SessionID, "msg-uuid", "Forked Session")
```

## Hooks

```go
a := agent.New(agent.Options{
    Hooks: hooks.HookConfig{
        PreToolUse: []hooks.HookRule{{
            Matcher: "Bash",
            Hooks: []hooks.HookFn{
                func(ctx context.Context, tool string, input map[string]interface{}) (string, error) {
                    cmd, _ := input["command"].(string)
                    if strings.Contains(cmd, "rm -rf") {
                        return "Blocked: dangerous command", nil
                    }
                    return "", nil
                },
            },
        }},
        // Also: PostToolUse, PostToolUseFailure, UserPromptSubmit,
        // Stop, SubagentStop, SubagentStart, PreCompact,
        // Notification, PermissionRequest, PostSampling
    },
})
```

## Permissions

```go
import "github.com/codeany-ai/open-agent-sdk-go/permissions"

config := &permissions.Config{
    Mode: types.PermissionModeDefault,
    AllowRules: []permissions.Rule{
        {ToolName: "Read"},
        {ToolName: "Glob"},
        {ToolName: "Bash", Pattern: "git *"},
    },
    DenyRules: []permissions.Rule{
        {ToolName: "Bash", Pattern: "rm *"},
    },
    AllowedDirs: []string{"/home/user/projects"},
}

// Runtime updates (thread-safe)
config.SetMode(types.PermissionModeAcceptEdits)
config.AddRules([]permissions.Rule{{ToolName: "Write"}}, "allow")
config.AddDirectories([]string{"/tmp/workspace"})
```

## MCP Servers

```go
a := agent.New(agent.Options{
    MCPServers: map[string]types.MCPServerConfig{
        "filesystem": {
            Type:    types.MCPTransportStdio,
            Command: "npx",
            Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
        },
        "api": {
            Type: types.MCPTransportHTTP,
            URL:  "http://localhost:3000/mcp",
        },
    },
})
a.Init(ctx) // Connects to MCP servers
```

### In-Process MCP SDK Server

```go
import "github.com/codeany-ai/open-agent-sdk-go/mcp"

server := mcp.NewSdkServer("my-tools", "1.0.0")
server.RegisterTool(&mcp.SdkMcpTool{
    Name:        "get_weather",
    Description: "Get weather for a city",
    InputSchema: types.ToolInputSchema{
        Type:       "object",
        Properties: map[string]interface{}{
            "city": map[string]interface{}{"type": "string"},
        },
        Required: []string{"city"},
    },
    Handler: func(ctx context.Context, input map[string]interface{}) (*types.ToolResult, error) {
        city := input["city"].(string)
        return &types.ToolResult{
            Content: []types.ContentBlock{{Type: types.ContentBlockText, Text: "Sunny in " + city}},
        }, nil
    },
})
```

## File Checkpointing

```go
import "github.com/codeany-ai/open-agent-sdk-go/checkpoint"

mgr := checkpoint.NewManager(true)
mgr.TrackFile("/path/to/important/file.go")

mgr.CreateCheckpoint("msg-001")  // Snapshot current state
// ... file gets modified ...
mgr.RewindTo("msg-001")          // Restore to snapshot
```

## Rate Limiting

```go
import "github.com/codeany-ai/open-agent-sdk-go/ratelimit"

tracker := ratelimit.NewTracker(func(event ratelimit.RateLimitEvent) {
    if event.Info.Status == ratelimit.RateLimitRejected {
        log.Println("Rate limited! Resets at:", event.Info.ResetsAt)
    }
})

// Called automatically with API response headers
tracker.ParseHeaders(resp.Header)
```

## Sandbox

```go
import "github.com/codeany-ai/open-agent-sdk-go/sandbox"

validator := sandbox.NewValidator(sandbox.Settings{
    Enabled:          true,
    ExcludedCommands: []string{"rm", "kill", "shutdown"},
    Network: &sandbox.NetworkConfig{
        AllowLocalBinding: true,
    },
    IgnoreViolations: &sandbox.IgnoreViolations{
        NetworkHosts: []string{"localhost"},
    },
})

validator.IsCommandAllowed("git status")  // true
validator.IsCommandAllowed("rm -rf /")    // false
```

## Custom Tools

Implement the `types.Tool` interface:

```go
type MyTool struct{}

func (t *MyTool) Name() string                                    { return "MyTool" }
func (t *MyTool) Description() string                             { return "Does something useful" }
func (t *MyTool) InputSchema() types.ToolInputSchema              { return types.ToolInputSchema{...} }
func (t *MyTool) IsConcurrencySafe(map[string]interface{}) bool   { return true }
func (t *MyTool) IsReadOnly(map[string]interface{}) bool          { return true }
func (t *MyTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
    return &types.ToolResult{
        Content: []types.ContentBlock{{Type: types.ContentBlockText, Text: "result"}},
    }, nil
}

a := agent.New(agent.Options{
    CustomTools: []types.Tool{&MyTool{}},
})
```

## Examples

| #   | Example                                                   | Description                                      |
| --- | --------------------------------------------------------- | ------------------------------------------------ |
| 01  | [Simple Query](examples/01-simple-query/)                 | Streaming query with tool calls                  |
| 02  | [Multi-Tool](examples/02-multi-tool/)                     | Glob + Bash multi-tool orchestration             |
| 03  | [Multi-Turn](examples/03-multi-turn/)                     | Multi-turn conversation with session persistence |
| 04  | [Prompt API](examples/04-prompt-api/)                     | Blocking `Prompt()` for one-shot queries         |
| 05  | [Custom System Prompt](examples/05-custom-system-prompt/) | Custom system prompt for code review             |
| 06  | [MCP Server](examples/06-mcp-server/)                     | MCP server integration (stdio transport)         |
| 07  | [Custom Tools](examples/07-custom-tools/)                 | Define and use custom tools                      |
| 08  | [One-shot Query](examples/08-official-api-compat/)        | Quick one-shot agent query                       |
| 09  | [Subagents](examples/09-subagents/)                       | Specialized subagent with restricted tools       |
| 10  | [Permissions](examples/10-permissions/)                   | Read-only agent with AllowedTools                |
| 11  | [Web Chat](examples/web/)                                 | Web-based chat UI with streaming                 |

Run any example:

```bash
export CODEANY_BASE_URL=https://openrouter.ai/api
export CODEANY_API_KEY=your-api-key
export CODEANY_MODEL=anthropic/claude-sonnet-4
go run ./examples/01-simple-query/
```

## Architecture

```
open-agent-sdk-go/
├── agent/              # Agent loop, query engine, effort, fallback model
├── api/                # API client (Anthropic + OpenAI dual protocol)
├── types/              # Core types: Message, Tool, ContentBlock, MCP
├── tools/              # 32 built-in tools + registry + executor
│   └── diff/           # Unified diff generation
├── mcp/                # MCP client + SDK server + resources + reconnection
├── permissions/        # Permission rules, runtime management, filesystem validation
├── hooks/              # 11 hook events with extended hook support
├── costtracker/        # Token usage and cost tracking
├── context/            # System/user context injection (git status, CODEANY.md)
├── history/            # Conversation history persistence (JSONL)
├── session/            # Session management (list, get, rename, tag, delete, fork)
├── ratelimit/          # Rate limit header parsing and tracking
├── contextusage/       # Context window usage tracking
├── checkpoint/         # File state checkpointing and rewind
├── sandbox/            # Sandbox access control (commands, files, network)
├── plugins/            # Local plugin loading and management
└── examples/           # 11 runnable examples
```

## Configuration

Environment variables:

| Variable                     | Description                                  |
| ---------------------------- | -------------------------------------------- |
| `CODEANY_API_KEY`            | API key (required)                           |
| `CODEANY_MODEL`              | Default model (default: `sonnet-4-6`)        |
| `CODEANY_BASE_URL`           | API base URL override                        |
| `CODEANY_CUSTOM_HEADERS`     | Custom headers (comma-separated `key:value`) |
| `API_TIMEOUT_MS`             | API request timeout in ms                    |
| `HTTPS_PROXY` / `HTTP_PROXY` | Proxy URL                                    |
| `EXA_API_KEY`                | API key for the `ExaSearch` tool (optional)  |

Also supports `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_MODEL` for compatibility.

## Links

- Website: [codeany.ai](https://codeany.ai)
- TypeScript SDK: [github.com/codeany-ai/open-agent-sdk-typescript](https://github.com/codeany-ai/open-agent-sdk-typescript)
- Issues: [github.com/codeany-ai/open-agent-sdk-go/issues](https://github.com/codeany-ai/open-agent-sdk-go/issues)

## License

MIT — see [LICENSE](LICENSE)
