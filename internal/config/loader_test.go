package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/storage"
)

func TestLoaderPrecedence(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	sysDir := filepath.Join(root, "system")
	sysDefaults := filepath.Join(sysDir, "system-defaults.json")
	sysSettings := filepath.Join(sysDir, "settings.json")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatalf("mkdir system: %v", err)
	}
	t.Setenv(storage.SystemDefaultsEnvVar, sysDefaults)
	t.Setenv(storage.SystemSettingsEnvVar, sysSettings)

	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".gemini"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	writeJSONFile(t, sysDefaults, `{"layer":"system-defaults","nested":{"a":1}}`)
	writeJSONFile(t, sysSettings, `{"layer":"system","nested":{"b":2}}`)
	writeJSONFile(t, storage.GlobalSettingsPath(), `{"layer":"global","nested":{"a":3}}`)
	writeJSONFile(t, storage.WorkspaceSettingsPath(workspace), `{"layer":"workspace","nested":{"c":4}}`)

	loader := Loader{Store: JSONStore{}}
	result, err := loader.Load(context.Background(), workspace)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if result.SystemDefaults == nil || result.System == nil || result.Global == nil || result.Workspace == nil {
		t.Fatalf("expected all layers present")
	}

	if got, _ := result.Merged.GetString("layer"); got != "workspace" {
		t.Fatalf("layer mismatch: %q", got)
	}

	nestedVal, ok := result.Merged["nested"]
	if !ok {
		t.Fatalf("expected nested settings")
	}
	nested, ok := nestedVal.(map[string]any)
	if !ok {
		t.Fatalf("unexpected nested type: %T", nestedVal)
	}
	assertNumberString(t, nested["a"], "3")
	assertNumberString(t, nested["b"], "2")
	assertNumberString(t, nested["c"], "4")
}

func assertNumberString(t *testing.T, v any, want string) {
	t.Helper()
	got, ok := numberString(v)
	if !ok {
		t.Fatalf("unexpected number type: %T", v)
	}
	if got != want {
		t.Fatalf("number mismatch: got %s want %s", got, want)
	}
}

func writeJSONFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
