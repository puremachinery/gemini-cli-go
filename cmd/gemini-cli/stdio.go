package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	maxStdinBytes    = 8 * 1024 * 1024
	stdinReadTimeout = 500 * time.Millisecond
)

func stdinIsPiped() bool {
	return !stdinIsTTY()
}

func stdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func stdoutIsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func stdoutWidth() int {
	if !stdoutIsTTY() {
		return 0
	}
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 0
	}
	return width
}

func readStdinWithTimeout(timeout time.Duration, maxBytes int64) (string, error) {
	if timeout <= 0 {
		return readStdinLimited(maxBytes)
	}
	done := make(chan struct{})
	timedOut := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		select {
		case <-done:
			return
		default:
		}
		close(timedOut)
		if err := os.Stdin.Close(); err != nil {
			_ = err
		}
	})
	data, truncated, err := readStdinLimitedBytes(maxBytes)
	close(done)
	timer.Stop()
	if isClosed(timedOut) {
		if len(data) == 0 {
			return "", nil
		}
	}
	if err != nil {
		return "", err
	}
	if truncated {
		fmt.Fprintf(os.Stderr, "Warning: stdin input truncated to %d bytes.\n", maxBytes)
	}
	return string(data), nil
}

func readStdinLimited(maxBytes int64) (string, error) {
	data, truncated, err := readStdinLimitedBytes(maxBytes)
	if err != nil {
		return "", err
	}
	if truncated {
		fmt.Fprintf(os.Stderr, "Warning: stdin input truncated to %d bytes.\n", maxBytes)
	}
	return string(data), nil
}

func readStdinLimitedBytes(maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		data, err := io.ReadAll(os.Stdin)
		return data, false, err
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, maxBytes+1))
	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	return data, truncated, err
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func combinePrompt(stdinInput, prompt string) string {
	if stdinInput == "" {
		return prompt
	}
	if prompt == "" {
		return stdinInput
	}
	trimmed := strings.TrimRight(stdinInput, "\r\n")
	return trimmed + "\n\n" + prompt
}

type stdinClosingReader struct {
	io.Reader
}

func (r stdinClosingReader) Close() error {
	return os.Stdin.Close()
}
