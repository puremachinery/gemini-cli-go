package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/config"
	"github.com/puremachinery/gemini-cli-go/internal/memory"
	"github.com/puremachinery/gemini-cli-go/internal/session"
	"github.com/puremachinery/gemini-cli-go/internal/storage"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

type runtimeConfig struct {
	loadResult           *config.LoadResult
	modelName            string
	previewFeatures      bool
	mode                 tools.ApprovalMode
	renderMarkdown       bool
	markdownWidth        int
	memoryState          *memory.State
	chatStore            session.Store
	maxSessionTurns      int
	maxHistoryMessages   int
	requireReadApproval  bool
	allowPrivateWebFetch bool
}

func loadRuntimeConfig(ctx context.Context, workspaceRoot, approvalMode string, yoloMode bool, headless bool) (runtimeConfig, error) {
	loader := config.Loader{}
	loadResult, err := loader.Load(ctx, workspaceRoot)
	if err != nil {
		return runtimeConfig{}, err
	}
	modelName := config.DefaultGeminiModelAuto
	requestedModel := ""
	if loadResult != nil {
		if value, ok := loadResult.Merged.GetString("model.name"); ok && value != "" {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				requestedModel = trimmed
				modelName = trimmed
			}
		}
	}
	previewFeatures := false
	if loadResult != nil {
		if value, ok := loadResult.Merged.Get("general.previewFeatures"); ok {
			if flag, ok := value.(bool); ok {
				previewFeatures = flag
			}
		}
	}
	modelName = config.ResolveModel(modelName, previewFeatures)
	if requestedModel != "" && !config.IsKnownModelAlias(requestedModel) && !config.IsKnownModelName(requestedModel) {
		if !strings.HasPrefix(requestedModel, "gemini-") && !strings.HasPrefix(requestedModel, "models/") {
			fmt.Fprintf(os.Stderr, "Warning: using custom model name %q; ensure it is valid.\n", requestedModel)
		}
	}
	mode, err := resolveApprovalMode(loadResult, approvalMode, yoloMode)
	if err != nil {
		return runtimeConfig{}, err
	}
	maxSessionTurns := 0
	if loadResult != nil {
		if value, ok := loadResult.Merged.Get("model.maxSessionTurns"); ok {
			parsed, err := parseOptionalIntSetting(value)
			if err != nil {
				return runtimeConfig{}, fmt.Errorf("invalid model.maxSessionTurns: %w", err)
			}
			if parsed < 0 {
				return runtimeConfig{}, errors.New("model.maxSessionTurns must be non-negative")
			}
			maxSessionTurns = parsed
		}
	}
	maxHistoryMessages := 0
	if loadResult != nil {
		if value, ok := loadResult.Merged.Get("model.maxHistoryMessages"); ok {
			parsed, err := parseOptionalIntSetting(value)
			if err != nil {
				return runtimeConfig{}, fmt.Errorf("invalid model.maxHistoryMessages: %w", err)
			}
			if parsed < 0 {
				return runtimeConfig{}, errors.New("model.maxHistoryMessages must be non-negative")
			}
			maxHistoryMessages = parsed
		}
	}
	renderMarkdown := renderMarkdownEnabled(loadResult, headless)
	markdownWidth := stdoutWidth()
	memoryState := memory.NewState(workspaceRoot)
	if err := memoryState.Refresh(); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] Warning: failed to load memory (%s): %v\n", time.Now().Format(time.RFC3339), memoryState.GlobalPath(), err)
	}
	chatStore := session.NewFileStore(storage.ProjectChatsDir(workspaceRoot))
	requireReadApproval := false
	allowPrivateWebFetch := false
	if loadResult != nil {
		if value, ok := loadResult.Merged.Get("tools.requireReadApproval"); ok {
			flag, ok := value.(bool)
			if !ok {
				return runtimeConfig{}, errors.New("tools.requireReadApproval must be a boolean")
			}
			requireReadApproval = flag
		}
		if value, ok := loadResult.Merged.Get("tools.webFetch.allowPrivate"); ok {
			flag, ok := value.(bool)
			if !ok {
				return runtimeConfig{}, errors.New("tools.webFetch.allowPrivate must be a boolean")
			}
			allowPrivateWebFetch = flag
		}
	}
	return runtimeConfig{
		loadResult:           loadResult,
		modelName:            modelName,
		previewFeatures:      previewFeatures,
		mode:                 mode,
		renderMarkdown:       renderMarkdown,
		markdownWidth:        markdownWidth,
		memoryState:          memoryState,
		chatStore:            chatStore,
		maxSessionTurns:      maxSessionTurns,
		maxHistoryMessages:   maxHistoryMessages,
		requireReadApproval:  requireReadApproval,
		allowPrivateWebFetch: allowPrivateWebFetch,
	}, nil
}

func parseOptionalIntSetting(value any) (int, error) {
	switch typed := value.(type) {
	case json.Number:
		raw := typed.String()
		if strings.ContainsAny(raw, ".eE") {
			return 0, fmt.Errorf("expected integer, got %q", raw)
		}
		parsed, err := typed.Int64()
		if err != nil {
			return 0, err
		}
		return int(parsed), nil
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("expected integer, got %v", typed)
		}
		return int(typed), nil
	case float32:
		if math.Trunc(float64(typed)) != float64(typed) {
			return 0, fmt.Errorf("expected integer, got %v", typed)
		}
		return int(typed), nil
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case int32:
		return int(typed), nil
	default:
		return 0, fmt.Errorf("expected integer, got %T", value)
	}
}

func resolveApprovalMode(loadResult *config.LoadResult, flagMode string, yolo bool) (tools.ApprovalMode, error) {
	flagMode = strings.TrimSpace(flagMode)
	if flagMode != "" {
		mode, err := tools.NormalizeApprovalMode(flagMode)
		if err != nil {
			return "", err
		}
		if mode == tools.ApprovalModePlan && !planEnabled(loadResult) {
			return "", errors.New("plan approval mode requires experimental.plan=true in settings")
		}
		return mode, nil
	}
	if yolo {
		return tools.ApprovalModeYolo, nil
	}
	if loadResult != nil {
		if raw, ok := loadResult.Merged.GetString("tools.approvalMode"); ok {
			raw = strings.TrimSpace(raw)
			if raw != "" {
				if strings.EqualFold(raw, string(tools.ApprovalModeYolo)) {
					return "", errors.New("tools.approvalMode cannot be yolo; use --yolo instead")
				}
				mode, err := tools.NormalizeApprovalMode(raw)
				if err != nil {
					return "", err
				}
				if mode == tools.ApprovalModePlan && !planEnabled(loadResult) {
					return "", errors.New("plan approval mode requires experimental.plan=true in settings")
				}
				return mode, nil
			}
		}
	}
	return tools.ApprovalModeDefault, nil
}

func planEnabled(loadResult *config.LoadResult) bool {
	if loadResult == nil {
		return false
	}
	value, ok := loadResult.Merged.Get("experimental.plan")
	if !ok {
		return false
	}
	enabled, ok := value.(bool)
	return ok && enabled
}

func renderMarkdownEnabled(loadResult *config.LoadResult, headless bool) bool {
	if headless {
		return false
	}
	if !stdoutIsTTY() {
		return false
	}
	if loadResult == nil {
		return true
	}
	if value, ok := loadResult.Merged.Get("ui.accessibility.screenReader"); ok {
		if flag, ok := value.(bool); ok && flag {
			return false
		}
	}
	return true
}
