package ui

import (
	"context"
	"errors"
	"io"
	"testing"
)

type stubLineReader struct {
	responses []stubLineResponse
	prompts   []string
	idx       int
}

type stubLineResponse struct {
	line string
	cont bool
	err  error
}

func (s *stubLineReader) ReadLine(_ context.Context, prompt string) (string, bool, error) {
	s.prompts = append(s.prompts, prompt)
	if s.idx >= len(s.responses) {
		return "", false, io.EOF
	}
	resp := s.responses[s.idx]
	s.idx++
	return resp.line, resp.cont, resp.err
}

func (s *stubLineReader) SaveHistory(string) error {
	return nil
}

func (s *stubLineReader) Close() error {
	return nil
}

func TestReadUserInputContinuation(t *testing.T) {
	reader := &stubLineReader{responses: []stubLineResponse{
		{line: "first", cont: true},
		{line: "second", cont: false},
	}}

	line, eof, err := readUserInput(context.Background(), reader, "> ")
	if err != nil {
		t.Fatalf("readUserInput error: %v", err)
	}
	if eof {
		t.Fatalf("expected eof=false")
	}
	if line != "first\nsecond" {
		t.Fatalf("expected multiline join, got %q", line)
	}
	if len(reader.prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(reader.prompts))
	}
	if reader.prompts[0] != "> " {
		t.Fatalf("expected first prompt to be main prompt, got %q", reader.prompts[0])
	}
	if reader.prompts[1] != continuationPrompt {
		t.Fatalf("expected continuation prompt, got %q", reader.prompts[1])
	}
}

func TestReadUserInputEOFWithData(t *testing.T) {
	reader := &stubLineReader{responses: []stubLineResponse{{
		line: "hello",
		err:  io.EOF,
	}}}

	line, eof, err := readUserInput(context.Background(), reader, "> ")
	if err != nil {
		t.Fatalf("readUserInput error: %v", err)
	}
	if !eof {
		t.Fatalf("expected eof=true")
	}
	if line != "hello" {
		t.Fatalf("expected line to be returned, got %q", line)
	}
}

func TestReadUserInputEOFEmpty(t *testing.T) {
	reader := &stubLineReader{responses: []stubLineResponse{{
		line: "",
		err:  io.EOF,
	}}}

	line, eof, err := readUserInput(context.Background(), reader, "> ")
	if err != nil {
		t.Fatalf("readUserInput error: %v", err)
	}
	if !eof {
		t.Fatalf("expected eof=true")
	}
	if line != "" {
		t.Fatalf("expected empty line, got %q", line)
	}
}

func TestReadUserInputPropagatesErrors(t *testing.T) {
	boom := errors.New("boom")
	reader := &stubLineReader{responses: []stubLineResponse{{
		line: "",
		err:  boom,
	}}}

	_, _, err := readUserInput(context.Background(), reader, "> ")
	if !errors.Is(err, boom) {
		t.Fatalf("expected error to propagate")
	}
}

func TestNormalizeHistoryLine(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "hello", want: "hello"},
		{in: "first\nsecond", want: "first\\nsecond"},
		{in: "one\ntwo\nthree", want: "one\\ntwo\\nthree"},
	}

	for _, tc := range cases {
		if got := normalizeHistoryLine(tc.in); got != tc.want {
			t.Fatalf("normalizeHistoryLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
