package ui

import (
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/memory"
)

// withMemoryMessagesInto prepends memory as a system message using a reusable buffer.
func withMemoryMessagesInto(dst []llm.Message, messages []llm.Message, state *memory.State) ([]llm.Message, bool) {
	if state == nil {
		return messages, false
	}
	content := strings.TrimSpace(state.Content)
	if content == "" {
		return messages, false
	}
	system := llm.Message{
		Role:  llm.RoleSystem,
		Parts: []llm.Part{{Text: content}},
	}
	out := dst
	if out == nil {
		out = make([]llm.Message, 0, len(messages)+1)
	}
	out = append(out, system)
	out = append(out, messages...)
	return out, true
}
