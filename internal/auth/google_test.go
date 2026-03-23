package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAuthWithWebUsesPKCEAndHost(t *testing.T) {
	t.Setenv("OAUTH_CALLBACK_HOST", "localhost")

	tokenReqCh := make(chan url.Values, 1)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		tokenReqCh <- r.PostForm
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"access_token":"token","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh"}`); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer tokenServer.Close()

	userInfoClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == userInfoURL {
				body := io.NopCloser(strings.NewReader(`{"email":"tester@example.com"}`))
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       body,
					Header:     make(http.Header),
				}, nil
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	provider := &GoogleProvider{
		ClientID:     "id",
		ClientSecret: "secret",
		Scopes:       []string{"scope"},
		HTTPClient:   userInfoClient,
		OpenBrowser:  func(string) error { return nil },
	}
	config := &oauth2.Config{
		ClientID:     provider.ClientID,
		ClientSecret: provider.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  tokenServer.URL + "/auth",
			TokenURL: tokenServer.URL + "/token",
		},
		Scopes: provider.Scopes,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, tokenServer.Client())
	login, err := provider.authWithWeb(ctx, config)
	if err != nil {
		t.Fatalf("authWithWeb: %v", err)
	}

	authURL, err := url.Parse(login.authURL)
	if err != nil {
		t.Fatalf("parse authURL: %v", err)
	}
	query := authURL.Query()
	if query.Get("code_challenge") == "" || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("expected PKCE params in authURL, got %q", login.authURL)
	}
	redirectURI := query.Get("redirect_uri")
	if !strings.Contains(redirectURI, "localhost") {
		t.Fatalf("expected redirect_uri to use localhost, got %q", redirectURI)
	}
	state := query.Get("state")
	if state == "" {
		t.Fatal("expected state in authURL")
	}

	callbackURL := redirectURI + "?state=" + url.QueryEscape(state) + "&code=test-code"
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	creds, err := login.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if creds == nil || creds.AccessToken != "token" {
		t.Fatalf("expected credentials, got %#v", creds)
	}

	select {
	case form := <-tokenReqCh:
		if form.Get("code_verifier") == "" {
			t.Fatal("expected code_verifier in token request")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive token request")
	}
}

func TestAuthWithWebIPv6RedirectHost(t *testing.T) {
	t.Setenv("OAUTH_CALLBACK_HOST", "::1")

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"access_token":"token","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh"}`); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer tokenServer.Close()

	provider := &GoogleProvider{
		ClientID:     "id",
		ClientSecret: "secret",
		Scopes:       []string{"scope"},
		HTTPClient:   tokenServer.Client(),
		OpenBrowser:  func(string) error { return nil },
	}
	config := &oauth2.Config{
		ClientID:     provider.ClientID,
		ClientSecret: provider.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  tokenServer.URL + "/auth",
			TokenURL: tokenServer.URL + "/token",
		},
		Scopes: provider.Scopes,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, tokenServer.Client())
	login, err := provider.authWithWeb(ctx, config)
	if err != nil {
		t.Fatalf("authWithWeb: %v", err)
	}

	authURL, err := url.Parse(login.authURL)
	if err != nil {
		t.Fatalf("parse authURL: %v", err)
	}
	redirectURI := authURL.Query().Get("redirect_uri")
	if !strings.Contains(redirectURI, "[::1]") {
		t.Fatalf("expected IPv6 host to be bracketed, got %q", redirectURI)
	}
}

func TestNewGoogleProviderUsesEnvOverrides(t *testing.T) {
	t.Setenv("GEMINI_OAUTH_CLIENT_ID", "override-id")
	t.Setenv("GEMINI_OAUTH_CLIENT_SECRET", "override-secret")

	provider := NewGoogleProvider()
	if provider.ClientID != "override-id" {
		t.Fatalf("expected override client ID, got %q", provider.ClientID)
	}
	if provider.ClientSecret != "override-secret" {
		t.Fatalf("expected override client secret, got %q", provider.ClientSecret)
	}
}

func TestParseAuthCodeInput(t *testing.T) {
	testCases := []struct {
		name          string
		input         string
		expectedState string
		wantCode      string
		wantErr       string
	}{
		{
			name:     "raw code with spaces",
			input:    "  abc123  ",
			wantCode: "abc123",
		},
		{
			name:          "valid callback URL",
			input:         "http://127.0.0.1:8085/oauth2callback?state=" + url.QueryEscape("expected-state") + "&code=test-code",
			expectedState: "expected-state",
			wantCode:      "test-code",
		},
		{
			name:          "state mismatch",
			input:         "http://127.0.0.1:8085/oauth2callback?state=wrong&code=test-code",
			expectedState: "expected-state",
			wantErr:       "oauth state mismatch",
		},
		{
			name:          "oauth error with description",
			input:         "http://127.0.0.1:8085/oauth2callback?error=access_denied&error_description=user+denied",
			expectedState: "expected-state",
			wantErr:       "oauth error: access_denied (user denied)",
		},
		{
			name:          "oauth error without description",
			input:         "http://127.0.0.1:8085/oauth2callback?error=access_denied",
			expectedState: "expected-state",
			wantErr:       "oauth error: access_denied",
		},
		{
			name:          "missing code",
			input:         "http://127.0.0.1:8085/oauth2callback?state=expected-state",
			expectedState: "expected-state",
			wantErr:       "callback URL is missing code",
		},
		{
			name:          "missing state",
			input:         "http://127.0.0.1:8085/oauth2callback?code=test-code",
			expectedState: "expected-state",
			wantErr:       "callback URL is missing state",
		},
		{
			name:    "empty input",
			input:   "   ",
			wantErr: "authorization code is required",
		},
		{
			name:    "malformed URL",
			input:   "http://[::1",
			wantErr: "parse callback URL",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotCode, err := parseAuthCodeInput(tc.input, tc.expectedState)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotCode != tc.wantCode {
				t.Fatalf("expected code %q, got %q", tc.wantCode, gotCode)
			}
		})
	}
}
