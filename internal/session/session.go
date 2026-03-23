// Package session manages chat session state and persistence.
package session

import (
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

const sessionSchemaVersion = 1

// Session tracks a single chat conversation.
type Session struct {
	SchemaVersion int           `json:"schemaVersion,omitempty"`
	ID            string        `json:"id"`
	StartedAt     time.Time     `json:"startedAt"`
	UpdatedAt     time.Time     `json:"updatedAt,omitempty"`
	AuthType      string        `json:"authType,omitempty"`
	Messages      []llm.Message `json:"messages"`
}

// Preview provides lightweight session metadata for resume listings.
type Preview struct {
	Session      Session
	MessageCount int
	PreviewText  string
}

// Store persists sessions for resume.
type Store interface {
	Load(id string) (*Session, error)
	Save(s *Session) error
	List() ([]Session, error)
	Delete(id string) error
}
