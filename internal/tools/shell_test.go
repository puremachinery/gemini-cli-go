package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellToolBuildRequiresCommand(t *testing.T) {
	tool := NewShellTool(Context{})
	if _, err := tool.Build(map[string]any{}); err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestShellToolExecuteRunsCommand(t *testing.T) {
	root := t.TempDir()
	tool := NewShellTool(Context{WorkspaceRoot: root})
	inv, err := tool.Build(map[string]any{
		"command":  "echo hello",
		"dir_path": ".",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	res, err := inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected error: %#v", res)
	}
	if strings.TrimSpace(res.Output) != "hello" {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestShellToolDescriptionIncludesDir(t *testing.T) {
	tool := NewShellTool(Context{WorkspaceRoot: t.TempDir()})
	inv, err := tool.Build(map[string]any{
		"command":  "pwd",
		"dir_path": "subdir",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(inv.Description(), filepath.ToSlash("subdir")) {
		t.Fatalf("expected description to include dir, got %q", inv.Description())
	}
}
