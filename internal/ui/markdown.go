package ui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

const markdownFlushThreshold = 2048

type markdownRenderer interface {
	Render(string) (string, error)
}

func newMarkdownRenderer(width int) (markdownRenderer, error) {
	opts := []glamour.TermRendererOption{
		glamour.WithAutoStyle(),
	}
	if width > 0 {
		opts = append(opts, glamour.WithWordWrap(width))
	}
	return glamour.NewTermRenderer(opts...)
}

func renderMarkdown(renderer markdownRenderer, content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return content, nil
	}
	if renderer == nil {
		return content, nil
	}
	return renderer.Render(content)
}

// splitMarkdownForRender splits content at a safe point to preserve Markdown structure.
func splitMarkdownForRender(content string, maxPending int) (string, string) {
	if content == "" {
		return "", ""
	}
	split := findLastSafeSplitPoint(content)
	if split == len(content) {
		split = findLastLineBreakOutsideCodeBlock(content)
	}
	if split <= 0 || split >= len(content) {
		if split >= len(content) && strings.HasSuffix(content, "\n") && !isIndexInsideCodeBlock(content, len(content)) {
			return content, ""
		}
		if maxPending > 0 && len(content) >= maxPending && !isIndexInsideCodeBlock(content, len(content)) {
			return content, ""
		}
		return "", content
	}
	return content[:split], content[split:]
}

// findLastSafeSplitPoint locates the last safe split before a code block.
func findLastSafeSplitPoint(content string) int {
	if content == "" {
		return 0
	}
	enclosingBlockStart := findEnclosingCodeBlockStart(content, len(content))
	if enclosingBlockStart != -1 {
		return enclosingBlockStart
	}
	searchStart := len(content)
	for searchStart > 0 {
		index := strings.LastIndex(content[:searchStart], "\n\n")
		if index == -1 {
			break
		}
		split := index + 2
		if !isIndexInsideCodeBlock(content, split) {
			return split
		}
		searchStart = index
	}
	return len(content)
}

// findLastLineBreakOutsideCodeBlock returns the last newline outside fenced blocks.
func findLastLineBreakOutsideCodeBlock(content string) int {
	if content == "" {
		return 0
	}
	searchStart := len(content)
	for searchStart > 0 {
		index := strings.LastIndex(content[:searchStart], "\n")
		if index == -1 {
			break
		}
		split := index + 1
		if !isIndexInsideCodeBlock(content, split) {
			return split
		}
		searchStart = index
	}
	return 0
}

// isIndexInsideCodeBlock reports whether index is inside a fenced code block.
func isIndexInsideCodeBlock(content string, index int) bool {
	fenceCount := 0
	searchPos := 0
	for searchPos < len(content) {
		nextFence := strings.Index(content[searchPos:], "```")
		if nextFence == -1 {
			break
		}
		nextFence += searchPos
		if nextFence >= index {
			break
		}
		fenceCount++
		searchPos = nextFence + 3
	}
	return fenceCount%2 == 1
}

// findEnclosingCodeBlockStart returns the start index of the enclosing fenced block.
func findEnclosingCodeBlockStart(content string, index int) int {
	if !isIndexInsideCodeBlock(content, index) {
		return -1
	}
	searchPos := 0
	for searchPos < index {
		blockStart := strings.Index(content[searchPos:], "```")
		if blockStart == -1 {
			break
		}
		blockStart += searchPos
		if blockStart >= index {
			break
		}
		blockEnd := strings.Index(content[blockStart+3:], "```")
		if blockEnd == -1 {
			return blockStart
		}
		blockEnd += blockStart + 3
		if index < blockEnd+3 {
			return blockStart
		}
		searchPos = blockEnd + 3
	}
	return -1
}
