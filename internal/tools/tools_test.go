package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceToolExpectedReplacements(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("hello\nhello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tool := NewReplaceTool(Context{WorkspaceRoot: root})
	inv, err := tool.Build(map[string]any{
		"file_path":  "sample.txt",
		"old_string": "hello",
		"new_string": "hi",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	res, err := inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error == "" || !strings.Contains(res.Error, "expected 1 replacements") {
		t.Fatalf("expected replacement count error, got: %#v", res)
	}

	inv, err = tool.Build(map[string]any{
		"file_path":             "sample.txt",
		"old_string":            "hello",
		"new_string":            "hi",
		"expected_replacements": float64(2),
	})
	if err != nil {
		t.Fatalf("Build (expected): %v", err)
	}
	res, err = inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute (expected): %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected error: %#v", res)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := strings.Count(string(data), "hi"); got != 2 {
		t.Fatalf("expected 2 replacements, got %d", got)
	}
}

func TestReadFileRequiresLimitWithOffset(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tool := NewReadFileTool(Context{WorkspaceRoot: root})
	_, err := tool.Build(map[string]any{
		"file_path": "sample.txt",
		"offset":    float64(1),
	})
	if err == nil {
		t.Fatal("expected error for offset without limit")
	}
	_, err = tool.Build(map[string]any{
		"file_path": "sample.txt",
		"limit":     float64(0),
	})
	if err == nil {
		t.Fatal("expected error for explicit limit=0")
	}
}

func TestReadFileOffsetBeyondEnd(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tool := NewReadFileTool(Context{WorkspaceRoot: root})
	inv, err := tool.Build(map[string]any{
		"file_path": "sample.txt",
		"offset":    float64(10),
		"limit":     float64(1),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	res, err := inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error == "" || !strings.Contains(res.Error, "offset 10 beyond end") {
		t.Fatalf("expected offset error, got: %#v", res)
	}
}

func TestReadManyFilesDirectoryInclude(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "guide.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tool := NewReadManyFilesTool(Context{WorkspaceRoot: root})
	inv, err := tool.Build(map[string]any{
		"include": []any{"docs"},
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
	if !strings.Contains(res.Output, "hello") {
		t.Fatalf("expected output to contain file contents, got: %q", res.Output)
	}
	if !strings.Contains(res.Output, "docs/guide.txt") {
		t.Fatalf("expected output to include relative path, got: %q", res.Output)
	}
}

func TestReadManyFilesDirectoryIncludeTrailingSlash(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "guide.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tool := NewReadManyFilesTool(Context{WorkspaceRoot: root})
	inv, err := tool.Build(map[string]any{
		"include": []any{"docs/"},
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
	if !strings.Contains(res.Output, "docs/guide.txt") {
		t.Fatalf("expected output to include relative path, got: %q", res.Output)
	}
}

func TestReadManyFilesDirectoryIncludeAbsolute(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "guide.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tool := NewReadManyFilesTool(Context{WorkspaceRoot: root})
	inv, err := tool.Build(map[string]any{
		"include": []any{dir + string(filepath.Separator)},
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
	if !strings.Contains(res.Output, "docs/guide.txt") {
		t.Fatalf("expected output to include relative path, got: %q", res.Output)
	}
}

func TestReadManyFilesHonorsGitIgnore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	init := exec.Command("git", "init")
	init.Dir = root
	if err := init.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("secret.txt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("top-secret"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep-me"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tool := NewReadManyFilesTool(Context{WorkspaceRoot: root})
	inv, err := tool.Build(map[string]any{
		"include": []any{"*"},
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
	if strings.Contains(res.Output, "--- secret.txt ---") || strings.Contains(res.Output, "top-secret") {
		t.Fatalf("expected gitignored file to be excluded, got: %q", res.Output)
	}
	if !strings.Contains(res.Output, "keep.txt") {
		t.Fatalf("expected output to include keep.txt, got: %q", res.Output)
	}
}

func TestReadManyFilesSeesNewFileAfterWrite(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "first.txt"), []byte("one"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	readTool := NewReadManyFilesTool(Context{WorkspaceRoot: root})
	readInv, err := readTool.Build(map[string]any{
		"include": []any{"*"},
	})
	if err != nil {
		t.Fatalf("Build (read): %v", err)
	}
	res, err := readInv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute (read): %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected error: %#v", res)
	}
	if !strings.Contains(res.Output, "first.txt") {
		t.Fatalf("expected output to include first.txt, got: %q", res.Output)
	}

	writeTool := NewWriteFileTool(Context{WorkspaceRoot: root})
	writeInv, err := writeTool.Build(map[string]any{
		"file_path": "second.txt",
		"content":   "two",
	})
	if err != nil {
		t.Fatalf("Build (write): %v", err)
	}
	writeRes, err := writeInv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute (write): %v", err)
	}
	if writeRes.Error != "" {
		t.Fatalf("unexpected write error: %#v", writeRes)
	}

	readInv, err = readTool.Build(map[string]any{
		"include": []any{"*"},
	})
	if err != nil {
		t.Fatalf("Build (read2): %v", err)
	}
	res, err = readInv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute (read2): %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected error: %#v", res)
	}
	if !strings.Contains(res.Output, "second.txt") {
		t.Fatalf("expected output to include second.txt, got: %q", res.Output)
	}
}

func TestRegistryRegistersWebSearchOnlyWithGeminiClient(t *testing.T) {
	registry := NewRegistry(Context{WorkspaceRoot: t.TempDir()})
	if _, ok := registry.Lookup(WebSearchToolName); ok {
		t.Fatal("expected google_web_search to be absent without gemini client")
	}
	if _, ok := registry.Lookup(WebFetchToolName); ok {
		t.Fatal("expected web_fetch to be absent without gemini client")
	}

	registry = NewRegistry(Context{
		WorkspaceRoot: t.TempDir(),
		GeminiClient:  &stubClient{},
	})
	if _, ok := registry.Lookup(WebSearchToolName); !ok {
		t.Fatal("expected google_web_search to be registered with gemini client")
	}
	if _, ok := registry.Lookup(WebFetchToolName); !ok {
		t.Fatal("expected web_fetch to be registered with gemini client")
	}
}
