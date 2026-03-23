package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/authselect"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

func handleAuthCommand(ctx context.Context, cmd commandContext, line string) error {
	if cmd.authManager == nil {
		return errors.New("authentication management is not configured")
	}
	args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "/auth")))
	if len(args) == 0 {
		return handleAuthLogin(ctx, cmd)
	}
	switch strings.ToLower(args[0]) {
	case "signin", "login":
		return handleAuthLogin(ctx, cmd)
	case "signout", "logout":
		return handleAuthLogout(ctx, cmd)
	case "help", "-h", "--help":
		_, err := fmt.Fprintln(cmd.writer, "Usage: /auth [signin|signout]")
		return err
	default:
		if _, err := fmt.Fprintf(cmd.writer, "Unknown auth command: %s\n", args[0]); err != nil {
			return err
		}
		_, err := fmt.Fprintln(cmd.writer, "Usage: /auth [signin|signout]")
		return err
	}
}

func handleAuthLogin(ctx context.Context, cmd commandContext) error {
	state, err := cmd.authManager.GetPromptState(ctx)
	if err != nil {
		return err
	}
	choice, err := promptForUIAuthChoice(ctx, cmd.reader, cmd.writer, authselect.PromptState{
		SelectedType: state.SelectedType,
		HasAPIKey:    state.HasAPIKey,
	})
	if err != nil || choice == "" {
		return err
	}
	bundle, err := cmd.authManager.Activate(ctx, choice)
	if err != nil {
		return err
	}
	if bundle.Client == nil {
		return errors.New("auth activation did not return a client")
	}
	if bundle.ToolExecutor != nil && bundle.ToolExecutor.Approver == nil {
		bundle.ToolExecutor.Approver = tools.NewModeApprover(cmd.approvalMode, cmd.baseApprover)
	}
	if cmd.runtime != nil {
		cmd.runtime.client = bundle.Client
		cmd.runtime.toolExecutor = bundle.ToolExecutor
		cmd.runtime.authType = bundle.AuthType
	}
	if cmd.autoSaver != nil {
		cmd.autoSaver.SetAuthType(bundle.AuthType)
	}
	_, err = fmt.Fprintf(cmd.writer, "Authentication set to %s.\n", authTypeLabel(bundle.AuthType))
	return err
}

func handleAuthLogout(ctx context.Context, cmd commandContext) error {
	if err := cmd.authManager.Clear(ctx); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(cmd.writer, "Signed out and cleared cached credentials."); err != nil {
		return err
	}
	if err := flushOutput(cmd.writer); err != nil {
		return err
	}
	for {
		line, _, err := cmd.reader.ReadLine(ctx, "Sign in again now? [Y/n]: ")
		eof := errors.Is(err, io.EOF)
		if err != nil && !eof {
			if errors.Is(err, errPromptInterrupted) || errors.Is(err, os.ErrClosed) || ctx.Err() != nil {
				return printQuit(cmd.writer)
			}
			return err
		}
		answer := strings.TrimSpace(strings.ToLower(line))
		if eof && answer == "" {
			return printQuit(cmd.writer)
		}
		if answer == "" || answer == "y" || answer == "yes" {
			return handleAuthLogin(ctx, cmd)
		}
		if answer == "n" || answer == "no" || eof {
			return printQuit(cmd.writer)
		}
		if _, err := fmt.Fprintln(cmd.writer, "Please answer yes or no."); err != nil {
			return err
		}
		if err := flushOutput(cmd.writer); err != nil {
			return err
		}
	}
}

func promptForUIAuthChoice(ctx context.Context, reader lineReader, writer io.Writer, state authselect.PromptState) (string, error) {
	for {
		if _, err := fmt.Fprint(writer, authselect.PromptText(state)); err != nil {
			return "", err
		}
		if err := flushOutput(writer); err != nil {
			return "", err
		}
		prompt := fmt.Sprintf("Select auth method [1/2] (default %d): ", authselect.DefaultOptionNumber(state))
		line, _, err := reader.ReadLine(ctx, prompt)
		eof := errors.Is(err, io.EOF)
		if err != nil && !eof {
			if errors.Is(err, errPromptInterrupted) || errors.Is(err, os.ErrClosed) || ctx.Err() != nil {
				return "", nil
			}
			return "", err
		}
		if eof && line == "" {
			return "", nil
		}
		choice, ok := authselect.ParseChoice(line, state)
		if !ok {
			if _, err := fmt.Fprintln(writer, "Invalid selection. Choose 1 for Google sign-in or 2 for API key."); err != nil {
				return "", err
			}
			if eof {
				return "", nil
			}
			continue
		}
		if choice == authselect.AuthTypeAPIKey && !state.HasAPIKey {
			if _, err := fmt.Fprintln(writer, "GEMINI_API_KEY is not set. Configure it first or choose Google sign-in."); err != nil {
				return "", err
			}
			if eof {
				return "", nil
			}
			continue
		}
		return choice, nil
	}
}

func authTypeLabel(authType string) string {
	switch authType {
	case authselect.AuthTypeAPIKey:
		return "Gemini API key"
	case authselect.AuthTypeOAuthPersonal:
		return "Google sign-in"
	default:
		return authType
	}
}

func flushOutput(w io.Writer) error {
	if flusher, ok := w.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}
