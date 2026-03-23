package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/session"
)

func handleResumeCommand(
	ctx context.Context,
	reader lineReader,
	w io.Writer,
	line string,
	messages *[]llm.Message,
	store session.Store,
	authType string,
) error {
	args := strings.TrimSpace(strings.TrimPrefix(line, "/resume"))
	if args != "" {
		return handleResumeSelection(w, args, messages, store, authType)
	}
	if store == nil {
		return errors.New("chat storage is not configured")
	}
	entries, err := listAutoSessionEntries(store, authType)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		_, err := fmt.Fprintln(w, chatNoCheckpointsMessage)
		return err
	}
	if _, err := fmt.Fprintln(w, "Auto-saved conversations:"); err != nil {
		return err
	}
	for i, entry := range entries {
		ts := sessionTimestamp(entry.Session)
		display := "Unknown"
		if !ts.IsZero() {
			display = ts.Local().Format("2006-01-02 15:04:05")
		}
		line := fmt.Sprintf("%d) %s (%s)", i+1, entry.Session.ID, display)
		if entry.MessageCount > 0 {
			line = fmt.Sprintf("%s [%d msg%s]", line, entry.MessageCount, pluralSuffix(entry.MessageCount))
		}
		if entry.Preview != "" {
			line = fmt.Sprintf("%s - %s", line, entry.Preview)
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	if writer, ok := w.(*bufio.Writer); ok {
		if err := writer.Flush(); err != nil {
			return err
		}
	} else if flusher, ok := w.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			return err
		}
	}
	if reader == nil {
		return errors.New("input reader is not configured")
	}
	selection, _, err := reader.ReadLine(ctx, "Resume which session? Enter number or tag (blank to cancel): ")
	eof := errors.Is(err, io.EOF)
	if err != nil && !eof {
		if errors.Is(err, errPromptInterrupted) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return nil
		}
		return err
	}
	selection = strings.TrimSpace(selection)
	if selection == "" {
		return nil
	}
	if idx, convErr := strconv.Atoi(selection); convErr == nil {
		if idx < 1 || idx > len(entries) {
			_, err := fmt.Fprintln(w, "Invalid selection.")
			return err
		}
		return handleChatResume(w, entries[idx-1].Session.ID, messages, store, authType, true)
	}
	return handleChatResume(w, selection, messages, store, authType, true)
}

type autoSessionEntry struct {
	Session      session.Session
	MessageCount int
	Preview      string
}

func filterAutoSessions(sessions []session.Session, authType string) []session.Session {
	out := make([]session.Session, 0, len(sessions))
	for _, sess := range sessions {
		if isAutoSessionID(sess.ID) {
			if authType != "" && sess.AuthType != "" && sess.AuthType != authType {
				continue
			}
			out = append(out, sess)
		}
	}
	return out
}

func handleResumeSelection(w io.Writer, selection string, messages *[]llm.Message, store session.Store, authType string) error {
	selection = strings.TrimSpace(selection)
	if selection == "" {
		return nil
	}
	if store == nil {
		return errors.New("chat storage is not configured")
	}
	if selection == "latest" {
		entries, err := listAutoSessionEntries(store, authType)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			_, err := fmt.Fprintln(w, chatNoCheckpointsMessage)
			return err
		}
		return handleChatResume(w, entries[len(entries)-1].Session.ID, messages, store, authType, true)
	}
	if idx, convErr := strconv.Atoi(selection); convErr == nil {
		entries, err := listAutoSessionEntries(store, authType)
		if err != nil {
			return err
		}
		if idx < 1 || idx > len(entries) {
			_, err := fmt.Fprintln(w, "Invalid selection.")
			return err
		}
		return handleChatResume(w, entries[idx-1].Session.ID, messages, store, authType, true)
	}
	return handleChatResume(w, selection, messages, store, authType, true)
}

func listAutoSessionEntries(store session.Store, authType string) ([]autoSessionEntry, error) {
	if store == nil {
		return nil, nil
	}
	if lister, ok := store.(interface {
		ListPreviews() ([]session.Preview, error)
	}); ok {
		previews, err := lister.ListPreviews()
		if err != nil {
			return nil, err
		}
		entries := make([]autoSessionEntry, 0, len(previews))
		for _, preview := range previews {
			entry := autoSessionEntry{Session: preview.Session}
			entry.MessageCount = preview.MessageCount
			entry.Preview = buildPreview(preview.PreviewText)
			if authType != "" && entry.Session.AuthType != "" && entry.Session.AuthType != authType {
				continue
			}
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(i, j int) bool {
			return sessionTimestamp(entries[i].Session).Before(sessionTimestamp(entries[j].Session))
		})
		return entries, nil
	}
	var sessions []session.Session
	var err error
	if lister, ok := store.(interface {
		ListWithMessages() ([]session.Session, error)
	}); ok {
		sessions, err = lister.ListWithMessages()
	} else {
		sessions, err = store.List()
	}
	if err != nil {
		return nil, err
	}
	autoSessions := filterAutoSessions(sessions, authType)
	entries := make([]autoSessionEntry, 0, len(autoSessions))
	for _, sess := range autoSessions {
		entry := autoSessionEntry{Session: sess}
		entry.MessageCount = len(sess.Messages)
		entry.Preview = sessionPreview(sess.Messages)
		if authType != "" && entry.Session.AuthType != "" && entry.Session.AuthType != authType {
			continue
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return sessionTimestamp(entries[i].Session).Before(sessionTimestamp(entries[j].Session))
	})
	return entries, nil
}

func buildPreview(text string) string {
	cleaned := cleanPreviewText(text)
	if cleaned == "" {
		return "Empty conversation"
	}
	return trimPreview(cleaned, 80)
}

func sessionPreview(messages []llm.Message) string {
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

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
