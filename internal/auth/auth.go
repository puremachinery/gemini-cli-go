// Package auth defines authentication interfaces and credentials.
package auth

import (
	"context"
	"time"
)

// Credentials holds OAuth tokens for a Google login session.
type Credentials struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scope        string
	Expiry       time.Time
	IDToken      string
	AccountEmail string
}

// Provider can initiate or refresh authentication.
type Provider interface {
	Name() string
	Login(ctx context.Context) (*Credentials, error)
	Refresh(ctx context.Context, creds *Credentials) (*Credentials, error)
}

// Store persists credentials for reuse.
type Store interface {
	Load(ctx context.Context) (*Credentials, error)
	Save(ctx context.Context, creds *Credentials) error
	Clear(ctx context.Context) error
}
