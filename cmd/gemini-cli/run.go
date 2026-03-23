package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/auth"
	"github.com/puremachinery/gemini-cli-go/internal/authselect"
	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/config"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
	"github.com/puremachinery/gemini-cli-go/internal/ui"
)

type runBundle struct {
	client       client.Client
	authType     string
	toolExecutor *tools.Executor
}

func warnYoloMode(mode tools.ApprovalMode) {
	if mode == tools.ApprovalModeYolo {
		fmt.Fprintln(os.Stderr, "Warning: yolo mode auto-approves all tools (including shell). Use with caution.")
	}
}

func configureHeadlessApprover(executor *tools.Executor, mode tools.ApprovalMode, headless bool) {
	if executor == nil || !headless {
		return
	}
	if mode == tools.ApprovalModeAutoEdit || mode == tools.ApprovalModeYolo || mode == tools.ApprovalModePlan {
		executor.Approver = tools.NewModeApprover(mode, nil)
	}
}

func runConfigured(
	ctx context.Context,
	workspaceRoot string,
	runtimeCfg runtimeConfig,
	headless bool,
	input io.Reader,
	prompt string,
	interrupt <-chan os.Signal,
) error {
	warnYoloMode(runtimeCfg.mode)
	bundle, err := buildRunBundle(ctx, workspaceRoot, runtimeCfg, headless, buildRunBundleOptions{
		allowLogin: !headless,
		promptForAuthChoice: func(ctx context.Context, state authselect.PromptState) (string, error) {
			if headless {
				return "", nil
			}
			return promptForStartupAuthChoice(ctx, state)
		},
	})
	if err != nil {
		return err
	}
	if headless {
		return ui.RunHeadless(ctx, ui.HeadlessOptions{
			Client:             bundle.client,
			Model:              runtimeCfg.modelName,
			Prompt:             prompt,
			ToolExecutor:       bundle.toolExecutor,
			Memory:             runtimeCfg.memoryState,
			ChatStore:          runtimeCfg.chatStore,
			AuthType:           bundle.authType,
			RenderMarkdown:     runtimeCfg.renderMarkdown,
			MarkdownWidth:      runtimeCfg.markdownWidth,
			Now:                time.Now,
			MaxSessionTurns:    runtimeCfg.maxSessionTurns,
			MaxHistoryMessages: runtimeCfg.maxHistoryMessages,
		})
	}
	return ui.Run(ctx, ui.RunOptions{
		Client:       bundle.client,
		Model:        runtimeCfg.modelName,
		ShowIntro:    true,
		Input:        input,
		ToolExecutor: bundle.toolExecutor,
		ChatStore:    runtimeCfg.chatStore,
		AuthType:     bundle.authType,
		AuthManager: &ui.AuthManager{
			GetPromptState: func(ctx context.Context) (ui.AuthPromptState, error) {
				state, err := inspectAuthState(ctx, auth.NewFileStore())
				if err != nil {
					return ui.AuthPromptState{}, err
				}
				return ui.AuthPromptState{
					SelectedType: state.selectedType,
					HasAPIKey:    state.hasAPIKey,
				}, nil
			},
			Activate: func(ctx context.Context, authType string) (ui.AuthBundle, error) {
				bundle, err := buildRunBundle(ctx, workspaceRoot, runtimeCfg, false, buildRunBundleOptions{
					allowLogin:     true,
					forcedAuthType: authType,
				})
				if err != nil {
					return ui.AuthBundle{}, err
				}
				return ui.AuthBundle{
					Client:       bundle.client,
					ToolExecutor: bundle.toolExecutor,
					AuthType:     bundle.authType,
				}, nil
			},
			Clear: clearAuthState,
		},
		Memory:         runtimeCfg.memoryState,
		ApprovalMode:   runtimeCfg.mode,
		RenderMarkdown: runtimeCfg.renderMarkdown,
		MarkdownWidth:  runtimeCfg.markdownWidth,
		ResolveModel: func(name string) string {
			return config.ResolveModel(name, runtimeCfg.previewFeatures)
		},
		PersistModel:       persistModelSelection,
		Interrupt:          interrupt,
		Now:                time.Now,
		MaxSessionTurns:    runtimeCfg.maxSessionTurns,
		MaxHistoryMessages: runtimeCfg.maxHistoryMessages,
	})
}

func buildRunBundle(ctx context.Context, workspaceRoot string, runtimeCfg runtimeConfig, headless bool, opts buildRunBundleOptions) (runBundle, error) {
	store := auth.NewFileStore()
	state, err := inspectAuthState(ctx, store)
	if err != nil {
		return runBundle{}, err
	}

	authType, err := resolveAuthType(ctx, state, opts)
	if err != nil {
		return runBundle{}, err
	}

	if authType == authselect.AuthTypeAPIKey {
		apiKey := os.Getenv("GEMINI_API_KEY")
		geminiClient, err := client.NewGeminiAPIClient(apiKey, client.GeminiAPIOptions{})
		if err != nil {
			return runBundle{}, err
		}
		if err := setSelectedAuthType(authselect.AuthTypeAPIKey); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update auth selection: %v\n", err)
		}
		toolExecutor := newToolExecutor(
			workspaceRoot,
			runtimeCfg.mode,
			headless,
			geminiClient,
			runtimeCfg.requireReadApproval,
			runtimeCfg.allowPrivateWebFetch,
		)
		configureHeadlessApprover(toolExecutor, runtimeCfg.mode, headless)
		return runBundle{
			client:       geminiClient,
			authType:     authselect.AuthTypeAPIKey,
			toolExecutor: toolExecutor,
		}, nil
	}

	provider := auth.NewGoogleProvider()
	creds, _, err := loginWithGoogle(ctx, provider, store, state.cachedCreds)
	if err != nil {
		return runBundle{}, err
	}
	if creds != nil && creds.AccountEmail != "" {
		manager := auth.NewAccountManager()
		if err := manager.Cache(creds.AccountEmail); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to cache account email: %v\n", err)
		}
	}
	if err := setSelectedAuthType(authselect.AuthTypeOAuthPersonal); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update auth selection: %v\n", err)
	}

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	authHTTPClient := client.NewAuthenticatedClient(provider, store, nil)

	var apiClient client.Client
	hasAccess, err := client.CheckCodeAssistAccess(ctx, authHTTPClient, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Code Assist access check failed: %v; falling back to Gemini API\n", err)
	}
	if hasAccess {
		apiClient = client.NewCodeAssistClient(provider, store, client.CodeAssistOptions{
			ProjectID: projectID,
		})
	} else {
		apiClient, err = client.NewGeminiOAuthClient(authHTTPClient, client.GeminiAPIOptions{})
		if err != nil {
			return runBundle{}, fmt.Errorf("create Gemini OAuth client: %w", err)
		}
	}

	toolExecutor := newToolExecutor(
		workspaceRoot,
		runtimeCfg.mode,
		headless,
		nil,
		runtimeCfg.requireReadApproval,
		runtimeCfg.allowPrivateWebFetch,
	)
	configureHeadlessApprover(toolExecutor, runtimeCfg.mode, headless)
	return runBundle{
		client:       apiClient,
		authType:     authselect.AuthTypeOAuthPersonal,
		toolExecutor: toolExecutor,
	}, nil
}

func runInteractive(approvalMode string, yoloMode bool) error {
	return runInteractiveWithInput(nil, approvalMode, yoloMode)
}

func runInteractiveWithInput(input io.Reader, approvalMode string, yoloMode bool) error {
	if input != nil {
		if _, ok := input.(io.ReadCloser); !ok {
			input = stdinClosingReader{Reader: input}
		}
	}

	setup, err := prepareRun(approvalMode, yoloMode, false)
	if err != nil {
		return err
	}
	defer setup.stop()
	return runConfigured(setup.ctx, setup.workspaceRoot, setup.runtimeCfg, false, input, "", setup.interrupt)
}

func runHeadless(prompt string, approvalMode string, yoloMode bool) error {
	setup, err := prepareRun(approvalMode, yoloMode, true)
	if err != nil {
		return err
	}
	defer setup.stop()
	return runConfigured(setup.ctx, setup.workspaceRoot, setup.runtimeCfg, true, nil, prompt, nil)
}

func newToolExecutor(workspaceRoot string, mode tools.ApprovalMode, headless bool, geminiClient client.Client, requireReadApproval bool, allowPrivateWebFetch bool) *tools.Executor {
	registry := tools.NewRegistry(tools.Context{
		WorkspaceRoot:        workspaceRoot,
		GeminiClient:         geminiClient,
		Headless:             headless,
		RequireReadApproval:  requireReadApproval,
		AllowPrivateWebFetch: allowPrivateWebFetch,
	})
	tools.FilterRegistryForApprovalMode(registry, mode, headless)
	return &tools.Executor{Registry: registry}
}

type runSetup struct {
	ctx           context.Context
	stop          func()
	interrupt     <-chan os.Signal
	runtimeCfg    runtimeConfig
	workspaceRoot string
}

func prepareRun(approvalMode string, yoloMode bool, headless bool) (runSetup, error) {
	ctx, stop, interrupt := newRunContext(headless)
	workspaceRoot, err := os.Getwd()
	if err != nil {
		stop()
		return runSetup{}, err
	}
	runtimeCfg, err := loadRuntimeConfig(ctx, workspaceRoot, approvalMode, yoloMode, headless)
	if err != nil {
		stop()
		return runSetup{}, err
	}
	return runSetup{
		ctx:           ctx,
		stop:          stop,
		interrupt:     interrupt,
		runtimeCfg:    runtimeCfg,
		workspaceRoot: workspaceRoot,
	}, nil
}

func newRunContext(headless bool) (context.Context, func(), <-chan os.Signal) {
	if headless {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		return ctx, stop, nil
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	stop := func() {
		signal.Stop(sigCh)
	}
	return context.Background(), stop, sigCh
}
