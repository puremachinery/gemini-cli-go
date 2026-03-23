package ui

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/llmfmt"
	"github.com/puremachinery/gemini-cli-go/internal/session"
)

const (
	chatUsage                = "Usage: /chat <list|save|resume|delete|share> [arg]"
	chatNoCheckpointsMessage = "No saved conversation checkpoints found."
	replayMaxMessages        = 20
	replayMaxChars           = 8000
)

func handleChatCommand(w io.Writer, line string, messages *[]llm.Message, store session.Store, authType string, now func() time.Time) error {
	args := strings.TrimSpace(strings.TrimPrefix(line, "/chat"))
	if args == "" {
		_, err := fmt.Fprintln(w, chatUsage)
		return err
	}
	sub, rest := splitFirst(args)
	switch sub {
	case "list":
		return handleChatList(w, store)
	case "save":
		return handleChatSave(w, rest, messages, store, authType, now)
	case "resume", "load":
		return handleChatResume(w, rest, messages, store, authType, true)
	case "delete":
		return handleChatDelete(w, rest, store)
	case "share":
		return handleChatShare(w, rest, messages, now)
	default:
		_, err := fmt.Fprintf(w, "Unknown /chat command: %s\n", sub)
		return err
	}
}

func splitFirst(input string) (string, string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ""
	}
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", ""
	}
	sub := parts[0]
	rest := strings.TrimSpace(strings.TrimPrefix(input, sub))
	return sub, rest
}

func handleChatList(w io.Writer, store session.Store) error {
	if store == nil {
		return errors.New("chat storage is not configured")
	}
	sessions, err := store.List()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		_, err := fmt.Fprintln(w, chatNoCheckpointsMessage)
		return err
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessionTimestamp(sessions[i]).Before(sessionTimestamp(sessions[j]))
	})
	if _, err := fmt.Fprintln(w, "Saved conversation checkpoints:"); err != nil {
		return err
	}
	for _, sess := range sessions {
		ts := sessionTimestamp(sess)
		display := "Unknown"
		if !ts.IsZero() {
			display = ts.Local().Format("2006-01-02 15:04:05")
		}
		if _, err := fmt.Fprintf(w, "- %s (%s)\n", sess.ID, display); err != nil {
			return err
		}
	}
	return nil
}

func handleChatSave(w io.Writer, tag string, messages *[]llm.Message, store session.Store, authType string, now func() time.Time) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		_, err := fmt.Fprintln(w, "Missing tag. Usage: /chat save <tag>")
		return err
	}
	if store == nil {
		return errors.New("chat storage is not configured")
	}
	if messages == nil || len(*messages) == 0 {
		_, err := fmt.Fprintln(w, "No conversation found to save.")
		return err
	}
	overwrote := false
	if checker, ok := store.(interface{ Exists(string) (bool, error) }); ok {
		exists, err := checker.Exists(tag)
		if err == nil && exists {
			overwrote = true
		}
	}
	timestamp := now()
	sess := &session.Session{
		ID:        tag,
		StartedAt: timestamp,
		UpdatedAt: timestamp,
		AuthType:  authType,
		Messages:  append([]llm.Message{}, (*messages)...),
	}
	if err := store.Save(sess); err != nil {
		return err
	}
	if overwrote {
		if _, err := fmt.Fprintf(w, "Conversation checkpoint saved with tag: %s (overwritten).\n", session.DecodeTagName(tag)); err != nil {
			return err
		}
		return nil
	}
	_, err := fmt.Fprintf(w, "Conversation checkpoint saved with tag: %s.\n", session.DecodeTagName(tag))
	return err
}

func handleChatResume(w io.Writer, tag string, messages *[]llm.Message, store session.Store, authType string, replay bool) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		_, err := fmt.Fprintln(w, "Missing tag. Usage: /chat resume <tag>")
		return err
	}
	if store == nil {
		return errors.New("chat storage is not configured")
	}
	sess, err := store.Load(tag)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			_, err := fmt.Fprintf(w, "No saved checkpoint found with tag: %s.\n", session.DecodeTagName(tag))
			return err
		}
		return err
	}
	if sess == nil || len(sess.Messages) == 0 {
		_, err := fmt.Fprintf(w, "No saved checkpoint found with tag: %s.\n", session.DecodeTagName(tag))
		return err
	}
	if sess.AuthType != "" && authType != "" && sess.AuthType != authType {
		_, err := fmt.Fprintf(w, "Cannot resume chat. It was saved with a different authentication method (%s) than the current one (%s).\n", sess.AuthType, authType)
		return err
	}
	if messages != nil {
		*messages = append([]llm.Message{}, sess.Messages...)
	}
	_, err = fmt.Fprintf(w, "Resumed conversation from tag: %s.\n", session.DecodeTagName(tag))
	if err != nil {
		return err
	}
	if replay {
		return replayConversation(w, sess.Messages)
	}
	return nil
}

func handleChatDelete(w io.Writer, tag string, store session.Store) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		_, err := fmt.Fprintln(w, "Missing tag. Usage: /chat delete <tag>")
		return err
	}
	if store == nil {
		return errors.New("chat storage is not configured")
	}
	if err := store.Delete(tag); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			_, err := fmt.Fprintf(w, "Error: No checkpoint found with tag '%s'.\n", session.DecodeTagName(tag))
			return err
		}
		return err
	}
	_, err := fmt.Fprintf(w, "Conversation checkpoint '%s' has been deleted.\n", session.DecodeTagName(tag))
	return err
}

func handleChatShare(w io.Writer, arg string, messages *[]llm.Message, now func() time.Time) error {
	filePath := strings.TrimSpace(arg)
	if filePath == "" {
		filePath = fmt.Sprintf("gemini-conversation-%d.json", now().UnixMilli())
	}
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}
	if messages == nil || len(*messages) == 0 {
		_, err := fmt.Fprintln(w, "No conversation found to share.")
		return err
	}
	if err := session.ExportHistoryToFile(*messages, abs); err != nil {
		_, writeErr := fmt.Fprintf(w, "Error sharing conversation: %v\n", err)
		if writeErr != nil {
			return writeErr
		}
		return nil
	}
	_, err = fmt.Fprintf(w, "Conversation shared to %s\n", abs)
	return err
}

func sessionTimestamp(sess session.Session) time.Time {
	if !sess.UpdatedAt.IsZero() {
		return sess.UpdatedAt
	}
	return sess.StartedAt
}

func replayConversation(w io.Writer, messages []llm.Message) error {
	if len(messages) == 0 {
		return nil
	}
	start := 0
	if len(messages) > replayMaxMessages {
		start = len(messages) - replayMaxMessages
	}
	header := "Conversation replay:"
	if start > 0 {
		header = fmt.Sprintf("Conversation replay (last %d of %d messages):", len(messages)-start, len(messages))
	}
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	remaining := replayMaxChars
	truncated := false
	for _, msg := range messages[start:] {
		cont, err := writeReplayMessage(w, msg, &remaining)
		if err != nil {
			return err
		}
		if !cont {
			truncated = true
			break
		}
	}
	if truncated {
		if _, err := fmt.Fprintln(w, "[replay truncated]"); err != nil {
			return err
		}
	}
	return nil
}

func writeReplayMessage(w io.Writer, msg llm.Message, remaining *int) (bool, error) {
	text := strings.TrimSpace(joinTextParts(msg.Parts))
	if text != "" {
		cont, err := writeReplayBlock(w, roleLabel(msg.Role), text, remaining)
		if err != nil || !cont {
			return cont, err
		}
	}
	for _, part := range msg.Parts {
		if part.FunctionCall != nil {
			cont, err := writeReplayBlock(w, "Tool call", llmfmt.FormatFunctionCallInline(part.FunctionCall), remaining)
			if err != nil || !cont {
				return cont, err
			}
		}
		if part.FunctionResponse != nil {
			cont, err := writeReplayBlock(w, "Tool response", llmfmt.FormatFunctionResponseInline(part.FunctionResponse), remaining)
			if err != nil || !cont {
				return cont, err
			}
		}
	}
	return true, nil
}

func writeReplayBlock(w io.Writer, label, content string, remaining *int) (bool, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return true, nil
	}
	if remaining != nil && *remaining <= 0 {
		return false, nil
	}
	truncated := false
	if remaining != nil {
		runes := []rune(content)
		if len(runes) > *remaining {
			if *remaining <= 3 {
				content = string(runes[:*remaining])
			} else {
				content = string(runes[:*remaining-3]) + "..."
			}
			truncated = true
			*remaining = 0
		} else {
			*remaining -= len(runes)
		}
	}
	if _, err := fmt.Fprintf(w, "%s:\n", label); err != nil {
		return false, err
	}
	if _, err := fmt.Fprintln(w, content); err != nil {
		return false, err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return false, err
	}
	if truncated {
		return false, nil
	}
	return true, nil
}

func roleLabel(role llm.Role) string {
	switch role {
	case llm.RoleUser:
		return "User"
	case llm.RoleSystem:
		return "System"
	default:
		return "Assistant"
	}
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
