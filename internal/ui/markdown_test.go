package ui

import (
	"strings"
	"testing"
)

func TestRenderMarkdownIncludesCode(t *testing.T) {
	renderer, err := newMarkdownRenderer(80)
	if err != nil {
		t.Fatalf("newMarkdownRenderer: %v", err)
	}
	rendered, err := renderMarkdown(renderer, "```go\nfmt.Println(\"hi\")\n```\n")
	if err != nil {
		t.Fatalf("renderMarkdown: %v", err)
	}
	if !strings.Contains(rendered, "fmt.Println") {
		t.Fatalf("expected rendered output to include code, got %q", rendered)
	}
}

func TestFindLastSafeSplitPoint(t *testing.T) {
	content := "paragraph1\n\nparagraph2\n\nparagraph3"
	if got := findLastSafeSplitPoint(content); got != 24 {
		t.Fatalf("expected split at 24, got %d", got)
	}
	content = "longstringwithoutanysafesplitpoint"
	if got := findLastSafeSplitPoint(content); got != len(content) {
		t.Fatalf("expected split at end, got %d", got)
	}
	content = "```\nignore this\n\nnewline\n```KeepThis"
	if got := findLastSafeSplitPoint(content); got != len(content) {
		t.Fatalf("expected split at end, got %d", got)
	}
	content = "text\n```go\ncode"
	if got := findLastSafeSplitPoint(content); got != 5 {
		t.Fatalf("expected split at 5, got %d", got)
	}
	if got := findLastSafeSplitPoint(""); got != 0 {
		t.Fatalf("expected split at 0, got %d", got)
	}
}

func TestSplitMarkdownForRender(t *testing.T) {
	content := "first\nsecond"
	flush, remaining := splitMarkdownForRender(content, 100)
	if flush != "first\n" || remaining != "second" {
		t.Fatalf("unexpected split: %q | %q", flush, remaining)
	}

	content = "text\n```go\ncode"
	flush, remaining = splitMarkdownForRender(content, 100)
	if flush != "text\n" || remaining != "```go\ncode" {
		t.Fatalf("unexpected code split: %q | %q", flush, remaining)
	}

	content = "abcdefgh"
	flush, remaining = splitMarkdownForRender(content, 4)
	if flush != content || remaining != "" {
		t.Fatalf("unexpected threshold split: %q | %q", flush, remaining)
	}

	content = "short"
	flush, remaining = splitMarkdownForRender(content, 10)
	if flush != "" || remaining != content {
		t.Fatalf("unexpected short split: %q | %q", flush, remaining)
	}

	content = "line\n"
	flush, remaining = splitMarkdownForRender(content, 100)
	if flush != content || remaining != "" {
		t.Fatalf("unexpected newline split: %q | %q", flush, remaining)
	}
}
