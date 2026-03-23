package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/tools"
)

const (
	referenceContentStart  = "--- Content from referenced files ---"
	referenceContentEnd    = "--- End of content ---"
	referenceContentHeader = "\n" + referenceContentStart
	referenceContentFooter = "\n" + referenceContentEnd
)

type atCommandPartType int

const (
	atCommandText atCommandPartType = iota
	atCommandPath
)

type atCommandPart struct {
	typ     atCommandPartType
	content string
}

func isAtCommand(query string) bool {
	if strings.HasPrefix(query, "@") {
		return true
	}
	for i, r := range query {
		if r == '@' && i > 0 {
			if isWhitespace(rune(query[i-1])) {
				return true
			}
		}
	}
	return false
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func parseAtCommands(query string) []atCommandPart {
	parts := make([]atCommandPart, 0, 4)
	currentIndex := 0
	for currentIndex < len(query) {
		atIndex := -1
		for i := currentIndex; i < len(query); i++ {
			if query[i] == '@' && (i == 0 || query[i-1] != '\\') {
				atIndex = i
				break
			}
		}
		if atIndex == -1 {
			if currentIndex < len(query) {
				parts = append(parts, atCommandPart{typ: atCommandText, content: query[currentIndex:]})
			}
			break
		}
		if atIndex > currentIndex {
			parts = append(parts, atCommandPart{typ: atCommandText, content: query[currentIndex:atIndex]})
		}

		pathEnd := atIndex + 1
		inEscape := false
		for pathEnd < len(query) {
			ch := query[pathEnd]
			if inEscape {
				inEscape = false
				pathEnd++
				continue
			}
			if ch == '\\' {
				inEscape = true
				pathEnd++
				continue
			}
			if isAtTerminator(ch) {
				break
			}
			if ch == '.' {
				next := byte(0)
				if pathEnd+1 < len(query) {
					next = query[pathEnd+1]
				}
				if next == 0 || isWhitespace(rune(next)) {
					break
				}
			}
			pathEnd++
		}
		rawAtPath := query[atIndex:pathEnd]
		parts = append(parts, atCommandPart{typ: atCommandPath, content: unescapePath(rawAtPath)})
		currentIndex = pathEnd
	}

	filtered := parts[:0]
	for _, part := range parts {
		if part.typ == atCommandText && strings.TrimSpace(part.content) == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return filtered
}

func isAtTerminator(ch byte) bool {
	switch ch {
	case ',', ' ', '\t', '\n', '\r', ';', '!', '?', '(', ')', '[', ']', '{', '}':
		return true
	default:
		return false
	}
}

func unescapePath(path string) string {
	if path == "" {
		return path
	}
	var b strings.Builder
	b.Grow(len(path))
	for i := 0; i < len(path); i++ {
		ch := path[i]
		if ch == '\\' && i+1 < len(path) && isShellSpecial(path[i+1]) {
			i++
			b.WriteByte(path[i])
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func isShellSpecial(ch byte) bool {
	switch ch {
	case ' ', '\t', '(', ')', '[', ']', '{', '}', ';', '|', '*', '?', '$', '`', '\'', '"', '#', '&', '<', '>', '!', '~':
		return true
	default:
		return false
	}
}

func buildPartsFromQuery(_ context.Context, query string, executor *tools.Executor) ([]llm.Part, error) {
	if !isAtCommand(query) {
		return []llm.Part{{Text: query}}, nil
	}

	commandParts := parseAtCommands(query)
	if len(commandParts) == 0 {
		return []llm.Part{{Text: query}}, nil
	}

	var initialText strings.Builder
	pathSpecs := make([]string, 0, len(commandParts))
	for _, part := range commandParts {
		switch part.typ {
		case atCommandText:
			initialText.WriteString(part.content)
		case atCommandPath:
			if part.content == "@" {
				continue
			}
			pathName := strings.TrimPrefix(part.content, "@")
			if pathName == "" {
				return nil, errors.New("invalid @ command: no path specified")
			}
			pathSpecs = append(pathSpecs, pathName)
		}
	}

	if len(pathSpecs) == 0 {
		if strings.TrimSpace(query) == "@" {
			return []llm.Part{{Text: query}}, nil
		}
		if strings.TrimSpace(initialText.String()) == "" && strings.TrimSpace(query) != "" {
			return []llm.Part{{Text: query}}, nil
		}
		return []llm.Part{{Text: initialText.String()}}, nil
	}

	if executor == nil || executor.Registry == nil {
		return nil, errors.New("read_many_files tool not available")
	}
	if _, ok := executor.Registry.Lookup(tools.ReadManyFilesToolName); !ok {
		return nil, errors.New("read_many_files tool not available")
	}
	root := executor.Registry.WorkspaceRoot()
	validSpecs := make([]string, 0, len(pathSpecs))
	for _, spec := range pathSpecs {
		if _, err := tools.ResolvePath(root, spec); err != nil {
			continue
		}
		validSpecs = append(validSpecs, spec)
	}
	if len(validSpecs) == 0 {
		return []llm.Part{{Text: query}}, nil
	}

	fileEntries, readErrors, err := tools.CollectReadManyFiles(tools.Context{WorkspaceRoot: root}, validSpecs, nil)
	if err != nil {
		if errors.Is(err, tools.ErrNoFilesMatched) {
			return []llm.Part{{Text: query}}, nil
		}
		if errors.Is(err, tools.ErrReadManyFilesFailed) && len(readErrors) > 0 {
			return nil, fmt.Errorf("failed to read %d file(s): %s", len(readErrors), readErrors[0])
		}
		return nil, err
	}
	if len(fileEntries) == 0 {
		return []llm.Part{{Text: query}}, nil
	}

	parts := []llm.Part{{Text: initialText.String()}}
	parts = append(parts, llm.Part{Text: referenceContentHeader})
	for _, entry := range fileEntries {
		parts = append(parts, llm.Part{Text: fmt.Sprintf("\nContent from @%s:\n", entry.Path)})
		parts = append(parts, llm.Part{Text: strings.TrimSpace(entry.Content)})
	}
	parts = append(parts, llm.Part{Text: referenceContentFooter})
	return parts, nil
}
