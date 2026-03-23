package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// loadCodeAssistRequest is the POST body for the loadCodeAssist RPC.
type loadCodeAssistRequest struct {
	UserMetadata loadCodeAssistUserMetadata `json:"userMetadata"`
}

type loadCodeAssistUserMetadata struct {
	ClientID string `json:"clientId"`
}

// loadCodeAssistResponse is the JSON returned by loadCodeAssist.
type loadCodeAssistResponse struct {
	CurrentTier  string   `json:"currentTier,omitempty"`
	AllowedTiers []string `json:"allowedTiers,omitempty"`
}

// CheckCodeAssistAccess calls the loadCodeAssist endpoint to determine whether
// the authenticated user has a Code Assist license. It returns true when the
// response contains a currentTier or at least one allowedTier, false otherwise.
// On network/HTTP errors the error is returned so callers can decide how to
// handle it (e.g. log a warning and fall back).
func CheckCodeAssistAccess(ctx context.Context, httpClient *http.Client, projectID string) (hasAccess bool, err error) {
	endpoint := resolveEndpoint("")
	version := resolveAPIVersion("")
	url := fmt.Sprintf("%s/%s:loadCodeAssist", endpoint, version)

	payload := loadCodeAssistRequest{
		UserMetadata: loadCodeAssistUserMetadata{
			ClientID: "gemini-cli-go",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("marshal loadCodeAssist request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("create loadCodeAssist request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if projectID != "" {
		req.Header.Set("x-goog-user-project", projectID)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("loadCodeAssist request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = fmt.Errorf("close loadCodeAssist response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return false, fmt.Errorf("loadCodeAssist returned status %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return false, fmt.Errorf("loadCodeAssist returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result loadCodeAssistResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode loadCodeAssist response: %w", err)
	}

	if result.CurrentTier != "" {
		return true, nil
	}
	if len(result.AllowedTiers) > 0 {
		return true, nil
	}
	return false, nil
}
