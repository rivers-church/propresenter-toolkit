// Package parse turns the lines of a sermon-notes PDF into slide content.
//
// Two formats are supported:
//
//   - Prompts: a running script broken into "SLIDE 1:", "SLIDE 2:", ...
//     sections, where bold words in the PDF become highlighted words.
//   - Notes: a labelled outline (Title:, POINT N:, scripture references,
//     Image:, Back to X when I say: Y).
package parse

import (
	"regexp"
	"strings"

	"github.com/thatguycleeb/propresenter-toolkit/internal/pdftext"
)

// DefaultSlideMarker is the word that starts each section of a prompts script.
const DefaultSlideMarker = "SLIDE"

// The colon is optional: real scripts sometimes have "SLIDE 26" without one.
var slideNumberRE = regexp.MustCompile(`^\d+:?$`)

// PromptSlide is one slide's worth of text: a list of lines, each a list of
// word runs carrying their own bold flag.
type PromptSlide [][]pdftext.Run

// Text returns the slide's text flattened to one string, for previews.
func (s PromptSlide) Text() string {
	var parts []string
	for _, line := range s {
		parts = append(parts, pdftext.Line{Runs: line}.Text())
	}
	return strings.Join(parts, " ")
}

// Prompts splits lines into slides at each "<marker> N:" pair. Anything
// before the first marker (the document heading) is ignored, as are
// markers with no content after them.
func Prompts(lines []pdftext.Line, marker string) []PromptSlide {
	if marker == "" {
		marker = DefaultSlideMarker
	}
	var slides []PromptSlide
	var cur PromptSlide
	started := false

	flush := func() {
		if started && len(cur) > 0 {
			slides = append(slides, cur)
		}
		cur = nil
	}

	for _, line := range lines {
		var pending []pdftext.Run
		runs := line.Runs
		for i := 0; i < len(runs); i++ {
			if runs[i].Text == marker && i+1 < len(runs) && slideNumberRE.MatchString(runs[i+1].Text) {
				if len(pending) > 0 && started {
					cur = append(cur, pending)
				}
				pending = nil
				flush()
				started = true
				i++ // skip the "N:" token too
				continue
			}
			r := runs[i]
			if len(pending) == 0 {
				r.SpaceBefore = false
			}
			pending = append(pending, r)
		}
		if started && len(pending) > 0 {
			cur = append(cur, pending)
		}
	}
	flush()
	return slides
}
