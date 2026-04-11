package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMemshToolInfo(t *testing.T) {
	tl, err := NewMemshTool(false)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()

	info, err := tl.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "memsh" {
		t.Errorf("Name = %q, want %q", info.Name, "memsh")
	}
	if info.Desc == "" {
		t.Error("Desc is empty")
	}
	if info.ParamsOneOf == nil {
		t.Error("ParamsOneOf is nil")
	}
}

func TestMemshToolRun(t *testing.T) {
	tl, err := NewMemshTool(false)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()

	args, _ := json.Marshal(map[string]string{
		"command": "echo hello && mkdir /test && echo world > /test/f.txt && cat /test/f.txt",
	})

	result, err := tl.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "hello") {
		t.Errorf("result missing 'hello': %s", result)
	}
	if !strings.Contains(result, "world") {
		t.Errorf("result missing 'world': %s", result)
	}
	if !strings.Contains(result, "Cwd: /") {
		t.Errorf("result missing 'Cwd: /': %s", result)
	}
}

func TestMemshToolRunEmpty(t *testing.T) {
	tl, err := NewMemshTool(false)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()

	args, _ := json.Marshal(map[string]string{"command": ""})

	_, err = tl.InvokableRun(context.Background(), string(args))
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestMemshToolRunInvalidJSON(t *testing.T) {
	tl, err := NewMemshTool(false)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()

	_, err = tl.InvokableRun(context.Background(), "not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMemshToolRunPersistence(t *testing.T) {
	tl, err := NewMemshTool(false)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()

	args1, _ := json.Marshal(map[string]string{
		"command": "echo persisted > /data.txt",
	})
	result1, err := tl.InvokableRun(context.Background(), string(args1))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result1, "Cwd: /") {
		t.Errorf("result1 missing cwd: %s", result1)
	}

	args2, _ := json.Marshal(map[string]string{
		"command": "cat /data.txt",
	})
	result2, err := tl.InvokableRun(context.Background(), string(args2))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result2, "persisted") {
		t.Errorf("result2 missing 'persisted': %s", result2)
	}
}

func TestMemshToolRunError(t *testing.T) {
	tl, err := NewMemshTool(false)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()

	args, _ := json.Marshal(map[string]string{
		"command": "nonexistent_command",
	})

	result, err := tl.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "error:") {
		t.Errorf("result should contain error: %s", result)
	}
}
