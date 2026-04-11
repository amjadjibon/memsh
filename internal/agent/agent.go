package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/amjadjibon/memsh/internal/agent/tools"
)

type State struct {
	History   []*schema.Message
	UserInput string
}

type Agent struct {
	runner compose.Runnable[string, string]
	tool   *tools.MemshTool
}

const (
	nodeInputConvert  = "InputConverter"
	nodeChatModel     = "ChatModel"
	nodeToolsNode     = "ToolsNode"
	nodeHuman         = "Human"
	nodeOutputConvert = "OutputConverter"
)

func New(ctx context.Context, wasmEnabled bool, chatModel model.ToolCallingChatModel, agentTools []tool.BaseTool) (*Agent, error) {
	memshTool, err := tools.NewMemshTool(wasmEnabled)
	if err != nil {
		return nil, err
	}

	allTools := append(agentTools, memshTool)

	boundModel, err := bindTools(ctx, chatModel, allTools)
	if err != nil {
		memshTool.Close()
		return nil, fmt.Errorf("agent: bind tools: %w", err)
	}

	runner, err := composeGraph(ctx, boundModel, allTools)
	if err != nil {
		memshTool.Close()
		return nil, err
	}

	return &Agent{
		runner: runner,
		tool:   memshTool,
	}, nil
}

func bindTools(ctx context.Context, cm model.ToolCallingChatModel, agentTools []tool.BaseTool) (model.ToolCallingChatModel, error) {
	infos := make([]*schema.ToolInfo, 0, len(agentTools))
	for _, t := range agentTools {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("get tool info: %w", err)
		}
		infos = append(infos, info)
	}
	bound, err := cm.WithTools(infos)
	if err != nil {
		return nil, fmt.Errorf("bind tools to model: %w", err)
	}
	return bound, nil
}

func composeGraph(ctx context.Context, cm model.ToolCallingChatModel, agentTools []tool.BaseTool) (compose.Runnable[string, string], error) {
	g := compose.NewGraph[string, string](compose.WithGenLocalState(func(ctx context.Context) *State {
		return &State{History: []*schema.Message{}}
	}))

	err := g.AddLambdaNode(nodeInputConvert, compose.InvokableLambda(
		func(ctx context.Context, input string) ([]*schema.Message, error) {
			return []*schema.Message{
				schema.SystemMessage(systemPrompt),
				schema.UserMessage(input),
			}, nil
		},
	), compose.WithNodeName(nodeInputConvert))
	if err != nil {
		return nil, fmt.Errorf("add node %s: %w", nodeInputConvert, err)
	}

	err = g.AddChatModelNode(nodeChatModel, cm,
		compose.WithNodeName(nodeChatModel),
		compose.WithStatePreHandler(func(ctx context.Context, in []*schema.Message, s *State) ([]*schema.Message, error) {
			s.History = append(s.History, in...)
			return s.History, nil
		}),
		compose.WithStatePostHandler(func(ctx context.Context, out *schema.Message, s *State) (*schema.Message, error) {
			s.History = append(s.History, out)
			return out, nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("add node %s: %w", nodeChatModel, err)
	}

	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{Tools: agentTools})
	if err != nil {
		return nil, fmt.Errorf("create tools node: %w", err)
	}
	err = g.AddToolsNode(nodeToolsNode, toolsNode,
		compose.WithNodeName(nodeToolsNode),
		compose.WithStatePostHandler(func(ctx context.Context, out []*schema.Message, s *State) ([]*schema.Message, error) {
			return append(out, schema.UserMessage(nextStepPrompt)), nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("add node %s: %w", nodeToolsNode, err)
	}

	err = g.AddLambdaNode(nodeHuman, compose.InvokableLambda(
		func(ctx context.Context, input *schema.Message) ([]*schema.Message, error) {
			return []*schema.Message{input}, nil
		},
	), compose.WithNodeName(nodeHuman),
		compose.WithStatePostHandler(func(ctx context.Context, in []*schema.Message, s *State) ([]*schema.Message, error) {
			if len(s.UserInput) > 0 {
				return []*schema.Message{schema.UserMessage(s.UserInput)}, nil
			}
			return in, nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("add node %s: %w", nodeHuman, err)
	}

	err = g.AddLambdaNode(nodeOutputConvert, compose.InvokableLambda(
		func(ctx context.Context, input []*schema.Message) (string, error) {
			return input[len(input)-1].Content, nil
		},
	))
	if err != nil {
		return nil, fmt.Errorf("add node %s: %w", nodeOutputConvert, err)
	}

	err = g.AddEdge(compose.START, nodeInputConvert)
	if err != nil {
		return nil, err
	}
	err = g.AddEdge(nodeInputConvert, nodeChatModel)
	if err != nil {
		return nil, err
	}
	err = g.AddBranch(nodeChatModel, compose.NewGraphBranch(
		func(ctx context.Context, in *schema.Message) (string, error) {
			if len(in.ToolCalls) > 0 {
				return nodeToolsNode, nil
			}
			return nodeHuman, nil
		},
		map[string]bool{nodeToolsNode: true, nodeHuman: true},
	))
	if err != nil {
		return nil, err
	}
	err = g.AddBranch(nodeHuman, compose.NewGraphBranch(
		func(ctx context.Context, in []*schema.Message) (string, error) {
			if in[len(in)-1].Role == schema.User {
				return nodeChatModel, nil
			}
			return nodeOutputConvert, nil
		},
		map[string]bool{nodeChatModel: true, nodeOutputConvert: true},
	))
	if err != nil {
		return nil, err
	}
	err = g.AddEdge(nodeToolsNode, nodeChatModel)
	if err != nil {
		return nil, err
	}
	err = g.AddEdge(nodeOutputConvert, compose.END)
	if err != nil {
		return nil, err
	}

	runner, err := g.Compile(ctx,
		compose.WithCheckPointStore(newInMemoryStore()),
		compose.WithInterruptBeforeNodes([]string{nodeHuman}),
	)
	if err != nil {
		return nil, fmt.Errorf("compile graph: %w", err)
	}

	return runner, nil
}

type InMemoryStore struct {
	m map[string][]byte
}

func newInMemoryStore() *InMemoryStore {
	return &InMemoryStore{m: make(map[string][]byte)}
}

func (s *InMemoryStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	data, ok := s.m[checkPointID]
	return data, ok, nil
}

func (s *InMemoryStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	s.m[checkPointID] = checkPoint
	return nil
}

func ExtractInterruptInfo(err error) (*compose.InterruptInfo, bool) {
	return compose.ExtractInterruptInfo(err)
}

func WithStateModifier(fn func(ctx context.Context, path compose.NodePath, s any) error) compose.Option {
	return compose.WithStateModifier(fn)
}

func WithCheckPointID(id string) compose.Option {
	return compose.WithCheckPointID(id)
}

func (a *Agent) Invoke(ctx context.Context, input string, opts ...compose.Option) (string, error) {
	return a.runner.Invoke(ctx, input, opts...)
}

func (a *Agent) Close() error {
	return a.tool.Close()
}

const systemPrompt = `You are memsh-agent, an AI assistant with access to a sandboxed bash shell called memsh.

You can execute bash commands in a safe, in-memory filesystem using the memsh tool. The filesystem
persists across calls within a session — use it as a scratch-pad for file operations, data processing,
scripting, and exploration. The real OS filesystem is never touched.

Always explain what you are doing before running commands. When tasks require multiple steps,
break them down and execute one step at a time. Verify results after each step.

Available capabilities inside memsh:
- File operations: mkdir, touch, cp, mv, rm, ls, cat, head, tail, chmod
- Text processing: echo, printf, sort, uniq, cut, tr, sed, awk, grep, wc
- Data tools: jq (JSON), yq (YAML), base64, tar, gzip
- Scripting: lua, goja (JavaScript), awk
- Network: curl (outbound HTTP only)
- Other: sqlite3, find, diff, stat, date, seq, env, git
`

const nextStepPrompt = `Based on the tool results, decide the next step. If the task is complete, provide a final summary to the user. If more steps are needed, continue with the next command.`

func init() {
	log.SetFlags(0)
	schema.RegisterName[State]("agent-state")
}
