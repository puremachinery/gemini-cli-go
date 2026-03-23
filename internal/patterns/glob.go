// Package patterns provides shared glob helpers.
package patterns

import (
	"regexp"
	"strings"
)

// Matcher pairs a glob pattern with its compiled regex.
type Matcher struct {
	Pattern string
	Regex   *regexp.Regexp
}

// CompileMatchers compiles glob patterns into matchers.
func CompileMatchers(patterns []string) ([]Matcher, error) {
	matchers := make([]Matcher, 0, len(patterns))
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		regex, err := GlobToRegex(pattern)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, Matcher{Pattern: pattern, Regex: regex})
	}
	return matchers, nil
}

// MatchAny reports whether any matcher matches the path.
func MatchAny(path string, matchers []Matcher) bool {
	for _, matcher := range matchers {
		if matcher.Regex.MatchString(path) {
			return true
		}
	}
	return false
}

// GlobToRegex converts a glob pattern into a compiled regex.
// Supports **/, **, *, and ? wildcards.
func GlobToRegex(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == '*' {
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 2
					continue
				}
				b.WriteString(".*")
				i++
				continue
			}
			b.WriteString(`[^/]*`)
			continue
		}
		if ch == '?' {
			b.WriteString(".")
			continue
		}
		b.WriteString(regexp.QuoteMeta(string(ch)))
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
