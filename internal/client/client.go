// Package client defines the model client interface.
package client

import (
	"context"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

// Stream yields ChatChunk values until Done or error.
type Stream interface {
	Recv(ctx context.Context) (llm.ChatChunk, error)
	Close() error
}

// Client is the minimal Gemini/Vertex client interface for streaming chat.
type Client interface {
	ChatStream(ctx context.Context, req llm.ChatRequest) (Stream, error)
	Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatChunk, error)
	CountTokens(ctx context.Context, req llm.CountTokensRequest) (llm.CountTokensResponse, error)
}
