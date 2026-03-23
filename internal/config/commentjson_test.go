package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommentJSONPreserveCommentsOnUpdate(t *testing.T) {
	original := `{
  // Model configuration
  "model": "gemini-2.5-pro",
  "ui": {
    // Theme setting
    "theme": "dark"
  }
}`
	updated := updateSettingsFile(t, original, Settings{
		"model": "gemini-2.5-flash",
		"ui": map[string]any{
			"theme": "dark",
		},
	})

	assertContains(t, updated, "// Model configuration")
	assertContains(t, updated, "// Theme setting")
	settings := parseSettings(t, updated)
	assertSettingString(t, settings, "model", "gemini-2.5-flash")
	assertSettingString(t, settings, "ui.theme", "dark")
}

func TestCommentJSONNestedUpdate(t *testing.T) {
	original := `{
  "ui": {
    "theme": "dark",
    "showLineNumbers": true
  }
}`
	updated := updateSettingsFile(t, original, Settings{
		"ui": map[string]any{
			"theme":           "light",
			"showLineNumbers": true,
		},
	})
	settings := parseSettings(t, updated)
	assertSettingString(t, settings, "ui.theme", "light")
	assertSettingBool(t, settings, "ui.showLineNumbers", true)
}

func TestCommentJSONAddFieldPreservesStructure(t *testing.T) {
	original := `{
  // Existing config
  "model": "gemini-2.5-pro"
}`
	updated := updateSettingsFile(t, original, Settings{
		"model":    "gemini-2.5-pro",
		"newField": "newValue",
	})
	assertContains(t, updated, "// Existing config")
	settings := parseSettings(t, updated)
	assertSettingString(t, settings, "newField", "newValue")
}

func TestCommentJSONCreateFileWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	store := JSONStore{}
	file := &File{Path: path, Settings: Settings{"model": "gemini-2.5-pro"}}
	if err := store.Save(file); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	settings := parseSettings(t, string(content))
	assertSettingString(t, settings, "model", "gemini-2.5-pro")
}

func TestCommentJSONComplexScenario(t *testing.T) {
	original := `{
  // Settings
  "model": "gemini-2.5-pro",
  "mcpServers": {
    // Active server
    "context7": {
      "headers": {
        "API_KEY": "test-key" // API key
      }
    }
  }
}`
	updated := updateSettingsFile(t, original, Settings{
		"model": "gemini-2.5-flash",
		"mcpServers": map[string]any{
			"context7": map[string]any{
				"headers": map[string]any{
					"API_KEY": "new-test-key",
				},
			},
		},
		"newSection": map[string]any{
			"setting": "value",
		},
	})

	assertContains(t, updated, "// Settings")
	assertContains(t, updated, "// Active server")
	assertContains(t, updated, "// API key")
	settings := parseSettings(t, updated)
	assertSettingString(t, settings, "model", "gemini-2.5-flash")
	assertSettingString(t, settings, "mcpServers.context7.headers.API_KEY", "new-test-key")
	if _, ok := settings.Get("newSection"); !ok {
		t.Fatalf("expected newSection present")
	}
}

func TestCommentJSONArrayUpdatePreservesComments(t *testing.T) {
	original := `{
  // Server configurations
  "servers": [
    // First server
    "server1",
    "server2" // Second server
  ]
}`
	updated := updateSettingsFile(t, original, Settings{
		"servers": []any{"server1", "server3"},
	})

	assertContains(t, updated, "// Server configurations")
	settings := parseSettings(t, updated)
	assertSettingArrayStrings(t, settings, "servers", []string{"server1", "server3"})
}

func TestCommentJSONSyncNestedObjectsRemovesOmitted(t *testing.T) {
	original := `{
  // Configuration
  "model": "gemini-2.5-pro",
  "ui": {
    "theme": "dark",
    "existingSetting": "value"
  },
  "preservedField": "keep me"
}`
	updated := updateSettingsFile(t, original, Settings{
		"model": "gemini-2.5-flash",
		"ui": map[string]any{
			"theme": "light",
		},
		"preservedField": "keep me",
	})

	assertContains(t, updated, "// Configuration")
	settings := parseSettings(t, updated)
	assertSettingString(t, settings, "model", "gemini-2.5-flash")
	assertSettingString(t, settings, "ui.theme", "light")
	if _, ok := settings.Get("ui.existingSetting"); ok {
		t.Fatalf("expected existingSetting removed")
	}
	assertSettingString(t, settings, "preservedField", "keep me")
}

func TestCommentJSONMcpServersDeletionPreservesComment(t *testing.T) {
	original := `{
  "model": "gemini-2.5-pro",
  "mcpServers": {
    // Server to keep
    "context7": {
      "command": "node",
      "args": ["server.js"]
    },
    // Server to remove
    "oldServer": {
      "command": "old",
      "args": ["old.js"]
    }
  }
}`
	updated := updateSettingsFile(t, original, Settings{
		"model": "gemini-2.5-pro",
		"mcpServers": map[string]any{
			"context7": map[string]any{
				"command": "node",
				"args":    []any{"server.js"},
			},
		},
	})

	assertContains(t, updated, "// Server to keep")
	assertContains(t, updated, "// Server to remove")
	settings := parseSettings(t, updated)
	if _, ok := settings.Get("mcpServers.oldServer"); ok {
		t.Fatalf("expected oldServer removed")
	}
	if _, ok := settings.Get("mcpServers.context7"); !ok {
		t.Fatalf("expected context7 present")
	}
}

func TestCommentJSONPreservesCommentedOutBlocks(t *testing.T) {
	original := `{
  "mcpServers": {
    // "sleep": {
    //   "command": "node",
    //   "args": [
    //     "/Users/testUser/test-mcp-server/sleep-mcp/build/index.js"
    //   ],
    //   "timeout": 300000
    // },
    "playwright": {
      "command": "npx",
      "args": [
        "@playwright/mcp@latest",
        "--headless",
        "--isolated"
      ]
    }
  }
}`
	updated := updateSettingsFile(t, original, Settings{
		"mcpServers": map[string]any{},
	})

	assertContains(t, updated, "// \"sleep\": {")
	settings := parseSettings(t, updated)
	if _, ok := settings.Get("mcpServers"); !ok {
		t.Fatalf("expected mcpServers present")
	}
	if _, ok := settings.Get("mcpServers.playwright"); ok {
		t.Fatalf("expected playwright removed")
	}
}

func TestCommentJSONTypeConversionObjectToArray(t *testing.T) {
	original := `{
  "data": {
    "key": "value"
  }
}`
	updated := updateSettingsFile(t, original, Settings{
		"data": []any{"item1", "item2"},
	})
	settings := parseSettings(t, updated)
	assertSettingArrayStrings(t, settings, "data", []string{"item1", "item2"})
}

func TestCommentJSONRemoveNestedAndNonNestedObjects(t *testing.T) {
	original := `{
  // Top-level config
  "topLevelObject": {
    "field1": "value1",
    "field2": "value2"
  },
  // Parent object
  "parent": {
    "nestedObject": {
      "nestedField1": "value1",
      "nestedField2": "value2"
    },
    "keepThis": "value"
  },
  // This should be preserved
  "preservedObject": {
    "data": "keep"
  }
}`
	updated := updateSettingsFile(t, original, Settings{
		"parent": map[string]any{
			"keepThis": "value",
		},
		"preservedObject": map[string]any{
			"data": "keep",
		},
	})

	settings := parseSettings(t, updated)
	if _, ok := settings.Get("topLevelObject"); ok {
		t.Fatalf("expected topLevelObject removed")
	}
	if _, ok := settings.Get("parent.nestedObject"); ok {
		t.Fatalf("expected nestedObject removed")
	}
	assertSettingString(t, settings, "parent.keepThis", "value")
	assertSettingString(t, settings, "preservedObject.data", "keep")
	assertContains(t, updated, "// This should be preserved")
}

func TestCommentJSONCorruptedInputNoOverwrite(t *testing.T) {
	original := `{
  "model": "gemini-2.5-pro",
  "ui": {
    "theme": "dark"
  // Missing closing brace
`
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	store := JSONStore{}
	file := &File{Path: path, Settings: Settings{"model": "gemini-2.5-flash"}, Raw: []byte(original)}
	if err := store.Save(file); err == nil {
		t.Fatal("expected error for corrupted JSON")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(data) != original {
		t.Fatalf("expected file unchanged, got:\n%s", string(data))
	}
}

func updateSettingsFile(t *testing.T, original string, updates Settings) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	store := JSONStore{}
	file, err := store.Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	file.Settings = updates
	if err := store.Save(file); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	return string(data)
}

func assertContains(t *testing.T, content string, substr string) {
	t.Helper()
	if !strings.Contains(content, substr) {
		t.Fatalf("expected to contain %q, got:\n%s", substr, content)
	}
}

func parseSettings(t *testing.T, content string) Settings {
	t.Helper()
	settings, err := parseJSONWithComments([]byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return settings
}

func assertSettingString(t *testing.T, settings Settings, path string, want string) {
	t.Helper()
	got, ok := settings.GetString(path)
	if !ok {
		t.Fatalf("expected %s present", path)
	}
	if got != want {
		t.Fatalf("unexpected %s: got %q want %q", path, got, want)
	}
}

func assertSettingBool(t *testing.T, settings Settings, path string, want bool) {
	t.Helper()
	value, ok := settings.Get(path)
	if !ok {
		t.Fatalf("expected %s present", path)
	}
	got, ok := value.(bool)
	if !ok {
		t.Fatalf("unexpected %s type %T", path, value)
	}
	if got != want {
		t.Fatalf("unexpected %s: got %v want %v", path, got, want)
	}
}

func assertSettingArrayStrings(t *testing.T, settings Settings, path string, want []string) {
	t.Helper()
	value, ok := settings.Get(path)
	if !ok {
		t.Fatalf("expected %s present", path)
	}
	arr, ok := value.([]any)
	if !ok {
		t.Fatalf("unexpected %s type %T", path, value)
	}
	if len(arr) != len(want) {
		t.Fatalf("unexpected %s length: got %d want %d", path, len(arr), len(want))
	}
	for i, exp := range want {
		got, ok := arr[i].(string)
		if !ok {
			t.Fatalf("unexpected %s[%d] type %T", path, i, arr[i])
		}
		if got != exp {
			t.Fatalf("unexpected %s[%d]: got %q want %q", path, i, got, exp)
		}
	}
}
