package config

import "testing"

func TestResolveModel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		preview  bool
		expected string
	}{
		{name: "preview auto", input: PreviewGeminiModelAuto, expected: PreviewGeminiModel},
		{name: "default auto", input: DefaultGeminiModelAuto, expected: DefaultGeminiModel},
		{name: "alias auto", input: GeminiModelAliasAuto, expected: DefaultGeminiModel},
		{name: "alias auto preview", input: GeminiModelAliasAuto, preview: true, expected: PreviewGeminiModel},
		{name: "alias pro", input: GeminiModelAliasPro, expected: DefaultGeminiModel},
		{name: "alias pro preview", input: GeminiModelAliasPro, preview: true, expected: PreviewGeminiModel},
		{name: "alias flash", input: GeminiModelAliasFlash, expected: DefaultGeminiFlashModel},
		{name: "alias flash preview", input: GeminiModelAliasFlash, preview: true, expected: PreviewGeminiFlashModel},
		{name: "alias flash lite", input: GeminiModelAliasFlashLite, expected: DefaultGeminiFlashLite},
		{name: "concrete model passthrough", input: "gemini-2.5-pro", expected: "gemini-2.5-pro"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveModel(tt.input, tt.preview)
			if got != tt.expected {
				t.Fatalf("ResolveModel(%q, %v) = %q; want %q", tt.input, tt.preview, got, tt.expected)
			}
		})
	}
}
