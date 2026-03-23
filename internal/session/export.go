package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/llmfmt"
)

// ExportHistoryToFile writes messages to a JSON or Markdown file.
func ExportHistoryToFile(messages []llm.Message, filePath string) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".json":
		return exportJSON(messages, filePath)
	case ".md":
		return exportMarkdown(messages, filePath)
	default:
		return fmt.Errorf("invalid file format. Only .md and .json are supported")
	}
}

func exportJSON(messages []llm.Message, filePath string) error {
	payload := struct {
		ExportedAt time.Time     `json:"exportedAt"`
		Messages   []llm.Message `json:"messages"`
	}{
		ExportedAt: time.Now(),
		Messages:   messages,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0o600)
}

func exportMarkdown(messages []llm.Message, filePath string) error {
	var b strings.Builder
	for _, msg := range messages {
		role := "Assistant"
		if msg.Role == llm.RoleUser {
			role = "User"
		}
		content := messageContent(msg)
		if strings.TrimSpace(content) == "" {
			continue
		}
		b.WriteString("### ")
		b.WriteString(role)
		b.WriteString("\n\n")
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	return os.WriteFile(filePath, []byte(strings.TrimRight(b.String(), "\n")+"\n"), 0o600)
}

func messageContent(msg llm.Message) string {
	parts := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		switch {
		case part.Text != "":
			parts = append(parts, part.Text)
		case part.FunctionCall != nil:
			parts = append(parts, llmfmt.FormatFunctionCall(part.FunctionCall))
		case part.FunctionResponse != nil:
			parts = append(parts, llmfmt.FormatFunctionResponse(part.FunctionResponse))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
