package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

// WebSearchToolName is the tool identifier for google_web_search.
const WebSearchToolName = "google_web_search"

// WebSearchTool implements the google_web_search tool.
type WebSearchTool struct {
	ctx Context
}

// WebSearchParams holds arguments for google_web_search.
type WebSearchParams struct {
	Query string `json:"query"`
}

type webSearchInvocation struct {
	params WebSearchParams
	ctx    Context
}

// NewWebSearchTool constructs a WebSearchTool.
func NewWebSearchTool(ctx Context) *WebSearchTool {
	return &WebSearchTool{ctx: ctx}
}

// Name returns the tool name.
func (t *WebSearchTool) Name() string { return WebSearchToolName }

// Description returns the tool description.
func (t *WebSearchTool) Description() string {
	return "Performs a web search using Google Search (via the Gemini API) and returns the results."
}

// Parameters returns the JSON schema for google_web_search.
func (t *WebSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query to find information on the web.",
			},
		},
		"required": []string{"query"},
	}
}

// Build validates args and returns an invocation.
func (t *WebSearchTool) Build(args map[string]any) (Invocation, error) {
	query, err := getStringArg(args, "query")
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, errors.New("query cannot be empty")
	}
	return &webSearchInvocation{
		params: WebSearchParams{Query: trimmed},
		ctx:    t.ctx,
	}, nil
}

func (i *webSearchInvocation) Name() string { return WebSearchToolName }

func (i *webSearchInvocation) Description() string {
	return fmt.Sprintf("Searching the web for: %q", i.params.Query)
}

func (i *webSearchInvocation) ConfirmationRequest() *ConfirmationRequest {
	return nil
}

func (i *webSearchInvocation) Execute(ctx context.Context) (Result, error) {
	if i.ctx.GeminiClient == nil {
		return Result{Error: "model client is not configured"}, nil
	}
	if strings.TrimSpace(i.params.Query) == "" {
		return Result{Error: "query is required"}, nil
	}
	req := llm.ChatRequest{
		Model: "web-search",
		Messages: []llm.Message{{
			Role: llm.RoleUser,
			Parts: []llm.Part{{
				Text: i.params.Query,
			}},
		}},
	}
	resp, err := i.ctx.GeminiClient.Chat(ctx, req)
	if err != nil {
		message := fmt.Sprintf("Error during web search for query %q: %v", i.params.Query, err)
		return Result{
			Output:  "Error: " + message,
			Error:   message,
			Display: "Error performing web search.",
		}, nil
	}
	responseText := resp.Text
	if strings.TrimSpace(responseText) == "" {
		return Result{
			Output:  fmt.Sprintf("No search results or information found for query: %q", i.params.Query),
			Display: "No information found.",
		}, nil
	}

	modified := responseText
	grounding := resp.Grounding
	var sources []llm.GroundingChunk
	var supports []llm.GroundingSupport
	if grounding != nil {
		sources = grounding.GroundingChunks
		supports = grounding.GroundingSupports
	}
	if len(sources) > 0 {
		if len(supports) > 0 {
			modified = insertCitations(modified, supports)
		}
		sourceList := make([]string, 0, len(sources))
		for idx, source := range sources {
			title := "Untitled"
			uri := "No URI"
			if source.Web != nil {
				if strings.TrimSpace(source.Web.Title) != "" {
					title = source.Web.Title
				}
				if strings.TrimSpace(source.Web.URI) != "" {
					uri = source.Web.URI
				}
			}
			sourceList = append(sourceList, fmt.Sprintf("[%d] %s (%s)", idx+1, title, uri))
		}
		modified += "\n\nSources:\n" + strings.Join(sourceList, "\n")
	}

	return Result{
		Output:  fmt.Sprintf("Web search results for %q:\n\n%s", i.params.Query, modified),
		Display: fmt.Sprintf("Search results for %q returned.", i.params.Query),
	}, nil
}
