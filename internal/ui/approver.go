package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

type promptApprover struct {
	reader lineReader
	writer *bufio.Writer
}

func (a *promptApprover) Confirm(ctx context.Context, req tools.ConfirmationRequest) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if req.Title != "" {
		if _, err := fmt.Fprintf(a.writer, "\n%s\n", req.Title); err != nil {
			return false, err
		}
		if err := a.writer.Flush(); err != nil {
			return false, err
		}
	}
	prompt := req.Prompt
	if prompt == "" {
		prompt = "Proceed?"
	}
	for {
		if err := a.writer.Flush(); err != nil {
			return false, err
		}
		line, _, err := a.reader.ReadLine(ctx, fmt.Sprintf("%s [y/N]: ", prompt))
		eof := errors.Is(err, io.EOF)
		if err != nil && !eof {
			if errors.Is(err, errPromptInterrupted) || errors.Is(err, os.ErrClosed) || ctx.Err() != nil {
				return false, nil
			}
			return false, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return false, nil
		}
		switch strings.ToLower(line) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			if eof {
				return false, nil
			}
		}
	}
}
