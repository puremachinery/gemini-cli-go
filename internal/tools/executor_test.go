package tools

import (
	"context"
	"errors"
	"testing"
)

type stubTool struct {
	name        string
	buildErr    error
	invocation  Invocation
	description string
}

func (t stubTool) Name() string { return t.name }
func (t stubTool) Description() string {
	if t.description != "" {
		return t.description
	}
	return t.name
}
func (t stubTool) Parameters() map[string]any { return map[string]any{"type": "object"} }
func (t stubTool) Build(map[string]any) (Invocation, error) {
	if t.buildErr != nil {
		return nil, t.buildErr
	}
	return t.invocation, nil
}

type stubInvocation struct {
	request *ConfirmationRequest
	result  Result
	err     error
}

func (i stubInvocation) Name() string                              { return "stub" }
func (i stubInvocation) Description() string                       { return "stub" }
func (i stubInvocation) ConfirmationRequest() *ConfirmationRequest { return i.request }
func (i stubInvocation) Execute(context.Context) (Result, error)   { return i.result, i.err }

type execApprover struct {
	approved bool
	err      error
}

func (a *execApprover) Confirm(context.Context, ConfirmationRequest) (bool, error) {
	return a.approved, a.err
}

func TestExecutorUnknownTool(t *testing.T) {
	exec := &Executor{Registry: NewRegistry(Context{})}
	res := exec.Execute(context.Background(), ToolCall{Name: "missing"})
	if res.Error == "" {
		t.Fatal("expected error for unknown tool")
	}
}

func TestExecutorBuildError(t *testing.T) {
	reg := &Registry{tools: map[string]Tool{}}
	reg.Register(stubTool{name: "stub", buildErr: errors.New("bad args")})
	exec := &Executor{Registry: reg}
	res := exec.Execute(context.Background(), ToolCall{Name: "stub"})
	if res.Error != "bad args" {
		t.Fatalf("expected build error, got %q", res.Error)
	}
}

func TestExecutorRequiresApproval(t *testing.T) {
	inv := stubInvocation{
		request: &ConfirmationRequest{ToolName: "stub"},
		result:  Result{Output: "ok"},
	}
	reg := &Registry{tools: map[string]Tool{}}
	reg.Register(stubTool{name: "stub", invocation: inv})
	exec := &Executor{Registry: reg}
	res := exec.Execute(context.Background(), ToolCall{Name: "stub"})
	if res.Error == "" {
		t.Fatal("expected error when approver missing")
	}
}

func TestExecutorApproverRejects(t *testing.T) {
	inv := stubInvocation{
		request: &ConfirmationRequest{ToolName: "stub"},
		result:  Result{Output: "ok"},
	}
	reg := &Registry{tools: map[string]Tool{}}
	reg.Register(stubTool{name: "stub", invocation: inv})
	exec := &Executor{Registry: reg, Approver: &execApprover{approved: false}}
	res := exec.Execute(context.Background(), ToolCall{Name: "stub"})
	if res.Error != "tool execution canceled by user" {
		t.Fatalf("expected canceled error, got %q", res.Error)
	}
}

func TestExecutorApproverAllows(t *testing.T) {
	inv := stubInvocation{
		request: &ConfirmationRequest{ToolName: "stub"},
		result:  Result{Output: "ok"},
	}
	reg := &Registry{tools: map[string]Tool{}}
	reg.Register(stubTool{name: "stub", invocation: inv})
	exec := &Executor{Registry: reg, Approver: &execApprover{approved: true}}
	res := exec.Execute(context.Background(), ToolCall{Name: "stub"})
	if res.Error != "" || res.Output != "ok" {
		t.Fatalf("unexpected result: %#v", res)
	}
}
