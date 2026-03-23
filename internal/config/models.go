package config

import "strings"

// Model defaults and aliases mirror upstream constants.
const (
	PreviewGeminiModel        = "gemini-3-pro-preview"
	PreviewGeminiFlashModel   = "gemini-3-flash-preview"
	DefaultGeminiModel        = "gemini-2.5-pro"
	DefaultGeminiFlashModel   = "gemini-2.5-flash"
	DefaultGeminiFlashLite    = "gemini-2.5-flash-lite"
	PreviewGeminiModelAuto    = "auto-gemini-3"
	DefaultGeminiModelAuto    = "auto-gemini-2.5"
	GeminiModelAliasAuto      = "auto"
	GeminiModelAliasPro       = "pro"
	GeminiModelAliasFlash     = "flash"
	GeminiModelAliasFlashLite = "flash-lite"
)

// IsKnownModelAlias reports whether name matches a built-in alias.
func IsKnownModelAlias(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case GeminiModelAliasAuto, GeminiModelAliasPro, GeminiModelAliasFlash, GeminiModelAliasFlashLite:
		return true
	case strings.ToLower(PreviewGeminiModelAuto), strings.ToLower(DefaultGeminiModelAuto):
		return true
	default:
		return false
	}
}

// IsKnownModelName reports whether name matches a known model constant.
func IsKnownModelName(name string) bool {
	switch strings.TrimSpace(name) {
	case PreviewGeminiModel, PreviewGeminiFlashModel, DefaultGeminiModel, DefaultGeminiFlashModel, DefaultGeminiFlashLite:
		return true
	default:
		return false
	}
}

// ResolveModel converts model aliases into concrete model names.
func ResolveModel(requested string, previewFeaturesEnabled bool) string {
	switch requested {
	case PreviewGeminiModelAuto:
		return PreviewGeminiModel
	case DefaultGeminiModelAuto:
		return DefaultGeminiModel
	case GeminiModelAliasAuto, GeminiModelAliasPro:
		if previewFeaturesEnabled {
			return PreviewGeminiModel
		}
		return DefaultGeminiModel
	case GeminiModelAliasFlash:
		if previewFeaturesEnabled {
			return PreviewGeminiFlashModel
		}
		return DefaultGeminiFlashModel
	case GeminiModelAliasFlashLite:
		return DefaultGeminiFlashLite
	default:
		return requested
	}
}
