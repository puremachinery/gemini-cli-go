package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/storage"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

func TestBuildRunBundleUsesAPIKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(storage.SystemSettingsEnvVar, filepath.Join(tmp, "system-settings.json"))
	t.Setenv(storage.SystemDefaultsEnvVar, filepath.Join(tmp, "system-defaults.json"))
	t.Setenv("GEMINI_API_KEY", "test-key")
	if err := os.MkdirAll(filepath.Join(tmp, storage.GeminiDirName), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	workspace := t.TempDir()
	cfg := runtimeConfig{
		mode:                 tools.ApprovalModeDefault,
		requireReadApproval:  true,
		allowPrivateWebFetch: true,
	}
	bundle, err := buildRunBundle(context.Background(), workspace, cfg, false, buildRunBundleOptions{})
	if err != nil {
		t.Fatalf("buildRunBundle: %v", err)
	}
	if bundle.client == nil {
		t.Fatal("expected client to be configured")
	}
	if bundle.authType != "api-key" {
		t.Fatalf("expected authType=api-key, got %q", bundle.authType)
	}
	if bundle.toolExecutor == nil || bundle.toolExecutor.Registry == nil {
		t.Fatal("expected tool executor registry")
	}
	if _, ok := bundle.toolExecutor.Registry.Lookup(tools.WebSearchToolName); !ok {
		t.Fatal("expected web search tool registered for API key auth")
	}
	if _, ok := bundle.toolExecutor.Registry.Lookup(tools.WebFetchToolName); !ok {
		t.Fatal("expected web fetch tool registered for API key auth")
	}
}

func TestPrepareRunInteractiveProvidesInterrupt(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(storage.SystemSettingsEnvVar, filepath.Join(tmp, "system-settings.json"))
	t.Setenv(storage.SystemDefaultsEnvVar, filepath.Join(tmp, "system-defaults.json"))

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("Chdir cleanup: %v", err)
		}
	})
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	setup, err := prepareRun("", false, false)
	if err != nil {
		t.Fatalf("prepareRun: %v", err)
	}
	defer setup.stop()
	if setup.interrupt == nil {
		t.Fatal("expected interrupt channel for interactive mode")
	}
	want := tmp
	if resolved, err := filepath.EvalSymlinks(tmp); err == nil {
		want = resolved
	}
	got := setup.workspaceRoot
	if resolved, err := filepath.EvalSymlinks(setup.workspaceRoot); err == nil {
		got = resolved
	}
	if got != want {
		t.Fatalf("expected workspaceRoot %q, got %q", want, got)
	}
}

func TestPrepareRunHeadlessHasNoInterrupt(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(storage.SystemSettingsEnvVar, filepath.Join(tmp, "system-settings.json"))
	t.Setenv(storage.SystemDefaultsEnvVar, filepath.Join(tmp, "system-defaults.json"))

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("Chdir cleanup: %v", err)
		}
	})
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	setup, err := prepareRun("", false, true)
	if err != nil {
		t.Fatalf("prepareRun: %v", err)
	}
	defer setup.stop()
	if setup.interrupt != nil {
		t.Fatal("expected no interrupt channel for headless mode")
	}
	want := tmp
	if resolved, err := filepath.EvalSymlinks(tmp); err == nil {
		want = resolved
	}
	got := setup.workspaceRoot
	if resolved, err := filepath.EvalSymlinks(setup.workspaceRoot); err == nil {
		got = resolved
	}
	if got != want {
		t.Fatalf("expected workspaceRoot %q, got %q", want, got)
	}
}

func TestLoadRuntimeConfigPrivateWebFetchDefaultDisabled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("AppData", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv(storage.SystemSettingsEnvVar, filepath.Join(tmp, "system-settings.json"))
	t.Setenv(storage.SystemDefaultsEnvVar, filepath.Join(tmp, "system-defaults.json"))

	workspace := t.TempDir()
	cfg, err := loadRuntimeConfig(context.Background(), workspace, "", false, true)
	if err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	if cfg.allowPrivateWebFetch {
		t.Fatal("expected private web fetch to be disabled by default")
	}
}

func TestLoadRuntimeConfigPrivateWebFetchCanBeEnabled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("AppData", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv(storage.SystemSettingsEnvVar, filepath.Join(tmp, "system-settings.json"))
	t.Setenv(storage.SystemDefaultsEnvVar, filepath.Join(tmp, "system-defaults.json"))

	globalDir := filepath.Join(tmp, storage.GeminiDirName)
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	settingsPath := filepath.Join(globalDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"tools":{"webFetch":{"allowPrivate":true}}}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	workspace := t.TempDir()
	cfg, err := loadRuntimeConfig(context.Background(), workspace, "", false, true)
	if err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	if !cfg.allowPrivateWebFetch {
		t.Fatal("expected private web fetch to be enabled from settings")
	}
}
