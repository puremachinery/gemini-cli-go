package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
)

// ErrNoCredentials indicates the data does not contain usable credentials.
var ErrNoCredentials = errors.New("no usable credentials")

const tokenFileSchemaVersion = 1

type tokenFile struct {
	SchemaVersion int    `json:"schema_version,omitempty"`
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	TokenType     string `json:"token_type,omitempty"`
	Scope         string `json:"scope,omitempty"`
	ExpiryDate    int64  `json:"expiry_date,omitempty"`
	IDToken       string `json:"id_token,omitempty"`
}

type tokenFileRaw struct {
	SchemaVersion int         `json:"schema_version"`
	AccessToken   string      `json:"access_token"`
	RefreshToken  string      `json:"refresh_token"`
	TokenType     string      `json:"token_type"`
	Scope         string      `json:"scope"`
	ExpiryDate    json.Number `json:"expiry_date"`
	Expiry        string      `json:"expiry"`
	IDToken       string      `json:"id_token"`
}

func decodeTokenFile(data []byte) (*Credentials, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, ErrNoCredentials
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw tokenFileRaw
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	if raw.SchemaVersion > tokenFileSchemaVersion && (raw.AccessToken != "" || raw.RefreshToken != "") {
		return nil, fmt.Errorf("unsupported credentials schema version %d", raw.SchemaVersion)
	}

	creds := &Credentials{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		TokenType:    raw.TokenType,
		Scope:        raw.Scope,
		IDToken:      raw.IDToken,
	}

	if raw.ExpiryDate != "" {
		if ms, err := parseExpiryMillis(raw.ExpiryDate.String()); err == nil {
			creds.Expiry = time.Unix(0, ms*int64(time.Millisecond))
		}
	}

	if creds.Expiry.IsZero() && raw.Expiry != "" {
		if ts, err := time.Parse(time.RFC3339Nano, raw.Expiry); err == nil {
			creds.Expiry = ts
		}
	}

	return creds, nil
}

func encodeTokenFile(creds *Credentials) ([]byte, error) {
	if creds == nil {
		return nil, errors.New("credentials are nil")
	}
	file := tokenFile{
		SchemaVersion: tokenFileSchemaVersion,
		AccessToken:   creds.AccessToken,
		RefreshToken:  creds.RefreshToken,
		TokenType:     creds.TokenType,
		Scope:         strings.TrimSpace(creds.Scope),
		IDToken:       creds.IDToken,
	}
	if !creds.Expiry.IsZero() {
		file.ExpiryDate = creds.Expiry.UnixNano() / int64(time.Millisecond)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	return data, nil
}

func decodeCredentialsJSON(ctx context.Context, data []byte) (*Credentials, error) {
	creds, err := decodeTokenFile(data)
	if err == nil {
		if creds.AccessToken != "" || creds.RefreshToken != "" {
			return creds, nil
		}
	}

	googleCreds, err := decodeGoogleCredentials(ctx, data)
	if err != nil {
		return nil, err
	}
	return googleCreds, nil
}

func decodeGoogleCredentials(ctx context.Context, data []byte) (*Credentials, error) {
	var meta struct {
		Type         string `json:"type"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, ErrNoCredentials
	}
	switch meta.Type {
	case "authorized_user", "external_account_authorized_user":
	default:
		return nil, ErrNoCredentials
	}

	//nolint:staticcheck // Credential JSON is sourced from local user files and intentionally parsed here.
	gcreds, err := google.CredentialsFromJSONWithParams(ctx, data, google.CredentialsParams{Scopes: oauthScopes})
	if err != nil {
		return nil, ErrNoCredentials
	}
	if gcreds == nil || gcreds.TokenSource == nil {
		return nil, ErrNoCredentials
	}
	token, err := gcreds.TokenSource.Token()
	if err != nil {
		return nil, ErrNoCredentials
	}
	if token == nil || token.AccessToken == "" {
		return nil, ErrNoCredentials
	}
	scope := ""
	if extra := token.Extra("scope"); extra != nil {
		if s, ok := extra.(string); ok {
			scope = s
		}
	}
	if scope == "" && len(oauthScopes) > 0 {
		scope = strings.Join(oauthScopes, " ")
	}
	creds := &Credentials{
		AccessToken:  token.AccessToken,
		RefreshToken: meta.RefreshToken,
		TokenType:    token.TokenType,
		Scope:        scope,
		Expiry:       token.Expiry,
	}
	if extra := token.Extra("id_token"); extra != nil {
		if s, ok := extra.(string); ok {
			creds.IDToken = s
		}
	}
	return creds, nil
}

func parseExpiryMillis(value string) (int64, error) {
	if value == "" {
		return 0, errors.New("expiry_date is empty")
	}
	if strings.ContainsAny(value, ".eE") {
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, err
		}
		return int64(floatVal), nil
	}
	return strconv.ParseInt(value, 10, 64)
}
