package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/auth"
)

type fakeStore struct {
	loadErr error
	saveErr error
	creds   *auth.Credentials
	saved   *auth.Credentials
}

func (s *fakeStore) Load(ctx context.Context) (*auth.Credentials, error) {
	_ = ctx
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.creds, nil
}

func (s *fakeStore) Save(ctx context.Context, creds *auth.Credentials) error {
	_ = ctx
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = creds
	return nil
}

func (s *fakeStore) Clear(ctx context.Context) error {
	_ = ctx
	s.creds = nil
	return nil
}

type fakeProvider struct {
	refreshFn func(context.Context, *auth.Credentials) (*auth.Credentials, error)
	calls     int
}

func (p *fakeProvider) Name() string { return "fake" }

func (p *fakeProvider) Login(ctx context.Context) (*auth.Credentials, error) {
	_ = ctx
	return nil, nil
}

func (p *fakeProvider) Refresh(ctx context.Context, creds *auth.Credentials) (*auth.Credentials, error) {
	p.calls++
	if p.refreshFn != nil {
		return p.refreshFn(ctx, creds)
	}
	return creds, nil
}

type captureTransport struct {
	req *http.Request
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.req = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestAuthRoundTripperUsesCachedToken(t *testing.T) {
	store := &fakeStore{
		creds: &auth.Credentials{
			AccessToken: "token-123",
			Expiry:      time.Now().Add(10 * time.Minute),
		},
	}
	provider := &fakeProvider{}
	manager := NewTokenManager(provider, store)
	rt := &AuthRoundTripper{
		Base:   &captureTransport{},
		Tokens: manager,
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	capture, ok := rt.Base.(*captureTransport)
	if !ok {
		t.Fatal("expected capture transport")
	}
	if got := capture.req.Header.Get("Authorization"); got != "Bearer token-123" {
		t.Fatalf("Authorization header mismatch: %q", got)
	}
	if provider.calls != 0 {
		t.Fatalf("expected refresh to be unused, got %d", provider.calls)
	}
}

func TestAuthRoundTripperRefreshesExpiredToken(t *testing.T) {
	store := &fakeStore{
		creds: &auth.Credentials{
			AccessToken:  "expired",
			RefreshToken: "refresh",
			Expiry:       time.Now().Add(-time.Minute),
		},
	}
	provider := &fakeProvider{
		refreshFn: func(ctx context.Context, creds *auth.Credentials) (*auth.Credentials, error) {
			_ = ctx
			return &auth.Credentials{
				AccessToken:  "new-token",
				RefreshToken: creds.RefreshToken,
				Expiry:       time.Now().Add(10 * time.Minute),
			}, nil
		},
	}
	manager := NewTokenManager(provider, store)
	rt := &AuthRoundTripper{
		Base:   &captureTransport{},
		Tokens: manager,
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	capture, ok := rt.Base.(*captureTransport)
	if !ok {
		t.Fatal("expected capture transport")
	}
	if got := capture.req.Header.Get("Authorization"); got != "Bearer new-token" {
		t.Fatalf("Authorization header mismatch: %q", got)
	}
	if provider.calls != 1 {
		t.Fatalf("expected refresh to be called once, got %d", provider.calls)
	}
	if store.saved == nil || store.saved.AccessToken != "new-token" {
		t.Fatalf("expected refreshed credentials to be saved")
	}
}

func TestAuthRoundTripperRespectsExistingHeader(t *testing.T) {
	store := &fakeStore{
		creds: &auth.Credentials{AccessToken: "token-ignored"},
	}
	provider := &fakeProvider{}
	manager := NewTokenManager(provider, store)
	rt := &AuthRoundTripper{
		Base:   &captureTransport{},
		Tokens: manager,
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer preset")
	_, err = rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	capture, ok := rt.Base.(*captureTransport)
	if !ok {
		t.Fatal("expected capture transport")
	}
	if got := capture.req.Header.Get("Authorization"); got != "Bearer preset" {
		t.Fatalf("Authorization header mismatch: %q", got)
	}
	if provider.calls != 0 {
		t.Fatalf("expected refresh to be unused, got %d", provider.calls)
	}
}
