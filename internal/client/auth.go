// Package client defines the model client interface.
package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/auth"
	"golang.org/x/oauth2"
)

const refreshSkew = time.Minute

// TokenManager loads, refreshes, and caches OAuth credentials.
type TokenManager struct {
	Provider auth.Provider
	Store    auth.Store
	Now      func() time.Time

	mu    sync.Mutex
	creds *auth.Credentials
}

// NewTokenManager constructs a manager using the provided auth provider and store.
func NewTokenManager(provider auth.Provider, store auth.Store) *TokenManager {
	return &TokenManager{
		Provider: provider,
		Store:    store,
		Now:      time.Now,
	}
}

// Token returns a valid access token, refreshing and persisting as needed.
func (m *TokenManager) Token(ctx context.Context) (string, error) {
	if m == nil {
		return "", errors.New("token manager is nil")
	}
	if m.Provider == nil || m.Store == nil {
		return "", errors.New("token manager requires provider and store")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Now == nil {
		m.Now = time.Now
	}

	if m.creds == nil {
		creds, err := m.Store.Load(ctx)
		if err != nil {
			return "", err
		}
		m.creds = creds
	}
	if m.creds == nil {
		return "", errors.New("credentials not available")
	}

	if m.creds.AccessToken != "" && !m.needsRefresh(m.creds) {
		return m.creds.AccessToken, nil
	}

	if m.creds.RefreshToken == "" {
		return "", errors.New("refresh token is missing")
	}

	refreshed, err := refreshWithRetry(ctx, m.Provider, m.creds)
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}
	if refreshed == nil || refreshed.AccessToken == "" {
		return "", errors.New("refresh returned empty access token")
	}
	m.creds = refreshed
	if err := m.Store.Save(ctx, refreshed); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

func refreshWithRetry(ctx context.Context, provider auth.Provider, creds *auth.Credentials) (*auth.Credentials, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		refreshed, err := provider.Refresh(ctx, creds)
		if err == nil {
			return refreshed, nil
		}
		lastErr = err
		if !isRetryableAuthError(err) || attempt == maxRetryAttempts-1 {
			return nil, err
		}
		if err := sleepWithContext(ctx, retryDelay(attempt)); err != nil {
			return nil, err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("token refresh retries exhausted")
}

func isRetryableAuthError(err error) bool {
	if isRetryableError(err) {
		return true
	}
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) && retrieveErr.Response != nil {
		return isRetryableStatus(retrieveErr.Response.StatusCode)
	}
	return false
}

func (m *TokenManager) needsRefresh(creds *auth.Credentials) bool {
	if creds == nil {
		return true
	}
	if creds.AccessToken == "" {
		return true
	}
	if creds.Expiry.IsZero() {
		return false
	}
	return m.Now().Add(refreshSkew).After(creds.Expiry)
}

// AuthRoundTripper attaches Authorization headers using a TokenManager.
type AuthRoundTripper struct {
	Base   http.RoundTripper
	Tokens *TokenManager
}

// RoundTrip implements http.RoundTripper.
func (r *AuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	base := r.Base
	if base == nil {
		base = http.DefaultTransport
	}

	if req.Header.Get("Authorization") == "" {
		if r.Tokens == nil {
			return nil, errors.New("token manager is nil")
		}
		token, err := r.Tokens.Token(req.Context())
		if err != nil {
			return nil, err
		}
		if token != "" {
			cloned := req.Clone(req.Context())
			cloned.Header.Set("Authorization", "Bearer "+token)
			return base.RoundTrip(cloned)
		}
	}

	return base.RoundTrip(req)
}

// NewAuthenticatedClient wraps a base client with OAuth authentication.
func NewAuthenticatedClient(provider auth.Provider, store auth.Store, base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	manager := NewTokenManager(provider, store)
	baseTransport := base.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	clone := *base
	clone.Transport = &AuthRoundTripper{
		Base:   baseTransport,
		Tokens: manager,
	}
	return &clone
}
