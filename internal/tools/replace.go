package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/storage"
)

// ReplaceToolName is the tool identifier for replace.
const ReplaceToolName = "replace"

// ReplaceTool implements the replace tool.
type ReplaceTool struct {
	ctx Context
}

// ReplaceParams holds arguments for replace.
type ReplaceParams struct {
	FilePath             string `json:"file_path"`
	OldString            string `json:"old_string"`
	NewString            string `json:"new_string"`
	ExpectedReplacements int    `json:"expected_replacements,omitempty"`
}

type replaceInvocation struct {
	params ReplaceParams
	ctx    Context
}

// NewReplaceTool constructs a ReplaceTool.
func NewReplaceTool(ctx Context) *ReplaceTool {
	return &ReplaceTool{ctx: ctx}
}

// Name returns the tool name.
func (t *ReplaceTool) Name() string { return ReplaceToolName }

// Description returns the tool description.
func (t *ReplaceTool) Description() string {
	return "Replace text in a file by matching an exact string."
}

// Parameters returns the JSON schema for replace.
func (t *ReplaceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Text to replace.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement text.",
			},
			"expected_replacements": map[string]any{
				"type":        "number",
				"description": "Optional number of expected replacements (defaults to 1).",
			},
		},
		"required": []string{"file_path", "old_string", "new_string"},
	}
}

// Build validates args and returns an invocation.
func (t *ReplaceTool) Build(args map[string]any) (Invocation, error) {
	path, err := getStringArg(args, "file_path")
	if err != nil {
		return nil, err
	}
	oldString, err := getStringArg(args, "old_string")
	if err != nil {
		return nil, err
	}
	newString, err := getStringArg(args, "new_string")
	if err != nil {
		return nil, err
	}
	expected, expectedSet, err := getOptionalInt(args, "expected_replacements")
	if err != nil {
		return nil, err
	}
	if expectedSet && expected <= 0 {
		return nil, fmt.Errorf("expected_replacements must be greater than 0")
	}
	return &replaceInvocation{
		params: ReplaceParams{
			FilePath:             path,
			OldString:            oldString,
			NewString:            newString,
			ExpectedReplacements: expected,
		},
		ctx: t.ctx,
	}, nil
}

func (i *replaceInvocation) Name() string { return ReplaceToolName }

func (i *replaceInvocation) Description() string {
	return i.params.FilePath
}

func (i *replaceInvocation) ConfirmationRequest() *ConfirmationRequest {
	return &ConfirmationRequest{
		ToolName: ReplaceToolName,
		Title:    "Confirm Edit",
		Prompt:   fmt.Sprintf("Allow editing %q?", i.params.FilePath),
	}
}

func (i *replaceInvocation) Execute(ctx context.Context) (Result, error) {
	_ = ctx
	resolved, err := ResolvePath(i.ctx.WorkspaceRoot, i.params.FilePath)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}
	content := string(data)
	expected := i.params.ExpectedReplacements
	if expected == 0 {
		expected = 1
	}
	count := strings.Count(content, i.params.OldString)
	if count == 0 {
		return Result{Error: "old_string not found in file"}, nil
	}
	if count != expected {
		return Result{Error: fmt.Sprintf("expected %d replacements, found %d", expected, count)}, nil
	}
	updated := strings.ReplaceAll(content, i.params.OldString, i.params.NewString)
	perm := os.FileMode(0o644)
	if info, statErr := os.Stat(resolved); statErr == nil {
		perm = info.Mode().Perm()
	}
	if err := storage.WriteFileAtomic(resolved, []byte(updated), perm); err != nil {
		return Result{Error: err.Error()}, nil
	}
	rootAbs, absErr := filepath.Abs(EnsureWorkspaceRoot(i.ctx.WorkspaceRoot))
	if absErr == nil {
		invalidateWorkspaceIndex(rootAbs)
	}
	return Result{
		Output:  fmt.Sprintf("Replaced %d occurrence(s) in %s", expected, resolved),
		Display: fmt.Sprintf("Edited %s", resolved),
	}, nil
}
