package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/session"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

type fakeStream struct {
	chunks []llm.ChatChunk
	index  int
}

func (s *fakeStream) Recv(ctx context.Context) (llm.ChatChunk, error) {
	_ = ctx
	if s.index >= len(s.chunks) {
		return llm.ChatChunk{Done: true}, nil
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (s *fakeStream) Close() error {
	return nil
}

type fakeClient struct {
	t       *testing.T
	reqs    []llm.ChatRequest
	streams []client.Stream
}

func (c *fakeClient) ChatStream(ctx context.Context, req llm.ChatRequest) (client.Stream, error) {
	_ = ctx
	if c.t != nil && len(c.streams) == 0 {
		c.t.Fatalf("unexpected ChatStream call")
	}
	c.reqs = append(c.reqs, req)
	if len(c.streams) == 0 {
		return nil, errors.New("no stream configured")
	}
	stream := c.streams[0]
	c.streams = c.streams[1:]
	return stream, nil
}

func (c *fakeClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatChunk, error) {
	_ = ctx
	_ = req
	return llm.ChatChunk{}, errors.New("not implemented")
}

func (c *fakeClient) CountTokens(ctx context.Context, req llm.CountTokensRequest) (llm.CountTokensResponse, error) {
	_ = ctx
	_ = req
	return llm.CountTokensResponse{}, errors.New("not implemented")
}

type blockingStream struct {
	started chan struct{}
}

func (s *blockingStream) Recv(ctx context.Context) (llm.ChatChunk, error) {
	if s.started != nil {
		select {
		case <-s.started:
		default:
			close(s.started)
		}
	}
	<-ctx.Done()
	return llm.ChatChunk{}, ctx.Err()
}

func (s *blockingStream) Close() error {
	return nil
}

func TestRunHelpAndQuit(t *testing.T) {
	input := bytes.NewBufferString("/help\n/quit\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, helpMessage) {
		t.Fatalf("expected help output, got: %q", out)
	}
	if !strings.Contains(out, quitMessage) {
		t.Fatalf("expected quit output, got: %q", out)
	}
}

func TestRunClearAndQuit(t *testing.T) {
	input := bytes.NewBufferString("/clear\n/quit\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, clearANSIReset) {
		t.Fatalf("expected clear sequence, got: %q", out)
	}
	if !strings.Contains(out, "Tips for getting started:") {
		t.Fatalf("expected intro text, got: %q", out)
	}
	if !strings.Contains(out, quitMessage) {
		t.Fatalf("expected quit output, got: %q", out)
	}
}

func TestRunModelCommandShowsCurrent(t *testing.T) {
	input := bytes.NewBufferString("/model\n/quit\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
		PersistModel: func(string) error {
			t.Fatalf("PersistModel should not be called")
			return nil
		},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "Current model: test-model") {
		t.Fatalf("expected current model output, got: %q", out)
	}
}

func TestRunModelCommandUpdatesModel(t *testing.T) {
	input := bytes.NewBufferString("/model flash\nhello\n/quit\n")
	var output bytes.Buffer
	stream := &fakeStream{chunks: []llm.ChatChunk{{Text: "ok", Done: true}}}
	client := &fakeClient{t: t, streams: []client.Stream{stream}}
	var persisted string

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
		ResolveModel: func(name string) string {
			if name == "flash" {
				return "gemini-2.5-flash"
			}
			return name
		},
		PersistModel: func(value string) error {
			persisted = value
			return nil
		},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if persisted != "flash" {
		t.Fatalf("expected persisted model to be flash, got %q", persisted)
	}
	if len(client.reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(client.reqs))
	}
	if got := client.reqs[0].Model; got != "gemini-2.5-flash" {
		t.Fatalf("expected model to be updated, got %q", got)
	}
}

func TestRunModelCommandDoesNotMatchPrefix(t *testing.T) {
	input := bytes.NewBufferString("/modeling\n/quit\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
		PersistModel: func(string) error {
			t.Fatalf("PersistModel should not be called")
			return nil
		},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "Unknown command: /modeling") {
		t.Fatalf("expected unknown command output, got: %q", out)
	}
}

func TestRunClearNoIntro(t *testing.T) {
	input := bytes.NewBufferString("/clear\n/quit\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, clearANSIReset) {
		t.Fatalf("expected clear sequence, got: %q", out)
	}
	if strings.Contains(out, "Tips for getting started:") {
		t.Fatalf("expected no intro text, got: %q", out)
	}
}

func TestRunEOFPartialLine(t *testing.T) {
	input := bytes.NewBufferString("hello")
	var output bytes.Buffer
	stream := &fakeStream{chunks: []llm.ChatChunk{{Text: "Hi", Done: true}}}
	client := &fakeClient{streams: []client.Stream{stream}}

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(client.reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(client.reqs))
	}
	if client.reqs[0].Model != "test-model" {
		t.Fatalf("unexpected model: %q", client.reqs[0].Model)
	}
	if len(client.reqs[0].Messages) != 1 || client.reqs[0].Messages[0].Parts[0].Text != "hello" {
		t.Fatalf("unexpected request messages: %#v", client.reqs[0].Messages)
	}
	if !strings.Contains(output.String(), "Hi") {
		t.Fatalf("expected response output, got: %q", output.String())
	}
}

func TestRunCancelWhileIdle(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() {
		if err := writer.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		client := &fakeClient{t: t}
		done <- Run(ctx, RunOptions{
			Client:    client,
			Model:     "test-model",
			Input:     reader,
			Output:    io.Discard,
			ShowIntro: false,
		})
	}()

	cancel()
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunInterruptCancelsStreamAndContinues(t *testing.T) {
	started := make(chan struct{})
	stream := &blockingStream{started: started}
	client := &fakeClient{streams: []client.Stream{stream}}
	input := bytes.NewBufferString("hello\n/quit\n")
	var output bytes.Buffer
	sigCh := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), RunOptions{
			Client:    client,
			Model:     "test-model",
			Input:     input,
			Output:    &output,
			ShowIntro: false,
			Interrupt: sigCh,
		})
	}()

	select {
	case <-started:
		sigCh <- os.Interrupt
	case <-time.After(500 * time.Millisecond):
		t.Fatal("stream did not start")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Run did not return after interrupt")
	}

	if !strings.Contains(output.String(), quitMessage) {
		t.Fatalf("expected quit output, got: %q", output.String())
	}
}

func TestRunInterruptWhileIdleExitsCleanly(t *testing.T) {
	reader, writer := io.Pipe()
	sigCh := make(chan os.Signal, 1)
	done := make(chan error, 1)

	go func() {
		client := &fakeClient{t: t}
		done <- Run(context.Background(), RunOptions{
			Client:    client,
			Model:     "test-model",
			Input:     reader,
			Output:    io.Discard,
			ShowIntro: false,
			Interrupt: sigCh,
		})
	}()

	sigCh <- os.Interrupt
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Run did not return after interrupt")
	}
}

func TestRunToolLoopExecutesAndContinues(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	first := &fakeStream{chunks: []llm.ChatChunk{{
		Tools: []llm.FunctionCall{{
			Name: "read_file",
			Args: map[string]any{"file_path": "note.txt"},
		}},
		Done: true,
	}}}
	second := &fakeStream{chunks: []llm.ChatChunk{{Text: "done", Done: true}}}

	client := &fakeClient{streams: []client.Stream{first, second}}
	registry := tools.NewRegistry(tools.Context{WorkspaceRoot: root})
	executor := &tools.Executor{Registry: registry}

	input := bytes.NewBufferString("hello\n")
	var output bytes.Buffer

	if err := Run(context.Background(), RunOptions{
		Client:       client,
		Model:        "test-model",
		Input:        input,
		Output:       &output,
		ShowIntro:    false,
		ToolExecutor: executor,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(client.reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(client.reqs))
	}
	if len(client.reqs[1].Messages) != 3 {
		t.Fatalf("expected 3 messages in second request, got %#v", client.reqs[1].Messages)
	}
	last := client.reqs[1].Messages[2]
	if last.Role != llm.RoleUser || len(last.Parts) != 1 || last.Parts[0].FunctionResponse == nil {
		t.Fatalf("expected function response message, got %#v", last)
	}
	if got := last.Parts[0].FunctionResponse.Response["output"]; got != "hi" {
		t.Fatalf("unexpected tool response: %#v", last.Parts[0].FunctionResponse.Response)
	}
	if !strings.Contains(output.String(), "done") {
		t.Fatalf("expected final response in output, got: %q", output.String())
	}
	if !strings.Contains(output.String(), "hi") {
		t.Fatalf("expected tool output in output, got: %q", output.String())
	}
}

func TestRunToolApprovalDeniesWrite(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")

	first := &fakeStream{chunks: []llm.ChatChunk{{
		Tools: []llm.FunctionCall{{
			Name: "write_file",
			Args: map[string]any{"file_path": "note.txt", "content": "hi"},
		}},
		Done: true,
	}}}
	second := &fakeStream{chunks: []llm.ChatChunk{{Text: "done", Done: true}}}

	client := &fakeClient{streams: []client.Stream{first, second}}
	registry := tools.NewRegistry(tools.Context{WorkspaceRoot: root})
	executor := &tools.Executor{Registry: registry}

	input := bytes.NewBufferString("hello\nn\n")
	var output bytes.Buffer

	if err := Run(context.Background(), RunOptions{
		Client:       client,
		Model:        "test-model",
		Input:        input,
		Output:       &output,
		ShowIntro:    false,
		ToolExecutor: executor,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected file to not be written, stat err: %v", err)
	}
	if !strings.Contains(output.String(), "tool execution canceled by user") {
		t.Fatalf("expected cancellation message in output, got: %q", output.String())
	}
}

type memoryChatStore struct {
	sessions map[string]*session.Session
}

func newMemoryChatStore() *memoryChatStore {
	return &memoryChatStore{sessions: map[string]*session.Session{}}
}

func (s *memoryChatStore) Load(id string) (*session.Session, error) {
	sess, ok := s.sessions[id]
	if !ok {
		return nil, session.ErrNotFound
	}
	copySess := *sess
	copySess.Messages = append([]llm.Message{}, sess.Messages...)
	return &copySess, nil
}

func (s *memoryChatStore) Save(sess *session.Session) error {
	if sess == nil {
		return errors.New("nil session")
	}
	copySess := *sess
	copySess.Messages = append([]llm.Message{}, sess.Messages...)
	s.sessions[sess.ID] = &copySess
	return nil
}

func (s *memoryChatStore) List() ([]session.Session, error) {
	out := make([]session.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, *sess)
	}
	return out, nil
}

func (s *memoryChatStore) Delete(id string) error {
	if _, ok := s.sessions[id]; !ok {
		return session.ErrNotFound
	}
	delete(s.sessions, id)
	return nil
}

func TestRunChatSaveListResumeDelete(t *testing.T) {
	store := newMemoryChatStore()
	input := bytes.NewBufferString("/chat save test\n/chat list\n/chat resume test\n/chat delete test\n/chat list\n/quit\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}
	initial := []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello"}}}}

	if err := Run(context.Background(), RunOptions{
		Client:      client,
		Model:       "test-model",
		Input:       input,
		Output:      &output,
		ShowIntro:   false,
		InitialMsgs: initial,
		ChatStore:   store,
		AuthType:    "oauth-personal",
		Now: func() time.Time {
			return time.Date(2026, 2, 1, 1, 2, 3, 0, time.UTC)
		},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "Conversation checkpoint saved with tag: test.") {
		t.Fatalf("expected save output, got: %q", out)
	}
	if !strings.Contains(out, "Saved conversation checkpoints:") || !strings.Contains(out, "- test") {
		t.Fatalf("expected list output, got: %q", out)
	}
	if !strings.Contains(out, "Resumed conversation from tag: test.") {
		t.Fatalf("expected resume output, got: %q", out)
	}
	if !strings.Contains(out, "Conversation replay") {
		t.Fatalf("expected replay output, got: %q", out)
	}
	if !strings.Contains(out, "User:") {
		t.Fatalf("expected replay content, got: %q", out)
	}
	if !strings.Contains(out, "Conversation checkpoint 'test' has been deleted.") {
		t.Fatalf("expected delete output, got: %q", out)
	}
	if !strings.Contains(out, chatNoCheckpointsMessage) {
		t.Fatalf("expected empty list output, got: %q", out)
	}
}

func TestRunChatResumeUsesSavedMessages(t *testing.T) {
	store := newMemoryChatStore()
	if err := store.Save(&session.Session{
		ID:       "resume-tag",
		Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "prior"}}}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	input := bytes.NewBufferString("/chat resume resume-tag\nhello\n/quit\n")
	var output bytes.Buffer
	stream := &fakeStream{chunks: []llm.ChatChunk{{Text: "ok", Done: true}}}
	client := &fakeClient{t: t, streams: []client.Stream{stream}}

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
		ChatStore: store,
		AuthType:  "oauth-personal",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(client.reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(client.reqs))
	}
	if got := client.reqs[0].Messages[0].Parts[0].Text; got != "prior" {
		t.Fatalf("expected resumed message in request, got %q", got)
	}
	if !strings.Contains(output.String(), "Conversation replay") {
		t.Fatalf("expected replay output, got: %q", output.String())
	}
}

func TestRunChatShareWritesFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "chat.md")
	input := bytes.NewBufferString("/chat share " + path + "\n/quit\n")
	var output bytes.Buffer
	client := &fakeClient{t: t}
	initial := []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello"}}}}

	if err := Run(context.Background(), RunOptions{
		Client:      client,
		Model:       "test-model",
		Input:       input,
		Output:      &output,
		ShowIntro:   false,
		InitialMsgs: initial,
		ChatStore:   newMemoryChatStore(),
		AuthType:    "oauth-personal",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Fatalf("expected shared file to include content, got: %q", string(data))
	}
}

func TestRunAutoSavesSession(t *testing.T) {
	store := newMemoryChatStore()
	input := bytes.NewBufferString("hello\n/quit\n")
	var output bytes.Buffer
	stream := &fakeStream{chunks: []llm.ChatChunk{{Text: "ok", Done: true}}}
	client := &fakeClient{t: t, streams: []client.Stream{stream}}

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
		ChatStore: store,
		AuthType:  "oauth-personal",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.sessions) != 1 {
		t.Fatalf("expected 1 auto-saved session, got %d", len(store.sessions))
	}
	for _, sess := range store.sessions {
		if sess.AuthType != "oauth-personal" {
			t.Fatalf("expected auth type to be saved, got %q", sess.AuthType)
		}
		if len(sess.Messages) < 2 {
			t.Fatalf("expected at least 2 messages, got %d", len(sess.Messages))
		}
		if sess.Messages[0].Role != llm.RoleUser {
			t.Fatalf("expected first message to be user, got %q", sess.Messages[0].Role)
		}
	}
}

func TestRunResumeSelectsAutoSession(t *testing.T) {
	store := newMemoryChatStore()
	if err := store.Save(&session.Session{
		ID:        "session-123",
		StartedAt: time.Date(2026, 2, 1, 2, 3, 4, 0, time.UTC),
		AuthType:  "oauth-personal",
		Messages:  []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "prior"}}}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Save(&session.Session{
		ID:        "session-456",
		StartedAt: time.Date(2026, 2, 1, 2, 3, 6, 0, time.UTC),
		AuthType:  "api-key",
		Messages:  []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "other"}}}},
	}); err != nil {
		t.Fatalf("Save other: %v", err)
	}
	if err := store.Save(&session.Session{
		ID:        "manual",
		StartedAt: time.Date(2026, 2, 1, 2, 3, 5, 0, time.UTC),
		Messages:  []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "manual"}}}},
	}); err != nil {
		t.Fatalf("Save manual: %v", err)
	}

	input := bytes.NewBufferString("/resume\n1\nhello\n/quit\n")
	var output bytes.Buffer
	stream := &fakeStream{chunks: []llm.ChatChunk{{Text: "ok", Done: true}}}
	client := &fakeClient{t: t, streams: []client.Stream{stream}}

	if err := Run(context.Background(), RunOptions{
		Client:    client,
		Model:     "test-model",
		Input:     input,
		Output:    &output,
		ShowIntro: false,
		ChatStore: store,
		AuthType:  "oauth-personal",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(client.reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(client.reqs))
	}
	if got := client.reqs[0].Messages[0].Parts[0].Text; got != "prior" {
		t.Fatalf("expected resumed message in request, got %q", got)
	}
	if !strings.Contains(output.String(), "Auto-saved conversations:") {
		t.Fatalf("expected resume list output, got: %q", output.String())
	}
	if !strings.Contains(output.String(), "Conversation replay") {
		t.Fatalf("expected replay output, got: %q", output.String())
	}
	if strings.Contains(output.String(), "manual") {
		t.Fatalf("expected manual session to be filtered out, got: %q", output.String())
	}
	if strings.Contains(output.String(), "session-456") {
		t.Fatalf("expected auth-mismatched session to be filtered out, got: %q", output.String())
	}
}
