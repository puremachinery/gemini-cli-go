package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAllowsEmptyContent(t *testing.T) {
	root := t.TempDir()
	tool := NewWriteFileTool(Context{WorkspaceRoot: root})

	inv, err := tool.Build(map[string]any{
		"file_path": "empty.txt",
		"content":   "",
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
	data, err := os.ReadFile(filepath.Join(root, "empty.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty file, got %q", string(data))
	}
}

func TestWriteFileWritesContent(t *testing.T) {
	root := t.TempDir()
	tool := NewWriteFileTool(Context{WorkspaceRoot: root})

	inv, err := tool.Build(map[string]any{
		"file_path": "notes.txt",
		"content":   "hello",
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
	data, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected content, got %q", string(data))
	}
}
