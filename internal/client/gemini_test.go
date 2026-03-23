package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

func TestGeminiChatStreamParsesSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/v1beta/models/test-model:streamGenerateContent" {
			t.Fatalf("unexpected path: %s", got)
		}
		if got := r.URL.Query().Get("alt"); got != "sse" {
			t.Fatalf("unexpected alt query: %s", got)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "api-key" {
			t.Fatalf("unexpected api key header: %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]}}]}\n\n"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
		if _, err := io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\" World\"}]}}]}\n\n"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewGeminiAPIClient("api-key", GeminiAPIOptions{
		Endpoint:   server.URL,
		APIVersion: "v1beta",
	})
	if err != nil {
		t.Fatalf("NewGeminiAPIClient: %v", err)
	}

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

func TestGeminiChatIncludesToolsAndFunctionParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload geminiGenerateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if err := r.Body.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if len(payload.Tools) != 1 {
			t.Fatalf("unexpected tools: %#v", payload.Tools)
		}
		decls := payload.Tools[0].FunctionDeclarations
		if len(decls) != 1 || decls[0].Name != "read_file" {
			t.Fatalf("unexpected tool declarations: %#v", decls)
		}
		if len(payload.Contents) != 3 {
			t.Fatalf("unexpected contents: %#v", payload.Contents)
		}
		if payload.Contents[0].Role != "user" {
			t.Fatalf("unexpected role: %q", payload.Contents[0].Role)
		}
		if payload.Contents[1].Role != "model" || payload.Contents[1].Parts[0].FunctionCall == nil {
			t.Fatalf("expected model functionCall content, got %#v", payload.Contents[1])
		}
		if payload.Contents[2].Role != "user" || payload.Contents[2].Parts[0].FunctionResponse == nil {
			t.Fatalf("expected user functionResponse content, got %#v", payload.Contents[2])
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, "{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]}}]}"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewGeminiAPIClient("api-key", GeminiAPIOptions{
		Endpoint:   server.URL,
		APIVersion: "v1beta",
	})
	if err != nil {
		t.Fatalf("NewGeminiAPIClient: %v", err)
	}

	_, err = client.Chat(context.Background(), llm.ChatRequest{
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

func TestGeminiCountTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/v1beta/models/test-model:countTokens" {
			t.Fatalf("unexpected path: %s", got)
		}
		var payload geminiCountTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if err := r.Body.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if len(payload.Contents) != 2 {
			t.Fatalf("unexpected contents: %#v", payload.Contents)
		}
		if payload.Contents[0].Role != "user" {
			t.Fatalf("unexpected role: %q", payload.Contents[0].Role)
		}
		if payload.Contents[1].Role != "model" {
			t.Fatalf("unexpected role: %q", payload.Contents[1].Role)
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, "{\"totalTokens\":42}"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewGeminiAPIClient("api-key", GeminiAPIOptions{
		Endpoint:   server.URL,
		APIVersion: "v1beta",
	})
	if err != nil {
		t.Fatalf("NewGeminiAPIClient: %v", err)
	}

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

func TestGeminiChatRetriesOnRetryableStatus(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			if _, err := io.WriteString(w, "rate limited"); err != nil {
				t.Fatalf("WriteString: %v", err)
			}
			return
		}
		if got := r.URL.Path; got != "/v1beta/models/test-model:generateContent" {
			t.Fatalf("unexpected path: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, "{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]}}]}"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewGeminiAPIClient("api-key", GeminiAPIOptions{
		Endpoint:   server.URL,
		APIVersion: "v1beta",
	})
	if err != nil {
		t.Fatalf("NewGeminiAPIClient: %v", err)
	}

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

func TestNewGeminiOAuthClientUsesTransportAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "" {
			t.Fatalf("expected no x-goog-api-key header, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer oauth-token" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, "{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]}}]}"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	// Create an HTTP client whose transport injects an OAuth bearer token.
	oauthClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.Header.Set("Authorization", "Bearer oauth-token")
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	client, err := NewGeminiOAuthClient(oauthClient, GeminiAPIOptions{
		Endpoint:   server.URL,
		APIVersion: "v1beta",
	})
	if err != nil {
		t.Fatalf("NewGeminiOAuthClient: %v", err)
	}

	_, err = client.Chat(context.Background(), llm.ChatRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestNewGeminiOAuthClientNilHTTPClient(t *testing.T) {
	_, err := NewGeminiOAuthClient(nil, GeminiAPIOptions{})
	if err == nil {
		t.Fatal("expected error for nil httpClient")
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGeminiAuthMechanismBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer api-key" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, "{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]}}]}"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewGeminiAPIClient("api-key", GeminiAPIOptions{
		Endpoint:      server.URL,
		APIVersion:    "v1beta",
		AuthMechanism: "bearer",
	})
	if err != nil {
		t.Fatalf("NewGeminiAPIClient: %v", err)
	}

	_, err = client.Chat(context.Background(), llm.ChatRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Text: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}
