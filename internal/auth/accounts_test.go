package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAccountManagerCacheAndClear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "google_accounts.json")
	manager := AccountManager{Path: path}

	if err := manager.Cache("first@example.com"); err != nil {
		t.Fatalf("Cache first: %v", err)
	}
	if err := manager.Cache("second@example.com"); err != nil {
		t.Fatalf("Cache second: %v", err)
	}

	state := readAccountFile(t, path)
	if state.Active == nil || *state.Active != "second@example.com" {
		t.Fatalf("active account mismatch: %#v", state.Active)
	}
	if len(state.Old) != 1 || state.Old[0] != "first@example.com" {
		t.Fatalf("old accounts mismatch: %#v", state.Old)
	}

	if err := manager.ClearActive(); err != nil {
		t.Fatalf("ClearActive: %v", err)
	}
	state = readAccountFile(t, path)
	if state.Active != nil {
		t.Fatalf("expected active to be nil after clear, got %#v", *state.Active)
	}
	if len(state.Old) != 2 {
		t.Fatalf("expected old accounts to include both entries, got %#v", state.Old)
	}
}

func readAccountFile(t *testing.T, path string) accountFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var state accountFile
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return state
}
