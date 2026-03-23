package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/puremachinery/gemini-cli-go/internal/storage"
	"github.com/tailscale/hujson"
)

// JSONStore loads and saves settings files with JSON + comments support.
type JSONStore struct{}

// Load reads a JSON settings file, allowing // and /* */ comments.
func (s JSONStore) Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	settings, err := parseJSONWithComments(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return &File{Path: path, Settings: settings, Raw: data}, nil
}

// Save writes settings as pretty JSON while preserving comments/formatting
// from the existing file if present.
func (s JSONStore) Save(file *File) error {
	if file == nil {
		return errors.New("settings file is nil")
	}
	if file.Path == "" {
		return errors.New("settings path is empty")
	}
	if file.Settings == nil {
		file.Settings = Settings{}
	}

	perm := os.FileMode(0o644)
	dirPerm := os.FileMode(0o755)
	if storage.IsGlobalGeminiPath(file.Path) {
		perm = 0o600
		dirPerm = 0o700
	}
	if err := os.MkdirAll(filepath.Dir(file.Path), dirPerm); err != nil {
		return err
	}
	if len(file.Raw) == 0 {
		data, err := json.MarshalIndent(file.Settings, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		return storage.WriteFileAtomic(file.Path, data, perm)
	}

	updated, err := updateJSONWithComments(file.Raw, file.Settings)
	if err != nil {
		return err
	}
	return storage.WriteFileAtomic(file.Path, updated, perm)
}

func parseJSONWithComments(data []byte) (Settings, error) {
	if len(bytes.TrimSpace(stripJSONComments(data))) == 0 {
		return Settings{}, nil
	}
	copyData := append([]byte(nil), data...)
	standard, err := hujson.Standardize(copyData)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(standard))
	dec.UseNumber()
	var settings Settings
	if err := dec.Decode(&settings); err != nil {
		if errors.Is(err, io.EOF) {
			return Settings{}, nil
		}
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return settings, nil
		}
		return nil, err
	}
	return nil, errors.New("invalid JSON: trailing content")
}

// stripJSONComments removes // and /* */ comments while preserving newlines.
func stripJSONComments(input []byte) []byte {
	const (
		stateNormal = iota
		stateString
		stateStringEscape
		stateLineComment
		stateBlockComment
	)

	state := stateNormal
	out := make([]byte, 0, len(input))

	for i := 0; i < len(input); i++ {
		c := input[i]
		switch state {
		case stateNormal:
			if c == '"' {
				out = append(out, c)
				state = stateString
				continue
			}
			if c == '/' && i+1 < len(input) {
				next := input[i+1]
				if next == '/' {
					state = stateLineComment
					i++
					continue
				}
				if next == '*' {
					state = stateBlockComment
					i++
					continue
				}
			}
			out = append(out, c)
		case stateString:
			out = append(out, c)
			if c == '\\' {
				state = stateStringEscape
				continue
			}
			if c == '"' {
				state = stateNormal
			}
		case stateStringEscape:
			out = append(out, c)
			state = stateString
		case stateLineComment:
			if c == '\n' {
				out = append(out, c)
				state = stateNormal
			}
		case stateBlockComment:
			if c == '\n' {
				out = append(out, c)
				continue
			}
			if c == '*' && i+1 < len(input) && input[i+1] == '/' {
				state = stateNormal
				i++
			}
		}
	}

	return out
}
