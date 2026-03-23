// Package llm defines shared model request/response types.
package llm

// Role identifies the speaker in a chat turn.
type Role string

const (
	// RoleUser represents a user message.
	RoleUser Role = "user"
	// RoleAssistant represents a model response.
	RoleAssistant Role = "assistant"
	// RoleSystem represents a system instruction message.
	RoleSystem Role = "system"
)

// Part is a single message component.
type Part struct {
	Text             string            `json:"text,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
}

// Message is a role + ordered parts.
type Message struct {
	Role  Role   `json:"role"`
	Parts []Part `json:"parts"`
}

// ChatRequest is a model request for a streamed chat response.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

// Usage captures token usage when available.
type Usage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

// GroundingMetadata captures grounding sources and support indices.
type GroundingMetadata struct {
	GroundingChunks   []GroundingChunk   `json:"groundingChunks,omitempty"`
	GroundingSupports []GroundingSupport `json:"groundingSupports,omitempty"`
}

// URLContextMetadata captures URL fetch status metadata when available.
type URLContextMetadata struct {
	URLMetadata []URLMetadata `json:"urlMetadata,omitempty"`
}

// URLMetadata holds retrieval status for a URL.
type URLMetadata struct {
	URLRetrievalStatus string `json:"urlRetrievalStatus,omitempty"`
}

// GroundingChunk describes a single grounded source.
type GroundingChunk struct {
	Web *GroundingWeb `json:"web,omitempty"`
}

// GroundingWeb describes a web source for grounding.
type GroundingWeb struct {
	URI   string `json:"uri,omitempty"`
	Title string `json:"title,omitempty"`
}

// GroundingSupport maps a text segment to grounding chunks.
type GroundingSupport struct {
	Segment               *GroundingSegment `json:"segment,omitempty"`
	GroundingChunkIndices []int             `json:"groundingChunkIndices,omitempty"`
	ConfidenceScores      []float64         `json:"confidenceScores,omitempty"`
}

// GroundingSegment describes a segment of the response text.
type GroundingSegment struct {
	StartIndex int    `json:"startIndex"`
	EndIndex   int    `json:"endIndex"`
	Text       string `json:"text,omitempty"`
}

// ChatChunk is a streamed response fragment.
type ChatChunk struct {
	Text       string              `json:"text"`
	Done       bool                `json:"done"`
	Usage      *Usage              `json:"usage,omitempty"`
	Tools      []FunctionCall      `json:"toolCalls,omitempty"`
	Grounding  *GroundingMetadata  `json:"grounding,omitempty"`
	URLContext *URLContextMetadata `json:"urlContext,omitempty"`
}

// FunctionDeclaration describes a callable tool.
type FunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// Tool wraps function declarations for model requests.
type Tool struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations,omitempty"`
}

// FunctionCall represents a model tool call.
type FunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// FunctionResponse represents a tool result.
type FunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response,omitempty"`
}

// CountTokensRequest is a model request for token counting.
type CountTokensRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// CountTokensResponse captures token counts for a request.
type CountTokensResponse struct {
	TotalTokens int `json:"totalTokens"`
}
