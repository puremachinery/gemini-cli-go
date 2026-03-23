package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

func TestExportHistoryToFileJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "history.json")
	messages := []llm.Message{{
		Role:  llm.RoleUser,
		Parts: []llm.Part{{Text: "hello"}},
	}}
	if err := ExportHistoryToFile(messages, path); err != nil {
		t.Fatalf("ExportHistoryToFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var payload struct {
		ExportedAt time.Time     `json:"exportedAt"`
		Messages   []llm.Message `json:"messages"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.ExportedAt.IsZero() {
		t.Fatal("expected exportedAt timestamp")
	}
	if len(payload.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(payload.Messages))
	}
	if payload.Messages[0].Role != llm.RoleUser {
		t.Fatalf("unexpected role: %v", payload.Messages[0].Role)
	}
}

func TestExportHistoryToFileMarkdown(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "history.md")
	messages := []llm.Message{{
		Role:  llm.RoleUser,
		Parts: []llm.Part{{Text: "hello"}},
	}}
	if err := ExportHistoryToFile(messages, path); err != nil {
		t.Fatalf("ExportHistoryToFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "### User") {
		t.Fatalf("expected user header, got: %q", text)
	}
	if !strings.Contains(text, "hello") {
		t.Fatalf("expected message content, got: %q", text)
	}
}

func TestExportHistoryToFileRejectsUnknownExtension(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "history.txt")
	if err := ExportHistoryToFile(nil, path); err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}
