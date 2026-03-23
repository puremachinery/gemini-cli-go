package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/puremachinery/gemini-cli-go/internal/storage"
)

// FileStore persists OAuth credentials in the global ~/.gemini directory.
type FileStore struct {
	Path string
}

// NewFileStore returns a FileStore using the default OAuth credentials path.
func NewFileStore() FileStore {
	return FileStore{Path: storage.OAuthCredsPath()}
}

// Load reads cached credentials from disk or GOOGLE_APPLICATION_CREDENTIALS.
func (s FileStore) Load(ctx context.Context) (*Credentials, error) {
	_ = ctx
	paths := []string{s.Path, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")}
	for _, candidate := range paths {
		if candidate == "" {
			continue
		}
		data, err := os.ReadFile(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		creds, err := decodeCredentialsJSON(ctx, data)
		if err != nil {
			if errors.Is(err, ErrNoCredentials) {
				continue
			}
			return nil, err
		}
		return creds, nil
	}
	return nil, os.ErrNotExist
}

// Save writes credentials to disk with restrictive permissions.
func (s FileStore) Save(ctx context.Context, creds *Credentials) error {
	_ = ctx
	if creds == nil {
		return errors.New("credentials are nil")
	}
	if s.Path == "" {
		return errors.New("credentials path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	data, err := encodeTokenFile(creds)
	if err != nil {
		return err
	}
	return storage.WithFileLock(s.Path, func() error {
		return storage.WriteFileAtomic(s.Path, data, 0o600)
	})
}

// Clear removes cached credentials.
func (s FileStore) Clear(ctx context.Context) error {
	_ = ctx
	if s.Path == "" {
		return nil
	}
	if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
