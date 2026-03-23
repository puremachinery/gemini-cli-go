package tools

import (
	"testing"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

func TestInsertCitationsByRune(t *testing.T) {
	text := "A😀B"
	supports := []llm.GroundingSupport{{
		Segment: &llm.GroundingSegment{
			StartIndex: 0,
			EndIndex:   2,
		},
		GroundingChunkIndices: []int{0},
	}}
	out := insertCitationsByRune(text, supports)
	if out != "A😀[1]B" {
		t.Fatalf("unexpected citation insertion: %q", out)
	}
}
