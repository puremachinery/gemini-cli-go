package tools

import (
	"context"
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

type approvalStubClient struct{}

func (s *approvalStubClient) ChatStream(context.Context, llm.ChatRequest) (client.Stream, error) {
	return nil, nil
}

func (s *approvalStubClient) Chat(context.Context, llm.ChatRequest) (llm.ChatChunk, error) {
	return llm.ChatChunk{}, nil
}

func (s *approvalStubClient) CountTokens(context.Context, llm.CountTokensRequest) (llm.CountTokensResponse, error) {
	return llm.CountTokensResponse{}, nil
}

func TestNormalizeApprovalMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		want    ApprovalMode
		wantErr bool
	}{
		{name: "empty", input: "", want: ApprovalModeDefault},
		{name: "default", input: "default", want: ApprovalModeDefault},
		{name: "auto_edit", input: "auto_edit", want: ApprovalModeAutoEdit},
		{name: "yolo", input: "yolo", want: ApprovalModeYolo},
		{name: "plan", input: "plan", want: ApprovalModePlan},
		{name: "upper", input: "AUTO_EDIT", want: ApprovalModeAutoEdit},
		{name: "invalid", input: "nope", wantErr: true},
	}
	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeApprovalMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestFilterRegistryForApprovalMode(t *testing.T) {
	t.Parallel()
	t.Run("headless-default", func(t *testing.T) {
		reg := NewRegistry(Context{})
		FilterRegistryForApprovalMode(reg, ApprovalModeDefault, true)
		if _, ok := reg.Lookup(WriteFileToolName); ok {
			t.Fatal("expected write_file removed in headless default mode")
		}
		if _, ok := reg.Lookup(ReplaceToolName); ok {
			t.Fatal("expected replace removed in headless default mode")
		}
		if _, ok := reg.Lookup(ShellToolName); ok {
			t.Fatal("expected shell removed in headless default mode")
		}
		if _, ok := reg.Lookup(WebFetchToolName); ok {
			t.Fatal("expected web_fetch removed in headless default mode")
		}
	})
	t.Run("headless-auto-edit", func(t *testing.T) {
		reg := NewRegistry(Context{GeminiClient: &approvalStubClient{}})
		FilterRegistryForApprovalMode(reg, ApprovalModeAutoEdit, true)
		if _, ok := reg.Lookup(WriteFileToolName); !ok {
			t.Fatal("expected write_file kept in headless auto_edit mode")
		}
		if _, ok := reg.Lookup(ReplaceToolName); !ok {
			t.Fatal("expected replace kept in headless auto_edit mode")
		}
		if _, ok := reg.Lookup(ShellToolName); ok {
			t.Fatal("expected shell removed in headless auto_edit mode")
		}
		if _, ok := reg.Lookup(WebFetchToolName); !ok {
			t.Fatal("expected web_fetch kept in headless auto_edit mode")
		}
	})
	t.Run("headless-yolo", func(t *testing.T) {
		reg := NewRegistry(Context{})
		FilterRegistryForApprovalMode(reg, ApprovalModeYolo, true)
		if _, ok := reg.Lookup(WriteFileToolName); !ok {
			t.Fatal("expected write_file kept in headless yolo mode")
		}
		if _, ok := reg.Lookup(ReplaceToolName); !ok {
			t.Fatal("expected replace kept in headless yolo mode")
		}
		if _, ok := reg.Lookup(ShellToolName); !ok {
			t.Fatal("expected shell kept in headless yolo mode")
		}
	})
	t.Run("interactive-plan", func(t *testing.T) {
		reg := NewRegistry(Context{})
		FilterRegistryForApprovalMode(reg, ApprovalModePlan, false)
		if _, ok := reg.Lookup(WriteFileToolName); ok {
			t.Fatal("expected write_file removed in plan mode")
		}
		if _, ok := reg.Lookup(ReplaceToolName); ok {
			t.Fatal("expected replace removed in plan mode")
		}
		if _, ok := reg.Lookup(ShellToolName); ok {
			t.Fatal("expected shell removed in plan mode")
		}
	})
}

type stubApprover struct {
	called bool
	answer bool
}

func (s *stubApprover) Confirm(_ context.Context, _ ConfirmationRequest) (bool, error) {
	s.called = true
	return s.answer, nil
}

func TestModeApprover(t *testing.T) {
	t.Parallel()
	t.Run("auto-edit", func(t *testing.T) {
		prompt := &stubApprover{answer: false}
		approver := &ModeApprover{Mode: ApprovalModeAutoEdit, Prompt: prompt}
		ok, err := approver.Confirm(context.Background(), ConfirmationRequest{ToolName: WriteFileToolName})
		if err != nil || !ok {
			t.Fatalf("expected auto-approve for edit tool, got ok=%v err=%v", ok, err)
		}
		if prompt.called {
			t.Fatal("did not expect prompt for edit tool in auto_edit mode")
		}
		ok, err = approver.Confirm(context.Background(), ConfirmationRequest{ToolName: WebFetchToolName})
		if err != nil || !ok {
			t.Fatalf("expected auto-approve for web_fetch, got ok=%v err=%v", ok, err)
		}
		if prompt.called {
			t.Fatal("did not expect prompt for web_fetch in auto_edit mode")
		}
		ok, err = approver.Confirm(context.Background(), ConfirmationRequest{ToolName: ShellToolName})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatal("expected prompt result for shell tool in auto_edit mode")
		}
		if !prompt.called {
			t.Fatal("expected prompt to be called for shell tool in auto_edit mode")
		}
	})
	t.Run("yolo", func(t *testing.T) {
		prompt := &stubApprover{}
		approver := &ModeApprover{Mode: ApprovalModeYolo, Prompt: prompt}
		ok, err := approver.Confirm(context.Background(), ConfirmationRequest{ToolName: ShellToolName})
		if err != nil || !ok {
			t.Fatalf("expected yolo to auto-approve, got ok=%v err=%v", ok, err)
		}
		if prompt.called {
			t.Fatal("did not expect prompt in yolo mode")
		}
	})
	t.Run("plan", func(t *testing.T) {
		approver := &ModeApprover{Mode: ApprovalModePlan}
		ok, err := approver.Confirm(context.Background(), ConfirmationRequest{ToolName: WriteFileToolName})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatal("expected plan mode to deny approvals")
		}
	})
}
