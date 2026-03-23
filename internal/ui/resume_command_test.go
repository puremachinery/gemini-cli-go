package ui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/session"
)

type flushWriter struct {
	buf     []byte
	flushed bool
}

func (w *flushWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *flushWriter) Flush() error {
	w.flushed = true
	return nil
}

type checkingReader struct {
	writer *flushWriter
	called bool
}

func (r *checkingReader) ReadLine(_ context.Context, _ string) (string, bool, error) {
	r.called = true
	if !r.writer.flushed {
		return "", false, errors.New("resume list not flushed")
	}
	return "1", false, nil
}

func (r *checkingReader) SaveHistory(string) error {
	return nil
}

func (r *checkingReader) Close() error {
	return nil
}

func TestHandleResumeCommandFlushesListBeforePrompt(t *testing.T) {
	store := newMemoryChatStore()
	if err := store.Save(&session.Session{
		ID:        "session-123",
		StartedAt: time.Date(2026, 2, 1, 2, 3, 4, 0, time.UTC),
		Messages:  []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "prior"}}}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	writer := &flushWriter{}
	reader := &checkingReader{writer: writer}
	var messages []llm.Message

	if err := handleResumeCommand(context.Background(), reader, writer, "/resume", &messages, store, ""); err != nil {
		t.Fatalf("handleResumeCommand: %v", err)
	}
	if !reader.called {
		t.Fatalf("expected reader to be called")
	}
	if !writer.flushed {
		t.Fatalf("expected writer to be flushed")
	}
	if !strings.Contains(string(writer.buf), "[1 msg] - prior") {
		t.Fatalf("expected preview output, got: %q", string(writer.buf))
	}
	if !strings.Contains(string(writer.buf), "Conversation replay") {
		t.Fatalf("expected replay output, got: %q", string(writer.buf))
	}
	if len(messages) == 0 {
		t.Fatalf("expected messages to be loaded")
	}
	if messages[0].Parts[0].Text != "prior" {
		t.Fatalf("expected resumed message, got %q", messages[0].Parts[0].Text)
	}
	if len(writer.buf) == 0 {
		t.Fatalf("expected output to be written")
	}
}

func TestHandleResumeCommandCancelOnPromptInterrupt(t *testing.T) {
	store := newMemoryChatStore()
	if err := store.Save(&session.Session{
		ID:        "session-123",
		StartedAt: time.Date(2026, 2, 1, 2, 3, 4, 0, time.UTC),
		Messages:  []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "prior"}}}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	writer := &flushWriter{}
	reader := &errorReader{err: errPromptInterrupted}
	var messages []llm.Message

	if err := handleResumeCommand(context.Background(), reader, writer, "/resume", &messages, store, ""); err != nil {
		t.Fatalf("handleResumeCommand: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected messages to remain empty")
	}
}

func TestHandleResumeCommandPreviewCleansCommands(t *testing.T) {
	store := newMemoryChatStore()
	if err := store.Save(&session.Session{
		ID:        "session-999",
		StartedAt: time.Date(2026, 2, 1, 2, 3, 7, 0, time.UTC),
		Messages: []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{{Text: "/chat save test"}}},
			{Role: llm.RoleUser, Parts: []llm.Part{{Text: "?help"}}},
			{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello\u0007\nworld"}}},
		},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	writer := &flushWriter{}
	reader := &checkingReader{writer: writer}
	var messages []llm.Message

	if err := handleResumeCommand(context.Background(), reader, writer, "/resume", &messages, store, ""); err != nil {
		t.Fatalf("handleResumeCommand: %v", err)
	}
	if !strings.Contains(string(writer.buf), "hello world") {
		t.Fatalf("expected cleaned preview, got: %q", string(writer.buf))
	}
}

func TestCleanPreviewTextPreservesUnicode(t *testing.T) {
	input := "こんにちは 世界"
	if got := cleanPreviewText(input); got != input {
		t.Fatalf("expected unicode preserved, got: %q", got)
	}
}

func TestHandleResumeCommandLatestSelection(t *testing.T) {
	store := newMemoryChatStore()
	if err := store.Save(&session.Session{
		ID:        "session-100",
		StartedAt: time.Date(2026, 2, 1, 2, 3, 4, 0, time.UTC),
		Messages:  []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "older"}}}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Save(&session.Session{
		ID:        "session-200",
		StartedAt: time.Date(2026, 2, 1, 2, 3, 5, 0, time.UTC),
		Messages:  []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "newer"}}}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var messages []llm.Message
	writer := &flushWriter{}
	if err := handleResumeCommand(context.Background(), nil, writer, "/resume latest", &messages, store, ""); err != nil {
		t.Fatalf("handleResumeCommand: %v", err)
	}
	if len(messages) == 0 || messages[0].Parts[0].Text != "newer" {
		t.Fatalf("expected latest session to load, got %+v", messages)
	}
}

func TestHandleResumeCommandIndexSelection(t *testing.T) {
	store := newMemoryChatStore()
	if err := store.Save(&session.Session{
		ID:        "session-100",
		StartedAt: time.Date(2026, 2, 1, 2, 3, 4, 0, time.UTC),
		Messages:  []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "first"}}}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Save(&session.Session{
		ID:        "session-200",
		StartedAt: time.Date(2026, 2, 1, 2, 3, 5, 0, time.UTC),
		Messages:  []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "second"}}}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var messages []llm.Message
	writer := &flushWriter{}
	if err := handleResumeCommand(context.Background(), nil, writer, "/resume 2", &messages, store, ""); err != nil {
		t.Fatalf("handleResumeCommand: %v", err)
	}
	if len(messages) == 0 || messages[0].Parts[0].Text != "second" {
		t.Fatalf("expected index selection to load second session, got %+v", messages)
	}
}

type errorReader struct {
	err error
}

func (r *errorReader) ReadLine(_ context.Context, _ string) (string, bool, error) {
	return "", false, r.err
}

func (r *errorReader) SaveHistory(string) error {
	return nil
}

func (r *errorReader) Close() error {
	return nil
}

var _ io.Writer = (*flushWriter)(nil)
