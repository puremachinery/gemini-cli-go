package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// Current default OAuth credentials mirror the upstream gemini-cli installed-app flow.
	// This compatibility setup is temporary and not a recommended long-term downstream default.
	// Public/downstream forks should register and migrate to their own OAuth client when possible.
	oauthClientID        = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	oauthClientSecret    = "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"
	oauthClientIDEnv     = "GEMINI_OAUTH_CLIENT_ID"
	oauthClientSecretEnv = "GEMINI_OAUTH_CLIENT_SECRET"

	signInSuccessURL  = "https://developers.google.com/gemini-code-assist/auth_success_gemini"
	signInFailureURL  = "https://developers.google.com/gemini-code-assist/auth_failure_gemini"
	manualRedirectURI = "http://127.0.0.1:8085/oauth2callback"

	userInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
)

var oauthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}

// GoogleProvider implements OAuth login for Google accounts.
type GoogleProvider struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
	HTTPClient   *http.Client
	OpenBrowser  func(string) error
}

// NewGoogleProvider returns a provider configured for Gemini CLI OAuth.
func NewGoogleProvider() *GoogleProvider {
	return &GoogleProvider{
		ClientID:     defaultOAuthClientID(),
		ClientSecret: defaultOAuthClientSecret(),
		Scopes:       append([]string(nil), oauthScopes...),
		HTTPClient:   http.DefaultClient,
		OpenBrowser:  openBrowser,
	}
}

// Name identifies the auth provider.
func (p *GoogleProvider) Name() string {
	return "oauth-personal"
}

// Login starts an interactive OAuth login flow.
func (p *GoogleProvider) Login(ctx context.Context) (*Credentials, error) {
	if p == nil {
		return nil, errors.New("provider is nil")
	}
	p.applyDefaults()
	config := p.oauthConfig()

	if isNoBrowser() {
		return p.loginWithUserCode(ctx, config)
	}

	consent, err := getConsentForOauth()
	if err != nil {
		return nil, err
	}
	if !consent {
		return nil, FatalCancellationError{Message: "Authentication cancelled by user."}
	}

	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	waitCtx, cancel := context.WithTimeout(signalCtx, 5*time.Minute)
	defer cancel()

	webLogin, err := p.authWithWeb(waitCtx, config)
	if err != nil {
		return nil, err
	}

	if _, err := fmt.Fprintf(os.Stdout,
		"\n\nCode Assist login required.\nAttempting to open authentication page in your browser.\nOtherwise navigate to:\n\n%s\n\n\n",
		webLogin.authURL,
	); err != nil {
		return nil, err
	}

	if err := p.OpenBrowser(webLogin.authURL); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr,
			"Failed to open browser with error: %v\nPlease try running again with NO_BROWSER=true set.\n",
			err,
		); writeErr != nil {
			return nil, writeErr
		}
		return nil, FatalAuthenticationError{
			Message: fmt.Sprintf("Failed to open browser: %v", err),
		}
	}

	if _, err := fmt.Fprint(os.Stdout, "Waiting for authentication...\n"); err != nil {
		return nil, err
	}

	creds, err := webLogin.Wait(waitCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, FatalAuthenticationError{
				Message: "Authentication timed out after 5 minutes. The browser tab may have gotten stuck in a loading state. Please try again or use NO_BROWSER=true for manual authentication.",
			}
		}
		if errors.Is(err, context.Canceled) {
			return nil, FatalCancellationError{Message: "Authentication cancelled by user."}
		}
		return nil, err
	}

	if _, err := fmt.Fprint(os.Stdout, "Authentication succeeded\n"); err != nil {
		return nil, err
	}
	return creds, nil
}

// Refresh uses the refresh token to obtain a fresh access token.
func (p *GoogleProvider) Refresh(ctx context.Context, creds *Credentials) (*Credentials, error) {
	if p == nil {
		return nil, errors.New("provider is nil")
	}
	if creds == nil {
		return nil, errors.New("credentials are nil")
	}
	p.applyDefaults()
	if creds.RefreshToken == "" {
		if creds.AccessToken != "" && !creds.Expiry.IsZero() && time.Now().Before(creds.Expiry) {
			return creds, nil
		}
		return nil, errors.New("refresh token is missing")
	}
	token := &oauth2.Token{
		AccessToken:  creds.AccessToken,
		TokenType:    creds.TokenType,
		RefreshToken: creds.RefreshToken,
		Expiry:       creds.Expiry,
	}
	refreshed, err := p.oauthConfig().TokenSource(ctx, token).Token()
	if err != nil {
		return nil, err
	}
	updated, err := p.credentialsFromToken(ctx, refreshed, p.Scopes)
	if err != nil {
		return nil, err
	}
	if updated.RefreshToken == "" {
		updated.RefreshToken = creds.RefreshToken
	}
	return updated, nil
}

type webLogin struct {
	authURL  string
	resultCh <-chan authResult
}

func (w webLogin) Wait(ctx context.Context) (*Credentials, error) {
	select {
	case res := <-w.resultCh:
		return res.creds, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type authResult struct {
	creds *Credentials
	err   error
}

func (p *GoogleProvider) applyDefaults() {
	if p.ClientID == "" {
		p.ClientID = defaultOAuthClientID()
	}
	if p.ClientSecret == "" {
		p.ClientSecret = defaultOAuthClientSecret()
	}
	if len(p.Scopes) == 0 {
		p.Scopes = append([]string(nil), oauthScopes...)
	}
	if p.HTTPClient == nil {
		p.HTTPClient = http.DefaultClient
	}
	if p.OpenBrowser == nil {
		p.OpenBrowser = openBrowser
	}
}

func defaultOAuthClientID() string {
	if value := strings.TrimSpace(os.Getenv(oauthClientIDEnv)); value != "" {
		return value
	}
	return oauthClientID
}

func defaultOAuthClientSecret() string {
	if value := strings.TrimSpace(os.Getenv(oauthClientSecretEnv)); value != "" {
		return value
	}
	return oauthClientSecret
}

func (p *GoogleProvider) oauthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       append([]string(nil), p.Scopes...),
	}
}

func (p *GoogleProvider) loginWithUserCode(ctx context.Context, config *oauth2.Config) (*Credentials, error) {
	const maxRetries = 2
	for attempt := 0; attempt < maxRetries; attempt++ {
		creds, ok, err := p.authWithUserCode(ctx, config)
		if err != nil {
			return nil, err
		}
		if ok {
			return creds, nil
		}
		if _, err := fmt.Fprint(os.Stderr, "\nFailed to authenticate with user code."); err != nil {
			return nil, err
		}
		if attempt < maxRetries-1 {
			if _, err := fmt.Fprint(os.Stderr, " Retrying...\n"); err != nil {
				return nil, err
			}
		} else {
			if _, err := fmt.Fprint(os.Stderr, "\n"); err != nil {
				return nil, err
			}
		}
	}
	return nil, FatalAuthenticationError{Message: "Failed to authenticate with user code."}
}

func (p *GoogleProvider) authWithUserCode(ctx context.Context, config *oauth2.Config) (*Credentials, bool, error) {
	redirectURI := manualRedirectURI
	verifier, challenge, err := generatePKCEVerifier()
	if err != nil {
		return nil, false, err
	}
	state, err := generateState()
	if err != nil {
		return nil, false, err
	}
	config.RedirectURL = redirectURI
	authURL := config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("code_challenge", challenge),
	)
	if _, err := fmt.Fprintf(os.Stdout,
		"Please visit the following URL to authorize the application:\n\n%s\n\n",
		authURL,
	); err != nil {
		return nil, false, err
	}

	codeInput, err := promptLine("Enter the full callback URL (preferred) or the authorization code: ")
	if err != nil {
		return nil, false, err
	}
	code, parseErr := parseAuthCodeInput(codeInput, state)
	if parseErr != nil {
		if _, err := fmt.Fprintf(os.Stderr, "Invalid authorization response: %v\n", parseErr); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}
	token, err := config.Exchange(
		ctx,
		code,
		oauth2.SetAuthURLParam("code_verifier", verifier),
		oauth2.SetAuthURLParam("redirect_uri", redirectURI),
	)
	if err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Failed to authenticate with authorization code: %v\n", err); writeErr != nil {
			return nil, false, writeErr
		}
		return nil, false, nil
	}
	creds, err := p.credentialsFromToken(ctx, token, config.Scopes)
	if err != nil {
		return nil, false, err
	}
	return creds, true, nil
}

func (p *GoogleProvider) authWithWeb(ctx context.Context, config *oauth2.Config) (webLogin, error) {
	host := os.Getenv("OAUTH_CALLBACK_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	host = normalizeCallbackHost(host)
	listener, port, err := callbackListener(host)
	if err != nil {
		return webLogin{}, err
	}

	redirectHost := host
	if redirectHost == "0.0.0.0" || redirectHost == "::" {
		redirectHost = "127.0.0.1"
	}
	redirectHost = normalizeCallbackHost(redirectHost)
	redirectURI := fmt.Sprintf("http://%s/oauth2callback", net.JoinHostPort(redirectHost, strconv.Itoa(port)))
	state, err := generateState()
	if err != nil {
		if closeErr := listener.Close(); closeErr != nil {
			return webLogin{}, fmt.Errorf("%w (cleanup failed: %v)", err, closeErr)
		}
		return webLogin{}, err
	}
	verifier, challenge, err := generatePKCEVerifier()
	if err != nil {
		if closeErr := listener.Close(); closeErr != nil {
			return webLogin{}, fmt.Errorf("%w (cleanup failed: %v)", err, closeErr)
		}
		return webLogin{}, err
	}

	config.RedirectURL = redirectURI
	authURL := config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("code_challenge", challenge),
	)

	resultCh := make(chan authResult, 1)
	server := &http.Server{}
	var once sync.Once
	sendResult := func(res authResult) {
		once.Do(func() {
			resultCh <- res
			close(resultCh)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return
			}
		})
	}

	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/oauth2callback") {
			http.Redirect(w, r, signInFailureURL, http.StatusMovedPermanently)
			sendResult(authResult{
				err: FatalAuthenticationError{
					Message: fmt.Sprintf("OAuth callback not received. Unexpected request: %s", r.URL.Path),
				},
			})
			return
		}

		query := r.URL.Query()
		if query.Get("error") != "" {
			http.Redirect(w, r, signInFailureURL, http.StatusMovedPermanently)
			errCode := query.Get("error")
			errDesc := query.Get("error_description")
			if errDesc == "" {
				errDesc = "No additional details provided"
			}
			sendResult(authResult{
				err: FatalAuthenticationError{
					Message: fmt.Sprintf("Google OAuth error: %s. %s", errCode, errDesc),
				},
			})
			return
		}

		if query.Get("state") != state {
			sendResult(authResult{
				err: FatalAuthenticationError{
					Message: "OAuth state mismatch. Possible CSRF attack or browser session issue.",
				},
			})
			if _, err := fmt.Fprint(w, "State mismatch. Possible CSRF attack"); err != nil {
				return
			}
			return
		}

		code := query.Get("code")
		if code == "" {
			sendResult(authResult{
				err: FatalAuthenticationError{
					Message: "No authorization code received from Google OAuth. Please try authenticating again.",
				},
			})
			return
		}

		token, err := config.Exchange(
			ctx,
			code,
			oauth2.SetAuthURLParam("redirect_uri", redirectURI),
			oauth2.SetAuthURLParam("code_verifier", verifier),
		)
		if err != nil {
			http.Redirect(w, r, signInFailureURL, http.StatusMovedPermanently)
			sendResult(authResult{
				err: FatalAuthenticationError{
					Message: fmt.Sprintf("Failed to exchange authorization code for tokens: %v", err),
				},
			})
			return
		}

		creds, err := p.credentialsFromToken(ctx, token, config.Scopes)
		if err != nil {
			http.Redirect(w, r, signInFailureURL, http.StatusMovedPermanently)
			sendResult(authResult{err: err})
			return
		}

		http.Redirect(w, r, signInSuccessURL, http.StatusMovedPermanently)
		sendResult(authResult{creds: creds})
	})

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return
		}
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return
		}
	}()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			sendResult(authResult{err: err})
		}
	}()

	return webLogin{authURL: authURL, resultCh: resultCh}, nil
}

func (p *GoogleProvider) credentialsFromToken(ctx context.Context, token *oauth2.Token, fallbackScopes []string) (*Credentials, error) {
	if token == nil {
		return nil, errors.New("token is nil")
	}
	scope := ""
	if extra := token.Extra("scope"); extra != nil {
		if s, ok := extra.(string); ok {
			scope = s
		}
	}
	if scope == "" && len(fallbackScopes) > 0 {
		scope = strings.Join(fallbackScopes, " ")
	}
	creds := &Credentials{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Scope:        scope,
		Expiry:       token.Expiry,
	}
	if extra := token.Extra("id_token"); extra != nil {
		if s, ok := extra.(string); ok {
			creds.IDToken = s
		}
	}
	email, err := p.fetchUserEmail(ctx, creds.AccessToken)
	if err == nil {
		creds.AccountEmail = email
	} else {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch user email: %v\n", err)
	}
	return creds, nil
}

func (p *GoogleProvider) fetchUserEmail(ctx context.Context, accessToken string) (string, error) {
	if accessToken == "" {
		return "", errors.New("access token is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("user info request failed: %s", resp.Status)
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.Email, nil
}

func callbackListener(host string) (net.Listener, int, error) {
	port := 0
	if value := os.Getenv("OAUTH_CALLBACK_PORT"); value != "" {
		parsed, err := strconvToPort(value)
		if err != nil {
			return nil, 0, err
		}
		port = parsed
	}
	host = normalizeCallbackHost(host)
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, 0, err
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		if err := listener.Close(); err != nil {
			return nil, 0, fmt.Errorf("unexpected listener address (cleanup failed: %v)", err)
		}
		return nil, 0, errors.New("unexpected listener address")
	}
	return listener, addr.Port, nil
}

func normalizeCallbackHost(host string) string {
	if host == "" {
		return host
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	return host
}

func strconvToPort(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("port is empty")
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid value for OAUTH_CALLBACK_PORT: %q", value)
	}
	if port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid value for OAUTH_CALLBACK_PORT: %q", value)
	}
	return port, nil
}

func isNoBrowser() bool {
	value := strings.TrimSpace(os.Getenv("NO_BROWSER"))
	return strings.EqualFold(value, "true") || value == "1"
}

func generatePKCEVerifier() (string, string, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func generateState() (string, error) {
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(stateBytes), nil
}

func parseAuthCodeInput(input, expectedState string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", errors.New("authorization code is required")
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("parse callback URL: %w", err)
		}
		query := parsed.Query()
		if oauthErr := strings.TrimSpace(query.Get("error")); oauthErr != "" {
			desc := strings.TrimSpace(query.Get("error_description"))
			if desc == "" {
				return "", fmt.Errorf("oauth error: %s", oauthErr)
			}
			return "", fmt.Errorf("oauth error: %s (%s)", oauthErr, desc)
		}
		if expectedState != "" {
			gotState := strings.TrimSpace(query.Get("state"))
			if gotState == "" {
				return "", errors.New("callback URL is missing state")
			}
			if gotState != expectedState {
				return "", errors.New("oauth state mismatch")
			}
		}
		code := strings.TrimSpace(query.Get("code"))
		if code == "" {
			return "", errors.New("callback URL is missing code")
		}
		return code, nil
	}
	return raw, nil
}

func promptLine(prompt string) (string, error) {
	if !isTerminal(os.Stdin) {
		return "", FatalAuthenticationError{
			Message: "Code Assist login required, but interactive consent could not be obtained. Please run Gemini CLI in an interactive terminal to authenticate, or use NO_BROWSER=true for manual authentication.",
		}
	}
	if _, err := fmt.Fprint(os.Stdout, prompt); err != nil {
		return "", err
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func getConsentForOauth() (bool, error) {
	if !isTerminal(os.Stdin) {
		return false, FatalAuthenticationError{
			Message: "Code Assist login required, but interactive consent could not be obtained. Please run Gemini CLI in an interactive terminal to authenticate, or use NO_BROWSER=true for manual authentication.",
		}
	}
	prompt := "Code Assist login required. Opening authentication page in your browser. Do you want to continue? [Y/n]: "
	if _, err := fmt.Fprint(os.Stdout, "\n"+prompt); err != nil {
		return false, err
	}
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "" || answer == "y" || answer == "yes", nil
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
