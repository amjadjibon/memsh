package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/amjadjibon/memsh/pkg/shell"
)

type MemshTool struct {
	sh  *shell.Shell
	buf *strings.Builder
	mu  sync.Mutex
}

func NewMemshTool(wasmEnabled bool) (*MemshTool, error) {
	var buf strings.Builder
	sh, err := shell.New(
		shell.WithInheritEnv(false),
		shell.WithWASMEnabled(wasmEnabled),
		shell.WithStdIO(strings.NewReader(""), &buf, &buf),
	)
	if err != nil {
		return nil, fmt.Errorf("agent: shell init: %w", err)
	}
	return &MemshTool{sh: sh, buf: &buf}, nil
}

func (t *MemshTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "memsh",
		Desc: "Execute bash commands in a sandboxed in-memory filesystem. " +
			"The filesystem persists across calls — use it as a scratch-pad. " +
			"The real OS filesystem is never touched and no host commands can escape the sandbox. " +
			"Stdin is not available; commands that read input receive EOF.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command": {
				Type:     schema.String,
				Desc:     "The bash command or script to execute",
				Required: true,
			},
		}),
	}, nil
}

func (t *MemshTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("memsh: invalid arguments: %w", err)
	}
	if input.Command == "" {
		return "", fmt.Errorf("memsh: command is required")
	}

	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	t.mu.Lock()
	t.buf.Reset()
	runErr := t.sh.Run(callCtx, input.Command)
	output := t.buf.String()
	cwd := t.sh.Cwd()
	t.mu.Unlock()

	result := output + "\nCwd: " + cwd

	if runErr != nil && !errors.Is(runErr, shell.ErrExit) {
		if errors.Is(runErr, context.DeadlineExceeded) {
			result += "\nerror: timeout after 30s"
		} else {
			result += "\nerror: " + runErr.Error()
		}
	}

	return result, nil
}

func (t *MemshTool) Close() error {
	return t.sh.Close()
}
