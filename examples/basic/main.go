package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shipany-ai/open-agent-sdk-go/agent"
	"github.com/shipany-ai/open-agent-sdk-go/types"
)

func main() {
	ctx := context.Background()

	// Create agent
	a := agent.New(agent.Options{
		Model:  "claude-sonnet-4-6",
		APIKey: os.Getenv("ANTHROPIC_API_KEY"),
		CWD:    ".",
	})
	defer a.Close()

	// Initialize (connects MCP servers if configured)
	if err := a.Init(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Init error: %v\n", err)
		os.Exit(1)
	}

	prompt := "What files are in the current directory? Use the Bash tool to run 'ls -la'."
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	fmt.Printf("Prompt: %s\n\n", prompt)

	// Streaming mode
	events, errs := a.Query(ctx, prompt)
	for event := range events {
		switch event.Type {
		case types.MessageTypeAssistant:
			if event.Message != nil {
				text := types.ExtractText(event.Message)
				if text != "" {
					fmt.Print(text)
				}
			}
		case types.MessageTypeResult:
			fmt.Printf("\n\n--- Result ---\n")
			fmt.Printf("Turns: %d\n", event.NumTurns)
			if event.Usage != nil {
				fmt.Printf("Tokens: %d in / %d out\n", event.Usage.InputTokens, event.Usage.OutputTokens)
			}
			fmt.Printf("Cost: $%.4f\n", event.Cost)
		}
	}

	// Check for errors
	select {
	case err := <-errs:
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			os.Exit(1)
		}
	default:
	}
}
