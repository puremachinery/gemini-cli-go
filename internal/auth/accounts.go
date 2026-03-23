package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/puremachinery/gemini-cli-go/internal/storage"
)

// AccountManager caches Google account emails for OAuth logins.
type AccountManager struct {
	Path string
}

// NewAccountManager returns a manager using the default accounts file path.
func NewAccountManager() AccountManager {
	return AccountManager{Path: storage.GoogleAccountsPath()}
}

type accountFile struct {
	Active *string  `json:"active"`
	Old    []string `json:"old"`
}

// Cache records the active account email and preserves historical values.
func (m AccountManager) Cache(email string) error {
	if email == "" {
		return nil
	}
	path := m.Path
	if path == "" {
		return nil
	}
	state, err := m.readAccounts(path)
	if err != nil {
		return err
	}
	if state.Active != nil && *state.Active != email {
		if !containsString(state.Old, *state.Active) {
			state.Old = append(state.Old, *state.Active)
		}
	}
	state.Old = filterOut(state.Old, email)
	state.Active = &email
	return m.writeAccounts(path, state)
}

// Active returns the active cached account email, if any.
func (m AccountManager) Active() (string, bool) {
	if m.Path == "" {
		return "", false
	}
	state, err := m.readAccounts(m.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read account cache: %v\n", err)
		return "", false
	}
	if state.Active == nil {
		return "", false
	}
	return *state.Active, true
}

// ClearActive clears the active account while preserving history.
func (m AccountManager) ClearActive() error {
	if m.Path == "" {
		return nil
	}
	state, err := m.readAccounts(m.Path)
	if err != nil {
		return err
	}
	if state.Active != nil {
		if !containsString(state.Old, *state.Active) {
			state.Old = append(state.Old, *state.Active)
		}
		state.Active = nil
	}
	return m.writeAccounts(m.Path, state)
}

func (m AccountManager) readAccounts(path string) (accountFile, error) {
	state := accountFile{Active: nil, Old: []string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if len(data) == 0 {
		return state, nil
	}
	var parsed accountFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return state, fmt.Errorf("invalid accounts cache %s: %w", path, err)
	}
	if parsed.Old == nil {
		parsed.Old = []string{}
	}
	return parsed, nil
}

func (m AccountManager) writeAccounts(path string, state accountFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return storage.WithFileLock(path, func() error {
		return storage.WriteFileAtomic(path, data, 0o600)
	})
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func filterOut(items []string, value string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != value {
			out = append(out, item)
		}
	}
	return out
}
