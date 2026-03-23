package tools

import (
	"fmt"
	"math"
)

func getStringArg(args map[string]any, key string) (string, error) {
	value, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	str, ok := value.(string)
	if !ok || str == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return str, nil
}

func getOptionalString(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return str
}

func getOptionalInt(args map[string]any, key string) (int, bool, error) {
	value, ok := args[key]
	if !ok {
		return 0, false, nil
	}
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	switch v := value.(type) {
	case int:
		return v, true, nil
	case int64:
		if v > maxInt || v < minInt {
			return 0, false, fmt.Errorf("%s is out of range", key)
		}
		return int(v), true, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false, fmt.Errorf("%s must be a finite number", key)
		}
		if v != math.Trunc(v) {
			return 0, false, fmt.Errorf("%s must be an integer", key)
		}
		if v > float64(maxInt) || v < float64(minInt) {
			return 0, false, fmt.Errorf("%s is out of range", key)
		}
		return int(v), true, nil
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false, fmt.Errorf("%s must be a finite number", key)
		}
		if f != math.Trunc(f) {
			return 0, false, fmt.Errorf("%s must be an integer", key)
		}
		if f > float64(maxInt) || f < float64(minInt) {
			return 0, false, fmt.Errorf("%s is out of range", key)
		}
		return int(f), true, nil
	default:
		return 0, false, fmt.Errorf("%s must be a number", key)
	}
}
