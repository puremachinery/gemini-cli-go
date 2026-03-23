package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

func TestFileStoreSaveLoadListDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	tag := "my tag/with space"
	msgs := []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello"}}}}

	sess := &Session{
		ID:        tag,
		StartedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		Messages:  msgs,
	}
	if err := store.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}
	encoded := EncodeTagName(tag)
	path := filepath.Join(dir, checkpointPrefix+encoded+checkpointSuffix)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected session file: %v", err)
	}

	loaded, err := store.Load(tag)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != tag {
		t.Fatalf("expected tag %q, got %q", tag, loaded.ID)
	}
	if loaded.SchemaVersion == 0 {
		t.Fatalf("expected schema version to be set")
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Parts[0].Text != "hello" {
		t.Fatalf("unexpected messages: %#v", loaded.Messages)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != tag {
		t.Fatalf("unexpected list: %#v", list)
	}
	if !list[0].StartedAt.Equal(sess.StartedAt) {
		t.Fatalf("expected list StartedAt %s, got %s", sess.StartedAt, list[0].StartedAt)
	}

	if err := store.Delete(tag); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Load(tag); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFileStoreListPreviews(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	tag := "preview"
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Parts: []llm.Part{{Text: "system"}}},
		{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello"}, {Text: " world"}}},
		{Role: llm.RoleAssistant, Parts: []llm.Part{{Text: "ok"}}},
	}
	sess := &Session{
		ID:        tag,
		StartedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		Messages:  msgs,
	}
	if err := store.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	previews, err := store.ListPreviews()
	if err != nil {
		t.Fatalf("ListPreviews: %v", err)
	}
	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}
	preview := previews[0]
	if preview.Session.ID != tag {
		t.Fatalf("expected id %q, got %q", tag, preview.Session.ID)
	}
	if preview.MessageCount != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), preview.MessageCount)
	}
	if preview.PreviewText != "hello world" {
		t.Fatalf("expected preview text, got %q", preview.PreviewText)
	}
}

func TestFileStoreRejectsFutureSchema(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	tag := "future"
	path, err := store.pathFor(tag)
	if err != nil {
		t.Fatalf("pathFor: %v", err)
	}
	payload := []byte(`{"schemaVersion":99,"id":"future","startedAt":"2026-02-02T00:00:00Z","messages":[]}`)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := store.Load(tag); err == nil {
		t.Fatal("expected error for future schema")
	}
}
