// Package llmfmt formats tool calls and responses for display.
package llmfmt

import (
	"encoding/json"
	"fmt"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

// FormatFunctionCall renders a tool call for display or export.
func FormatFunctionCall(call *llm.FunctionCall) string {
	return formatToolCall(call, "[tool call]")
}

// FormatFunctionCallInline renders a tool call without a label prefix.
func FormatFunctionCallInline(call *llm.FunctionCall) string {
	return formatToolCall(call, "")
}

// FormatFunctionResponse renders a tool response for display or export.
func FormatFunctionResponse(resp *llm.FunctionResponse) string {
	return formatToolResponse(resp, "[tool response]")
}

// FormatFunctionResponseInline renders a tool response without a label prefix.
func FormatFunctionResponseInline(resp *llm.FunctionResponse) string {
	return formatToolResponse(resp, "")
}

func formatToolCall(call *llm.FunctionCall, label string) string {
	if call == nil {
		return ""
	}
	args := ""
	if call.Args != nil {
		if data, err := json.MarshalIndent(call.Args, "", "  "); err == nil {
			args = string(data)
		}
	}
	if label != "" {
		label += " "
	}
	if args == "" {
		return fmt.Sprintf("%s%s", label, call.Name)
	}
	return fmt.Sprintf("%s%s\n%s", label, call.Name, args)
}

func formatToolResponse(resp *llm.FunctionResponse, label string) string {
	if resp == nil {
		return ""
	}
	body := ""
	if resp.Response != nil {
		if data, err := json.MarshalIndent(resp.Response, "", "  "); err == nil {
			body = string(data)
		}
	}
	if label != "" {
		label += " "
	}
	if body == "" {
		return fmt.Sprintf("%s%s", label, resp.Name)
	}
	return fmt.Sprintf("%s%s\n%s", label, resp.Name, body)
}
