// Package client defines the model client interface.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

const (
	defaultGeminiEndpoint = "https://generativelanguage.googleapis.com"
	defaultGeminiVersion  = "v1beta"
)

// GeminiAPIOptions configures a Gemini API client.
type GeminiAPIOptions struct {
	Endpoint      string
	APIVersion    string
	APIKey        string
	Headers       map[string]string
	HTTPClient    *http.Client
	AuthMechanism string
}

// GeminiAPIClient implements streaming chat via the Gemini API.
type GeminiAPIClient struct {
	httpClient    *http.Client
	endpoint      string
	apiVersion    string
	apiKey        string
	headers       map[string]string
	authMechanism string
}

// NewGeminiOAuthClient returns a Gemini API client that delegates auth to the
// provided httpClient (typically wrapped with AuthRoundTripper). No API key is
// required — applyAuth is a no-op when apiKey is empty.
func NewGeminiOAuthClient(httpClient *http.Client, opts GeminiAPIOptions) (*GeminiAPIClient, error) {
	if httpClient == nil {
		return nil, errors.New("httpClient is required for OAuth Gemini client")
	}
	return &GeminiAPIClient{
		httpClient: httpClient,
		endpoint:   resolveGeminiEndpoint(opts.Endpoint),
		apiVersion: resolveGeminiVersion(opts.APIVersion),
		apiKey:     "", // auth handled by httpClient transport
		headers:    cloneHeaders(opts.Headers),
	}, nil
}

// NewGeminiAPIClient returns a Gemini API client wired for API key auth.
func NewGeminiAPIClient(apiKey string, opts GeminiAPIOptions) (*GeminiAPIClient, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		key = strings.TrimSpace(opts.APIKey)
	}
	if key == "" {
		key = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	}
	if key == "" {
		return nil, errors.New("GEMINI_API_KEY is required for Gemini API auth")
	}
	base := opts.HTTPClient
	if base == nil {
		base = defaultHTTPClient()
	}
	authMechanism := strings.TrimSpace(opts.AuthMechanism)
	if authMechanism == "" {
		authMechanism = strings.TrimSpace(os.Getenv("GEMINI_API_KEY_AUTH_MECHANISM"))
	}
	if authMechanism == "" {
		authMechanism = "x-goog-api-key"
	}
	switch strings.ToLower(authMechanism) {
	case "x-goog-api-key":
		authMechanism = "x-goog-api-key"
	case "bearer":
		authMechanism = "bearer"
	default:
		return nil, fmt.Errorf("unsupported GEMINI_API_KEY_AUTH_MECHANISM %q (expected x-goog-api-key or bearer)", authMechanism)
	}
	return &GeminiAPIClient{
		httpClient:    base,
		endpoint:      resolveGeminiEndpoint(opts.Endpoint),
		apiVersion:    resolveGeminiVersion(opts.APIVersion),
		apiKey:        key,
		headers:       cloneHeaders(opts.Headers),
		authMechanism: authMechanism,
	}, nil
}

// ChatStream sends a streaming chat request to Gemini API.
func (c *GeminiAPIClient) ChatStream(ctx context.Context, req llm.ChatRequest) (Stream, error) {
	if c == nil {
		return nil, errors.New("gemini api client is nil")
	}
	if c.httpClient == nil {
		return nil, errors.New("http client is nil")
	}
	model := normalizeModel(req.Model)
	if model == "" {
		return nil, errors.New("model is required")
	}

	contents, system := toGeminiContents(req.Messages)
	payload := geminiGenerateContentRequest{
		Contents:          contents,
		SystemInstruction: system,
		Tools:             toGeminiTools(req.Tools),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := c.methodURL(model, "streamGenerateContent")
	reqURL := url + "?alt=sse"
	return doStreamRequest(requestOptions{
		ctx:       ctx,
		client:    c.httpClient,
		url:       reqURL,
		body:      body,
		accept:    "text/event-stream",
		headers:   c.headers,
		auth:      c.applyAuth,
		errPrefix: fmt.Sprintf("gemini api request failed for model %s (%s)", model, reqURL),
	}, "gemini", func(data []byte) (llm.ChatChunk, error) {
		var respPayload geminiGenerateContentResponse
		if err := json.Unmarshal(data, &respPayload); err != nil {
			return llm.ChatChunk{}, err
		}
		return llm.ChatChunk{
			Text:       extractGeminiText(respPayload),
			Usage:      toGeminiUsage(respPayload.UsageMetadata),
			Done:       false,
			Tools:      extractGeminiToolCalls(respPayload),
			Grounding:  extractGeminiGrounding(respPayload),
			URLContext: extractGeminiURLContext(respPayload),
		}, nil
	})
}

// Chat sends a non-streaming chat request to Gemini API.
func (c *GeminiAPIClient) Chat(ctx context.Context, req llm.ChatRequest) (chunk llm.ChatChunk, err error) {
	if c == nil {
		return llm.ChatChunk{}, errors.New("gemini api client is nil")
	}
	if c.httpClient == nil {
		return llm.ChatChunk{}, errors.New("http client is nil")
	}
	model := normalizeModel(req.Model)
	if model == "" {
		return llm.ChatChunk{}, errors.New("model is required")
	}

	contents, system := toGeminiContents(req.Messages)
	payload := geminiGenerateContentRequest{
		Contents:          contents,
		SystemInstruction: system,
		Tools:             toGeminiTools(req.Tools),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.ChatChunk{}, err
	}

	url := c.methodURL(model, "generateContent")
	resp, err := doJSONRequest(requestOptions{
		ctx:       ctx,
		client:    c.httpClient,
		url:       url,
		body:      body,
		headers:   c.headers,
		auth:      c.applyAuth,
		errPrefix: fmt.Sprintf("gemini api request failed for model %s (%s)", model, url),
	})
	if err != nil {
		return llm.ChatChunk{}, err
	}

	var respPayload geminiGenerateContentResponse
	if err := decodeJSONResponse(resp, &respPayload); err != nil {
		return llm.ChatChunk{}, err
	}

	chunk = llm.ChatChunk{
		Text:       extractGeminiText(respPayload),
		Usage:      toGeminiUsage(respPayload.UsageMetadata),
		Done:       true,
		Tools:      extractGeminiToolCalls(respPayload),
		Grounding:  extractGeminiGrounding(respPayload),
		URLContext: extractGeminiURLContext(respPayload),
	}
	return chunk, nil
}

// CountTokens requests token counts for the provided messages.
func (c *GeminiAPIClient) CountTokens(ctx context.Context, req llm.CountTokensRequest) (resp llm.CountTokensResponse, err error) {
	if c == nil {
		return llm.CountTokensResponse{}, errors.New("gemini api client is nil")
	}
	if c.httpClient == nil {
		return llm.CountTokensResponse{}, errors.New("http client is nil")
	}
	model := normalizeModel(req.Model)
	if model == "" {
		return llm.CountTokensResponse{}, errors.New("model is required")
	}

	payload := geminiCountTokenRequest{
		Contents: toGeminiCountTokenContents(req.Messages),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.CountTokensResponse{}, err
	}

	url := c.methodURL(model, "countTokens")
	httpResp, err := doJSONRequest(requestOptions{
		ctx:       ctx,
		client:    c.httpClient,
		url:       url,
		body:      body,
		headers:   c.headers,
		auth:      c.applyAuth,
		errPrefix: fmt.Sprintf("gemini api request failed for model %s (%s)", model, url),
	})
	if err != nil {
		return llm.CountTokensResponse{}, err
	}

	var respPayload geminiCountTokenResponse
	if err := decodeJSONResponse(httpResp, &respPayload); err != nil {
		return llm.CountTokensResponse{}, err
	}
	resp = llm.CountTokensResponse{TotalTokens: respPayload.TotalTokens}
	return resp, nil
}

func (c *GeminiAPIClient) methodURL(model, method string) string {
	return fmt.Sprintf("%s/%s/models/%s:%s", c.endpoint, c.apiVersion, model, method)
}

func (c *GeminiAPIClient) applyAuth(req *http.Request) {
	if c.apiKey == "" {
		return
	}
	if strings.EqualFold(c.authMechanism, "bearer") {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		return
	}
	req.Header.Set("x-goog-api-key", c.apiKey)
}

func resolveGeminiEndpoint(value string) string {
	if value != "" {
		return strings.TrimRight(value, "/")
	}
	return defaultGeminiEndpoint
}

func resolveGeminiVersion(value string) string {
	if value != "" {
		return value
	}
	return defaultGeminiVersion
}

type geminiGenerateContentRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Tools             []geminiTool    `json:"tools,omitempty"`
}

type geminiGenerateContentResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
}

type geminiCountTokenRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiCountTokenResponse struct {
	TotalTokens int `json:"totalTokens"`
}

type geminiCandidate struct {
	Content            geminiContent             `json:"content"`
	GroundingMetadata  *geminiGroundingMetadata  `json:"groundingMetadata,omitempty"`
	URLContextMetadata *geminiURLContextMetadata `json:"urlContextMetadata,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall   `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResult `json:"functionResponse,omitempty"`
}

type geminiGroundingMetadata struct {
	GroundingChunks   []geminiGroundingChunk   `json:"groundingChunks,omitempty"`
	GroundingSupports []geminiGroundingSupport `json:"groundingSupports,omitempty"`
}

type geminiGroundingChunk struct {
	Web *geminiGroundingWeb `json:"web,omitempty"`
}

type geminiGroundingWeb struct {
	URI   string `json:"uri,omitempty"`
	Title string `json:"title,omitempty"`
}

type geminiGroundingSupport struct {
	Segment               *geminiGroundingSegment `json:"segment,omitempty"`
	GroundingChunkIndices []int                   `json:"groundingChunkIndices,omitempty"`
	ConfidenceScores      []float64               `json:"confidenceScores,omitempty"`
}

type geminiGroundingSegment struct {
	StartIndex int    `json:"startIndex"`
	EndIndex   int    `json:"endIndex"`
	Text       string `json:"text,omitempty"`
}

type geminiURLContextMetadata struct {
	URLMetadata []geminiURLMetadata `json:"urlMetadata,omitempty"`
}

type geminiURLMetadata struct {
	URLRetrievalStatus string `json:"urlRetrievalStatus,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiFunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiFunctionResult struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response,omitempty"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func toGeminiUsage(meta *geminiUsage) *llm.Usage {
	if meta == nil {
		return nil
	}
	return &llm.Usage{
		PromptTokens:     meta.PromptTokenCount,
		CompletionTokens: meta.CandidatesTokenCount,
		TotalTokens:      meta.TotalTokenCount,
	}
}

func extractGeminiText(resp geminiGenerateContentResponse) string {
	if len(resp.Candidates) == 0 {
		return ""
	}
	first := resp.Candidates[0]
	var b strings.Builder
	for _, part := range first.Content.Parts {
		if part.Text != "" {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

func extractGeminiToolCalls(resp geminiGenerateContentResponse) []llm.FunctionCall {
	if len(resp.Candidates) == 0 {
		return nil
	}
	first := resp.Candidates[0]
	var calls []llm.FunctionCall
	for _, part := range first.Content.Parts {
		if part.FunctionCall == nil {
			continue
		}
		call := llm.FunctionCall{
			ID:   part.FunctionCall.ID,
			Name: part.FunctionCall.Name,
			Args: part.FunctionCall.Args,
		}
		calls = append(calls, call)
	}
	if len(calls) == 0 {
		return nil
	}
	return calls
}

func extractGeminiGrounding(resp geminiGenerateContentResponse) *llm.GroundingMetadata {
	if len(resp.Candidates) == 0 {
		return nil
	}
	meta := resp.Candidates[0].GroundingMetadata
	if meta == nil {
		return nil
	}
	out := &llm.GroundingMetadata{}
	if len(meta.GroundingChunks) > 0 {
		out.GroundingChunks = make([]llm.GroundingChunk, 0, len(meta.GroundingChunks))
		for _, chunk := range meta.GroundingChunks {
			var web *llm.GroundingWeb
			if chunk.Web != nil {
				web = &llm.GroundingWeb{
					URI:   chunk.Web.URI,
					Title: chunk.Web.Title,
				}
			}
			out.GroundingChunks = append(out.GroundingChunks, llm.GroundingChunk{Web: web})
		}
	}
	if len(meta.GroundingSupports) > 0 {
		out.GroundingSupports = make([]llm.GroundingSupport, 0, len(meta.GroundingSupports))
		for _, support := range meta.GroundingSupports {
			var segment *llm.GroundingSegment
			if support.Segment != nil {
				segment = &llm.GroundingSegment{
					StartIndex: support.Segment.StartIndex,
					EndIndex:   support.Segment.EndIndex,
					Text:       support.Segment.Text,
				}
			}
			out.GroundingSupports = append(out.GroundingSupports, llm.GroundingSupport{
				Segment:               segment,
				GroundingChunkIndices: append([]int(nil), support.GroundingChunkIndices...),
				ConfidenceScores:      append([]float64(nil), support.ConfidenceScores...),
			})
		}
	}
	if len(out.GroundingChunks) == 0 && len(out.GroundingSupports) == 0 {
		return nil
	}
	return out
}

func extractGeminiURLContext(resp geminiGenerateContentResponse) *llm.URLContextMetadata {
	if len(resp.Candidates) == 0 {
		return nil
	}
	meta := resp.Candidates[0].URLContextMetadata
	if meta == nil || len(meta.URLMetadata) == 0 {
		return nil
	}
	out := &llm.URLContextMetadata{
		URLMetadata: make([]llm.URLMetadata, 0, len(meta.URLMetadata)),
	}
	for _, entry := range meta.URLMetadata {
		out.URLMetadata = append(out.URLMetadata, llm.URLMetadata{
			URLRetrievalStatus: entry.URLRetrievalStatus,
		})
	}
	return out
}

func toGeminiContents(messages []llm.Message) ([]geminiContent, *geminiContent) {
	var contents []geminiContent
	var systemParts []geminiPart
	for _, msg := range messages {
		parts := toGeminiParts(msg.Parts)
		if len(parts) == 0 {
			continue
		}
		switch msg.Role {
		case llm.RoleSystem:
			systemParts = append(systemParts, parts...)
		case llm.RoleAssistant:
			contents = append(contents, geminiContent{Role: "model", Parts: parts})
		default:
			contents = append(contents, geminiContent{Role: "user", Parts: parts})
		}
	}
	var system *geminiContent
	if len(systemParts) > 0 {
		system = &geminiContent{Role: "system", Parts: systemParts}
	}
	return contents, system
}

func toGeminiCountTokenContents(messages []llm.Message) []geminiContent {
	contents := make([]geminiContent, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == llm.RoleSystem {
			continue
		}
		parts := toGeminiParts(msg.Parts)
		if len(parts) == 0 {
			continue
		}
		role := "user"
		switch msg.Role {
		case llm.RoleAssistant:
			role = "model"
		case llm.RoleSystem:
			role = "system"
		}
		contents = append(contents, geminiContent{Role: role, Parts: parts})
	}
	return contents
}

func toGeminiParts(parts []llm.Part) []geminiPart {
	out := make([]geminiPart, 0, len(parts))
	for _, part := range parts {
		if part.Text != "" {
			out = append(out, geminiPart{Text: part.Text})
		}
		if part.FunctionCall != nil {
			out = append(out, geminiPart{FunctionCall: toGeminiFunctionCall(part.FunctionCall)})
		}
		if part.FunctionResponse != nil {
			out = append(out, geminiPart{FunctionResponse: toGeminiFunctionResponse(part.FunctionResponse)})
		}
	}
	return out
}

func toGeminiFunctionCall(call *llm.FunctionCall) *geminiFunctionCall {
	if call == nil {
		return nil
	}
	return &geminiFunctionCall{
		ID:   call.ID,
		Name: call.Name,
		Args: call.Args,
	}
}

func toGeminiFunctionResponse(resp *llm.FunctionResponse) *geminiFunctionResult {
	if resp == nil {
		return nil
	}
	return &geminiFunctionResult{
		ID:       resp.ID,
		Name:     resp.Name,
		Response: resp.Response,
	}
}

func toGeminiTools(tools []llm.Tool) []geminiTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]geminiTool, 0, len(tools))
	for _, tool := range tools {
		if len(tool.FunctionDeclarations) == 0 {
			continue
		}
		decls := make([]geminiFunctionDeclaration, 0, len(tool.FunctionDeclarations))
		for _, decl := range tool.FunctionDeclarations {
			decls = append(decls, geminiFunctionDeclaration{
				Name:        decl.Name,
				Description: decl.Description,
				Parameters:  decl.Parameters,
			})
		}
		out = append(out, geminiTool{FunctionDeclarations: decls})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
