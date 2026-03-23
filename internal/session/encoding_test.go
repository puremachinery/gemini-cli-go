package session

import "testing"

func TestEncodeDecodeTagName(t *testing.T) {
	cases := []string{
		"simple",
		"with space",
		"slash/segment",
		"percent%value",
	}
	for _, input := range cases {
		encoded := EncodeTagName(input)
		decoded := DecodeTagName(encoded)
		if decoded != input {
			t.Fatalf("roundtrip failed: %q -> %q -> %q", input, encoded, decoded)
		}
	}
}

func TestDecodePercentFallback(t *testing.T) {
	input := "hello%ZZworld"
	decoded := DecodeTagName(input)
	if decoded != input {
		t.Fatalf("expected fallback to preserve invalid escape, got %q", decoded)
	}
}
