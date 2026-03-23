package client

import "testing"

func TestNormalizeModel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"models/gemini-2.5-pro", "gemini-2.5-pro"},
		{" gemini-2.5-pro ", "gemini-2.5-pro"},
		{"gemini-2.5-pro", "gemini-2.5-pro"},
	}
	for _, tc := range cases {
		if got := normalizeModel(tc.input); got != tc.want {
			t.Fatalf("normalizeModel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
