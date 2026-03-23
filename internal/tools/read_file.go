package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ReadFileToolName is the tool identifier for read_file.
const ReadFileToolName = "read_file"

const maxReadBytes = 1 << 20

// ReadFileTool implements the read_file tool.
type ReadFileTool struct {
	ctx Context
}

// ReadFileParams holds arguments for read_file.
type ReadFileParams struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type readFileInvocation struct {
	params ReadFileParams
	ctx    Context
}

// NewReadFileTool constructs a ReadFileTool.
func NewReadFileTool(ctx Context) *ReadFileTool {
	return &ReadFileTool{ctx: ctx}
}

// Name returns the tool name.
func (t *ReadFileTool) Name() string { return ReadFileToolName }

// Description returns the tool description.
func (t *ReadFileTool) Description() string {
	return "Read a file from the workspace. Supports optional line offsets and limits."
}

// Parameters returns the JSON schema for read_file.
func (t *ReadFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read.",
			},
			"offset": map[string]any{
				"type":        "number",
				"description": "Optional 0-based line offset (requires limit).",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Optional maximum number of lines to read.",
			},
		},
		"required": []string{"file_path"},
	}
}

// Build validates args and returns an invocation.
func (t *ReadFileTool) Build(args map[string]any) (Invocation, error) {
	path, err := getStringArg(args, "file_path")
	if err != nil {
		return nil, err
	}
	offset, offsetSet, err := getOptionalInt(args, "offset")
	if err != nil {
		return nil, err
	}
	limit, limitSet, err := getOptionalInt(args, "limit")
	if err != nil {
		return nil, err
	}
	if offsetSet && (!limitSet || limit <= 0) {
		return nil, fmt.Errorf("limit must be set to a positive number when offset is provided")
	}
	if offsetSet && offset < 0 {
		return nil, fmt.Errorf("offset must be non-negative")
	}
	if limitSet && limit < 0 {
		return nil, fmt.Errorf("limit must be non-negative")
	}
	if limitSet && limit == 0 {
		return nil, fmt.Errorf("limit must be greater than 0 when provided")
	}
	params := ReadFileParams{
		FilePath: path,
	}
	if offsetSet {
		params.Offset = offset
	}
	if limitSet {
		params.Limit = limit
	}
	return &readFileInvocation{params: params, ctx: t.ctx}, nil
}

func (i *readFileInvocation) Name() string { return ReadFileToolName }

func (i *readFileInvocation) Description() string {
	return i.params.FilePath
}

func (i *readFileInvocation) ConfirmationRequest() *ConfirmationRequest {
	if !i.ctx.RequireReadApproval {
		return nil
	}
	return &ConfirmationRequest{
		ToolName: ReadFileToolName,
		Title:    "Confirm Read File",
		Prompt:   fmt.Sprintf("Allow reading %q?", i.params.FilePath),
	}
}

func (i *readFileInvocation) Execute(ctx context.Context) (Result, error) {
	_ = ctx
	resolved, err := ResolvePath(i.ctx.WorkspaceRoot, i.params.FilePath)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}

	info, err := os.Stat(resolved)
	if err == nil && info.Size() > maxReadBytes && i.params.Limit == 0 {
		return Result{
			Error:   fmt.Sprintf("file exceeds %d bytes; use offset/limit to read in chunks", maxReadBytes),
			Display: "File too large to read in one call.",
		}, nil
	}

	if i.params.Offset > 0 || i.params.Limit > 0 {
		file, err := os.Open(resolved)
		if err != nil {
			return Result{Error: err.Error()}, nil
		}
		content, err := readLines(file, i.params.Offset, i.params.Limit)
		closeErr := file.Close()
		if err != nil {
			return Result{Error: err.Error()}, nil
		}
		if closeErr != nil {
			return Result{Error: closeErr.Error()}, nil
		}
		return Result{
			Output:  content,
			Display: content,
		}, nil
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}
	if len(data) > maxReadBytes {
		data = data[:maxReadBytes]
	}
	content := string(data)
	return Result{
		Output:  content,
		Display: content,
	}, nil
}

func readLines(file *os.File, offset int, limit int) (string, error) {
	if offset < 0 || limit < 0 {
		return "", errors.New("offset and limit must be non-negative")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxReadBytes)
	lines := []string{}
	totalBytes := 0
	index := 0
	for scanner.Scan() {
		if index >= offset {
			line := scanner.Text()
			if len(lines) > 0 {
				totalBytes++
			}
			totalBytes += len(line)
			if totalBytes > maxReadBytes {
				return "", fmt.Errorf("read exceeds %d bytes; use a smaller limit", maxReadBytes)
			}
			lines = append(lines, line)
			if limit > 0 && len(lines) >= limit {
				break
			}
		}
		index++
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if len(lines) == 0 && offset > 0 {
		return "", fmt.Errorf("offset %d beyond end of file", offset)
	}
	return strings.Join(lines, "\n"), nil
}
