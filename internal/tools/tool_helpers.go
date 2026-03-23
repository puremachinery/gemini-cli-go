package tools

import (
	"sort"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

// ToolDeclarationsForExecutor returns tool declarations for a tool executor.
func ToolDeclarationsForExecutor(executor *Executor) []llm.Tool {
	if executor == nil || executor.Registry == nil {
		return nil
	}
	decls := executor.Registry.FunctionDeclarations()
	if len(decls) == 0 {
		return nil
	}
	return []llm.Tool{{FunctionDeclarations: decls}}
}

// ToolNamesForExecutor returns sorted tool names for display.
func ToolNamesForExecutor(executor *Executor) []string {
	if executor == nil || executor.Registry == nil {
		return nil
	}
	decls := executor.Registry.FunctionDeclarations()
	if len(decls) == 0 {
		return nil
	}
	names := make([]string, 0, len(decls))
	for _, decl := range decls {
		if decl.Name != "" {
			names = append(names, decl.Name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return names
}
