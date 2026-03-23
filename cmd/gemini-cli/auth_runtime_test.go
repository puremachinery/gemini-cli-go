package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/auth"
	"github.com/puremachinery/gemini-cli-go/internal/authselect"
)

func TestResolveAuthType(t *testing.T) {
	tests := []struct {
		name         string
		state        authState
		promptResult string
		want         string
		wantPrompt   bool
	}{
		{
			name: "uses selected api key without prompt",
			state: authState{
				selectedType: authselect.AuthTypeAPIKey,
				hasAPIKey:    true,
			},
			promptResult: authselect.AuthTypeOAuthPersonal,
			want:         authselect.AuthTypeAPIKey,
		},
		{
			name: "uses cached oauth without prompt",
			state: authState{
				cachedCreds: &auth.Credentials{AccessToken: "token"},
			},
			promptResult: authselect.AuthTypeAPIKey,
			want:         authselect.AuthTypeOAuthPersonal,
		},
		{
			name: "prompts when api key selected but unavailable",
			state: authState{
				selectedType: authselect.AuthTypeAPIKey,
			},
			promptResult: authselect.AuthTypeOAuthPersonal,
			want:         authselect.AuthTypeOAuthPersonal,
			wantPrompt:   true,
		},
		{
			name: "prefers api key over cached oauth when unselected",
			state: authState{
				hasAPIKey:   true,
				cachedCreds: &auth.Credentials{AccessToken: "token"},
			},
			promptResult: authselect.AuthTypeOAuthPersonal,
			want:         authselect.AuthTypeAPIKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			authType, err := resolveAuthType(context.Background(), tt.state, buildRunBundleOptions{
				allowLogin: true,
				promptForAuthChoice: func(context.Context, authselect.PromptState) (string, error) {
					called = true
					return tt.promptResult, nil
				},
			})
			if err != nil {
				t.Fatalf("resolveAuthType: %v", err)
			}
			if called != tt.wantPrompt {
				t.Fatalf("prompt called=%v, want %v", called, tt.wantPrompt)
			}
			if authType != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, authType)
			}
		})
	}
}

func TestResolveAuthTypeHeadlessRequiresConfiguredAuth(t *testing.T) {
	_, err := resolveAuthType(context.Background(), authState{}, buildRunBundleOptions{})
	if err == nil {
		t.Fatal("expected error when no auth is configured for headless mode")
	}
	if !strings.Contains(err.Error(), "start `gemini-cli` interactively") {
		t.Fatalf("expected interactive guidance, got %v", err)
	}
}

func TestPromptForAuthChoiceRepromptsWhenAPIKeyUnavailable(t *testing.T) {
	var out bytes.Buffer
	choice, err := promptForAuthChoice(context.Background(), strings.NewReader("2\n1\n"), &out, authselect.PromptState{})
	if err != nil {
		t.Fatalf("promptForAuthChoice: %v", err)
	}
	if choice != authselect.AuthTypeOAuthPersonal {
		t.Fatalf("expected oauth-personal, got %q", choice)
	}
	if !strings.Contains(out.String(), "GEMINI_API_KEY is not set") {
		t.Fatalf("expected missing API key message, got %q", out.String())
	}
}

func TestResolveAuthTypeForcedAPIKeyRequiresConfiguredKey(t *testing.T) {
	_, err := resolveAuthType(context.Background(), authState{}, buildRunBundleOptions{
		forcedAuthType: authselect.AuthTypeAPIKey,
	})
	if !errors.Is(err, errAPIKeyNotConfigured) {
		t.Fatalf("expected errAPIKeyNotConfigured, got %v", err)
	}
}
