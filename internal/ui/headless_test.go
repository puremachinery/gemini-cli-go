package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

func TestRunHeadlessOutputsResponse(t *testing.T) {
	stream := &fakeStream{chunks: []llm.ChatChunk{
		{Text: "Hello", Done: false},
		{Text: " world", Done: true},
	}}
	client := &fakeClient{t: t, streams: []client.Stream{stream}}
	var output bytes.Buffer

	if err := RunHeadless(context.Background(), HeadlessOptions{
		Client: client,
		Model:  "test-model",
		Prompt: "say hi",
		Output: &output,
	}); err != nil {
		t.Fatalf("RunHeadless: %v", err)
	}

	if len(client.reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(client.reqs))
	}
	if got := client.reqs[0].Messages[0].Parts[0].Text; got != "say hi" {
		t.Fatalf("unexpected prompt: %q", got)
	}

	if got := output.String(); got != "Hello world\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunHeadlessRequiresPrompt(t *testing.T) {
	err := RunHeadless(context.Background(), HeadlessOptions{
		Client: &fakeClient{t: t},
		Model:  "test-model",
		Prompt: "  ",
	})
	if err == nil || !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("expected prompt error, got %v", err)
	}
}

func TestRunHeadlessAutoSavesSession(t *testing.T) {
	store := newMemoryChatStore()
	stream := &fakeStream{chunks: []llm.ChatChunk{{Text: "done", Done: true}}}
	client := &fakeClient{t: t, streams: []client.Stream{stream}}

	if err := RunHeadless(context.Background(), HeadlessOptions{
		Client:    client,
		Model:     "test-model",
		Prompt:    "hello",
		ChatStore: store,
		AuthType:  "api-key",
	}); err != nil {
		t.Fatalf("RunHeadless: %v", err)
	}

	if len(store.sessions) != 1 {
		t.Fatalf("expected 1 auto-saved session, got %d", len(store.sessions))
	}
	for _, sess := range store.sessions {
		if sess.AuthType != "api-key" {
			t.Fatalf("expected auth type to be saved, got %q", sess.AuthType)
		}
		if len(sess.Messages) < 2 {
			t.Fatalf("expected at least 2 messages, got %d", len(sess.Messages))
		}
	}
}
