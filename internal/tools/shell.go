package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ShellToolName is the tool identifier for run_shell_command.
const ShellToolName = "run_shell_command"

const maxShellOutputBytes = 1 << 20
const shellCommandTimeout = 2 * time.Minute

// ShellTool implements the run_shell_command tool.
type ShellTool struct {
	ctx Context
}

// ShellParams holds arguments for run_shell_command.
type ShellParams struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
	DirPath     string `json:"dir_path,omitempty"`
}

type shellInvocation struct {
	params ShellParams
	ctx    Context
}

// NewShellTool constructs a ShellTool.
func NewShellTool(ctx Context) *ShellTool {
	return &ShellTool{ctx: ctx}
}

// Name returns the tool name.
func (t *ShellTool) Name() string { return ShellToolName }

// Description returns the tool description.
func (t *ShellTool) Description() string {
	return "Execute a shell command on the local machine."
}

// Parameters returns the JSON schema for run_shell_command.
func (t *ShellTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The command to run.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Optional description of the command.",
			},
			"dir_path": map[string]any{
				"type":        "string",
				"description": "Optional working directory.",
			},
		},
		"required": []string{"command"},
	}
}

// Build validates args and returns an invocation.
func (t *ShellTool) Build(args map[string]any) (Invocation, error) {
	command, err := getStringArg(args, "command")
	if err != nil {
		return nil, err
	}
	params := ShellParams{
		Command:     command,
		Description: getOptionalString(args, "description"),
		DirPath:     getOptionalString(args, "dir_path"),
	}
	return &shellInvocation{params: params, ctx: t.ctx}, nil
}

func (i *shellInvocation) Name() string { return ShellToolName }

func (i *shellInvocation) Description() string {
	desc := i.params.Command
	if i.params.DirPath != "" {
		desc = fmt.Sprintf("%s [in %s]", desc, i.params.DirPath)
	}
	if i.params.Description != "" {
		desc = fmt.Sprintf("%s (%s)", desc, strings.ReplaceAll(i.params.Description, "\n", " "))
	}
	return desc
}

func (i *shellInvocation) ConfirmationRequest() *ConfirmationRequest {
	return &ConfirmationRequest{
		ToolName: ShellToolName,
		Title:    "Confirm Shell Command",
		Prompt:   fmt.Sprintf("Allow execution of: %q?", i.params.Command),
	}
}

func (i *shellInvocation) Execute(ctx context.Context) (Result, error) {
	if i.params.Command == "" {
		return Result{Error: "command is required"}, nil
	}
	workDir := i.ctx.WorkspaceRoot
	if i.params.DirPath != "" {
		resolved, err := ResolvePath(i.ctx.WorkspaceRoot, i.params.DirPath)
		if err != nil {
			return Result{Error: err.Error()}, nil
		}
		workDir = resolved
	}
	if workDir != "" {
		if abs, err := filepath.Abs(workDir); err == nil {
			workDir = abs
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, shellCommandTimeout)
	defer cancel()
	cmd, err := buildShellCommand(execCtx, i.params.Command)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}
	if workDir != "" {
		cmd.Dir = workDir
	}
	var output limitedBuffer
	output.limit = maxShellOutputBytes
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(output.String())
		if output.truncated {
			out = strings.TrimSpace(out + "\n[output truncated]")
		}
		if out == "" {
			out = err.Error()
		}
		return Result{
			Output:  out,
			Error:   err.Error(),
			Display: out,
		}, nil
	}
	out := strings.TrimSpace(output.String())
	if output.truncated {
		out = strings.TrimSpace(out + "\n[output truncated]")
	}
	return Result{
		Output:  out,
		Display: out,
	}, nil
}

func buildShellCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	if command == "" {
		return nil, errors.New("command is empty")
	}
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-Command", command), nil
	}
	if _, err := exec.LookPath("bash"); err == nil {
		return exec.CommandContext(ctx, "bash", "-lc", command), nil
	}
	return exec.CommandContext(ctx, "sh", "-c", command), nil
}

type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit <= 0 {
		return b.buf.Write(p)
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
