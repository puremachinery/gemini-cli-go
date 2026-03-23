package tools

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

type fetchStubClient struct {
	requests  []llm.ChatRequest
	responses []llm.ChatChunk
	errs      []error
}

func (s *fetchStubClient) ChatStream(_ context.Context, _ llm.ChatRequest) (client.Stream, error) {
	return nil, errors.New("not implemented")
}

func (s *fetchStubClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatChunk, error) {
	s.requests = append(s.requests, req)
	index := len(s.requests) - 1
	if index < len(s.errs) && s.errs[index] != nil {
		return llm.ChatChunk{}, s.errs[index]
	}
	if index < len(s.responses) {
		return s.responses[index], nil
	}
	return llm.ChatChunk{}, nil
}

func (s *fetchStubClient) CountTokens(_ context.Context, _ llm.CountTokensRequest) (llm.CountTokensResponse, error) {
	return llm.CountTokensResponse{}, nil
}

func TestWebFetchBuildValidatesPrompt(t *testing.T) {
	tool := NewWebFetchTool(Context{AllowPrivateWebFetch: true})
	if _, err := tool.Build(map[string]any{"prompt": "   "}); err == nil {
		t.Fatal("expected error for empty prompt")
	}
	if _, err := tool.Build(map[string]any{"prompt": "Check ftp://example.com"}); err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
	if _, err := tool.Build(map[string]any{"prompt": "Check example.com"}); err == nil {
		t.Fatal("expected error for missing URL scheme")
	}
	if _, err := tool.Build(map[string]any{"prompt": "Check https://example.com"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebFetchPrimaryFormatsSources(t *testing.T) {
	stub := &fetchStubClient{
		responses: []llm.ChatChunk{{
			Text: "Cats are cute.",
			Grounding: &llm.GroundingMetadata{
				GroundingChunks: []llm.GroundingChunk{{
					Web: &llm.GroundingWeb{
						URI:   "https://example.com",
						Title: "Example",
					},
				}},
				GroundingSupports: []llm.GroundingSupport{{
					Segment: &llm.GroundingSegment{
						StartIndex: 0,
						EndIndex:   len("Cats"),
						Text:       "Cats",
					},
					GroundingChunkIndices: []int{0},
				}},
			},
		}},
	}
	tool := NewWebFetchTool(Context{GeminiClient: stub, AllowPrivateWebFetch: true})
	inv, err := tool.Build(map[string]any{"prompt": "Summarize https://example.com"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	res, err := inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected error: %#v", res)
	}
	if len(stub.requests) != 1 || stub.requests[0].Model != "web-fetch" {
		t.Fatalf("expected web-fetch request, got %#v", stub.requests)
	}
	if !strings.Contains(res.Output, "Cats[1] are cute.") {
		t.Fatalf("expected citation marker, got: %q", res.Output)
	}
	if !strings.Contains(res.Output, "Sources:\n[1] Example (https://example.com)") {
		t.Fatalf("expected sources list, got: %q", res.Output)
	}
}

func TestWebFetchFallbackUsesRawContent(t *testing.T) {
	stub := &fetchStubClient{
		responses: []llm.ChatChunk{{
			Text: "Summary",
		}},
	}
	tool := NewWebFetchTool(Context{GeminiClient: stub, AllowPrivateWebFetch: true})
	inv, err := tool.Build(map[string]any{"prompt": "Summarize http://127.0.0.1/test"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	origFetch := webFetchHTTPGet
	origHTML := webFetchHTMLToText
	webFetchHTTPGet = func(_ context.Context, _ string, _ time.Duration) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("Hello World")),
		}
		resp.Header.Set("content-type", "text/plain")
		return resp, nil
	}
	webFetchHTMLToText = func(raw string) string { return raw }
	t.Cleanup(func() {
		webFetchHTTPGet = origFetch
		webFetchHTMLToText = origHTML
	})

	res, err := inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected error: %#v", res)
	}
	if len(stub.requests) != 1 {
		t.Fatalf("expected one fallback request, got %d", len(stub.requests))
	}
	req := stub.requests[0]
	if req.Model != "web-fetch-fallback" {
		t.Fatalf("expected web-fetch-fallback model, got %q", req.Model)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Parts) != 1 {
		t.Fatalf("unexpected fallback prompt payload: %#v", req.Messages)
	}
	text := req.Messages[0].Parts[0].Text
	if !strings.Contains(text, "Hello World") {
		t.Fatalf("expected fetched content in fallback prompt, got: %q", text)
	}
	if !strings.Contains(text, "Do not attempt to access the URL again.") {
		t.Fatalf("expected safety instruction in fallback prompt, got: %q", text)
	}
	if res.Output != "Summary" {
		t.Fatalf("expected fallback summary, got: %q", res.Output)
	}
}

func TestWebFetchFallbackForLocalhost(t *testing.T) {
	stub := &fetchStubClient{
		responses: []llm.ChatChunk{{
			Text: "Summary",
		}},
	}
	tool := NewWebFetchTool(Context{GeminiClient: stub, AllowPrivateWebFetch: true})
	inv, err := tool.Build(map[string]any{"prompt": "Summarize http://localhost:8080/test"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	origFetch := webFetchHTTPGet
	origHTML := webFetchHTMLToText
	webFetchHTTPGet = func(_ context.Context, _ string, _ time.Duration) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("Hello World")),
		}
		resp.Header.Set("content-type", "text/plain")
		return resp, nil
	}
	webFetchHTMLToText = func(raw string) string { return raw }
	t.Cleanup(func() {
		webFetchHTTPGet = origFetch
		webFetchHTMLToText = origHTML
	})

	res, err := inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected error: %#v", res)
	}
	if len(stub.requests) != 1 {
		t.Fatalf("expected one fallback request, got %d", len(stub.requests))
	}
	if stub.requests[0].Model != "web-fetch-fallback" {
		t.Fatalf("expected web-fetch-fallback model, got %q", stub.requests[0].Model)
	}
}

func TestWebFetchFallbackOnURLMetadataFailure(t *testing.T) {
	stub := &fetchStubClient{
		responses: []llm.ChatChunk{{
			Text: "Hallucinated",
			URLContext: &llm.URLContextMetadata{
				URLMetadata: []llm.URLMetadata{{
					URLRetrievalStatus: "URL_RETRIEVAL_STATUS_FAILED",
				}},
			},
		}, {
			Text: "Fallback summary",
		}},
	}
	tool := NewWebFetchTool(Context{GeminiClient: stub, AllowPrivateWebFetch: true})
	inv, err := tool.Build(map[string]any{"prompt": "Summarize https://example.com"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	origFetch := webFetchHTTPGet
	origHTML := webFetchHTMLToText
	webFetchHTTPGet = func(_ context.Context, _ string, _ time.Duration) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("Hello World")),
		}
		resp.Header.Set("content-type", "text/plain")
		return resp, nil
	}
	webFetchHTMLToText = func(raw string) string { return raw }
	t.Cleanup(func() {
		webFetchHTTPGet = origFetch
		webFetchHTMLToText = origHTML
	})

	res, err := inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Output != "Fallback summary" {
		t.Fatalf("expected fallback summary, got: %q", res.Output)
	}
	if len(stub.requests) != 2 {
		t.Fatalf("expected two requests (primary + fallback), got %d", len(stub.requests))
	}
	if stub.requests[0].Model != "web-fetch" || stub.requests[1].Model != "web-fetch-fallback" {
		t.Fatalf("unexpected model sequence: %#v", stub.requests)
	}
}

func TestWebFetchCitationsUseRuneIndices(t *testing.T) {
	stub := &fetchStubClient{
		responses: []llm.ChatChunk{{
			Text: "猫は可愛い",
			Grounding: &llm.GroundingMetadata{
				GroundingChunks: []llm.GroundingChunk{{Web: &llm.GroundingWeb{URI: "https://example.com"}}},
				GroundingSupports: []llm.GroundingSupport{{
					Segment: &llm.GroundingSegment{
						StartIndex: 0,
						EndIndex:   1,
						Text:       "猫",
					},
					GroundingChunkIndices: []int{0},
				}},
			},
		}},
	}
	tool := NewWebFetchTool(Context{GeminiClient: stub, AllowPrivateWebFetch: true})
	inv, err := tool.Build(map[string]any{"prompt": "Summarize https://example.com"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	res, err := inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "猫[1]は可愛い") {
		t.Fatalf("expected citation marker after first rune, got: %q", res.Output)
	}
}
