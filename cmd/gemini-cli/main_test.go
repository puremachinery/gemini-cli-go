package main

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/config"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

func TestCombinePrompt(t *testing.T) {
	if got := combinePrompt("", "hi"); got != "hi" {
		t.Fatalf("expected prompt only, got %q", got)
	}
	if got := combinePrompt("stdin\n", ""); got != "stdin\n" {
		t.Fatalf("expected stdin only, got %q", got)
	}
	if got := combinePrompt("stdin\n", "prompt"); got != "stdin\n\nprompt" {
		t.Fatalf("expected combined prompt, got %q", got)
	}
}

func TestResolveApprovalMode(t *testing.T) {
	settings := config.Settings{}
	settings.Set("tools.approvalMode", "auto_edit")
	mode, err := resolveApprovalMode(&config.LoadResult{Merged: settings}, "", false)
	if err != nil {
		t.Fatalf("resolveApprovalMode: %v", err)
	}
	if mode != tools.ApprovalModeAutoEdit {
		t.Fatalf("expected auto_edit, got %q", mode)
	}

	settings.Set("tools.approvalMode", "yolo")
	if _, err := resolveApprovalMode(&config.LoadResult{Merged: settings}, "", false); err == nil {
		t.Fatal("expected error for yolo in config")
	}

	settings.Set("tools.approvalMode", "plan")
	if _, err := resolveApprovalMode(&config.LoadResult{Merged: settings}, "", false); err == nil {
		t.Fatal("expected error for plan without experimental flag")
	}
	settings.Set("experimental.plan", true)
	mode, err = resolveApprovalMode(&config.LoadResult{Merged: settings}, "", false)
	if err != nil {
		t.Fatalf("resolveApprovalMode (plan): %v", err)
	}
	if mode != tools.ApprovalModePlan {
		t.Fatalf("expected plan, got %q", mode)
	}
}

func TestParseOptionalIntSetting(t *testing.T) {
	if _, err := parseOptionalIntSetting(json.Number("1.5")); err == nil {
		t.Fatal("expected error for non-integer value")
	}
	got, err := parseOptionalIntSetting(json.Number("3"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestReadStdinWithTimeoutReadsData(t *testing.T) {
	orig := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = orig
		if err := r.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Fatalf("Close: %v", err)
		}
	}()

	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := readStdinWithTimeout(200*time.Millisecond, 1024)
	if err != nil {
		t.Fatalf("readStdinWithTimeout: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestReadStdinWithTimeoutReturnsEmptyOnTimeout(t *testing.T) {
	orig := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = orig
		if err := r.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Fatalf("Close: %v", err)
		}
		if err := w.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Fatalf("Close: %v", err)
		}
	}()

	start := time.Now()
	got, err := readStdinWithTimeout(50*time.Millisecond, 1024)
	if err != nil {
		t.Fatalf("readStdinWithTimeout: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty input, got %q", got)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("readStdinWithTimeout did not return promptly on timeout")
	}
}
