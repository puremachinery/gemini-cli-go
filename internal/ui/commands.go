package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/memory"
	"github.com/puremachinery/gemini-cli-go/internal/session"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

const (
	quitMessage    = "Agent powering down. Goodbye!"
	helpMessage    = "Available commands:\n  /help                 Show this help message\n  /auth [cmd]           Manage authentication\n  /model [name]         Show or set the active model\n  /chat <cmd> [args]    Manage conversation checkpoints\n  /resume [tag]         Browse and resume auto-saved conversations\n  /memory <cmd> [args]  Manage GEMINI.md memory\n  /clear                Clear the screen and reset the conversation\n  /quit                 Exit Gemini CLI\n\nKeyboard shortcuts:\n  Up/Down               Cycle prompt history\n  Ctrl+J                Insert newline (multiline)\n  Ctrl+C                Quit application"
	clearANSIReset = "\033[2J\033[H"
)

type commandContext struct {
	reader         lineReader
	writer         io.Writer
	messages       *[]llm.Message
	showIntro      bool
	model          *string
	resolveModel   func(string) string
	persistModel   func(string) error
	chatStore      session.Store
	runtime        *runtimeState
	authManager    *AuthManager
	autoSaver      *autoSessionSaver
	approvalMode   tools.ApprovalMode
	baseApprover   tools.Approver
	memoryState    *memory.State
	memoryApprover tools.Approver
	now            func() time.Time
}

func handleCommand(ctx context.Context, cmd commandContext, line string) (bool, error) {
	if !strings.HasPrefix(line, "/") {
		return false, nil
	}
	trimmed := strings.TrimSpace(line)
	if handled, err := handleBuiltinCommand(cmd, trimmed); handled {
		return true, err
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return true, nil
	}
	if handled, err := handleSubcommand(ctx, cmd, trimmed, fields[0]); handled {
		return true, err
	}
	return true, printUnknownCommand(cmd.writer, line)
}

func handleModelCommand(w io.Writer, line string, model *string, resolveModel func(string) string, persistModel func(string) error) error {
	arg := strings.TrimSpace(strings.TrimPrefix(line, "/model"))
	if arg == "" {
		current := ""
		if model != nil {
			current = *model
		}
		if current == "" {
			current = "auto"
		}
		if _, err := fmt.Fprintf(w, "Current model: %s\n", current); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w, "Usage: /model <auto|pro|flash|flash-lite|auto-gemini-2.5|auto-gemini-3|model-name>")
		return err
	}
	if persistModel == nil {
		return errors.New("model persistence is not configured")
	}
	if err := persistModel(arg); err != nil {
		return err
	}
	resolved := arg
	if resolveModel != nil {
		resolved = resolveModel(arg)
	}
	if model != nil {
		*model = resolved
	}
	_, err := fmt.Fprintf(w, "Model set to %s\n", resolved)
	return err
}

func handleBuiltinCommand(cmd commandContext, trimmed string) (bool, error) {
	switch trimmed {
	case "/help":
		return true, printHelp(cmd.writer)
	case "/clear":
		return true, clearConversation(cmd)
	case "/quit", "/exit":
		return true, printQuit(cmd.writer)
	default:
		return false, nil
	}
}

func handleSubcommand(ctx context.Context, cmd commandContext, trimmed, name string) (bool, error) {
	authType := ""
	if cmd.runtime != nil {
		authType = cmd.runtime.authType
	}
	switch name {
	case "/auth":
		return true, handleAuthCommand(ctx, cmd, trimmed)
	case "/resume":
		return true, handleResumeCommand(ctx, cmd.reader, cmd.writer, trimmed, cmd.messages, cmd.chatStore, authType)
	case "/model":
		return true, handleModelCommand(cmd.writer, trimmed, cmd.model, cmd.resolveModel, cmd.persistModel)
	case "/chat":
		return true, handleChatCommand(cmd.writer, trimmed, cmd.messages, cmd.chatStore, authType, cmd.now)
	case "/memory":
		return true, handleMemoryCommand(ctx, cmd.writer, trimmed, cmd.memoryState, cmd.memoryApprover)
	default:
		return false, nil
	}
}

func clearConversation(cmd commandContext) error {
	if _, err := fmt.Fprint(cmd.writer, clearANSIReset); err != nil {
		return err
	}
	if cmd.showIntro {
		if _, err := fmt.Fprintln(cmd.writer, strings.TrimPrefix(introText, "\n")); err != nil {
			return err
		}
	}
	if cmd.messages != nil {
		*cmd.messages = nil
	}
	return nil
}

func printHelp(w io.Writer) error {
	_, err := fmt.Fprintln(w, helpMessage)
	return err
}

func printQuit(w io.Writer) error {
	if _, err := fmt.Fprintln(w, quitMessage); err != nil {
		return err
	}
	return errQuit
}

func printUnknownCommand(w io.Writer, line string) error {
	_, err := fmt.Fprintf(w, "Unknown command: %s\n", strings.TrimSpace(line))
	return err
}
