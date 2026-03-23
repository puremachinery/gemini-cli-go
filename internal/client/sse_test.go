package client

import (
	"strings"
	"testing"
)

func TestSSEDecoderSingleEvent(t *testing.T) {
	input := "data: hello\ndata: world\n\n"
	decoder := newSSEDecoder(strings.NewReader(input))
	data, err := decoder.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got := string(data); got != "hello\nworld" {
		t.Fatalf("expected payload %q, got %q", "hello\nworld", got)
	}
}

func TestSSEDecoderEOFWithBufferedData(t *testing.T) {
	input := "data: hi"
	decoder := newSSEDecoder(strings.NewReader(input))
	data, err := decoder.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got := string(data); got != "hi" {
		t.Fatalf("expected payload %q, got %q", "hi", got)
	}
	if _, err := decoder.Next(); err == nil {
		t.Fatal("expected EOF on second read")
	}
}

func TestSSEDecoderLineTooLong(t *testing.T) {
	long := strings.Repeat("a", maxSSELineBytes+1)
	input := "data: " + long + "\n"
	decoder := newSSEDecoder(strings.NewReader(input))
	if _, err := decoder.Next(); err == nil {
		t.Fatal("expected error for long line")
	}
}

func TestSSEDecoderEventTooLarge(t *testing.T) {
	long := strings.Repeat("a", maxSSEEventBytes+1)
	input := "data: " + long + "\n\n"
	decoder := newSSEDecoder(strings.NewReader(input))
	if _, err := decoder.Next(); err == nil {
		t.Fatal("expected error for large event payload")
	}
}
