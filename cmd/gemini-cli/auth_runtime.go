package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/auth"
	"github.com/puremachinery/gemini-cli-go/internal/authselect"
)

var errAPIKeyNotConfigured = errors.New("GEMINI_API_KEY is not configured")

type authState struct {
	selectedType string
	hasAPIKey    bool
	cachedCreds  *auth.Credentials
}

type buildRunBundleOptions struct {
	allowLogin          bool
	forcedAuthType      string
	promptForAuthChoice func(context.Context, authselect.PromptState) (string, error)
}

func inspectAuthState(ctx context.Context, store auth.Store) (authState, error) {
	selectedType, err := getSelectedAuthType()
	if err != nil {
		return authState{}, err
	}
	state := authState{
		selectedType: authselect.NormalizeAuthType(selectedType),
		hasAPIKey:    strings.TrimSpace(os.Getenv("GEMINI_API_KEY")) != "",
	}
	creds, err := store.Load(ctx)
	if err == nil {
		state.cachedCreds = creds
		return state, nil
	}
	if os.IsNotExist(err) {
		return state, nil
	}
	return authState{}, err
}

func (s authState) promptState() authselect.PromptState {
	return authselect.PromptState{
		SelectedType: s.selectedType,
		HasAPIKey:    s.hasAPIKey,
	}
}

func resolveAuthType(ctx context.Context, state authState, opts buildRunBundleOptions) (string, error) {
	if forced := authselect.NormalizeAuthType(opts.forcedAuthType); forced != "" {
		if forced == authselect.AuthTypeAPIKey && !state.hasAPIKey {
			return "", errAPIKeyNotConfigured
		}
		return forced, nil
	}
	if !opts.allowLogin {
		if state.hasAPIKey {
			return authselect.AuthTypeAPIKey, nil
		}
		if state.cachedCreds != nil {
			return authselect.AuthTypeOAuthPersonal, nil
		}
		return "", errors.New("no cached credentials found; start `gemini-cli` interactively to sign in or set GEMINI_API_KEY")
	}
	if state.selectedType == authselect.AuthTypeAPIKey && !state.hasAPIKey && opts.promptForAuthChoice != nil {
		return opts.promptForAuthChoice(ctx, state.promptState())
	}
	if state.selectedType == authselect.AuthTypeAPIKey && state.hasAPIKey {
		return authselect.AuthTypeAPIKey, nil
	}
	if state.selectedType == authselect.AuthTypeOAuthPersonal {
		return authselect.AuthTypeOAuthPersonal, nil
	}
	if state.selectedType == "" && state.hasAPIKey {
		return authselect.AuthTypeAPIKey, nil
	}
	if state.cachedCreds != nil {
		return authselect.AuthTypeOAuthPersonal, nil
	}
	if opts.promptForAuthChoice == nil {
		if state.hasAPIKey {
			return authselect.AuthTypeAPIKey, nil
		}
		return authselect.AuthTypeOAuthPersonal, nil
	}
	return opts.promptForAuthChoice(ctx, state.promptState())
}

func promptForStartupAuthChoice(ctx context.Context, state authselect.PromptState) (string, error) {
	return promptForAuthChoice(ctx, os.Stdin, os.Stdout, state)
}

func promptForAuthChoice(ctx context.Context, in io.Reader, out io.Writer, state authselect.PromptState) (string, error) {
	reader := bufio.NewReader(in)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if _, err := fmt.Fprint(out, authselect.PromptText(state)); err != nil {
			return "", err
		}
		prompt := fmt.Sprintf("Select auth method [1/2] (default %d): ", authselect.DefaultOptionNumber(state))
		if _, err := fmt.Fprint(out, prompt); err != nil {
			return "", err
		}
		line, err := reader.ReadString('\n')
		eof := errors.Is(err, io.EOF)
		if err != nil && !eof {
			return "", err
		}
		choice, ok := authselect.ParseChoice(line, state)
		if !ok {
			if _, writeErr := fmt.Fprintln(out, "Invalid selection. Choose 1 for Google sign-in or 2 for API key."); writeErr != nil {
				return "", writeErr
			}
			if eof {
				return "", io.EOF
			}
			continue
		}
		if choice == authselect.AuthTypeAPIKey && !state.HasAPIKey {
			if _, writeErr := fmt.Fprintln(out, "GEMINI_API_KEY is not set. Configure it first or choose Google sign-in."); writeErr != nil {
				return "", writeErr
			}
			if eof {
				return "", io.EOF
			}
			continue
		}
		return choice, nil
	}
}

func loginWithGoogle(ctx context.Context, provider auth.Provider, store auth.Store, cached *auth.Credentials) (*auth.Credentials, bool, error) {
	if cached != nil {
		return cached, false, nil
	}
	creds, err := provider.Login(ctx)
	if err != nil {
		return nil, false, err
	}
	if err := store.Save(ctx, creds); err != nil {
		return nil, false, err
	}
	if creds.AccountEmail != "" {
		manager := auth.NewAccountManager()
		if err := manager.Cache(creds.AccountEmail); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to cache account email: %v\n", err)
		}
	}
	return creds, true, nil
}

func clearAuthState(ctx context.Context) error {
	store := auth.NewFileStore()
	if err := store.Clear(ctx); err != nil {
		return err
	}
	manager := auth.NewAccountManager()
	if err := manager.ClearActive(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to clear cached account email: %v\n", err)
	}
	return clearSelectedAuthType()
}
