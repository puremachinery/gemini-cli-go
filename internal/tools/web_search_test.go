package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/client"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

type stubClient struct {
	lastReq llm.ChatRequest
	resp    llm.ChatChunk
	err     error
}

func (s *stubClient) ChatStream(_ context.Context, _ llm.ChatRequest) (client.Stream, error) {
	return nil, errors.New("not implemented")
}

func (s *stubClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatChunk, error) {
	s.lastReq = req
	return s.resp, s.err
}

func (s *stubClient) CountTokens(_ context.Context, _ llm.CountTokensRequest) (llm.CountTokensResponse, error) {
	return llm.CountTokensResponse{}, nil
}

func TestWebSearchBuildRejectsEmptyQuery(t *testing.T) {
	tool := NewWebSearchTool(Context{})
	if _, err := tool.Build(map[string]any{"query": "   "}); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestWebSearchFormatsSourcesAndCitations(t *testing.T) {
	stub := &stubClient{
		resp: llm.ChatChunk{
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
		},
	}
	tool := NewWebSearchTool(Context{GeminiClient: stub})
	inv, err := tool.Build(map[string]any{"query": "cats"})
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
	if stub.lastReq.Model != "web-search" {
		t.Fatalf("expected web-search model, got %q", stub.lastReq.Model)
	}
	if len(stub.lastReq.Messages) != 1 || stub.lastReq.Messages[0].Role != llm.RoleUser {
		t.Fatalf("expected single user message, got %#v", stub.lastReq.Messages)
	}
	if got := stub.lastReq.Messages[0].Parts[0].Text; got != "cats" {
		t.Fatalf("unexpected query text: %q", got)
	}
	if !strings.Contains(res.Output, "Cats[1] are cute.") {
		t.Fatalf("expected citation marker, got: %q", res.Output)
	}
	if !strings.Contains(res.Output, "Sources:\n[1] Example (https://example.com)") {
		t.Fatalf("expected sources list, got: %q", res.Output)
	}
	if res.Display != "Search results for \"cats\" returned." {
		t.Fatalf("unexpected display: %q", res.Display)
	}
}

func TestWebSearchReturnsNoInformation(t *testing.T) {
	stub := &stubClient{resp: llm.ChatChunk{Text: "   "}}
	tool := NewWebSearchTool(Context{GeminiClient: stub})
	inv, err := tool.Build(map[string]any{"query": "empty"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	res, err := inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Display != "No information found." {
		t.Fatalf("unexpected display: %q", res.Display)
	}
	if !strings.Contains(res.Output, "No search results or information found") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestWebSearchErrorResponse(t *testing.T) {
	stub := &stubClient{err: errors.New("boom")}
	tool := NewWebSearchTool(Context{GeminiClient: stub})
	inv, err := tool.Build(map[string]any{"query": "oops"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	res, err := inv.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error == "" || !strings.Contains(res.Error, "boom") {
		t.Fatalf("expected error to include boom, got: %#v", res)
	}
	if res.Display != "Error performing web search." {
		t.Fatalf("unexpected display: %q", res.Display)
	}
}
