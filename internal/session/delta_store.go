package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/storage"
)

const (
	maxDeltaMessages = 25
	maxDeltaBytes    = 512 * 1024
)

// SaveDelta writes session updates using an append-only log and periodic snapshots.
func (s *FileStore) SaveDelta(sess *Session) error {
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
	copySess := *sess
	if copySess.StartedAt.IsZero() {
		copySess.StartedAt = time.Now()
	}
	copySess.SchemaVersion = sessionSchemaVersion
	copySess.UpdatedAt = time.Now()

	logPath := sessionLogPath(path)
	metaPath := sessionMetaPath(path)

	return storage.WithFileLock(path, func() error {
		meta, metaErr := loadSessionMetaFile(metaPath)
		if metaErr != nil || meta.SnapshotCount == 0 && meta.MessageCount > 0 {
			return s.writeSnapshotWithMeta(path, logPath, metaPath, copySess)
		}
		if meta.MessageCount > len(copySess.Messages) {
			return s.writeSnapshotWithMeta(path, logPath, metaPath, copySess)
		}
		newMessages := copySess.Messages[meta.MessageCount:]
		if len(newMessages) == 0 {
			meta.UpdatedAt = copySess.UpdatedAt
			meta.AuthType = copySess.AuthType
			meta.MessageCount = len(copySess.Messages)
			meta.PreviewText = previewFromMessages(copySess.Messages)
			return writeSessionMeta(metaPath, meta)
		}
		if err := appendSessionLog(logPath, newMessages); err != nil {
			return err
		}
		meta.DeltaCount += len(newMessages)
		meta.MessageCount = len(copySess.Messages)
		meta.UpdatedAt = copySess.UpdatedAt
		meta.AuthType = copySess.AuthType
		meta.PreviewText = previewFromMessages(copySess.Messages)
		if meta.DeltaCount >= maxDeltaMessages {
			return s.writeSnapshotWithMeta(path, logPath, metaPath, copySess)
		}
		if info, err := os.Stat(logPath); err == nil && info.Size() >= maxDeltaBytes {
			return s.writeSnapshotWithMeta(path, logPath, metaPath, copySess)
		}
		return writeSessionMeta(metaPath, meta)
	})
}

func (s *FileStore) writeSnapshotWithMeta(snapshotPath, logPath, metaPath string, sess Session) error {
	data, err := json.MarshalIndent(&sess, "", "  ")
	if err != nil {
		return err
	}
	meta := sessionMetaFromSession(sess, len(sess.Messages), len(sess.Messages), 0)
	if err := storage.WriteFileAtomic(snapshotPath, data, 0o600); err != nil {
		return err
	}
	if err := writeSessionMeta(metaPath, meta); err != nil {
		return err
	}
	if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func sessionLogPath(snapshotPath string) string {
	return strings.TrimSuffix(snapshotPath, checkpointSuffix) + logSuffix
}

func sessionMetaPath(snapshotPath string) string {
	return strings.TrimSuffix(snapshotPath, checkpointSuffix) + metaSuffix
}

func isSessionSnapshot(name string) bool {
	return strings.HasPrefix(name, checkpointPrefix) &&
		strings.HasSuffix(name, checkpointSuffix) &&
		!strings.HasSuffix(name, metaSuffix)
}

func appendSessionLog(path string, messages []llm.Message) (err error) {
	if len(messages) == 0 {
		return nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	writer := bufio.NewWriter(file)
	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		if _, err := writer.Write(data); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func readSessionLog(path string) (messages []llm.Message, err error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimSpace(line)
			if len(line) > 0 {
				var msg llm.Message
				if err := json.Unmarshal(line, &msg); err != nil {
					return nil, err
				}
				messages = append(messages, msg)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return messages, nil
}

func loadSessionMetaFile(path string) (sessionMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionMeta{}, err
	}
	var meta sessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return sessionMeta{}, err
	}
	if err := validateSessionVersion(meta.SchemaVersion); err != nil {
		return sessionMeta{}, err
	}
	return meta, nil
}

func writeSessionMeta(path string, meta sessionMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return storage.WriteFileAtomic(path, data, 0o600)
}

func sessionMetaFromSession(sess Session, messageCount, snapshotCount, deltaCount int) sessionMeta {
	return sessionMeta{
		SchemaVersion: sess.SchemaVersion,
		ID:            sess.ID,
		StartedAt:     sess.StartedAt,
		UpdatedAt:     sess.UpdatedAt,
		AuthType:      sess.AuthType,
		MessageCount:  messageCount,
		SnapshotCount: snapshotCount,
		DeltaCount:    deltaCount,
		PreviewText:   previewFromMessages(sess.Messages),
	}
}

func previewFromMessages(messages []llm.Message) string {
	fallback := ""
	for _, msg := range messages {
		if msg.Role != llm.RoleUser {
			continue
		}
		text := cleanPreviewText(joinTextParts(msg.Parts))
		if text == "" {
			continue
		}
		if fallback == "" {
			fallback = text
		}
		if strings.HasPrefix(text, "/") || strings.HasPrefix(text, "?") {
			continue
		}
		return trimPreview(text, 80)
	}
	if fallback != "" {
		return trimPreview(fallback, 80)
	}
	return "Empty conversation"
}

func joinTextParts(parts []llm.Part) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Text == "" {
			continue
		}
		b.WriteString(part.Text)
	}
	return b.String()
}

func cleanPreviewText(message string) string {
	message = strings.ReplaceAll(message, "\r", " ")
	message = strings.ReplaceAll(message, "\n", " ")
	var b strings.Builder
	for _, r := range message {
		if !unicode.IsPrint(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), " "))
}

func trimPreview(text string, maxLen int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if maxLen <= 0 || len(runes) <= maxLen {
		return text
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
