// Package ui provides non-interactive helpers for headless mode.
package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/memory"
	"github.com/puremachinery/gemini-cli-go/internal/session"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

// HeadlessOptions configures a single headless prompt execution.
type HeadlessOptions struct {
	Client             client.Client
	Model              string
	Prompt             string
	Output             io.Writer
	RenderMarkdown     bool
	MarkdownWidth      int
	ToolExecutor       *tools.Executor
	Memory             *memory.State
	ChatStore          session.Store
	AuthType           string
	Now                func() time.Time
	MaxSessionTurns    int
	MaxHistoryMessages int
}

// RunHeadless executes a single prompt in non-interactive mode.
func RunHeadless(ctx context.Context, opts HeadlessOptions) error {
	if opts.Client == nil {
		return errors.New("client is nil")
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return errors.New("prompt is required")
	}
	autoSaver := newAutoSessionSaver(opts.ChatStore, opts.AuthType, opts.Now)
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}
	writer := bufio.NewWriter(out)
	defer func() {
		if err := writer.Flush(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
	}()

	parts, err := buildPartsFromQuery(ctx, opts.Prompt, opts.ToolExecutor)
	if err != nil {
		return err
	}
	messages := []llm.Message{{
		Role:  llm.RoleUser,
		Parts: parts,
	}}

	if err := runModelLoop(ctx, writer, &messages, modelLoopOptions{
		Client:          opts.Client,
		Model:           opts.Model,
		ToolExecutor:    opts.ToolExecutor,
		Memory:          opts.Memory,
		RenderMarkdown:  opts.RenderMarkdown,
		MarkdownWidth:   opts.MarkdownWidth,
		ShowToolOutputs: false,
		PrependNewline:  false,
	}); err != nil {
		return err
	}
	if dropped := pruneMessages(&messages, opts.MaxHistoryMessages); dropped > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: trimmed %d message(s) to keep history within maxHistoryMessages=%d.\n", dropped, opts.MaxHistoryMessages)
	}
	if autoSaver != nil {
		if err := autoSaver.Save(messages); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "[%s] Warning: failed to auto-save session: %v\n", time.Now().Format(time.RFC3339), err)
		}
	}
	return nil
}
