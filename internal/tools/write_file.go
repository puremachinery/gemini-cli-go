package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/puremachinery/gemini-cli-go/internal/storage"
)

// WriteFileToolName is the tool identifier for write_file.
const WriteFileToolName = "write_file"

// WriteFileTool implements the write_file tool.
type WriteFileTool struct {
	ctx Context
}

// WriteFileParams holds arguments for write_file.
type WriteFileParams struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

type writeFileInvocation struct {
	params WriteFileParams
	ctx    Context
}

// NewWriteFileTool constructs a WriteFileTool.
func NewWriteFileTool(ctx Context) *WriteFileTool {
	return &WriteFileTool{ctx: ctx}
}

// Name returns the tool name.
func (t *WriteFileTool) Name() string { return WriteFileToolName }

// Description returns the tool description.
func (t *WriteFileTool) Description() string {
	return "Write content to a file, creating directories as needed."
}

// Parameters returns the JSON schema for write_file.
func (t *WriteFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write.",
			},
		},
		"required": []string{"file_path", "content"},
	}
}

// Build validates args and returns an invocation.
func (t *WriteFileTool) Build(args map[string]any) (Invocation, error) {
	path, err := getStringArg(args, "file_path")
	if err != nil {
		return nil, err
	}
	rawContent, ok := args["content"]
	if !ok {
		return nil, fmt.Errorf("content is required")
	}
	content, ok := rawContent.(string)
	if !ok {
		return nil, fmt.Errorf("content must be a string")
	}
	return &writeFileInvocation{
		params: WriteFileParams{FilePath: path, Content: content},
		ctx:    t.ctx,
	}, nil
}

func (i *writeFileInvocation) Name() string { return WriteFileToolName }

func (i *writeFileInvocation) Description() string {
	return i.params.FilePath
}

func (i *writeFileInvocation) ConfirmationRequest() *ConfirmationRequest {
	return &ConfirmationRequest{
		ToolName: WriteFileToolName,
		Title:    "Confirm Write File",
		Prompt:   fmt.Sprintf("Allow writing to %q?", i.params.FilePath),
	}
}

func (i *writeFileInvocation) Execute(ctx context.Context) (Result, error) {
	_ = ctx
	resolved, err := ResolvePath(i.ctx.WorkspaceRoot, i.params.FilePath)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}
	perm := os.FileMode(0o644)
	dirPerm := os.FileMode(0o755)
	if storage.IsGlobalGeminiPath(resolved) {
		perm = 0o600
		dirPerm = 0o700
	}
	if err := os.MkdirAll(filepath.Dir(resolved), dirPerm); err != nil {
		return Result{Error: err.Error()}, nil
	}
	if err := storage.WriteFileAtomic(resolved, []byte(i.params.Content), perm); err != nil {
		return Result{Error: err.Error()}, nil
	}
	rootAbs, absErr := filepath.Abs(EnsureWorkspaceRoot(i.ctx.WorkspaceRoot))
	if absErr == nil {
		invalidateWorkspaceIndex(rootAbs)
	}
	return Result{
		Output:  fmt.Sprintf("Wrote %d bytes to %s", len(i.params.Content), resolved),
		Display: fmt.Sprintf("Wrote %s", resolved),
	}, nil
}
