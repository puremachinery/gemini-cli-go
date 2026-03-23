// Package storage provides filesystem path helpers for config and data.
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/logging"
)

const (
	// GeminiDirName is the global config directory name.
	GeminiDirName = ".gemini"
	// OAuthCredsFilename is the stored OAuth credentials filename.
	OAuthCredsFilename = "oauth_creds.json"
	// GoogleAccountsFile is the accounts registry filename.
	GoogleAccountsFile = "google_accounts.json"
	// InstallationIDFile stores the installation identifier.
	InstallationIDFile = "installation_id"
	// GlobalMemoryFile is the global memory filename.
	GlobalMemoryFile = "memory.md"
	// SystemSettingsEnvVar overrides the system settings path.
	SystemSettingsEnvVar = "GEMINI_CLI_SYSTEM_SETTINGS_PATH"
	// SystemDefaultsEnvVar overrides the system defaults path.
	SystemDefaultsEnvVar = "GEMINI_CLI_SYSTEM_DEFAULTS_PATH"
	// SystemDefaultsFile is the system defaults filename.
	SystemDefaultsFile = "system-defaults.json"
)

func homeDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home, nil
	}
	if runtime.GOOS == "windows" {
		if home := strings.TrimSpace(os.Getenv("USERPROFILE")); home != "" {
			return home, nil
		}
		drive := strings.TrimSpace(os.Getenv("HOMEDRIVE"))
		path := strings.TrimSpace(os.Getenv("HOMEPATH"))
		if drive != "" && path != "" {
			return drive + path, nil
		}
	}
	return os.UserHomeDir()
}

// GlobalGeminiDir returns the global config dir (e.g., ~/.gemini).
func GlobalGeminiDir() string {
	home, homeErr := homeDir()
	if homeErr == nil && home != "" {
		return filepath.Join(home, GeminiDirName)
	}
	configDir, cfgErr := os.UserConfigDir()
	if cfgErr == nil && configDir != "" {
		return filepath.Join(configDir, GeminiDirName)
	}
	fallback := filepath.Join(os.TempDir(), GeminiDirName)
	logging.Logger().Warn(
		"failed to resolve home/config dir; falling back to temp",
		"path", fallback,
		"home_err", homeErr,
		"config_err", cfgErr,
	)
	return fallback
}

// WorkspaceGeminiDir returns the workspace .gemini directory.
func WorkspaceGeminiDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, GeminiDirName)
}

// GlobalSettingsPath returns the global settings.json path.
func GlobalSettingsPath() string {
	return filepath.Join(GlobalGeminiDir(), "settings.json")
}

// WorkspaceSettingsPath returns the workspace settings.json path.
func WorkspaceSettingsPath(workspaceRoot string) string {
	return filepath.Join(WorkspaceGeminiDir(workspaceRoot), "settings.json")
}

// SystemConfigDir returns the OS-specific system config directory.
func SystemConfigDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/GeminiCli"
	case "windows":
		return "C:\\ProgramData\\gemini-cli"
	default:
		return "/etc/gemini-cli"
	}
}

// SystemSettingsPath returns the system-wide settings.json path.
func SystemSettingsPath() string {
	if v := os.Getenv(SystemSettingsEnvVar); v != "" {
		return v
	}
	return filepath.Join(SystemConfigDir(), "settings.json")
}

// SystemDefaultsPath returns the system-defaults.json path.
func SystemDefaultsPath() string {
	if v := os.Getenv(SystemDefaultsEnvVar); v != "" {
		return v
	}
	return filepath.Join(SystemConfigDir(), SystemDefaultsFile)
}

// OAuthCredsPath returns the global OAuth credentials path.
func OAuthCredsPath() string {
	return filepath.Join(GlobalGeminiDir(), OAuthCredsFilename)
}

// GoogleAccountsPath returns the global accounts registry path.
func GoogleAccountsPath() string {
	return filepath.Join(GlobalGeminiDir(), GoogleAccountsFile)
}

// InstallationIDPath returns the global installation id path.
func InstallationIDPath() string {
	return filepath.Join(GlobalGeminiDir(), InstallationIDFile)
}

// GlobalMemoryPath returns the memory.md path.
func GlobalMemoryPath() string {
	return filepath.Join(GlobalGeminiDir(), GlobalMemoryFile)
}

// ProjectTempDir returns a per-project temp directory.
func ProjectTempDir(workspaceRoot string) string {
	root := workspaceRoot
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(GlobalGeminiDir(), "tmp", hex.EncodeToString(sum[:]))
}

// ProjectChatsDir returns the project-specific chats directory.
func ProjectChatsDir(workspaceRoot string) string {
	return filepath.Join(ProjectTempDir(workspaceRoot), "chats")
}

// IsGlobalGeminiPath reports whether path is within the global Gemini config directory.
func IsGlobalGeminiPath(path string) bool {
	if path == "" {
		return false
	}
	root := GlobalGeminiDir()
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	targetAbs, err := filepath.Abs(path)
	if err != nil {
		targetAbs = path
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../")
}
