package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/gitignore"
	"github.com/puremachinery/gemini-cli-go/internal/patterns"
)

// ReadManyFilesToolName is the tool identifier for read_many_files.
const ReadManyFilesToolName = "read_many_files"

// ErrNoFilesMatched indicates that no files matched the include patterns.
var ErrNoFilesMatched = errors.New("no files matched")

// ErrReadManyFilesFailed indicates all file reads failed.
var ErrReadManyFilesFailed = errors.New("failed to read files")

const maxReadManyBytes = 2 * maxReadBytes

// ReadManyFilesTool implements the read_many_files tool.
type ReadManyFilesTool struct {
	ctx Context
}

// ReadManyFilesParams holds arguments for read_many_files.
type ReadManyFilesParams struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude,omitempty"`
}

type readManyFilesInvocation struct {
	params ReadManyFilesParams
	ctx    Context
}

// NewReadManyFilesTool constructs a ReadManyFilesTool.
func NewReadManyFilesTool(ctx Context) *ReadManyFilesTool {
	return &ReadManyFilesTool{ctx: ctx}
}

// Name returns the tool name.
func (t *ReadManyFilesTool) Name() string { return ReadManyFilesToolName }

// Description returns the tool description.
func (t *ReadManyFilesTool) Description() string {
	return "Read multiple files and concatenate their contents."
}

// Parameters returns the JSON schema for read_many_files.
func (t *ReadManyFilesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"include": map[string]any{
				"type":        "array",
				"description": "List of file paths or glob patterns to include.",
				"items": map[string]any{
					"type": "string",
				},
			},
			"exclude": map[string]any{
				"type":        "array",
				"description": "List of file paths or glob patterns to exclude.",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
		"required": []string{"include"},
	}
}

// Build validates args and returns an invocation.
func (t *ReadManyFilesTool) Build(args map[string]any) (Invocation, error) {
	rawInclude, ok := args["include"]
	if !ok {
		return nil, fmt.Errorf("include is required")
	}
	include, err := toStringSlice(rawInclude)
	if err != nil || len(include) == 0 {
		return nil, fmt.Errorf("include must be a non-empty string array")
	}
	include, err = normalizeIncludePatterns(t.ctx, include)
	if err != nil {
		return nil, err
	}
	exclude, err := toStringSlice(args["exclude"])
	if err != nil {
		return nil, fmt.Errorf("exclude must be a string array")
	}
	params := ReadManyFilesParams{
		Include: include,
		Exclude: exclude,
	}
	return &readManyFilesInvocation{params: params, ctx: t.ctx}, nil
}

func (i *readManyFilesInvocation) Name() string { return ReadManyFilesToolName }

func (i *readManyFilesInvocation) Description() string {
	return fmt.Sprintf("include=%v", i.params.Include)
}

func (i *readManyFilesInvocation) ConfirmationRequest() *ConfirmationRequest {
	if !i.ctx.RequireReadApproval {
		return nil
	}
	preview := strings.Join(i.params.Include, ", ")
	if len([]rune(preview)) > 200 {
		preview = truncateRunes(preview, 200)
	}
	return &ConfirmationRequest{
		ToolName: ReadManyFilesToolName,
		Title:    "Confirm Read Files",
		Prompt:   fmt.Sprintf("Allow reading files matching: %s?", preview),
	}
}

func (i *readManyFilesInvocation) Execute(ctx context.Context) (Result, error) {
	_ = ctx
	entries, readErrors, err := CollectReadManyFiles(i.ctx, i.params.Include, i.params.Exclude)
	if err != nil {
		if errors.Is(err, ErrNoFilesMatched) {
			return Result{Error: ErrNoFilesMatched.Error()}, nil
		}
		if errors.Is(err, ErrReadManyFilesFailed) && len(readErrors) > 0 {
			return Result{Error: fmt.Sprintf("failed to read %d file(s): %s", len(readErrors), readErrors[0])}, nil
		}
		return Result{Error: err.Error()}, nil
	}

	var builder strings.Builder
	for _, entry := range entries {
		fmt.Fprintf(&builder, "--- %s ---\n", entry.Path)
		builder.WriteString(entry.Content)
		if entry.Content == "" || entry.Content[len(entry.Content)-1] != '\n' {
			builder.WriteString("\n")
		}
	}

	output := strings.TrimRight(builder.String(), "\n")
	display := output
	if len(readErrors) > 0 {
		warning := fmt.Sprintf("Warning: failed to read %d file(s). First error: %s", len(readErrors), readErrors[0])
		if display != "" {
			display = warning + "\n" + display
		} else {
			display = warning
		}
	}
	return Result{
		Output:  output,
		Display: display,
	}, nil
}

// ReadManyFileEntry is a single read_many_files entry with display path and content.
type ReadManyFileEntry struct {
	Path    string
	Content string
}

// CollectReadManyFiles reads multiple files using the read_many_files tool semantics.
func CollectReadManyFiles(ctx Context, include []string, exclude []string) ([]ReadManyFileEntry, []string, error) {
	root := EnsureWorkspaceRoot(ctx.WorkspaceRoot)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	if len(include) == 0 {
		return nil, nil, ErrNoFilesMatched
	}

	normalizedInclude, err := normalizeIncludePatterns(ctx, include)
	if err != nil {
		return nil, nil, err
	}
	includeMatchers, err := compilePatterns(normalizedInclude)
	if err != nil {
		return nil, nil, err
	}
	excludeMatchers, err := compilePatterns(append(defaultExcludes(), exclude...))
	if err != nil {
		return nil, nil, err
	}

	indexFiles, err := listWorkspaceFiles(rootAbs)
	if err != nil {
		return nil, nil, err
	}
	files := make([]string, 0, len(indexFiles))
	for _, rel := range indexFiles {
		if matchesAny(rel, excludeMatchers) {
			continue
		}
		if !matchesAny(rel, includeMatchers) {
			continue
		}
		files = append(files, filepath.Join(rootAbs, filepath.FromSlash(rel)))
	}

	files = filterGitIgnored(rootAbs, files)
	if len(files) == 0 {
		return nil, nil, ErrNoFilesMatched
	}

	entries := make([]ReadManyFileEntry, 0, len(files))
	var readErrors []string
	totalBytes := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			displayPath := path
			if rel, relErr := filepath.Rel(rootAbs, path); relErr == nil {
				displayPath = filepath.ToSlash(rel)
			}
			readErrors = append(readErrors, fmt.Sprintf("%s: %v", displayPath, err))
			continue
		}
		displayPath := path
		if rel, relErr := filepath.Rel(rootAbs, path); relErr == nil {
			displayPath = filepath.ToSlash(rel)
		}
		if totalBytes >= maxReadManyBytes {
			readErrors = append(readErrors, fmt.Sprintf("output truncated after %d bytes (max total %d)", totalBytes, maxReadManyBytes))
			break
		}
		remaining := maxReadManyBytes - totalBytes
		if len(data) > remaining {
			data = data[:remaining]
			readErrors = append(readErrors, fmt.Sprintf("%s: content truncated to fit %d bytes", displayPath, maxReadManyBytes))
			totalBytes = maxReadManyBytes
		} else {
			totalBytes += len(data)
		}
		entries = append(entries, ReadManyFileEntry{
			Path:    displayPath,
			Content: string(data),
		})
		if totalBytes >= maxReadManyBytes {
			break
		}
	}

	if len(entries) == 0 && len(readErrors) > 0 {
		return nil, readErrors, ErrReadManyFilesFailed
	}
	return entries, readErrors, nil
}

func filterGitIgnored(rootAbs string, files []string) []string {
	if len(files) == 0 {
		return files
	}
	ignored, ok := gitignore.IgnoredSet(rootAbs, files)
	if !ok || len(ignored) == 0 {
		return files
	}
	out := make([]string, 0, len(files))
	for _, path := range files {
		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			out = append(out, path)
			continue
		}
		rel = filepath.ToSlash(rel)
		if _, skip := ignored[rel]; skip {
			continue
		}
		out = append(out, path)
	}
	return out
}

func toStringSlice(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []string:
		return typed, nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string array")
			}
			out = append(out, str)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected string array")
	}
}

func normalizeIncludePatterns(ctx Context, patterns []string) ([]string, error) {
	root := EnsureWorkspaceRoot(ctx.WorkspaceRoot)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if hasGlob(pattern) {
			out = append(out, filepath.ToSlash(pattern))
			continue
		}
		trimmed := strings.TrimRight(pattern, string(filepath.Separator))
		trimmed = strings.TrimRight(trimmed, "/")
		resolved, err := ResolvePath(rootAbs, trimmed)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(resolved)
		if err != nil {
			if strings.HasSuffix(pattern, string(filepath.Separator)) || strings.HasSuffix(pattern, "/") {
				out = append(out, filepath.ToSlash(trimmed)+"/**")
			} else {
				out = append(out, filepath.ToSlash(pattern))
			}
			continue
		}
		rel, err := filepath.Rel(rootAbs, resolved)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			out = append(out, rel+"/**")
		} else {
			out = append(out, rel)
		}
	}
	return out, nil
}

func hasGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func compilePatterns(entries []string) ([]patterns.Matcher, error) {
	return patterns.CompileMatchers(entries)
}

func matchesAny(path string, matchers []patterns.Matcher) bool {
	return patterns.MatchAny(path, matchers)
}

func defaultExcludes() []string {
	return []string{
		".git/**",
		"node_modules/**",
		"dist/**",
		"bin/**",
		".upstream/**",
	}
}
