package tools

import "context"

// ConfirmationRequest describes a user confirmation prompt.
type ConfirmationRequest struct {
	ToolName string
	Title    string
	Prompt   string
}

// Approver decides whether a tool call may proceed.
type Approver interface {
	Confirm(ctx context.Context, req ConfirmationRequest) (bool, error)
}
