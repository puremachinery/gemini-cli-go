package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckCodeAssistAccess(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		statusCode int
		wantAccess bool
		wantErr    bool
	}{
		{
			name:       "has current tier",
			response:   `{"currentTier":"TIER_1"}`,
			statusCode: http.StatusOK,
			wantAccess: true,
		},
		{
			name:       "has allowed tiers",
			response:   `{"allowedTiers":["TIER_1","TIER_2"]}`,
			statusCode: http.StatusOK,
			wantAccess: true,
		},
		{
			name:       "has both tiers",
			response:   `{"currentTier":"TIER_1","allowedTiers":["TIER_2"]}`,
			statusCode: http.StatusOK,
			wantAccess: true,
		},
		{
			name:       "ineligible empty response",
			response:   `{}`,
			statusCode: http.StatusOK,
			wantAccess: false,
		},
		{
			name:       "ineligible empty tiers",
			response:   `{"currentTier":"","allowedTiers":[]}`,
			statusCode: http.StatusOK,
			wantAccess: false,
		},
		{
			name:       "HTTP error",
			response:   `forbidden`,
			statusCode: http.StatusForbidden,
			wantAccess: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", r.Method)
				}
				if got := r.Header.Get("Content-Type"); got != "application/json" {
					t.Fatalf("unexpected Content-Type: %q", got)
				}
				w.WriteHeader(tt.statusCode)
				if _, err := io.WriteString(w, tt.response); err != nil {
					t.Fatalf("WriteString: %v", err)
				}
			}))
			defer server.Close()

			t.Setenv("CODE_ASSIST_ENDPOINT", server.URL)

			got, err := CheckCodeAssistAccess(context.Background(), server.Client(), "test-project")
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckCodeAssistAccess() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantAccess {
				t.Fatalf("CheckCodeAssistAccess() = %v, want %v", got, tt.wantAccess)
			}
		})
	}
}

func TestCheckCodeAssistAccessRequestShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("x-goog-user-project"); got != "my-project" {
			t.Fatalf("unexpected x-goog-user-project: %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected Accept: %q", got)
		}

		var payload loadCodeAssistRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if payload.UserMetadata.ClientID != "gemini-cli-go" {
			t.Fatalf("unexpected clientId: %q", payload.UserMetadata.ClientID)
		}

		// Check URL path contains loadCodeAssist
		if r.URL.Path != "/v1internal:loadCodeAssist" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"currentTier":"TIER_1"}`); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	t.Setenv("CODE_ASSIST_ENDPOINT", server.URL)

	got, err := CheckCodeAssistAccess(context.Background(), server.Client(), "my-project")
	if err != nil {
		t.Fatalf("CheckCodeAssistAccess: %v", err)
	}
	if !got {
		t.Fatal("expected access to be true")
	}
}

func TestCheckCodeAssistAccessNoProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-user-project"); got != "" {
			t.Fatalf("expected no x-goog-user-project header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{}`); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}))
	defer server.Close()

	t.Setenv("CODE_ASSIST_ENDPOINT", server.URL)

	got, err := CheckCodeAssistAccess(context.Background(), server.Client(), "")
	if err != nil {
		t.Fatalf("CheckCodeAssistAccess: %v", err)
	}
	if got {
		t.Fatal("expected access to be false")
	}
}
