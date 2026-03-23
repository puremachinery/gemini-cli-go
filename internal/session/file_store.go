package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/storage"
)

const (
	checkpointPrefix = "checkpoint-"
	checkpointSuffix = ".json"
	logSuffix        = ".log.jsonl"
	metaSuffix       = ".meta.json"
)

// ErrNotFound indicates a missing session.
var ErrNotFound = errors.New("session not found")

// FileStore persists sessions as JSON files in a directory.
type FileStore struct {
	dir string
}

// NewFileStore creates a FileStore rooted at dir.
func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

// Exists reports whether a checkpoint exists for the tag.
func (s *FileStore) Exists(id string) (bool, error) {
	path, err := s.pathFor(id)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Load loads a session by tag.
func (s *FileStore) Load(id string) (*Session, error) {
	path, err := s.pathFor(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			meta, metaErr := loadSessionMetaFile(sessionMetaPath(path))
			if metaErr != nil {
				return nil, ErrNotFound
			}
			messages, logErr := readSessionLog(sessionLogPath(path))
			if logErr != nil {
				return nil, logErr
			}
			sess := Session{
				SchemaVersion: meta.SchemaVersion,
				ID:            meta.ID,
				StartedAt:     meta.StartedAt,
				UpdatedAt:     meta.UpdatedAt,
				AuthType:      meta.AuthType,
				Messages:      messages,
			}
			if sess.ID == "" {
				sess.ID = id
			}
			return &sess, nil
		}
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	if err := validateSessionVersion(sess.SchemaVersion); err != nil {
		return nil, err
	}
	if sess.ID == "" {
		sess.ID = id
	}
	if sess.UpdatedAt.IsZero() {
		if info, err := os.Stat(path); err == nil {
			sess.UpdatedAt = info.ModTime()
		}
	}
	if messages, err := readSessionLog(sessionLogPath(path)); err == nil && len(messages) > 0 {
		sess.Messages = append(sess.Messages, messages...)
	}
	return &sess, nil
}

// Save writes a session to disk.
func (s *FileStore) Save(sess *Session) error {
	if sess == nil {
		return errors.New("session is nil")
	}
	path, err := s.pathFor(sess.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	now := time.Now()
	copySess := *sess
	if copySess.StartedAt.IsZero() {
		copySess.StartedAt = now
	}
	copySess.SchemaVersion = sessionSchemaVersion
	copySess.UpdatedAt = now
	data, err := json.MarshalIndent(&copySess, "", "  ")
	if err != nil {
		return err
	}
	meta := sessionMetaFromSession(copySess, len(copySess.Messages), len(copySess.Messages), 0)
	logPath := sessionLogPath(path)
	metaPath := sessionMetaPath(path)
	return storage.WithFileLock(path, func() error {
		if err := storage.WriteFileAtomic(path, data, 0o600); err != nil {
			return err
		}
		if err := writeSessionMeta(metaPath, meta); err != nil {
			return err
		}
		if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	})
}

// List returns available sessions.
func (s *FileStore) List() ([]Session, error) {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Session, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isSessionSnapshot(name) {
			continue
		}
		encoded := strings.TrimSuffix(strings.TrimPrefix(name, checkpointPrefix), checkpointSuffix)
		tag := DecodeTagName(encoded)
		path := filepath.Join(s.dir, name)
		meta, err := loadSessionMeta(path)
		if err != nil {
			continue
		}
		if meta.ID == "" {
			meta.ID = tag
		}
		out = append(out, meta)
	}
	return out, nil
}

// ListWithMessages loads full sessions for resume previews.
func (s *FileStore) ListWithMessages() ([]Session, error) {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Session, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isSessionSnapshot(name) {
			continue
		}
		encoded := strings.TrimSuffix(strings.TrimPrefix(name, checkpointPrefix), checkpointSuffix)
		tag := DecodeTagName(encoded)
		sess, err := s.Load(tag)
		if err != nil || sess == nil {
			continue
		}
		out = append(out, *sess)
	}
	return out, nil
}

// ListPreviews returns lightweight previews for resume listings.
func (s *FileStore) ListPreviews() ([]Preview, error) {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Preview, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isSessionSnapshot(name) {
			continue
		}
		encoded := strings.TrimSuffix(strings.TrimPrefix(name, checkpointPrefix), checkpointSuffix)
		tag := DecodeTagName(encoded)
		path := filepath.Join(s.dir, name)
		metaPath := sessionMetaPath(path)
		meta, err := loadSessionMetaFile(metaPath)
		if err == nil {
			entry := Preview{
				Session: Session{
					SchemaVersion: meta.SchemaVersion,
					ID:            meta.ID,
					StartedAt:     meta.StartedAt,
					UpdatedAt:     meta.UpdatedAt,
					AuthType:      meta.AuthType,
				},
				MessageCount: meta.MessageCount,
				PreviewText:  meta.PreviewText,
			}
			if entry.Session.ID == "" {
				entry.Session.ID = tag
			}
			out = append(out, entry)
			continue
		}
		preview, err := loadSessionPreview(path)
		if err != nil {
			continue
		}
		if preview.Session.ID == "" {
			preview.Session.ID = tag
		}
		out = append(out, preview)
	}
	return out, nil
}

// Delete removes a saved session by tag.
func (s *FileStore) Delete(id string) error {
	path, err := s.pathFor(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	if err := os.Remove(sessionLogPath(path)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(sessionMetaPath(path)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

type sessionMeta struct {
	SchemaVersion int       `json:"schemaVersion,omitempty"`
	ID            string    `json:"id"`
	StartedAt     time.Time `json:"startedAt"`
	UpdatedAt     time.Time `json:"updatedAt,omitempty"`
	AuthType      string    `json:"authType,omitempty"`
	MessageCount  int       `json:"messageCount,omitempty"`
	SnapshotCount int       `json:"snapshotCount,omitempty"`
	DeltaCount    int       `json:"deltaCount,omitempty"`
	PreviewText   string    `json:"preview,omitempty"`
}

func loadSessionMeta(path string) (Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}, err
	}
	var meta sessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return Session{}, err
		}
		return Session{
			StartedAt: info.ModTime(),
			UpdatedAt: info.ModTime(),
		}, nil
	}
	if err := validateSessionVersion(meta.SchemaVersion); err != nil {
		return Session{}, err
	}
	if meta.StartedAt.IsZero() {
		meta.StartedAt = meta.UpdatedAt
	}
	return Session{
		SchemaVersion: meta.SchemaVersion,
		ID:            meta.ID,
		StartedAt:     meta.StartedAt,
		UpdatedAt:     meta.UpdatedAt,
	}, nil
}

func loadSessionPreview(path string) (preview Preview, err error) {
	file, err := os.Open(path)
	if err != nil {
		return Preview{}, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	dec := json.NewDecoder(file)
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return Preview{}, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return Preview{}, errors.New("invalid session file")
	}

	preview = Preview{
		Session: Session{},
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return Preview{}, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return Preview{}, errors.New("invalid session file key")
		}
		switch key {
		case "schemaVersion":
			var value json.Number
			if err := dec.Decode(&value); err != nil {
				return Preview{}, err
			}
			if value != "" {
				if parsed, err := value.Int64(); err == nil {
					preview.Session.SchemaVersion = int(parsed)
				}
			}
		case "id":
			if err := dec.Decode(&preview.Session.ID); err != nil {
				return Preview{}, err
			}
		case "startedAt":
			if err := dec.Decode(&preview.Session.StartedAt); err != nil {
				return Preview{}, err
			}
		case "updatedAt":
			if err := dec.Decode(&preview.Session.UpdatedAt); err != nil {
				return Preview{}, err
			}
		case "authType":
			if err := dec.Decode(&preview.Session.AuthType); err != nil {
				return Preview{}, err
			}
		case "messages":
			if err := decodePreviewMessages(dec, &preview); err != nil {
				return Preview{}, err
			}
		default:
			var discard json.RawMessage
			if err := dec.Decode(&discard); err != nil {
				return Preview{}, err
			}
		}
	}
	if _, err := dec.Token(); err != nil {
		return Preview{}, err
	}
	if err := validateSessionVersion(preview.Session.SchemaVersion); err != nil {
		return Preview{}, err
	}
	if preview.Session.StartedAt.IsZero() {
		preview.Session.StartedAt = preview.Session.UpdatedAt
	}
	if preview.Session.StartedAt.IsZero() {
		if info, statErr := os.Stat(path); statErr == nil {
			preview.Session.StartedAt = info.ModTime()
			preview.Session.UpdatedAt = info.ModTime()
		}
	}
	return preview, nil
}

func decodePreviewMessages(dec *json.Decoder, preview *Preview) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return nil
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return errors.New("invalid messages payload")
	}
	if delim != '[' {
		return errors.New("invalid messages payload")
	}
	for dec.More() {
		var msg llm.Message
		if err := dec.Decode(&msg); err != nil {
			return err
		}
		preview.MessageCount++
		if preview.PreviewText == "" && msg.Role == llm.RoleUser {
			preview.PreviewText = joinPreviewText(msg.Parts)
		}
	}
	_, err = dec.Token()
	return err
}

func joinPreviewText(parts []llm.Part) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Text == "" {
			continue
		}
		b.WriteString(part.Text)
	}
	return b.String()
}

func validateSessionVersion(version int) error {
	if version == 0 {
		return nil
	}
	if version > sessionSchemaVersion {
		return fmt.Errorf("unsupported session schema version %d", version)
	}
	return nil
}

func (s *FileStore) pathFor(id string) (string, error) {
	if s == nil {
		return "", errors.New("session store is nil")
	}
	if strings.TrimSpace(s.dir) == "" {
		return "", errors.New("session store directory is empty")
	}
	if strings.TrimSpace(id) == "" {
		return "", errors.New("tag is required")
	}
	encoded := EncodeTagName(id)
	filename := fmt.Sprintf("%s%s%s", checkpointPrefix, encoded, checkpointSuffix)
	return filepath.Join(s.dir, filename), nil
}
