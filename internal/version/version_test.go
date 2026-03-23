package version

import "testing"

func TestStringPrefersInjectedVersion(t *testing.T) {
	original := Version
	Version = "v0.1.0"
	t.Cleanup(func() {
		Version = original
	})

	if got := String(); got != "v0.1.0" {
		t.Fatalf("String() = %q, want %q", got, "v0.1.0")
	}
}
