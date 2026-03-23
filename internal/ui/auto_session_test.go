package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/session"
)

type stubSessionStore struct {
	saves int
	last  *session.Session
}

func (s *stubSessionStore) Load(string) (*session.Session, error) { return nil, nil }
func (s *stubSessionStore) List() ([]session.Session, error)      { return nil, nil }
func (s *stubSessionStore) Delete(string) error                   { return nil }
func (s *stubSessionStore) Save(sess *session.Session) error {
	s.saves++
	s.last = sess
	return nil
}

func TestAutoSessionSaverDebounce(t *testing.T) {
	now := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	store := &stubSessionStore{}
	saver := newAutoSessionSaver(store, "auth", func() time.Time { return now })
	saver.minInterval = time.Second

	msgs := []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hi"}}}}
	if err := saver.Save(msgs); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := saver.Save(msgs); err != nil {
		t.Fatalf("Save (second): %v", err)
	}
	if store.saves != 1 {
		t.Fatalf("expected 1 save, got %d", store.saves)
	}
	if store.last == nil || store.last.AuthType != "auth" {
		t.Fatalf("expected auth type to be saved, got %#v", store.last)
	}
}

func TestAutoSessionSaverIDPrefix(t *testing.T) {
	now := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	saver := newAutoSessionSaver(&stubSessionStore{}, "auth", func() time.Time { return now })
	if saver == nil {
		t.Fatal("expected saver")
	}
	if !strings.HasPrefix(saver.id, autoSessionPrefix) {
		t.Fatalf("expected prefix %q, got %q", autoSessionPrefix, saver.id)
	}
}
