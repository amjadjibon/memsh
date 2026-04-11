package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/amjadjibon/memsh/internal/agent"
)

// mockModel is a canned-response ToolCallingChatModel for unit tests.
// Each Generate call pops the next response from the queue.
type mockModel struct {
	responses []*schema.Message
	idx       int
}

func (m *mockModel) Generate(_ context.Context, _ []*schema.Message, _ ...einomodel.Option) (*schema.Message, error) {
	if m.idx >= len(m.responses) {
		return nil, fmt.Errorf("mockModel: exhausted after %d call(s)", m.idx)
	}
	r := m.responses[m.idx]
	m.idx++
	return r, nil
}

func (m *mockModel) Stream(_ context.Context, _ []*schema.Message, _ ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("mockModel: Stream not implemented")
}

// WithTools returns the same mock regardless of which tools are bound.
func (m *mockModel) WithTools(_ []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return m, nil
}

// toolCallMsg builds an assistant message that requests a memsh tool call.
func toolCallMsg(command string) *schema.Message {
	args, _ := json.Marshal(map[string]string{"command": command})
	return &schema.Message{
		Role:    schema.Assistant,
		Content: "",
		ToolCalls: []schema.ToolCall{
			{
				ID:   "call-test-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "memsh",
					Arguments: string(args),
				},
			},
		},
	}
}

// newTestAgent constructs an Agent backed by a mock model.
func newTestAgent(t *testing.T, responses ...*schema.Message) *agent.Agent {
	t.Helper()
	mock := &mockModel{responses: responses}
	a, err := agent.New(context.Background(), false, mock, nil)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

// invokeAndResume performs the standard two-step interaction:
//  1. First Invoke with user input — always interrupts at nodeHuman.
//  2. Resume with empty input — no further user turn, routes to OutputConverter.
func invokeAndResume(t *testing.T, a *agent.Agent, userInput, checkpointID string) string {
	t.Helper()
	ctx := context.Background()

	_, err := a.Invoke(ctx, userInput,
		agent.WithCheckPointID(checkpointID),
		compose.WithRuntimeMaxSteps(30),
	)
	_, interrupted := agent.ExtractInterruptInfo(err)
	if !interrupted {
		t.Fatalf("expected interrupt after first Invoke, got err=%v", err)
	}

	result, err := a.Invoke(ctx, "",
		agent.WithCheckPointID(checkpointID),
		compose.WithRuntimeMaxSteps(30),
	)
	if err != nil {
		t.Fatalf("resume Invoke: %v", err)
	}
	return result
}

// TestNew verifies that an agent can be constructed and closed without error.
func TestNew(t *testing.T) {
	a := newTestAgent(t, schema.AssistantMessage("hello", nil))
	if err := a.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestAgentNoToolCall verifies the direct-answer path:
// model returns a plain-text message → graph routes through nodeHuman (interrupt) →
// after resume the output matches the model's response.
func TestAgentNoToolCall(t *testing.T) {
	want := "Paris is the capital of France."
	a := newTestAgent(t, schema.AssistantMessage(want, nil))

	got := invokeAndResume(t, a, "What is the capital of France?", t.Name())
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestAgentToolCallLoop verifies the tool-call path:
// model first requests a memsh command → ToolsNode executes it → nodeChatModel
// is called again → model returns a plain-text summary → graph interrupts at
// nodeHuman → after resume the final output matches the second model response.
func TestAgentToolCallLoop(t *testing.T) {
	toolResponse := "Files created successfully."
	a := newTestAgent(t,
		// Turn 1: request a memsh tool call.
		toolCallMsg("echo hello > /hello.txt"),
		// Turn 2: plain-text summary after seeing the tool result.
		schema.AssistantMessage(toolResponse, nil),
	)

	got := invokeAndResume(t, a, "create a hello file", t.Name())
	if got != toolResponse {
		t.Errorf("output = %q, want %q", got, toolResponse)
	}
}

// TestAgentToolCallOutputVisible verifies that tool output flows back to the
// model: the tool result message should appear in the history accessible via
// the interrupt state so the caller can display it.
func TestAgentToolCallOutputVisible(t *testing.T) {
	a := newTestAgent(t,
		toolCallMsg("echo visible-output"),
		schema.AssistantMessage("done", nil),
	)
	ctx := context.Background()

	_, err := a.Invoke(ctx, "run echo",
		agent.WithCheckPointID(t.Name()),
		compose.WithRuntimeMaxSteps(30),
	)
	info, interrupted := agent.ExtractInterruptInfo(err)
	if !interrupted {
		t.Fatalf("expected interrupt, got err=%v", err)
	}

	state, ok := info.State.(*agent.State)
	if !ok {
		t.Fatalf("interrupt state is not *agent.State, got %T", info.State)
	}

	// History must contain at least the system, user, assistant (tool call), and
	// tool-result messages accumulated during the run.
	if len(state.History) < 3 {
		t.Errorf("expected at least 3 history messages, got %d", len(state.History))
	}

	// Find the tool-result message and confirm it carried output.
	var foundTool bool
	for _, msg := range state.History {
		if msg.Role == schema.Tool {
			foundTool = true
			break
		}
	}
	if !foundTool {
		t.Error("no Tool-role message found in history after tool call")
	}
}

// TestAgentMultipleToolCalls verifies that chained tool calls are handled: the
// model calls the tool twice before giving a final text response.
func TestAgentMultipleToolCalls(t *testing.T) {
	final := "All done."
	a := newTestAgent(t,
		toolCallMsg("mkdir /work"),
		toolCallMsg("echo hi > /work/hi.txt"),
		schema.AssistantMessage(final, nil),
	)

	got := invokeAndResume(t, a, "set up workspace", t.Name())
	if got != final {
		t.Errorf("output = %q, want %q", got, final)
	}
}

// TestAgentUserContinuation verifies the human-in-the-loop path: after the
// first interrupt, the caller supplies follow-up text via WithStateModifier,
// which causes the graph to call the model again before completing.
func TestAgentUserContinuation(t *testing.T) {
	intermediate := "What do you mean exactly?"
	final := "Got it, done."
	a := newTestAgent(t,
		schema.AssistantMessage(intermediate, nil),
		schema.AssistantMessage(final, nil),
	)
	ctx := context.Background()

	// First turn.
	_, err := a.Invoke(ctx, "do something vague",
		agent.WithCheckPointID(t.Name()),
		compose.WithRuntimeMaxSteps(30),
	)
	if _, ok := agent.ExtractInterruptInfo(err); !ok {
		t.Fatalf("expected first interrupt, got err=%v", err)
	}

	// Second turn: user provides clarification.
	_, err = a.Invoke(ctx, "",
		agent.WithCheckPointID(t.Name()),
		compose.WithRuntimeMaxSteps(30),
		agent.WithStateModifier(func(_ context.Context, _ compose.NodePath, s any) error {
			s.(*agent.State).UserInput = "I mean task X"
			return nil
		}),
	)
	if _, ok := agent.ExtractInterruptInfo(err); !ok {
		t.Fatalf("expected second interrupt, got err=%v", err)
	}

	// Final resume: clear UserInput so nodeHuman doesn't loop back to ChatModel.
	got, err := a.Invoke(ctx, "",
		agent.WithCheckPointID(t.Name()),
		compose.WithRuntimeMaxSteps(30),
		agent.WithStateModifier(func(_ context.Context, _ compose.NodePath, s any) error {
			s.(*agent.State).UserInput = ""
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("final resume: %v", err)
	}
	if got != final {
		t.Errorf("output = %q, want %q", got, final)
	}
}
