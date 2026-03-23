// Package client defines the model client interface.
package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/auth"
	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

const (
	defaultCodeAssistEndpoint = "https://cloudcode-pa.googleapis.com"
	defaultCodeAssistVersion  = "v1internal"
)

// CodeAssistOptions configures a Code Assist client.
type CodeAssistOptions struct {
	Endpoint     string
	APIVersion   string
	ProjectID    string
	SessionID    string
	Headers      map[string]string
	HTTPClient   *http.Client
	UserPromptID func() string
}

// CodeAssistClient implements streaming chat via the Code Assist API.
type CodeAssistClient struct {
	httpClient   *http.Client
	endpoint     string
	apiVersion   string
	projectID    string
	sessionID    string
	headers      map[string]string
	userPromptID func() string
}

// NewCodeAssistClient returns a CodeAssist client wired for OAuth tokens.
func NewCodeAssistClient(provider auth.Provider, store auth.Store, opts CodeAssistOptions) *CodeAssistClient {
	base := opts.HTTPClient
	if base == nil {
		base = defaultHTTPClient()
	}
	if provider != nil && store != nil {
		base = NewAuthenticatedClient(provider, store, base)
	}
	return &CodeAssistClient{
		httpClient:   base,
		endpoint:     resolveEndpoint(opts.Endpoint),
		apiVersion:   resolveAPIVersion(opts.APIVersion),
		projectID:    opts.ProjectID,
		sessionID:    opts.SessionID,
		headers:      cloneHeaders(opts.Headers),
		userPromptID: resolvePromptID(opts.UserPromptID),
	}
}

// ChatStream sends a streaming chat request to Code Assist.
func (c *CodeAssistClient) ChatStream(ctx context.Context, req llm.ChatRequest) (Stream, error) {
	if c == nil {
		return nil, errors.New("code assist client is nil")
	}
	if c.httpClient == nil {
		return nil, errors.New("http client is nil")
	}
	model := normalizeModel(req.Model)
	if model == "" {
		return nil, errors.New("model is required")
	}

	contents, system := toCodeAssistContents(req.Messages)
	payload := caGenerateContentRequest{
		Model:        model,
		Project:      c.projectID,
		UserPromptID: c.userPromptID(),
		Request: caVertexGenerateContentRequest{
			Contents:          contents,
			SystemInstruction: system,
			SessionID:         c.sessionID,
			Tools:             toCodeAssistTools(req.Tools),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := c.methodURL("streamGenerateContent")
	reqURL := url + "?alt=sse"
	return doStreamRequest(requestOptions{
		ctx:       ctx,
		client:    c.httpClient,
		url:       reqURL,
		body:      body,
		accept:    "text/event-stream",
		headers:   c.headers,
		errPrefix: fmt.Sprintf("code assist request failed for model %s (%s)", model, reqURL),
	}, "codeassist", func(data []byte) (llm.ChatChunk, error) {
		var respPayload caGenerateContentResponse
		if err := json.Unmarshal(data, &respPayload); err != nil {
			return llm.ChatChunk{}, err
		}
		return llm.ChatChunk{
			Text:  extractText(respPayload),
			Usage: toUsage(respPayload.Response.UsageMetadata),
			Done:  false,
			Tools: extractToolCalls(respPayload),
		}, nil
	})
}

// Chat sends a non-streaming chat request to Code Assist.
func (c *CodeAssistClient) Chat(ctx context.Context, req llm.ChatRequest) (chunk llm.ChatChunk, err error) {
	if c == nil {
		return llm.ChatChunk{}, errors.New("code assist client is nil")
	}
	if c.httpClient == nil {
		return llm.ChatChunk{}, errors.New("http client is nil")
	}
	model := normalizeModel(req.Model)
	if model == "" {
		return llm.ChatChunk{}, errors.New("model is required")
	}

	contents, system := toCodeAssistContents(req.Messages)
	payload := caGenerateContentRequest{
		Model:        model,
		Project:      c.projectID,
		UserPromptID: c.userPromptID(),
		Request: caVertexGenerateContentRequest{
			Contents:          contents,
			SystemInstruction: system,
			SessionID:         c.sessionID,
			Tools:             toCodeAssistTools(req.Tools),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.ChatChunk{}, err
	}

	url := c.methodURL("generateContent")
	resp, err := doJSONRequest(requestOptions{
		ctx:       ctx,
		client:    c.httpClient,
		url:       url,
		body:      body,
		headers:   c.headers,
		errPrefix: fmt.Sprintf("code assist request failed for model %s (%s)", model, url),
	})
	if err != nil {
		return llm.ChatChunk{}, err
	}

	var respPayload caGenerateContentResponse
	if err := decodeJSONResponse(resp, &respPayload); err != nil {
		return llm.ChatChunk{}, err
	}

	chunk = llm.ChatChunk{
		Text:  extractText(respPayload),
		Usage: toUsage(respPayload.Response.UsageMetadata),
		Done:  true,
		Tools: extractToolCalls(respPayload),
	}
	return chunk, nil
}

// CountTokens requests token counts for the provided messages.
func (c *CodeAssistClient) CountTokens(ctx context.Context, req llm.CountTokensRequest) (resp llm.CountTokensResponse, err error) {
	if c == nil {
		return llm.CountTokensResponse{}, errors.New("code assist client is nil")
	}
	if c.httpClient == nil {
		return llm.CountTokensResponse{}, errors.New("http client is nil")
	}
	model := normalizeModel(req.Model)
	if model == "" {
		return llm.CountTokensResponse{}, errors.New("model is required")
	}

	payload := caCountTokenRequest{
		Request: caVertexCountTokenRequest{
			Model:    "models/" + model,
			Contents: toCountTokenContents(req.Messages),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.CountTokensResponse{}, err
	}

	url := c.methodURL("countTokens")
	httpResp, err := doJSONRequest(requestOptions{
		ctx:       ctx,
		client:    c.httpClient,
		url:       url,
		body:      body,
		headers:   c.headers,
		errPrefix: fmt.Sprintf("code assist request failed for model %s (%s)", model, url),
	})
	if err != nil {
		return llm.CountTokensResponse{}, err
	}

	var respPayload caCountTokenResponse
	if err := decodeJSONResponse(httpResp, &respPayload); err != nil {
		return llm.CountTokensResponse{}, err
	}
	resp = llm.CountTokensResponse{TotalTokens: respPayload.TotalTokens}
	return resp, nil
}

func (c *CodeAssistClient) methodURL(method string) string {
	return fmt.Sprintf("%s/%s:%s", c.endpoint, c.apiVersion, method)
}

func resolveEndpoint(value string) string {
	if value != "" {
		return strings.TrimRight(value, "/")
	}
	if env := os.Getenv("CODE_ASSIST_ENDPOINT"); env != "" {
		return strings.TrimRight(env, "/")
	}
	return defaultCodeAssistEndpoint
}

func resolveAPIVersion(value string) string {
	if value != "" {
		return value
	}
	if env := os.Getenv("CODE_ASSIST_API_VERSION"); env != "" {
		return env
	}
	return defaultCodeAssistVersion
}

func resolvePromptID(fn func() string) func() string {
	if fn != nil {
		return fn
	}
	return func() string {
		buf := make([]byte, 8)
		if _, err := rand.Read(buf); err != nil {
			return fmt.Sprintf("prompt-%d", time.Now().UnixNano())
		}
		return hex.EncodeToString(buf)
	}
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
	}
	return out
}

type caGenerateContentRequest struct {
	Model        string                         `json:"model"`
	Project      string                         `json:"project,omitempty"`
	UserPromptID string                         `json:"user_prompt_id,omitempty"`
	Request      caVertexGenerateContentRequest `json:"request"`
}

type caVertexGenerateContentRequest struct {
	Contents          []caContent `json:"contents"`
	SystemInstruction *caContent  `json:"systemInstruction,omitempty"`
	SessionID         string      `json:"session_id,omitempty"`
	Tools             []caTool    `json:"tools,omitempty"`
}

type caCountTokenRequest struct {
	Request caVertexCountTokenRequest `json:"request"`
}

type caVertexCountTokenRequest struct {
	Model    string      `json:"model"`
	Contents []caContent `json:"contents"`
}

type caCountTokenResponse struct {
	TotalTokens int `json:"totalTokens"`
}

type caGenerateContentResponse struct {
	Response caVertexGenerateContentResponse `json:"response"`
	TraceID  string                          `json:"traceId,omitempty"`
}

type caVertexGenerateContentResponse struct {
	Candidates    []caCandidate    `json:"candidates"`
	UsageMetadata *caUsageMetadata `json:"usageMetadata,omitempty"`
}

type caCandidate struct {
	Content caContent `json:"content"`
}

type caContent struct {
	Role  string   `json:"role,omitempty"`
	Parts []caPart `json:"parts"`
}

type caPart struct {
	Text             string              `json:"text,omitempty"`
	FunctionCall     *caFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *caFunctionResponse `json:"functionResponse,omitempty"`
}

type caTool struct {
	FunctionDeclarations []caFunctionDeclaration `json:"functionDeclarations"`
}

type caFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type caFunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type caFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response,omitempty"`
}

type caUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func toUsage(meta *caUsageMetadata) *llm.Usage {
	if meta == nil {
		return nil
	}
	return &llm.Usage{
		PromptTokens:     meta.PromptTokenCount,
		CompletionTokens: meta.CandidatesTokenCount,
		TotalTokens:      meta.TotalTokenCount,
	}
}

func extractText(resp caGenerateContentResponse) string {
	if len(resp.Response.Candidates) == 0 {
		return ""
	}
	first := resp.Response.Candidates[0]
	var b strings.Builder
	for _, part := range first.Content.Parts {
		if part.Text != "" {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

func extractToolCalls(resp caGenerateContentResponse) []llm.FunctionCall {
	if len(resp.Response.Candidates) == 0 {
		return nil
	}
	first := resp.Response.Candidates[0]
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

func toCodeAssistContents(messages []llm.Message) ([]caContent, *caContent) {
	var contents []caContent
	var systemParts []caPart
	for _, msg := range messages {
		parts := toCodeAssistParts(msg.Parts)
		if len(parts) == 0 {
			continue
		}
		switch msg.Role {
		case llm.RoleSystem:
			systemParts = append(systemParts, parts...)
		case llm.RoleAssistant:
			contents = append(contents, caContent{Role: "model", Parts: parts})
		default:
			contents = append(contents, caContent{Role: "user", Parts: parts})
		}
	}
	var system *caContent
	if len(systemParts) > 0 {
		system = &caContent{Role: "system", Parts: systemParts}
	}
	return contents, system
}

func toCountTokenContents(messages []llm.Message) []caContent {
	contents := make([]caContent, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == llm.RoleSystem {
			continue
		}
		parts := toCodeAssistParts(msg.Parts)
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
		contents = append(contents, caContent{Role: role, Parts: parts})
	}
	return contents
}

func toCodeAssistParts(parts []llm.Part) []caPart {
	out := make([]caPart, 0, len(parts))
	for _, part := range parts {
		if part.Text != "" {
			out = append(out, caPart{Text: part.Text})
		}
		if part.FunctionCall != nil {
			out = append(out, caPart{FunctionCall: toCodeAssistFunctionCall(part.FunctionCall)})
		}
		if part.FunctionResponse != nil {
			out = append(out, caPart{FunctionResponse: toCodeAssistFunctionResponse(part.FunctionResponse)})
		}
	}
	return out
}

func toCodeAssistFunctionCall(call *llm.FunctionCall) *caFunctionCall {
	if call == nil {
		return nil
	}
	return &caFunctionCall{
		ID:   call.ID,
		Name: call.Name,
		Args: call.Args,
	}
}

func toCodeAssistFunctionResponse(resp *llm.FunctionResponse) *caFunctionResponse {
	if resp == nil {
		return nil
	}
	return &caFunctionResponse{
		ID:       resp.ID,
		Name:     resp.Name,
		Response: resp.Response,
	}
}

func toCodeAssistTools(tools []llm.Tool) []caTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]caTool, 0, len(tools))
	for _, tool := range tools {
		if len(tool.FunctionDeclarations) == 0 {
			continue
		}
		decls := make([]caFunctionDeclaration, 0, len(tool.FunctionDeclarations))
		for _, decl := range tool.FunctionDeclarations {
			decls = append(decls, caFunctionDeclaration{
				Name:        decl.Name,
				Description: decl.Description,
				Parameters:  decl.Parameters,
			})
		}
		out = append(out, caTool{FunctionDeclarations: decls})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
