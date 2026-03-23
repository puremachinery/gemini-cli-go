package config

// Merge overlays settings in order, with later settings overriding earlier ones.
func Merge(layers ...Settings) Settings {
	merged := Settings{}
	for _, layer := range layers {
		merged = mergeMaps(merged, layer)
	}
	return merged
}

func mergeMaps(dst, src map[string]any) map[string]any {
	out := cloneMap(dst)
	for key, value := range src {
		if valueMap, ok := value.(map[string]any); ok {
			if existing, ok := out[key].(map[string]any); ok {
				out[key] = mergeMaps(existing, valueMap)
				continue
			}
			out[key] = mergeMaps(map[string]any{}, valueMap)
			continue
		}
		out[key] = value
	}
	return out
}

func cloneMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
