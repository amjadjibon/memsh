package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudwego/eino/compose"
	"github.com/spf13/cobra"

	"github.com/amjadjibon/memsh/internal/agent"
	"github.com/amjadjibon/memsh/internal/agent/tui"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run an AI agent with memsh as a tool",
	Long: `Start an interactive AI agent powered by Eino (cloudwego/eino) that uses memsh as a sandboxed bash tool.

The agent follows a manus-style ReAct loop with human-in-the-loop confirmation:
  1. You send a task
  2. The agent thinks and calls memsh tools as needed
  3. After each response, the agent pauses for your review
  4. You can continue, redirect, or end the conversation

Provider is detected from the model name:
  gpt-*      → OpenAI        (OPENAI_API_KEY)
  claude-*   → Anthropic     (ANTHROPIC_API_KEY)
  gemini-*   → Gemini        (GOOGLE_API_KEY)
  grok-*     → Grok / xAI   (XAI_API_KEY)
  --base-url → any OpenAI-compatible endpoint

Examples:
  memsh agent --model gpt-4o
  memsh agent --model claude-opus-4-5
  memsh agent --model gemini-2.0-flash
  memsh agent --model grok-3
  memsh agent --model gpt-4o --api-key sk-xxx --base-url https://api.openai.com/v1
  memsh agent --model gpt-4o --wasm
  memsh agent --query "list files in / and create a hello world script"`,
	RunE: runAgentCmd,
}

func runAgentCmd(cmd *cobra.Command, _ []string) error {
	modelName, _ := cmd.Flags().GetString("model")
	apiKey, _ := cmd.Flags().GetString("api-key")
	baseURL, _ := cmd.Flags().GetString("base-url")
	wasmEnabled, _ := cmd.Flags().GetBool("wasm")
	singleQuery, _ := cmd.Flags().GetString("query")

	if apiKey == "" {
		for _, env := range []string{
			"OPENAI_API_KEY",
			"ANTHROPIC_API_KEY",
			"GOOGLE_API_KEY",
			"XAI_API_KEY",
		} {
			if v := os.Getenv(env); v != "" {
				apiKey = v
				break
			}
		}
	}
	if apiKey == "" {
		return fmt.Errorf("agent: --api-key or one of OPENAI_API_KEY / ANTHROPIC_API_KEY / GOOGLE_API_KEY / XAI_API_KEY is required")
	}
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	chatModel, err := agent.NewChatModel(ctx, modelName, apiKey, baseURL)
	if err != nil {
		return fmt.Errorf("agent: init model: %w", err)
	}

	ag, err := agent.New(ctx, wasmEnabled, chatModel, nil)
	if err != nil {
		return err
	}
	defer ag.Close()

	if singleQuery != "" {
		return runAgentQuery(ctx, ag, singleQuery)
	}
	return runAgentTUI(ctx, stop, ag, modelName)
}

func runAgentQuery(ctx context.Context, ag *agent.Agent, query string) error {
	result, err := ag.Invoke(ctx, query,
		agent.WithCheckPointID("single"),
		compose.WithRuntimeMaxSteps(20),
	)
	if err != nil {
		return fmt.Errorf("agent: %w", err)
	}
	fmt.Println(result)
	return nil
}

func runAgentTUI(ctx context.Context, cancel context.CancelFunc, ag *agent.Agent, modelName string) error {
	if err := tui.Run(ctx, cancel, ag, modelName); err != nil {
		return fmt.Errorf("agent tui: %w", err)
	}
	return nil
}

func init() {
	agentCmd.Flags().String("model", "gpt-4o", "Model name (provider inferred from prefix)")
	agentCmd.Flags().String("api-key", "", "API key (or set OPENAI_API_KEY / ANTHROPIC_API_KEY / GOOGLE_API_KEY / XAI_API_KEY)")
	agentCmd.Flags().String("base-url", "", "OpenAI-compatible base URL override (or set OPENAI_BASE_URL)")
	agentCmd.Flags().Bool("wasm", false, "Enable WASM plugin loading")
	agentCmd.Flags().String("query", "", "Run a single query and exit (no TUI)")
	rootCmd.AddCommand(agentCmd)
}
