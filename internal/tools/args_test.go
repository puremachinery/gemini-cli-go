package tools

import "testing"

func TestGetOptionalInt(t *testing.T) {
	args := map[string]any{
		"okFloat":    float64(2),
		"badFloat":   float64(0.5),
		"okInteger":  int64(3),
		"badType":    "nope",
		"okNegative": float64(-4),
	}

	if value, ok, err := getOptionalInt(args, "missing"); err != nil || ok || value != 0 {
		t.Fatalf("expected missing key to return zero, got value=%d ok=%v err=%v", value, ok, err)
	}
	if value, ok, err := getOptionalInt(args, "okFloat"); err != nil || !ok || value != 2 {
		t.Fatalf("expected ok float to return 2, got value=%d ok=%v err=%v", value, ok, err)
	}
	if _, ok, err := getOptionalInt(args, "badFloat"); err == nil || ok {
		t.Fatalf("expected fractional float to error, got ok=%v err=%v", ok, err)
	}
	if value, ok, err := getOptionalInt(args, "okInteger"); err != nil || !ok || value != 3 {
		t.Fatalf("expected int64 to return 3, got value=%d ok=%v err=%v", value, ok, err)
	}
	if value, ok, err := getOptionalInt(args, "okNegative"); err != nil || !ok || value != -4 {
		t.Fatalf("expected negative int to return -4, got value=%d ok=%v err=%v", value, ok, err)
	}
	if _, ok, err := getOptionalInt(args, "badType"); err == nil || ok {
		t.Fatalf("expected bad type to error, got ok=%v err=%v", ok, err)
	}
}
