package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/memory"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

type modelLoopOptions struct {
	Client          client.Client
	Model           string
	ToolExecutor    *tools.Executor
	Memory          *memory.State
	RenderMarkdown  bool
	MarkdownWidth   int
	ToolsAnnounced  *bool
	ShowToolOutputs bool
	PrependNewline  bool
}

func initMarkdownRenderer(enabled bool, width int) (bool, markdownRenderer) {
	if !enabled {
		return false, nil
	}
	renderer, err := newMarkdownRenderer(width)
	if err != nil {
		return false, nil
	}
	return true, renderer
}

func announceToolsIfNeeded(writer *bufio.Writer, executor *tools.Executor, toolsAnnounced *bool) error {
	if toolsAnnounced == nil || *toolsAnnounced {
		return nil
	}
	if names := tools.ToolNamesForExecutor(executor); len(names) > 0 {
		if _, err := fmt.Fprintf(writer, "[tools enabled] %s\n", strings.Join(names, ", ")); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		*toolsAnnounced = true
	}
	return nil
}

func flushPendingMarkdown(writer *bufio.Writer, renderer markdownRenderer, pending *strings.Builder) (string, bool, error) {
	if pending == nil || pending.Len() == 0 {
		return "", false, nil
	}
	rendered, renderErr := renderMarkdown(renderer, pending.String())
	if renderErr != nil {
		rendered = pending.String()
	}
	if _, err := fmt.Fprint(writer, rendered); err != nil {
		return "", false, err
	}
	if err := writer.Flush(); err != nil {
		return "", false, err
	}
	pending.Reset()
	return rendered, true, nil
}

func executeToolCalls(ctx context.Context, writer *bufio.Writer, messages *[]llm.Message, opts modelLoopOptions, toolCalls []llm.FunctionCall) error {
	if opts.ToolExecutor == nil || opts.ToolExecutor.Registry == nil {
		return errors.New("tool calls requested but no tool executor configured")
	}
	if opts.ShowToolOutputs {
		if _, err := fmt.Fprintf(writer, "\n[tools] executing %d call(s) (blocking)\n", len(toolCalls)); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	responseParts := make([]llm.Part, 0, len(toolCalls))
	for _, call := range toolCalls {
		result := opts.ToolExecutor.Execute(ctx, tools.ToolCall{
			ID:   call.ID,
			Name: call.Name,
			Args: call.Args,
		})
		if opts.ShowToolOutputs {
			display := result.DisplayText()
			if display != "" {
				if _, err := fmt.Fprintf(writer, "\n[tool %s]\n%s\n", call.Name, display); err != nil {
					return err
				}
				if err := writer.Flush(); err != nil {
					return err
				}
			}
		}
		responseParts = append(responseParts, llm.Part{
			FunctionResponse: &llm.FunctionResponse{
				ID:       call.ID,
				Name:     call.Name,
				Response: result.Response(),
			},
		})
	}
	*messages = append(*messages, llm.Message{
		Role:  llm.RoleUser,
		Parts: responseParts,
	})
	return nil
}

func runModelLoop(ctx context.Context, writer *bufio.Writer, messages *[]llm.Message, opts modelLoopOptions) error {
	renderAsMarkdown, mdRenderer := initMarkdownRenderer(opts.RenderMarkdown, opts.MarkdownWidth)
	var memoryBuffer []llm.Message
	for turn := 0; ; turn++ {
		if turn >= maxModelTurns {
			return fmt.Errorf("model loop exceeded %d turns; aborting to prevent infinite tool calls", maxModelTurns)
		}
		if ctx.Err() != nil {
			return nil
		}
		if err := announceToolsIfNeeded(writer, opts.ToolExecutor, opts.ToolsAnnounced); err != nil {
			return err
		}
		requestMessages, usedBuffer := withMemoryMessagesInto(memoryBuffer[:0], *messages, opts.Memory)
		if usedBuffer {
			memoryBuffer = requestMessages[:0]
		}
		stream, err := opts.Client.ChatStream(ctx, llm.ChatRequest{
			Model:    opts.Model,
			Messages: requestMessages,
			Tools:    tools.ToolDeclarationsForExecutor(opts.ToolExecutor),
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, os.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			return err
		}

		if opts.PrependNewline {
			if _, err := fmt.Fprintln(writer); err != nil {
				if closeErr := stream.Close(); closeErr != nil {
					_, _ = fmt.Fprintln(os.Stderr, closeErr)
				}
				return err
			}
			if err := writer.Flush(); err != nil {
				if closeErr := stream.Close(); closeErr != nil {
					_, _ = fmt.Fprintln(os.Stderr, closeErr)
				}
				return err
			}
		}

		var assistant strings.Builder
		assistant.Grow(256)
		var toolCalls []llm.FunctionCall
		var pendingMarkdown strings.Builder
		wroteOutput := false
		endsWithNewline := false
		for {
			chunk, err := stream.Recv(ctx)
			if err != nil {
				if closeErr := stream.Close(); closeErr != nil {
					_, _ = fmt.Fprintln(os.Stderr, closeErr)
				}
				if errors.Is(err, context.Canceled) || errors.Is(err, os.ErrClosed) || ctx.Err() != nil {
					if renderAsMarkdown {
						rendered, flushed, err := flushPendingMarkdown(writer, mdRenderer, &pendingMarkdown)
						if err != nil {
							return err
						}
						if flushed {
							wroteOutput = true
							endsWithNewline = strings.HasSuffix(rendered, "\n")
						}
					}
					if wroteOutput && !endsWithNewline {
						if _, err := fmt.Fprintln(writer); err != nil {
							return err
						}
						if err := writer.Flush(); err != nil {
							return err
						}
					}
					return nil
				}
				return err
			}
			if chunk.Text != "" {
				assistant.WriteString(chunk.Text)
				if renderAsMarkdown {
					pendingMarkdown.WriteString(chunk.Text)
					pending := pendingMarkdown.String()
					flushText, remaining := splitMarkdownForRender(pending, markdownFlushThreshold)
					if flushText != "" {
						rendered, renderErr := renderMarkdown(mdRenderer, flushText)
						if renderErr != nil {
							renderAsMarkdown = false
							rendered = flushText + remaining
							pendingMarkdown.Reset()
						} else {
							pendingMarkdown.Reset()
							pendingMarkdown.WriteString(remaining)
						}
						if _, err := fmt.Fprint(writer, rendered); err != nil {
							if closeErr := stream.Close(); closeErr != nil {
								_, _ = fmt.Fprintln(os.Stderr, closeErr)
							}
							return err
						}
						if err := writer.Flush(); err != nil {
							if closeErr := stream.Close(); closeErr != nil {
								_, _ = fmt.Fprintln(os.Stderr, closeErr)
							}
							return err
						}
						wroteOutput = true
						endsWithNewline = strings.HasSuffix(rendered, "\n")
					}
				} else {
					if _, err := fmt.Fprint(writer, chunk.Text); err != nil {
						if closeErr := stream.Close(); closeErr != nil {
							_, _ = fmt.Fprintln(os.Stderr, closeErr)
						}
						return err
					}
					if err := writer.Flush(); err != nil {
						if closeErr := stream.Close(); closeErr != nil {
							_, _ = fmt.Fprintln(os.Stderr, closeErr)
						}
						return err
					}
					wroteOutput = true
					endsWithNewline = strings.HasSuffix(chunk.Text, "\n")
				}
			}
			if len(chunk.Tools) > 0 {
				toolCalls = append(toolCalls, chunk.Tools...)
			}
			if chunk.Done {
				break
			}
		}
		if err := stream.Close(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
		if renderAsMarkdown {
			if _, flushed, err := flushPendingMarkdown(writer, mdRenderer, &pendingMarkdown); err != nil {
				return err
			} else if flushed {
				wroteOutput = true
			}
		}
		if _, err := fmt.Fprintln(writer); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}

		appendAssistantMessage(messages, assistant.String(), toolCalls)

		if len(toolCalls) == 0 {
			return nil
		}
		if err := executeToolCalls(ctx, writer, messages, opts, toolCalls); err != nil {
			return err
		}
	}
}
