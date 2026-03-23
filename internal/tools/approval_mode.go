package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ApprovalMode controls tool approval behavior.
type ApprovalMode string

// ApprovalModeDefault is the standard prompt-based approval mode.
const (
	ApprovalModeDefault  ApprovalMode = "default"
	ApprovalModeAutoEdit ApprovalMode = "auto_edit"
	ApprovalModeYolo     ApprovalMode = "yolo"
	ApprovalModePlan     ApprovalMode = "plan"
)

// NormalizeApprovalMode validates and normalizes an approval mode string.
func NormalizeApprovalMode(raw string) (ApprovalMode, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "", string(ApprovalModeDefault):
		return ApprovalModeDefault, nil
	case string(ApprovalModeAutoEdit):
		return ApprovalModeAutoEdit, nil
	case string(ApprovalModeYolo):
		return ApprovalModeYolo, nil
	case string(ApprovalModePlan):
		return ApprovalModePlan, nil
	default:
		return "", fmt.Errorf("invalid approval mode: %s", raw)
	}
}

// IsEditTool reports whether the tool mutates files.
func IsEditTool(name string) bool {
	return name == WriteFileToolName || name == ReplaceToolName || name == SaveMemoryToolName
}

// IsShellTool reports whether the tool executes shell commands.
func IsShellTool(name string) bool {
	return name == ShellToolName
}

// FilterRegistryForApprovalMode removes tools that are not allowed for the mode.
func FilterRegistryForApprovalMode(registry *Registry, mode ApprovalMode, headless bool) {
	if registry == nil {
		return
	}
	removeInteractive := func() {
		registry.Remove(WriteFileToolName)
		registry.Remove(ReplaceToolName)
		registry.Remove(ShellToolName)
		registry.Remove(WebFetchToolName)
	}
	if headless && registry.ctx.RequireReadApproval && mode != ApprovalModeYolo {
		registry.Remove(ReadFileToolName)
		registry.Remove(ReadManyFilesToolName)
	}
	if mode == ApprovalModePlan {
		removeInteractive()
		return
	}
	if !headless {
		return
	}
	switch mode {
	case ApprovalModeYolo:
		return
	case ApprovalModeAutoEdit:
		registry.Remove(ShellToolName)
	default:
		removeInteractive()
	}
}

// ModeApprover auto-approves tool calls based on the configured mode.
type ModeApprover struct {
	Mode   ApprovalMode
	Prompt Approver
}

// NewModeApprover returns an approver that honors the approval mode.
func NewModeApprover(mode ApprovalMode, prompt Approver) Approver {
	switch mode {
	case ApprovalModeAutoEdit, ApprovalModeYolo, ApprovalModePlan:
		return &ModeApprover{Mode: mode, Prompt: prompt}
	default:
		if prompt != nil {
			return prompt
		}
		return &ModeApprover{Mode: ApprovalModeDefault}
	}
}

// Confirm implements Approver.
func (a *ModeApprover) Confirm(ctx context.Context, req ConfirmationRequest) (bool, error) {
	switch a.Mode {
	case ApprovalModeYolo:
		return true, nil
	case ApprovalModeAutoEdit:
		if IsEditTool(req.ToolName) || req.ToolName == WebFetchToolName {
			return true, nil
		}
	case ApprovalModePlan:
		return false, nil
	}
	if a.Prompt == nil {
		return false, errors.New("tool approval required but no approver configured")
	}
	return a.Prompt.Confirm(ctx, req)
}
