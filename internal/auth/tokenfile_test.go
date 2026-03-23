package auth

import (
	"strings"
	"testing"
	"time"
)

func TestTokenFileRoundTrip(t *testing.T) {
	expiry := time.Unix(0, 1736092800123*int64(time.Millisecond)).UTC()
	creds := &Credentials{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Scope:        "scope-a scope-b",
		Expiry:       expiry,
		IDToken:      "id-token",
	}
	data, err := encodeTokenFile(creds)
	if err != nil {
		t.Fatalf("encodeTokenFile: %v", err)
	}
	if !strings.Contains(string(data), `"schema_version": 1`) {
		t.Fatalf("expected schema_version in token file, got: %s", string(data))
	}
	decoded, err := decodeTokenFile(data)
	if err != nil {
		t.Fatalf("decodeTokenFile: %v", err)
	}
	if decoded.AccessToken != creds.AccessToken {
		t.Fatalf("AccessToken mismatch: got %q want %q", decoded.AccessToken, creds.AccessToken)
	}
	if decoded.RefreshToken != creds.RefreshToken {
		t.Fatalf("RefreshToken mismatch: got %q want %q", decoded.RefreshToken, creds.RefreshToken)
	}
	if decoded.TokenType != creds.TokenType {
		t.Fatalf("TokenType mismatch: got %q want %q", decoded.TokenType, creds.TokenType)
	}
	if decoded.Scope != creds.Scope {
		t.Fatalf("Scope mismatch: got %q want %q", decoded.Scope, creds.Scope)
	}
	if decoded.IDToken != creds.IDToken {
		t.Fatalf("IDToken mismatch: got %q want %q", decoded.IDToken, creds.IDToken)
	}
	expectedExpiry := expiry.Truncate(time.Millisecond)
	if !decoded.Expiry.Equal(expectedExpiry) {
		t.Fatalf("Expiry mismatch: got %s want %s", decoded.Expiry, expectedExpiry)
	}
}

func TestDecodeTokenFileExpiryString(t *testing.T) {
	raw := []byte(`{"access_token":"token","expiry":"2025-01-02T03:04:05Z"}`)
	decoded, err := decodeTokenFile(raw)
	if err != nil {
		t.Fatalf("decodeTokenFile: %v", err)
	}
	if decoded.Expiry.IsZero() {
		t.Fatal("expected expiry to be parsed from expiry string")
	}
}

func TestDecodeTokenFileRejectsFutureSchema(t *testing.T) {
	raw := []byte(`{"schema_version": 99, "access_token":"token"}`)
	if _, err := decodeTokenFile(raw); err == nil {
		t.Fatal("expected error for future schema version")
	}
}
