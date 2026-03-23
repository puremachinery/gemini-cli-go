package tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

type citationInsertion struct {
	index  int
	marker string
}

func insertCitations(text string, supports []llm.GroundingSupport) string {
	if len(supports) == 0 {
		return text
	}
	insertions := make([]citationInsertion, 0, len(supports))
	for _, support := range supports {
		if support.Segment == nil || len(support.GroundingChunkIndices) == 0 {
			continue
		}
		markers := make([]string, 0, len(support.GroundingChunkIndices))
		for _, idx := range support.GroundingChunkIndices {
			markers = append(markers, fmt.Sprintf("[%d]", idx+1))
		}
		insertions = append(insertions, citationInsertion{
			index:  support.Segment.EndIndex,
			marker: strings.Join(markers, ""),
		})
	}
	if len(insertions) == 0 {
		return text
	}
	sort.Slice(insertions, func(i, j int) bool {
		return insertions[i].index > insertions[j].index
	})
	data := []byte(text)
	lastIndex := len(data)
	parts := make([][]byte, 0, len(insertions)*2+1)
	for _, ins := range insertions {
		pos := ins.index
		if pos > lastIndex {
			pos = lastIndex
		}
		if pos < 0 {
			pos = 0
		}
		parts = append(parts, data[pos:lastIndex])
		parts = append(parts, []byte(ins.marker))
		lastIndex = pos
	}
	parts = append(parts, data[:lastIndex])
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	total := 0
	for _, part := range parts {
		total += len(part)
	}
	out := make([]byte, 0, total)
	for _, part := range parts {
		out = append(out, part...)
	}
	return string(out)
}

// insertCitationsByRune inserts citation markers using rune indices.
func insertCitationsByRune(text string, supports []llm.GroundingSupport) string {
	if len(supports) == 0 {
		return text
	}
	insertions := make([]citationInsertion, 0, len(supports))
	for _, support := range supports {
		if support.Segment == nil || len(support.GroundingChunkIndices) == 0 {
			continue
		}
		markers := make([]string, 0, len(support.GroundingChunkIndices))
		for _, idx := range support.GroundingChunkIndices {
			markers = append(markers, fmt.Sprintf("[%d]", idx+1))
		}
		insertions = append(insertions, citationInsertion{
			index:  support.Segment.EndIndex,
			marker: strings.Join(markers, ""),
		})
	}
	if len(insertions) == 0 {
		return text
	}
	sort.Slice(insertions, func(i, j int) bool {
		return insertions[i].index > insertions[j].index
	})
	runes := []rune(text)
	lastIndex := len(runes)
	parts := make([][]rune, 0, len(insertions)*2+1)
	for _, ins := range insertions {
		pos := ins.index
		if pos > lastIndex {
			pos = lastIndex
		}
		if pos < 0 {
			pos = 0
		}
		parts = append(parts, runes[pos:lastIndex])
		parts = append(parts, []rune(ins.marker))
		lastIndex = pos
	}
	parts = append(parts, runes[:lastIndex])
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	total := 0
	for _, part := range parts {
		total += len(part)
	}
	out := make([]rune, 0, total)
	for _, part := range parts {
		out = append(out, part...)
	}
	return string(out)
}
