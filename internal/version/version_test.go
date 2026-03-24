package version

import "testing"

func TestString(t *testing.T) {
	original := Version
	t.Cleanup(func() {
		Version = original
	})

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{
			name:    "prefers injected version",
			version: "v0.1.0",
			want:    "v0.1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			if got := String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
