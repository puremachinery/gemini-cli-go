package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/memory"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

func handleMemoryCommand(ctx context.Context, w io.Writer, line string, state *memory.State, approver tools.Approver) error {
	if state == nil {
		return errors.New("memory is not configured")
	}
	args := strings.TrimSpace(strings.TrimPrefix(line, "/memory"))
	if args == "" {
		return handleMemoryShow(w, state)
	}
	sub, rest := splitFirst(args)
	switch sub {
	case "show":
		return handleMemoryShow(w, state)
	case "refresh":
		return handleMemoryRefresh(w, state)
	case "list":
		return handleMemoryList(w, state)
	case "add":
		return handleMemoryAdd(ctx, w, rest, state, approver)
	default:
		_, err := fmt.Fprintf(w, "Unknown /memory command: %s\n", sub)
		return err
	}
}

func handleMemoryShow(w io.Writer, state *memory.State) error {
	if strings.TrimSpace(state.Content) == "" {
		_, err := fmt.Fprintln(w, "Memory is currently empty.")
		return err
	}
	_, err := fmt.Fprintf(w, "Current memory content from %d file(s):\n\n---\n%s\n---\n", len(state.FilePaths), state.Content)
	return err
}

func handleMemoryRefresh(w io.Writer, state *memory.State) error {
	if err := state.Refresh(); err != nil {
		return err
	}
	if strings.TrimSpace(state.Content) == "" {
		_, err := fmt.Fprintln(w, "Memory refreshed successfully. No memory content found.")
		return err
	}
	_, err := fmt.Fprintf(w, "Memory refreshed successfully. Loaded %d characters from %d file(s).\n", len(state.Content), len(state.FilePaths))
	return err
}

func handleMemoryList(w io.Writer, state *memory.State) error {
	if len(state.FilePaths) == 0 {
		_, err := fmt.Fprintln(w, "No GEMINI.md files in use.")
		return err
	}
	_, err := fmt.Fprintf(w, "There are %d GEMINI.md file(s) in use:\n\n%s\n", len(state.FilePaths), strings.Join(state.FilePaths, "\n"))
	return err
}

func handleMemoryAdd(ctx context.Context, w io.Writer, arg string, state *memory.State, approver tools.Approver) error {
	text := strings.TrimSpace(arg)
	if text == "" {
		_, err := fmt.Fprintln(w, "Usage: /memory add <text to remember>")
		return err
	}
	if approver == nil {
		return errors.New("memory save requires approval but no approver is configured")
	}
	memoryPath := state.GlobalPath()
	title := fmt.Sprintf("Confirm Memory Save: %s", tildePath(memoryPath))
	approved, err := approver.Confirm(ctx, tools.ConfirmationRequest{
		ToolName: tools.SaveMemoryToolName,
		Title:    title,
		Prompt:   "Proceed?",
	})
	if err != nil {
		return err
	}
	if !approved {
		_, err := fmt.Fprintln(w, "Memory save canceled.")
		return err
	}
	if err := memory.AddMemoryEntry(text, state.GlobalPath()); err != nil {
		return err
	}
	if err := state.Refresh(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Added memory: \"%s\"\n", text)
	return err
}

func tildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil {
		return path
	}
	if rel == "." {
		return "~"
	}
	return filepath.ToSlash(filepath.Join("~", rel))
}
