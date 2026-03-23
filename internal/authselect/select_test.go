package authselect

import "testing"

func TestDefaultAuthType(t *testing.T) {
	tests := []struct {
		name  string
		state PromptState
		want  string
	}{
		{
			name:  "selected oauth wins",
			state: PromptState{SelectedType: AuthTypeOAuthPersonal, HasAPIKey: true},
			want:  AuthTypeOAuthPersonal,
		},
		{
			name:  "selected api key wins",
			state: PromptState{SelectedType: AuthTypeAPIKey},
			want:  AuthTypeAPIKey,
		},
		{
			name:  "api key defaults when available",
			state: PromptState{HasAPIKey: true},
			want:  AuthTypeAPIKey,
		},
		{
			name:  "oauth defaults otherwise",
			state: PromptState{},
			want:  AuthTypeOAuthPersonal,
		},
	}

	for _, tt := range tests {
		if got := DefaultAuthType(tt.state); got != tt.want {
			t.Fatalf("%s: expected %q, got %q", tt.name, tt.want, got)
		}
	}
}

func TestParseChoice(t *testing.T) {
	state := PromptState{HasAPIKey: true}
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "", want: AuthTypeAPIKey, ok: true},
		{input: "1", want: AuthTypeOAuthPersonal, ok: true},
		{input: "google", want: AuthTypeOAuthPersonal, ok: true},
		{input: "2", want: AuthTypeAPIKey, ok: true},
		{input: "api-key", want: AuthTypeAPIKey, ok: true},
		{input: "wat", ok: false},
	}

	for _, tt := range tests {
		got, ok := ParseChoice(tt.input, state)
		if ok != tt.ok {
			t.Fatalf("ParseChoice(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if got != tt.want {
			t.Fatalf("ParseChoice(%q)=%q, want %q", tt.input, got, tt.want)
		}
	}
}
