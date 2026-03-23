package tools

import (
	"context"
	"errors"
	"fmt"
)

// Executor runs tool calls with optional approval.
type Executor struct {
	Registry *Registry
	Approver Approver
}

// Execute runs the tool call and returns a tool result.
func (e *Executor) Execute(ctx context.Context, call ToolCall) Result {
	if e == nil || e.Registry == nil {
		return Result{Error: "tool registry is not configured"}
	}
	tool, ok := e.Registry.Lookup(call.Name)
	if !ok {
		return Result{Error: fmt.Sprintf("unknown tool: %s", call.Name)}
	}
	invocation, err := tool.Build(call.Args)
	if err != nil {
		return Result{Error: err.Error()}
	}
	if req := invocation.ConfirmationRequest(); req != nil {
		if e.Approver == nil {
			return Result{Error: "tool approval required but no approver configured"}
		}
		approved, err := e.Approver.Confirm(ctx, *req)
		if err != nil {
			return Result{Error: err.Error()}
		}
		if !approved {
			return Result{Error: "tool execution canceled by user"}
		}
	}
	result, err := invocation.Execute(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return Result{Error: "tool execution canceled"}
		}
		return Result{Error: err.Error()}
	}
	return result
}
