package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/auth"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

type staticStore struct {
	creds *auth.Credentials
}

func (s *staticStore) Load(ctx context.Context) (*auth.Credentials, error) {
	_ = ctx
	return s.creds, nil
}

func (s *staticStore) Save(ctx context.Context, creds *auth.Credentials) error {
	_ = ctx
	s.creds = creds
	return nil
}

func (s *staticStore) Clear(ctx context.Context) error {
	_ = ctx
	s.creds = nil
	return nil
}

type passthroughProvider struct{}

func (p passthroughProvider) Name() string { return "pass" }

func (p passthroughProvider) Login(ctx context.Context) (*auth.Credentials, error) {
	_ = ctx
	return nil, nil
}

func (p passthroughProvider) Refresh(ctx context.Context, creds *auth.Credentials) (*auth.Credentials, error) {
	_ = ctx
	return creds, nil
}

func TestCodeAssistChatStreamParsesSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/v1internal:streamGenerateContent" {
			t.Fatalf("unexpected path: %s", got)
		}
		if got := r.URL.Query().Get("alt"); got != "sse" {
			t.Fatalf("unexpected alt query: %s", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]}}]}}\n\n"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
		if _, err := io.WriteString(w, "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\" World\"}]}}]}}\n\n"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client := NewCodeAssistClient(nil, nil, CodeAssistOptions{
		Endpoint:   server.URL,
		APIVersion: "v1internal",
	})

	stream, err := client.ChatStream(context.Background(), llm.ChatRequest{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() {
		if err := stream.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	chunk, err := stream.Recv(context.Background())
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if chunk.Text != "Hello" {
		t.Fatalf("unexpected first chunk: %q", chunk.Text)
	}
	chunk, err = stream.Recv(context.Background())
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if chunk.Text != " World" {
		t.Fatalf("unexpected second chunk: %q", chunk.Text)
	}
	chunk, err = stream.Recv(context.Background())
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if !chunk.Done {
		t.Fatal("expected Done chunk")
	}
}

func TestCodeAssistChatStreamExtractsToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"run_shell_command\",\"args\":{\"command\":\"pwd\"}}}]}}]}}\n\n"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client := NewCodeAssistClient(nil, nil, CodeAssistOptions{
		Endpoint:   server.URL,
		APIVersion: "v1internal",
	})

	stream, err := client.ChatStream(context.Background(), llm.ChatRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() {
		if err := stream.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	chunk, err := stream.Recv(context.Background())
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if len(chunk.Tools) != 1 {
		t.Fatalf("expected tool call, got %#v", chunk.Tools)
	}
	if chunk.Tools[0].Name != "run_shell_command" {
		t.Fatalf("unexpected tool name: %q", chunk.Tools[0].Name)
	}
}

func TestCodeAssistChatRetriesOnRetryableStatus(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := io.WriteString(w, "busy"); err != nil {
				t.Fatalf("WriteString: %v", err)
			}
			return
		}
		if got := r.URL.Path; got != "/v1internal:generateContent" {
			t.Fatalf("unexpected path: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, "{\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]}}]}}"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client := NewCodeAssistClient(nil, nil, CodeAssistOptions{
		Endpoint:   server.URL,
		APIVersion: "v1internal",
	})

	if _, err := client.Chat(context.Background(), llm.ChatRequest{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hi"}}},
		},
	}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestCodeAssistChatStreamAddsAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-abc" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]}}]}}\n\n"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	store := &staticStore{
		creds: &auth.Credentials{
			AccessToken: "token-abc",
			Expiry:      time.Now().Add(10 * time.Minute),
		},
	}

	client := NewCodeAssistClient(passthroughProvider{}, store, CodeAssistOptions{
		Endpoint:   server.URL,
		APIVersion: "v1internal",
		Headers:    map[string]string{"x-test": "1"},
	})

	stream, err := client.ChatStream(context.Background(), llm.ChatRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() {
		if err := stream.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	chunk, err := stream.Recv(context.Background())
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if strings.TrimSpace(chunk.Text) != "ok" {
		t.Fatalf("unexpected chunk: %q", chunk.Text)
	}
}

func TestCodeAssistChatNonStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/v1internal:generateContent" {
			t.Fatalf("unexpected path: %s", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected Accept header: %q", got)
		}
		var payload caGenerateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if err := r.Body.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if payload.Model != "test-model" {
			t.Fatalf("unexpected model: %q", payload.Model)
		}
		if payload.UserPromptID != "prompt-1" {
			t.Fatalf("unexpected prompt id: %q", payload.UserPromptID)
		}
		if payload.Request.SystemInstruction == nil || len(payload.Request.SystemInstruction.Parts) != 1 {
			t.Fatalf("missing system instruction")
		}
		if payload.Request.SystemInstruction.Parts[0].Text != "system" {
			t.Fatalf("unexpected system instruction: %q", payload.Request.SystemInstruction.Parts[0].Text)
		}
		if len(payload.Request.Contents) != 1 || payload.Request.Contents[0].Role != "user" {
			t.Fatalf("unexpected contents: %#v", payload.Request.Contents)
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, "{\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]}}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2,\"totalTokenCount\":3}}}"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client := NewCodeAssistClient(nil, nil, CodeAssistOptions{
		Endpoint:     server.URL,
		APIVersion:   "v1internal",
		UserPromptID: func() string { return "prompt-1" },
	})

	chunk, err := client.Chat(context.Background(), llm.ChatRequest{
		Model: "models/test-model",
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Parts: []llm.Part{{Text: "system"}}},
			{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !chunk.Done {
		t.Fatal("expected Done chunk")
	}
	if chunk.Text != "Hello" {
		t.Fatalf("unexpected chunk: %q", chunk.Text)
	}
	if chunk.Usage == nil || chunk.Usage.TotalTokens != 3 {
		t.Fatalf("unexpected usage: %#v", chunk.Usage)
	}
}

func TestCodeAssistChatIncludesToolsAndFunctionParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload caGenerateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if err := r.Body.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if len(payload.Request.Tools) != 1 {
			t.Fatalf("unexpected tools: %#v", payload.Request.Tools)
		}
		decls := payload.Request.Tools[0].FunctionDeclarations
		if len(decls) != 1 || decls[0].Name != "read_file" {
			t.Fatalf("unexpected tool declarations: %#v", decls)
		}
		if len(payload.Request.Contents) != 3 {
			t.Fatalf("unexpected contents: %#v", payload.Request.Contents)
		}
		if payload.Request.Contents[0].Role != "user" {
			t.Fatalf("unexpected role: %q", payload.Request.Contents[0].Role)
		}
		if payload.Request.Contents[0].Parts[0].Text != "hello" {
			t.Fatalf("unexpected first content text: %q", payload.Request.Contents[0].Parts[0].Text)
		}
		if payload.Request.Contents[1].Role != "model" || payload.Request.Contents[1].Parts[0].FunctionCall == nil {
			t.Fatalf("expected model functionCall content, got %#v", payload.Request.Contents[1])
		}
		if payload.Request.Contents[1].Parts[0].FunctionCall.Name != "read_file" {
			t.Fatalf("unexpected function call name: %q", payload.Request.Contents[1].Parts[0].FunctionCall.Name)
		}
		if payload.Request.Contents[2].Role != "user" || payload.Request.Contents[2].Parts[0].FunctionResponse == nil {
			t.Fatalf("expected user functionResponse content, got %#v", payload.Request.Contents[2])
		}
		if payload.Request.Contents[2].Parts[0].FunctionResponse.Name != "read_file" {
			t.Fatalf("unexpected function response name: %q", payload.Request.Contents[2].Parts[0].FunctionResponse.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, "{\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]}}]}}"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client := NewCodeAssistClient(nil, nil, CodeAssistOptions{
		Endpoint:   server.URL,
		APIVersion: "v1internal",
	})

	_, err := client.Chat(context.Background(), llm.ChatRequest{
		Model: "test-model",
		Tools: []llm.Tool{{
			FunctionDeclarations: []llm.FunctionDeclaration{{
				Name:       "read_file",
				Parameters: map[string]any{"type": "object"},
			}},
		}},
		Messages: []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello"}}},
			{Role: llm.RoleAssistant, Parts: []llm.Part{{FunctionCall: &llm.FunctionCall{
				Name: "read_file",
				Args: map[string]any{"file_path": "README.md"},
			}}}},
			{Role: llm.RoleUser, Parts: []llm.Part{{FunctionResponse: &llm.FunctionResponse{
				Name:     "read_file",
				Response: map[string]any{"content": "ok"},
			}}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestCodeAssistCountTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/v1internal:countTokens" {
			t.Fatalf("unexpected path: %s", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected Accept header: %q", got)
		}
		var payload caCountTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if err := r.Body.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if payload.Request.Model != "models/test-model" {
			t.Fatalf("unexpected model: %q", payload.Request.Model)
		}
		if len(payload.Request.Contents) != 2 {
			t.Fatalf("unexpected contents: %#v", payload.Request.Contents)
		}
		if payload.Request.Contents[0].Role != "user" {
			t.Fatalf("unexpected role: %q", payload.Request.Contents[0].Role)
		}
		if payload.Request.Contents[1].Role != "model" {
			t.Fatalf("unexpected role: %q", payload.Request.Contents[1].Role)
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, "{\"totalTokens\":42}"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client := NewCodeAssistClient(nil, nil, CodeAssistOptions{
		Endpoint:   server.URL,
		APIVersion: "v1internal",
	})

	resp, err := client.CountTokens(context.Background(), llm.CountTokensRequest{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Parts: []llm.Part{{Text: "system"}}},
			{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello"}}},
			{Role: llm.RoleAssistant, Parts: []llm.Part{{Text: "assistant"}}},
		},
	})
	if err != nil {
		t.Fatalf("CountTokens: %v", err)
	}
	if resp.TotalTokens != 42 {
		t.Fatalf("unexpected total tokens: %d", resp.TotalTokens)
	}
}
