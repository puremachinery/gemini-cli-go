package storage

import (
	"path/filepath"
	"testing"
)

func TestGlobalGeminiDirUsesHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := GlobalGeminiDir()
	expected := filepath.Join(tmp, GeminiDirName)
	if dir != expected {
		t.Fatalf("expected %q, got %q", expected, dir)
	}
}

func TestIsGlobalGeminiPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	root := GlobalGeminiDir()
	if !IsGlobalGeminiPath(filepath.Join(root, "settings.json")) {
		t.Fatal("expected path inside global dir to be true")
	}
	if IsGlobalGeminiPath(filepath.Join(tmp, "other", "file.txt")) {
		t.Fatal("expected path outside global dir to be false")
	}
}
