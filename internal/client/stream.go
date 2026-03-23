package client

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/logging"
)

type sseStream struct {
	decoder *sseDecoder
	body    io.ReadCloser
	parse   func([]byte) (llm.ChatChunk, error)
	name    string
	mu      sync.Mutex
	closed  bool
}

func newSSEStream(body io.ReadCloser, parse func([]byte) (llm.ChatChunk, error), name string) *sseStream {
	return &sseStream{
		decoder: newSSEDecoder(body),
		body:    body,
		parse:   parse,
		name:    name,
	}
}

func (s *sseStream) Recv(ctx context.Context) (llm.ChatChunk, error) {
	if s == nil || s.decoder == nil || s.parse == nil {
		return llm.ChatChunk{}, errors.New("stream is nil")
	}
	type streamResult struct {
		data []byte
		err  error
	}
	resultCh := make(chan streamResult, 1)
	go func() {
		data, err := s.decoder.Next()
		resultCh <- streamResult{data: data, err: err}
	}()
	select {
	case <-ctx.Done():
		s.closeWithLog()
		return llm.ChatChunk{}, ctx.Err()
	case result := <-resultCh:
		data, err := result.data, result.err
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.closeWithLog()
				return llm.ChatChunk{Done: true}, nil
			}
			return llm.ChatChunk{}, err
		}
		if strings.TrimSpace(string(data)) == "[DONE]" {
			s.closeWithLog()
			return llm.ChatChunk{Done: true}, nil
		}
		return s.parse(data)
	}
}

func (s *sseStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.body != nil {
		return s.body.Close()
	}
	return nil
}

func (s *sseStream) closeWithLog() {
	if err := s.Close(); err != nil {
		logging.Logger().Debug("stream close failed", "stream", s.name, "error", err)
	}
}
