package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

func TestBuildPartsFromQueryNoAt(t *testing.T) {
	parts, err := buildPartsFromQuery(context.Background(), "hello world", nil)
	if err != nil {
		t.Fatalf("buildPartsFromQuery: %v", err)
	}
	if len(parts) != 1 || parts[0].Text != "hello world" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
}

func TestBuildPartsFromQueryIncludesFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	registry := tools.NewRegistry(tools.Context{WorkspaceRoot: root})
	executor := &tools.Executor{Registry: registry}

	parts, err := buildPartsFromQuery(context.Background(), "Please read @notes.txt", executor)
	if err != nil {
		t.Fatalf("buildPartsFromQuery: %v", err)
	}
	var combined strings.Builder
	for _, part := range parts {
		combined.WriteString(part.Text)
	}
	out := combined.String()
	if !strings.Contains(out, referenceContentStart) {
		t.Fatalf("expected reference header, got: %q", out)
	}
	if !strings.Contains(out, "Content from @notes.txt") {
		t.Fatalf("expected file label, got: %q", out)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected file content, got: %q", out)
	}
}

func TestBuildPartsFromQuerySkipsInvalidPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	registry := tools.NewRegistry(tools.Context{WorkspaceRoot: root})
	executor := &tools.Executor{Registry: registry}

	parts, err := buildPartsFromQuery(context.Background(), "Check @../nope and @notes.txt", executor)
	if err != nil {
		t.Fatalf("buildPartsFromQuery: %v", err)
	}
	var combined strings.Builder
	for _, part := range parts {
		combined.WriteString(part.Text)
	}
	out := combined.String()
	if !strings.Contains(out, "Content from @notes.txt") {
		t.Fatalf("expected valid file to be included, got: %q", out)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected file content, got: %q", out)
	}
}

func TestBuildPartsFromQueryAllInvalidPathsFallsBackToQuery(t *testing.T) {
	root := t.TempDir()
	registry := tools.NewRegistry(tools.Context{WorkspaceRoot: root})
	executor := &tools.Executor{Registry: registry}
	query := "Read @../nope"

	parts, err := buildPartsFromQuery(context.Background(), query, executor)
	if err != nil {
		t.Fatalf("buildPartsFromQuery: %v", err)
	}
	var combined strings.Builder
	for _, part := range parts {
		combined.WriteString(part.Text)
	}
	if combined.String() != query {
		t.Fatalf("expected original query, got: %q", combined.String())
	}
}

func TestBuildPartsFromQueryPreservesSeparatorLine(t *testing.T) {
	root := t.TempDir()
	content := "line 1\n--- not a header ---\nline 3\n"
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	registry := tools.NewRegistry(tools.Context{WorkspaceRoot: root})
	executor := &tools.Executor{Registry: registry}

	parts, err := buildPartsFromQuery(context.Background(), "Read @notes.txt", executor)
	if err != nil {
		t.Fatalf("buildPartsFromQuery: %v", err)
	}
	var combined strings.Builder
	for _, part := range parts {
		combined.WriteString(part.Text)
	}
	out := combined.String()
	if !strings.Contains(out, "--- not a header ---") {
		t.Fatalf("expected separator-looking line preserved, got: %q", out)
	}
}
