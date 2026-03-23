// Package tools provides core tool definitions and execution helpers.
package tools

import "context"

// Tool describes a callable tool and its JSON schema.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Build(args map[string]any) (Invocation, error)
}

// Invocation represents a validated tool call ready to execute.
type Invocation interface {
	Name() string
	Description() string
	ConfirmationRequest() *ConfirmationRequest
	Execute(ctx context.Context) (Result, error)
}

// Result is returned by tool execution.
type Result struct {
	Output  string
	Error   string
	Display string
}

// Response converts a tool result into a function response payload.
func (r Result) Response() map[string]any {
	payload := map[string]any{
		"output": r.Output,
	}
	if r.Error != "" {
		payload["error"] = r.Error
	}
	return payload
}

// DisplayText returns the preferred UI display string.
func (r Result) DisplayText() string {
	if r.Display != "" {
		return r.Display
	}
	if r.Error != "" {
		return r.Error
	}
	return r.Output
}

// ToolCall captures a tool call from the model.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}
