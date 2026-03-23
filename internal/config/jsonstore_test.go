package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseJSONWithComments(t *testing.T) {
	input := []byte(`// header
{
  // inline comment
  "url": "http://example.com",
  "text": "not a // comment",
  "block": "not a /* comment */"
}
`)
	settings, err := parseJSONWithComments(input)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got, _ := settings.GetString("url"); got != "http://example.com" {
		t.Fatalf("url mismatch: %q", got)
	}
	if got, _ := settings.GetString("text"); got != "not a // comment" {
		t.Fatalf("text mismatch: %q", got)
	}
	if got, _ := settings.GetString("block"); got != "not a /* comment */" {
		t.Fatalf("block mismatch: %q", got)
	}
}

func TestParseJSONWithCommentsRejectsTrailing(t *testing.T) {
	_, err := parseJSONWithComments([]byte(`{"a":1} {"b":2}`))
	if err == nil {
		t.Fatal("expected error for trailing content")
	}
}

func TestParseJSONWithCommentsEmpty(t *testing.T) {
	settings, err := parseJSONWithComments([]byte(`// only comment`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(settings) != 0 {
		t.Fatalf("expected empty settings, got %v", settings)
	}
}

func TestJSONStoreSavePreservesHeaderFooterComments(t *testing.T) {
	store := JSONStore{}
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := []byte("// header\n{\n  \"a\": 1\n}\n// trailer\n")
	file := &File{Path: path, Settings: Settings{"a": 2}, Raw: raw}
	if err := store.Save(file); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "// header") || !strings.Contains(text, "// trailer") {
		t.Fatalf("expected header/trailer comments preserved, got:\n%s", text)
	}
}

func TestJSONStoreSavePreservesInlineComments(t *testing.T) {
	store := JSONStore{}
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := []byte("{\n  // inline\n  \"a\": 1\n}\n")
	file := &File{Path: path, Settings: Settings{"a": 2}, Raw: raw}
	if err := store.Save(file); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "// inline") {
		t.Fatalf("expected inline comment preserved, got:\n%s", text)
	}
}
