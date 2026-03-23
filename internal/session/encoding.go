package session

import (
	"net/url"
	"strconv"
	"strings"
)

// EncodeTagName encodes a tag into a filename-safe form.
func EncodeTagName(tag string) string {
	return url.PathEscape(tag)
}

// DecodeTagName decodes a filename-safe tag back to its original form.
func DecodeTagName(tag string) string {
	decoded, err := url.PathUnescape(tag)
	if err == nil {
		return decoded
	}
	return decodePercentFallback(tag)
}

func decodePercentFallback(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	for i := 0; i < len(input); i++ {
		if input[i] == '%' && i+2 < len(input) {
			if val, err := strconv.ParseUint(input[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(val))
				i += 2
				continue
			}
		}
		b.WriteByte(input[i])
	}
	return b.String()
}
