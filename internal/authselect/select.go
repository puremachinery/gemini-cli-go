package authselect

import (
	"fmt"
	"strings"
)

const (
	AuthTypeOAuthPersonal = "oauth-personal"
	AuthTypeAPIKey        = "api-key"
)

// PromptState captures the auth methods currently available to the user.
type PromptState struct {
	SelectedType string
	HasAPIKey    bool
}

// NormalizeAuthType canonicalizes supported auth type identifiers.
func NormalizeAuthType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case AuthTypeOAuthPersonal:
		return AuthTypeOAuthPersonal
	case AuthTypeAPIKey:
		return AuthTypeAPIKey
	default:
		return ""
	}
}

// DefaultAuthType returns the default auth choice for the current prompt state.
func DefaultAuthType(state PromptState) string {
	if selected := NormalizeAuthType(state.SelectedType); selected != "" {
		return selected
	}
	if state.HasAPIKey {
		return AuthTypeAPIKey
	}
	return AuthTypeOAuthPersonal
}

// DefaultOptionNumber reports which menu option should be selected by default.
func DefaultOptionNumber(state PromptState) int {
	if DefaultAuthType(state) == AuthTypeAPIKey {
		return 2
	}
	return 1
}

// PromptText renders the interactive auth selection menu.
func PromptText(state PromptState) string {
	apiKeyLabel := "Use Gemini API Key"
	if !state.HasAPIKey {
		apiKeyLabel += " (requires GEMINI_API_KEY)"
	}
	return fmt.Sprintf(
		"Select authentication method:\n  1. Sign in with Google\n  2. %s\n",
		apiKeyLabel,
	)
}

// ParseChoice resolves a user-entered auth selection.
func ParseChoice(input string, state PromptState) (string, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	if trimmed == "" {
		return DefaultAuthType(state), true
	}
	switch trimmed {
	case "1", "google", "g", "oauth", "oauth-personal":
		return AuthTypeOAuthPersonal, true
	case "2", "api", "key", "api-key":
		return AuthTypeAPIKey, true
	default:
		return "", false
	}
}
