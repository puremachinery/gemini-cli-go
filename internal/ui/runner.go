// Package ui provides the minimal interactive terminal UI.
package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/memory"
	"github.com/puremachinery/gemini-cli-go/internal/session"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

const introText = `
 ███         █████████
░░░███      ███░░░░░███
  ░░░███   ███     ░░░
    ░░░███░███
     ███░ ░███    █████
   ███░   ░░███  ░░███
 ███░      ░░█████████
░░░         ░░░░░░░░░░

Tips for getting started:
1. Ask questions, edit files, or run commands.
2. Be specific for the best results.
3. Create GEMINI.md files to customize your interactions with Gemini.
4. /help for more information.
`

const (
	promptPrefix  = "> "
	maxModelTurns = 50
)

var errQuit = errors.New("quit")

// RunOptions configures the interactive UI.
type RunOptions struct {
	Client             client.Client
	Model              string
	Input              io.Reader
	Output             io.Writer
	ShowIntro          bool
	RenderMarkdown     bool
	MarkdownWidth      int
	InitialMsgs        []llm.Message
	ToolExecutor       *tools.Executor
	ApprovalMode       tools.ApprovalMode
	ChatStore          session.Store
	AuthType           string
	AuthManager        *AuthManager
	Memory             *memory.State
	ResolveModel       func(string) string
	PersistModel       func(string) error
	Interrupt          <-chan os.Signal
	Now                func() time.Time
	MaxSessionTurns    int
	MaxHistoryMessages int
}

type AuthPromptState struct {
	SelectedType string
	HasAPIKey    bool
}

type AuthBundle struct {
	Client       client.Client
	ToolExecutor *tools.Executor
	AuthType     string
}

type AuthManager struct {
	GetPromptState func(context.Context) (AuthPromptState, error)
	Activate       func(context.Context, string) (AuthBundle, error)
	Clear          func(context.Context) error
}

type runtimeState struct {
	client       client.Client
	toolExecutor *tools.Executor
	authType     string
}

// Run starts the interactive UI loop.
func Run(ctx context.Context, opts RunOptions) error {
	if opts.Client == nil {
		return errors.New("client is nil")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	in := opts.Input
	if in == nil {
		in = os.Stdin
	}
	var inputCloser io.Closer
	if rc, ok := in.(io.ReadCloser); ok {
		inputCloser = rc
	}
	var closeInputOnce sync.Once
	closeInput := func() {
		if inputCloser == nil {
			return
		}
		closeInputOnce.Do(func() {
			if err := inputCloser.Close(); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err)
			}
		})
	}
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}
	inputReader, outputWriter, err := newLineReader(in, out)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(outputWriter)
	defer func() {
		if err := inputReader.Close(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
	}()
	defer func() {
		if err := writer.Flush(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
	}()
	if inputCloser != nil {
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				closeInput()
			case <-done:
			}
		}()
		defer close(done)
	}
	type turnState struct {
		mu       sync.Mutex
		inFlight bool
		cancel   context.CancelFunc
	}
	var (
		quitRequested atomic.Bool
		state         turnState
	)
	setTurnState := func(cancel context.CancelFunc, inFlight bool) {
		state.mu.Lock()
		state.cancel = cancel
		state.inFlight = inFlight
		state.mu.Unlock()
	}
	cancelIfActive := func() bool {
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.inFlight && state.cancel != nil {
			state.cancel()
			return true
		}
		return false
	}
	if opts.Interrupt != nil {
		done := make(chan struct{})
		go func() {
			for {
				select {
				case <-done:
					return
				case _, ok := <-opts.Interrupt:
					if !ok {
						return
					}
					if cancelIfActive() {
						continue
					}
					quitRequested.Store(true)
					closeInput()
				}
			}
		}()
		defer close(done)
	}
	baseApprover := &promptApprover{
		reader: inputReader,
		writer: writer,
	}
	if opts.ToolExecutor != nil && opts.ToolExecutor.Approver == nil {
		opts.ToolExecutor.Approver = tools.NewModeApprover(opts.ApprovalMode, baseApprover)
	}

	if opts.ShowIntro {
		if _, err := fmt.Fprintln(writer, strings.TrimPrefix(introText, "\n")); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}

	autoSaver := newAutoSessionSaver(opts.ChatStore, opts.AuthType, opts.Now)
	historyWarningShown := false
	messages := append([]llm.Message{}, opts.InitialMsgs...)
	toolsAnnounced := false
	currentModel := opts.Model
	runtime := &runtimeState{
		client:       opts.Client,
		toolExecutor: opts.ToolExecutor,
		authType:     opts.AuthType,
	}
	resolveModel := opts.ResolveModel
	if resolveModel == nil {
		resolveModel = func(model string) string {
			return model
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if opts.MaxSessionTurns > 0 && assistantTurnCount(messages) >= opts.MaxSessionTurns {
			return fmt.Errorf("reached max session turns for this session; increase the number of turns by specifying maxSessionTurns in settings.json")
		}

		line, eof, err := readUserInput(ctx, inputReader, promptPrefix)
		if err != nil {
			if quitRequested.Load() {
				return nil
			}
			if errors.Is(err, errPromptInterrupted) {
				return nil
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, os.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		if eof && line == "" {
			return nil
		}
		if strings.TrimSpace(line) == "" {
			if eof {
				return nil
			}
			continue
		}
		if err := inputReader.SaveHistory(normalizeHistoryLine(line)); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "Warning: failed to save input history:", err)
		}

		memoryApprover := tools.NewModeApprover(opts.ApprovalMode, baseApprover)
		if runtime.toolExecutor != nil && runtime.toolExecutor.Approver != nil {
			memoryApprover = runtime.toolExecutor.Approver
		}

		cmdCtx := commandContext{
			reader:         inputReader,
			writer:         writer,
			messages:       &messages,
			showIntro:      opts.ShowIntro,
			model:          &currentModel,
			resolveModel:   resolveModel,
			persistModel:   opts.PersistModel,
			chatStore:      opts.ChatStore,
			runtime:        runtime,
			authManager:    opts.AuthManager,
			autoSaver:      autoSaver,
			approvalMode:   opts.ApprovalMode,
			baseApprover:   baseApprover,
			memoryState:    opts.Memory,
			memoryApprover: memoryApprover,
			now:            opts.Now,
		}
		if handled, err := handleCommand(ctx, cmdCtx, line); err != nil {
			if errors.Is(err, errQuit) {
				return nil
			}
			return err
		} else if handled {
			if err := writer.Flush(); err != nil {
				return err
			}
			if eof {
				return nil
			}
			continue
		}

		parts, err := buildPartsFromQuery(ctx, line, runtime.toolExecutor)
		if err != nil {
			if _, writeErr := fmt.Fprintln(writer, err); writeErr != nil {
				return writeErr
			}
			if err := writer.Flush(); err != nil {
				return err
			}
			if eof {
				return nil
			}
			continue
		}

		messages = append(messages, llm.Message{
			Role:  llm.RoleUser,
			Parts: parts,
		})

		turnCtx, cancel := context.WithCancel(ctx)
		setTurnState(cancel, true)
		err = runModelLoop(turnCtx, writer, &messages, modelLoopOptions{
			Client:          runtime.client,
			Model:           currentModel,
			ToolExecutor:    runtime.toolExecutor,
			Memory:          opts.Memory,
			RenderMarkdown:  opts.RenderMarkdown,
			MarkdownWidth:   opts.MarkdownWidth,
			ToolsAnnounced:  &toolsAnnounced,
			ShowToolOutputs: true,
			PrependNewline:  true,
		})
		setTurnState(nil, false)
		cancel()
		if err != nil {
			return err
		}
		if dropped := pruneMessages(&messages, opts.MaxHistoryMessages); dropped > 0 && !historyWarningShown {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: trimmed %d message(s) to keep history within maxHistoryMessages=%d.\n", dropped, opts.MaxHistoryMessages)
			historyWarningShown = true
		}
		if autoSaver != nil {
			if err := autoSaver.Save(messages); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "[%s] Warning: failed to auto-save session: %v\n", time.Now().Format(time.RFC3339), err)
			}
		}
		if eof {
			return nil
		}
	}
}

func appendAssistantMessage(messages *[]llm.Message, text string, toolCalls []llm.FunctionCall) {
	if text == "" && len(toolCalls) == 0 {
		return
	}
	parts := make([]llm.Part, 0, 1+len(toolCalls))
	if text != "" {
		parts = append(parts, llm.Part{Text: text})
	}
	for _, call := range toolCalls {
		call := call
		parts = append(parts, llm.Part{FunctionCall: &call})
	}
	*messages = append(*messages, llm.Message{
		Role:  llm.RoleAssistant,
		Parts: parts,
	})
}

func assistantTurnCount(messages []llm.Message) int {
	count := 0
	for _, msg := range messages {
		if msg.Role == llm.RoleAssistant {
			count++
		}
	}
	return count
}
