package ui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/memory"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

type stubApprover struct {
	approved bool
	req      tools.ConfirmationRequest
}

func (s *stubApprover) Confirm(_ context.Context, req tools.ConfirmationRequest) (bool, error) {
	s.req = req
	return s.approved, nil
}

func TestRunMemoryShowEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	workspace := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	memState := memory.NewState(workspace)
	if err := memState.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	input := bytes.NewBufferString("/memory show\n/quit\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
		Memory:    memState,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(output.String(), "Memory is currently empty.") {
		t.Fatalf("expected empty memory output, got: %q", output.String())
	}
}

func TestRunMemoryListShowAdd(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	globalDir := filepath.Join(tmp, ".gemini")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	globalPath := filepath.Join(globalDir, memory.DefaultContextFilename)
	if err := os.WriteFile(globalPath, []byte("Global"), 0o644); err != nil {
		t.Fatalf("write global: %v", err)
	}

	workspace := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, memory.DefaultContextFilename), []byte("Project"), 0o644); err != nil {
		t.Fatalf("write root: %v", err)
	}
	subDir := filepath.Join(workspace, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, memory.DefaultContextFilename), []byte("Sub"), 0o644); err != nil {
		t.Fatalf("write sub: %v", err)
	}

	memState := memory.NewState(workspace)
	if err := memState.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	approver := &stubApprover{approved: true}
	input := bytes.NewBufferString("/memory list\n/memory show\n/memory add remember this\n/memory show\n/quit\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}

	if err := Run(context.Background(), RunOptions{
		Client:       client,
		Model:        "test-model",
		Input:        input,
		Output:       &output,
		ShowIntro:    false,
		Memory:       memState,
		ToolExecutor: &tools.Executor{Approver: approver},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "There are 3 GEMINI.md file(s) in use:") {
		t.Fatalf("expected list output, got: %q", out)
	}
	if !strings.Contains(out, globalPath) {
		t.Fatalf("expected global path in list, got: %q", out)
	}
	if !strings.Contains(out, "Current memory content from 3 file(s):") {
		t.Fatalf("expected show output, got: %q", out)
	}
	if !strings.Contains(out, "Added memory: \"remember this\"") {
		t.Fatalf("expected add output, got: %q", out)
	}
	if !strings.Contains(out, "## Gemini Added Memories") {
		t.Fatalf("expected memory section output, got: %q", out)
	}
	if !strings.Contains(out, "- remember this") {
		t.Fatalf("expected memory item output, got: %q", out)
	}
	if approver.req.ToolName != tools.SaveMemoryToolName {
		t.Fatalf("expected save_memory approval, got %q", approver.req.ToolName)
	}
}
