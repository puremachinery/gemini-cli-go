package config

import "testing"

func TestMergeDeepOverride(t *testing.T) {
	a := Settings{
		"a": 1,
		"nested": map[string]any{
			"x": 1,
			"y": 2,
		},
	}
	b := Settings{
		"b": 2,
		"nested": map[string]any{
			"y": 3,
			"z": 4,
		},
	}
	merged := Merge(a, b)
	assertInt(t, merged, "a", 1)
	assertInt(t, merged, "b", 2)
	nestedVal, ok := merged["nested"]
	if !ok {
		t.Fatalf("expected nested map")
	}
	nested, ok := nestedVal.(map[string]any)
	if !ok {
		t.Fatalf("unexpected nested type: %T", nestedVal)
	}
	assertIntInMap(t, nested, "x", 1)
	assertIntInMap(t, nested, "y", 3)
	assertIntInMap(t, nested, "z", 4)
}

func assertInt(t *testing.T, m map[string]any, key string, want int) {
	t.Helper()
	val, ok := m[key]
	if !ok {
		t.Fatalf("expected %s present", key)
	}
	got, ok := val.(int)
	if !ok {
		t.Fatalf("unexpected %s type: %T", key, val)
	}
	if got != want {
		t.Fatalf("unexpected %s: got %d want %d", key, got, want)
	}
}

func assertIntInMap(t *testing.T, m map[string]any, key string, want int) {
	t.Helper()
	val, ok := m[key]
	if !ok {
		t.Fatalf("expected %s present", key)
	}
	got, ok := val.(int)
	if !ok {
		t.Fatalf("unexpected %s type: %T", key, val)
	}
	if got != want {
		t.Fatalf("unexpected %s: got %d want %d", key, got, want)
	}
}
