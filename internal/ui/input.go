package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/ergochat/readline"
	"golang.org/x/term"
)

const continuationPrompt = "... "

var errPromptInterrupted = errors.New("prompt interrupted")

type lineReader interface {
	ReadLine(ctx context.Context, prompt string) (string, bool, error)
	SaveHistory(line string) error
	Close() error
}

type basicLineReader struct {
	reader *bufio.Reader
	writer io.Writer
}

func newLineReader(in io.Reader, out io.Writer) (lineReader, io.Writer, error) {
	if isTerminalIO(in, out) {
		readlineReader, err := newReadlineLineReader(in, out)
		if err == nil {
			return readlineReader, readlineReader.output, nil
		}
	}
	return &basicLineReader{
		reader: bufio.NewReader(in),
		writer: out,
	}, out, nil
}

func isTerminalIO(in io.Reader, out io.Writer) bool {
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	if !inOK || !outOK {
		return false
	}
	return term.IsTerminal(int(inFile.Fd())) && term.IsTerminal(int(outFile.Fd()))
}

func (b *basicLineReader) ReadLine(ctx context.Context, prompt string) (string, bool, error) {
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	if prompt != "" {
		if _, err := fmt.Fprint(b.writer, prompt); err != nil {
			return "", false, err
		}
		if flusher, ok := b.writer.(interface{ Flush() error }); ok {
			if err := flusher.Flush(); err != nil {
				return "", false, err
			}
		}
	}
	line, err := b.reader.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	return line, false, err
}

func (b *basicLineReader) SaveHistory(string) error {
	return nil
}

func (b *basicLineReader) Close() error {
	return nil
}

type readlineLineReader struct {
	rl           *readline.Instance
	output       io.Writer
	continuation atomic.Bool
}

func newReadlineLineReader(in io.Reader, out io.Writer) (*readlineLineReader, error) {
	reader := &readlineLineReader{}
	cfg := &readline.Config{
		Prompt:                 "",
		Stdin:                  in,
		Stdout:                 out,
		Stderr:                 out,
		DisableAutoSaveHistory: true,
		FuncFilterInputRune:    reader.filterInputRune,
	}
	rl, err := readline.NewEx(cfg)
	if err != nil {
		return nil, err
	}
	reader.rl = rl
	reader.output = rl.Stdout()
	return reader, nil
}

func (r *readlineLineReader) filterInputRune(ch rune) (rune, bool) {
	if ch == '\n' {
		r.continuation.Store(true)
		return '\r', true
	}
	return ch, true
}

func (r *readlineLineReader) ReadLine(ctx context.Context, prompt string) (string, bool, error) {
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	r.continuation.Store(false)
	r.rl.SetPrompt(prompt)
	line, err := r.rl.ReadLine()
	cont := r.continuation.Load()
	if errors.Is(err, readline.ErrInterrupt) {
		return "", cont, errPromptInterrupted
	}
	return line, cont, err
}

func (r *readlineLineReader) SaveHistory(line string) error {
	if strings.TrimSpace(line) == "" {
		return nil
	}
	return r.rl.SaveToHistory(line)
}

func (r *readlineLineReader) Close() error {
	if r.rl == nil {
		return nil
	}
	return r.rl.Close()
}

func readUserInput(ctx context.Context, reader lineReader, prompt string) (string, bool, error) {
	var lines []string
	currentPrompt := prompt
	for {
		line, cont, err := reader.ReadLine(ctx, currentPrompt)
		eof := errors.Is(err, io.EOF)
		if err != nil && !eof {
			return "", false, err
		}
		if eof && line == "" {
			if len(lines) == 0 {
				return "", true, nil
			}
		}
		lines = append(lines, line)
		if cont {
			currentPrompt = continuationPrompt
			if eof {
				return strings.Join(lines, "\n"), true, nil
			}
			continue
		}
		return strings.Join(lines, "\n"), eof, nil
	}
}

func normalizeHistoryLine(line string) string {
	if strings.Contains(line, "\n") {
		return strings.ReplaceAll(line, "\n", "\\n")
	}
	return line
}
