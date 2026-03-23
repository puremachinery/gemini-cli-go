package config

import "strings"

// SettingsFileName is the standard settings filename.
const SettingsFileName = "settings.json"

// Settings is an open-ended settings document.
type Settings map[string]any

func unwrapSettings(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case Settings:
		return map[string]any(typed), true
	case map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

// Get returns a value for a dotted path (e.g., "security.auth.selectedType").
func (s Settings) Get(path string) (any, bool) {
	if s == nil {
		return nil, false
	}
	keys := strings.Split(path, ".")
	var current any = map[string]any(s)
	for _, key := range keys {
		m, ok := unwrapSettings(current)
		if !ok {
			return nil, false
		}
		current, ok = m[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// GetString returns a string for a dotted path if present and of type string.
func (s Settings) GetString(path string) (string, bool) {
	v, ok := s.Get(path)
	if !ok {
		return "", false
	}
	str, ok := v.(string)
	return str, ok
}

// Set assigns a value for a dotted path, creating intermediate maps as needed.
// Returns false if an intermediate value is not a map.
func (s Settings) Set(path string, value any) bool {
	if s == nil {
		return false
	}
	keys := strings.Split(path, ".")
	last := len(keys) - 1
	current := map[string]any(s)
	for i, key := range keys {
		if i == last {
			current[key] = value
			return true
		}
		next, ok := current[key]
		if !ok {
			child := map[string]any{}
			current[key] = child
			current = child
			continue
		}
		m, ok := unwrapSettings(next)
		if !ok {
			return false
		}
		current = m
	}
	return false
}

// Delete removes a dotted path from the settings map.
func (s Settings) Delete(path string) bool {
	if s == nil {
		return false
	}
	keys := strings.Split(path, ".")
	last := len(keys) - 1
	current := map[string]any(s)
	for i, key := range keys {
		if i == last {
			if _, ok := current[key]; !ok {
				return false
			}
			delete(current, key)
			return true
		}
		next, ok := current[key]
		if !ok {
			return false
		}
		m, ok := unwrapSettings(next)
		if !ok {
			return false
		}
		current = m
	}
	return false
}
