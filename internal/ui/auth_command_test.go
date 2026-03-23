package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/authselect"
	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

func TestRunAuthCommandSwitchesAuthAndAutosave(t *testing.T) {
	store := newMemoryChatStore()
	input := bytes.NewBufferString("/auth\n1\nhello\n/quit\n")
	var output bytes.Buffer
	initialClient := &fakeClient{t: t}
	activatedClient := &fakeClient{t: t, streams: []client.Stream{
		&fakeStream{chunks: []llm.ChatChunk{{Text: "ok", Done: true}}},
	}}
	var activated string

	if err := Run(context.Background(), RunOptions{
		Client:    initialClient,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
		ChatStore: store,
		AuthType:  authselect.AuthTypeAPIKey,
		AuthManager: &AuthManager{
			GetPromptState: func(context.Context) (AuthPromptState, error) {
				return AuthPromptState{}, nil
			},
			Activate: func(_ context.Context, authType string) (AuthBundle, error) {
				activated = authType
				return AuthBundle{
					Client:   activatedClient,
					AuthType: authselect.AuthTypeOAuthPersonal,
				}, nil
			},
			Clear: func(context.Context) error { return nil },
		},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if activated != authselect.AuthTypeOAuthPersonal {
		t.Fatalf("expected oauth-personal activation, got %q", activated)
	}
	if len(activatedClient.reqs) != 1 {
		t.Fatalf("expected activated client to handle the chat request, got %d requests", len(activatedClient.reqs))
	}
	if len(store.sessions) != 1 {
		t.Fatalf("expected one auto-saved session, got %d", len(store.sessions))
	}
	for _, sess := range store.sessions {
		if sess.AuthType != authselect.AuthTypeOAuthPersonal {
			t.Fatalf("expected auto-save auth type oauth-personal, got %q", sess.AuthType)
		}
	}
	if !strings.Contains(output.String(), "Authentication set to Google sign-in.") {
		t.Fatalf("expected auth confirmation in output, got %q", output.String())
	}
}

func TestRunAuthCommandRepromptsWhenAPIKeyUnavailable(t *testing.T) {
	input := bytes.NewBufferString("/auth\n2\n1\n/quit\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}
	var activated string

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
		AuthType:  authselect.AuthTypeOAuthPersonal,
		AuthManager: &AuthManager{
			GetPromptState: func(context.Context) (AuthPromptState, error) {
				return AuthPromptState{}, nil
			},
			Activate: func(_ context.Context, authType string) (AuthBundle, error) {
				activated = authType
				return AuthBundle{
					Client:   client,
					AuthType: authselect.AuthTypeOAuthPersonal,
				}, nil
			},
			Clear: func(context.Context) error { return nil },
		},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if activated != authselect.AuthTypeOAuthPersonal {
		t.Fatalf("expected oauth-personal activation after re-prompt, got %q", activated)
	}
	if !strings.Contains(output.String(), "GEMINI_API_KEY is not set") {
		t.Fatalf("expected missing API key warning, got %q", output.String())
	}
}

func TestRunAuthLogoutClearsAndQuits(t *testing.T) {
	input := bytes.NewBufferString("/auth logout\nn\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}
	clearCalls := 0

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
		AuthType:  authselect.AuthTypeOAuthPersonal,
		AuthManager: &AuthManager{
			GetPromptState: func(context.Context) (AuthPromptState, error) {
				return AuthPromptState{}, nil
			},
			Activate: func(context.Context, string) (AuthBundle, error) {
				t.Fatal("Activate should not be called when declining re-login")
				return AuthBundle{}, nil
			},
			Clear: func(context.Context) error {
				clearCalls++
				return nil
			},
		},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if clearCalls != 1 {
		t.Fatalf("expected Clear to be called once, got %d", clearCalls)
	}
	if !strings.Contains(output.String(), "Signed out and cleared cached credentials.") {
		t.Fatalf("expected sign-out message, got %q", output.String())
	}
	if !strings.Contains(output.String(), quitMessage) {
		t.Fatalf("expected quit message, got %q", output.String())
	}
}
