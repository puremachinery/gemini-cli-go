package client

import (
	"strings"
	"testing"
)

func TestFormatHTTPErrorClassification(t *testing.T) {
	err := formatHTTPError("test", "429 Too Many Requests", 429, []byte("rate limit"))
	if err == nil || !strings.Contains(err.Error(), "(transient)") {
		t.Fatalf("expected transient classification, got %v", err)
	}
	err = formatHTTPError("test", "400 Bad Request", 400, []byte("bad"))
	if err == nil || !strings.Contains(err.Error(), "(permanent)") {
		t.Fatalf("expected permanent classification, got %v", err)
	}
}
