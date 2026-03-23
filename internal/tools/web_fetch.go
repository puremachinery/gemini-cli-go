package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"golang.org/x/net/html"
)

const (
	webFetchTimeout         = 10 * time.Second
	webFetchMaxContentBytes = 100000
	webFetchMaxTotalBytes   = 200000
)

// WebFetchToolName is the tool identifier for web_fetch.
const WebFetchToolName = "web_fetch"

// WebFetchTool implements the web_fetch tool.
type WebFetchTool struct {
	ctx Context
}

// WebFetchParams holds arguments for web_fetch.
type WebFetchParams struct {
	Prompt string `json:"prompt"`
}

type webFetchInvocation struct {
	params WebFetchParams
	ctx    Context
}

var webFetchHTTPGet = fetchWithTimeout
var webFetchHTMLToText = htmlToText

// NewWebFetchTool constructs a WebFetchTool.
func NewWebFetchTool(ctx Context) *WebFetchTool {
	return &WebFetchTool{ctx: ctx}
}

// Name returns the tool name.
func (t *WebFetchTool) Name() string { return WebFetchToolName }

// Description returns the tool description.
func (t *WebFetchTool) Description() string {
	if t.ctx.AllowPrivateWebFetch {
		return "Processes content from URL(s), including local and private network addresses (e.g., localhost), embedded in a prompt."
	}
	return "Processes content from URL(s) embedded in a prompt (private network URLs disabled by configuration)."
}

// Parameters returns the JSON schema for web_fetch.
func (t *WebFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "A prompt that includes URL(s) (up to 20) to fetch and instructions on how to process them.",
			},
		},
		"required": []string{"prompt"},
	}
}

// Build validates args and returns an invocation.
func (t *WebFetchTool) Build(args map[string]any) (Invocation, error) {
	prompt, err := getStringArg(args, "prompt")
	if err != nil {
		return nil, err
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, errors.New("prompt cannot be empty")
	}
	valid, parseErrs := parsePrompt(prompt)
	if len(parseErrs) > 0 {
		return nil, fmt.Errorf("error(s) in prompt URLs:\n- %s", strings.Join(parseErrs, "\n- "))
	}
	if len(valid) == 0 {
		return nil, errors.New("prompt must contain at least one valid URL starting with http:// or https://")
	}
	return &webFetchInvocation{
		params: WebFetchParams{Prompt: prompt},
		ctx:    t.ctx,
	}, nil
}

func (i *webFetchInvocation) Name() string { return WebFetchToolName }

func (i *webFetchInvocation) Description() string {
	display := i.params.Prompt
	if len([]rune(display)) > 100 {
		display = truncateRunes(display, 100)
	}
	return fmt.Sprintf("Processing URLs and instructions from prompt: %q", display)
}

func (i *webFetchInvocation) ConfirmationRequest() *ConfirmationRequest {
	display := i.params.Prompt
	if len([]rune(display)) > 160 {
		display = truncateRunes(display, 160)
	}
	return &ConfirmationRequest{
		ToolName: WebFetchToolName,
		Title:    "Confirm Web Fetch",
		Prompt:   fmt.Sprintf("Allow web fetch for: %q?", display),
	}
}

func (i *webFetchInvocation) Execute(ctx context.Context) (Result, error) {
	if i.ctx.GeminiClient == nil {
		return Result{Error: "model client is not configured"}, nil
	}
	if strings.TrimSpace(i.params.Prompt) == "" {
		return Result{Error: "prompt is required"}, nil
	}
	urls, _ := parsePrompt(i.params.Prompt)
	if len(urls) == 0 {
		return Result{Error: "prompt must contain at least one valid URL"}, nil
	}
	private := false
	for _, u := range urls {
		if isPrivateIP(u) {
			private = true
			break
		}
	}
	if private && !i.ctx.AllowPrivateWebFetch {
		return Result{Error: "private network URLs are disabled for web_fetch"}, nil
	}
	if private {
		return i.executeFallback(ctx, urls)
	}

	req := llm.ChatRequest{
		Model: "web-fetch",
		Messages: []llm.Message{{
			Role: llm.RoleUser,
			Parts: []llm.Part{{
				Text: i.params.Prompt,
			}},
		}},
	}
	resp, err := i.ctx.GeminiClient.Chat(ctx, req)
	if err != nil {
		message := fmt.Sprintf("Error processing web content for prompt %q: %v", truncatePrompt(i.params.Prompt), err)
		return Result{
			Output:  "Error: " + message,
			Error:   message,
			Display: "Error processing web content.",
		}, nil
	}

	responseText := resp.Text
	grounding := resp.Grounding
	if shouldFallbackWebFetch(responseText, grounding, resp.URLContext) {
		return i.executeFallback(ctx, urls)
	}

	if grounding != nil && len(grounding.GroundingChunks) > 0 {
		if len(grounding.GroundingSupports) > 0 {
			responseText = insertCitationsByRune(responseText, grounding.GroundingSupports)
		}
		sourceList := make([]string, 0, len(grounding.GroundingChunks))
		for idx, source := range grounding.GroundingChunks {
			title := "Untitled"
			uri := "Unknown URI"
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
		if len(sourceList) > 0 {
			responseText += "\n\nSources:\n" + strings.Join(sourceList, "\n")
		}
	}

	return Result{
		Output:  responseText,
		Display: "Content processed from prompt.",
	}, nil
}

func shouldFallbackWebFetch(text string, grounding *llm.GroundingMetadata, urlContext *llm.URLContextMetadata) bool {
	if urlContext != nil && len(urlContext.URLMetadata) > 0 {
		allFailed := true
		for _, entry := range urlContext.URLMetadata {
			if entry.URLRetrievalStatus == "URL_RETRIEVAL_STATUS_SUCCESS" {
				allFailed = false
				break
			}
		}
		if allFailed {
			return true
		}
	}
	if strings.TrimSpace(text) == "" && (grounding == nil || len(grounding.GroundingChunks) == 0) {
		return true
	}
	return false
}

func (i *webFetchInvocation) executeFallback(ctx context.Context, rawURLs []string) (Result, error) {
	if len(rawURLs) == 0 {
		return Result{Error: "prompt must contain at least one valid URL"}, nil
	}
	content, err := fetchFallbackContent(ctx, rawURLs)
	if err != nil {
		message := fmt.Sprintf("Error during fallback fetch: %v", err)
		return Result{
			Output:  "Error: " + message,
			Error:   message,
			Display: "Error: " + message,
		}, nil
	}

	fallbackPrompt := fmt.Sprintf(`The user requested the following: "%s".

I was unable to access the URL(s) directly. Instead, I have fetched the raw content of the page(s). Please use the following content to answer the request. Do not attempt to access the URL again.

---
%s
---
`, i.params.Prompt, content)

	req := llm.ChatRequest{
		Model: "web-fetch-fallback",
		Messages: []llm.Message{{
			Role: llm.RoleUser,
			Parts: []llm.Part{{
				Text: fallbackPrompt,
			}},
		}},
	}
	result, err := i.ctx.GeminiClient.Chat(ctx, req)
	if err != nil {
		message := fmt.Sprintf("Error during fallback fetch: %v", err)
		return Result{
			Output:  "Error: " + message,
			Error:   message,
			Display: "Error: " + message,
		}, nil
	}
	display := "Content processed using fallback fetch."
	return Result{
		Output:  result.Text,
		Display: display,
	}, nil
}

func parsePrompt(text string) ([]string, []string) {
	tokens := strings.Fields(text)
	valid := make([]string, 0, len(tokens))
	parseErrs := []string{}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if !strings.Contains(token, "://") {
			continue
		}
		parsed, err := url.Parse(token)
		if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
			parseErrs = append(parseErrs, fmt.Sprintf("Malformed URL detected: %q.", token))
			continue
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			parseErrs = append(parseErrs, fmt.Sprintf("Unsupported protocol in URL: %q. Only http and https are supported.", token))
			continue
		}
		valid = append(valid, parsed.String())
		if len(valid) >= 20 {
			break
		}
	}
	return valid, parseErrs
}

func isPrivateIP(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return false
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return false
	}
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") || lowerHost == "localhost.localdomain" {
		return true
	}
	if idx := strings.IndexByte(host, '%'); idx != -1 {
		host = host[:idx]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	return false
}

func normalizeGitHubRawURL(rawURL string) string {
	if strings.Contains(rawURL, "github.com") && strings.Contains(rawURL, "/blob/") {
		return strings.Replace(strings.Replace(rawURL, "github.com", "raw.githubusercontent.com", 1), "/blob/", "/", 1)
	}
	return rawURL
}

func readLimited(r io.Reader, maxBytes int) (string, error) {
	limited := io.LimitReader(r, int64(maxBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if len(data) > maxBytes {
		data = data[:maxBytes]
	}
	return string(data), nil
}

func fetchFallbackContent(ctx context.Context, rawURLs []string) (string, error) {
	var b strings.Builder
	total := 0
	truncated := false
	for i, raw := range rawURLs {
		targetURL := normalizeGitHubRawURL(raw)
		content, err := fetchURLContent(ctx, targetURL)
		if err != nil {
			return "", fmt.Errorf("%s: %w", targetURL, err)
		}
		if total+len(content) > webFetchMaxTotalBytes {
			if total >= webFetchMaxTotalBytes {
				truncated = true
				break
			}
			remain := webFetchMaxTotalBytes - total
			if remain < len(content) {
				content = content[:remain]
				truncated = true
			}
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "--- %s ---\n", targetURL)
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		total += len(content)
		if truncated {
			break
		}
	}
	if truncated {
		b.WriteString("[content truncated]\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func fetchURLContent(ctx context.Context, targetURL string) (string, error) {
	resp, err := webFetchHTTPGet(ctx, targetURL, webFetchTimeout)
	if err != nil {
		return "", err
	}
	if resp.Body == nil {
		return "", errors.New("empty response body")
	}
	rawContent, readErr := readLimited(resp.Body, webFetchMaxContentBytes)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return "", readErr
	}
	statusErr := error(nil)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		statusErr = fmt.Errorf("request failed with status %s", resp.Status)
	}
	if statusErr != nil {
		if closeErr != nil {
			return "", fmt.Errorf("%w (also failed to close response body: %v)", statusErr, closeErr)
		}
		return "", statusErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	content := rawContent
	contentType := strings.ToLower(resp.Header.Get("content-type"))
	if strings.Contains(contentType, "text/html") || contentType == "" {
		content = webFetchHTMLToText(rawContent)
	}
	if len(content) > webFetchMaxContentBytes {
		content = content[:webFetchMaxContentBytes]
	}
	return content, nil
}

func fetchWithTimeout(ctx context.Context, rawURL string, timeout time.Duration) (*http.Response, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func truncatePrompt(prompt string) string {
	return truncateRunes(prompt, 50)
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func htmlToText(raw string) string {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return raw
	}
	var b strings.Builder
	lastNewline := false
	writeNewline := func() {
		if b.Len() == 0 || lastNewline {
			return
		}
		b.WriteString("\n")
		lastNewline = true
	}
	writeText := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if b.Len() > 0 && !lastNewline {
			b.WriteString(" ")
		}
		b.WriteString(text)
		lastNewline = false
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			switch tag {
			case "script", "style", "noscript":
				return
			case "br", "p", "div", "li", "tr", "section", "article", "header", "footer", "h1", "h2", "h3", "h4", "h5", "h6":
				writeNewline()
			}
		}
		if n.Type == html.TextNode {
			writeText(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			switch tag {
			case "p", "div", "li", "tr", "section", "article", "header", "footer", "h1", "h2", "h3", "h4", "h5", "h6":
				writeNewline()
			}
		}
	}
	walk(doc)
	return strings.TrimSpace(b.String())
}
