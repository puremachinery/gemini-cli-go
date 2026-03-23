package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

// Context provides execution context for tools.
type Context struct {
	WorkspaceRoot        string
	GeminiClient         client.Client
	Headless             bool
	RequireReadApproval  bool
	AllowPrivateWebFetch bool
}

// Registry stores available tools.
type Registry struct {
	ctx   Context
	tools map[string]Tool
}

// NewRegistry creates a registry with the built-in tools.
func NewRegistry(ctx Context) *Registry {
	toolsMap := map[string]Tool{}
	reg := &Registry{ctx: ctx, tools: toolsMap}
	reg.Register(NewShellTool(ctx))
	reg.Register(NewReadFileTool(ctx))
	reg.Register(NewReadManyFilesTool(ctx))
	reg.Register(NewWriteFileTool(ctx))
	reg.Register(NewReplaceTool(ctx))
	if ctx.GeminiClient != nil {
		reg.Register(NewWebSearchTool(ctx))
		reg.Register(NewWebFetchTool(ctx))
	}
	return reg
}

// Register adds a tool.
func (r *Registry) Register(tool Tool) {
	if tool == nil {
		return
	}
	if r.tools == nil {
		r.tools = map[string]Tool{}
	}
	r.tools[tool.Name()] = tool
}

// Remove deletes a tool by name.
func (r *Registry) Remove(name string) {
	if r == nil || r.tools == nil {
		return
	}
	delete(r.tools, name)
}

// Lookup returns a tool by name.
func (r *Registry) Lookup(name string) (Tool, bool) {
	if r == nil || r.tools == nil {
		return nil, false
	}
	tool, ok := r.tools[name]
	return tool, ok
}

// WorkspaceRoot returns the registry's workspace root.
func (r *Registry) WorkspaceRoot() string {
	if r == nil {
		return ""
	}
	return r.ctx.WorkspaceRoot
}

// FunctionDeclarations returns tool declarations suitable for model requests.
func (r *Registry) FunctionDeclarations() []llm.FunctionDeclaration {
	if r == nil || r.tools == nil {
		return nil
	}
	out := make([]llm.FunctionDeclaration, 0, len(r.tools))
	for _, tool := range r.tools {
		out = append(out, llm.FunctionDeclaration{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}
	return out
}

// ResolvePath resolves a tool path relative to the workspace.
func ResolvePath(workspaceRoot, target string) (string, error) {
	if target == "" {
		return "", errors.New("path is required")
	}
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	var resolved string
	if filepath.IsAbs(target) {
		resolved = target
	} else {
		resolved = filepath.Join(root, target)
	}
	resolved = filepath.Clean(resolved)
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || hasPathTraversal(rel) {
		return "", fmt.Errorf("path %q is outside workspace", target)
	}
	return resolved, nil
}

// hasPathTraversal reports whether a relative path contains any .. segment.
func hasPathTraversal(rel string) bool {
	if rel == ".." {
		return true
	}
	for {
		dir, file := filepath.Split(rel)
		if file == ".." {
			return true
		}
		if dir == "" || dir == string(filepath.Separator) {
			return false
		}
		rel = filepath.Clean(dir)
	}
}

// EnsureWorkspaceRoot returns a valid workspace root.
func EnsureWorkspaceRoot(root string) string {
	if root != "" {
		return root
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
