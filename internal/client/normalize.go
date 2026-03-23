package client

import "strings"

// normalizeModel trims whitespace and strips the models/ prefix.
func normalizeModel(model string) string {
	model = strings.TrimSpace(model)
	if strings.HasPrefix(model, "models/") {
		return strings.TrimPrefix(model, "models/")
	}
	return model
}
