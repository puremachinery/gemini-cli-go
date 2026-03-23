package ui

import "github.com/puremachinery/gemini-cli-go/internal/llm"

func pruneMessages(messages *[]llm.Message, maxCount int) int {
	if messages == nil || maxCount <= 0 {
		return 0
	}
	current := *messages
	if len(current) <= maxCount {
		return 0
	}
	systemCount := 0
	for systemCount < len(current) && current[systemCount].Role == llm.RoleSystem {
		systemCount++
	}
	if systemCount >= maxCount {
		pruned := append([]llm.Message(nil), current[:maxCount]...)
		*messages = pruned
		return len(current) - len(pruned)
	}
	keep := maxCount - systemCount
	start := len(current) - keep
	if start < systemCount {
		start = systemCount
	}
	pruned := append([]llm.Message(nil), current[:systemCount]...)
	pruned = append(pruned, current[start:]...)
	*messages = pruned
	return len(current) - len(pruned)
}
