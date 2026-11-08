package parse

import (
	"strings"

	"github.com/rivers-church/propresenter-toolkit/internal/metrics"
)

// SplitVerse splits a verse or quotation into parts that each wrap to at
// most maxLines in box, cutting only after a full stop or semicolon (with
// any closing quote or bracket that follows it). The parts are spread
// evenly rather than filling each slide before starting the next. A
// sentence too long for one slide on its own is kept whole, as there is no
// acceptable place to cut it.
func SplitVerse(text string, box metrics.Box, maxLines int) []string {
	text = strings.Join(strings.Fields(text), " ")
	if maxLines <= 0 || box.Lines(text) <= maxLines {
		return []string{text}
	}
	units := verseUnits(text)
	join := func(from, to int) string { return strings.Join(units[from:to], " ") }
	var parts []string
	for _, c := range pack(len(units), maxLines, func(from, to int) int {
		return box.Lines(join(from, to))
	}) {
		parts = append(parts, join(c[0], c[1]))
	}
	return parts
}

// verseUnits cuts text after each word that ends in "." or ";" (ignoring
// closing quotes and brackets after it).
func verseUnits(text string) []string {
	var out []string
	var cur []string
	for _, w := range strings.Fields(text) {
		cur = append(cur, w)
		if t := strings.TrimRight(w, "\"'”’)]"); t != "" && (t[len(t)-1] == '.' || t[len(t)-1] == ';') {
			out = append(out, strings.Join(cur, " "))
			cur = nil
		}
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, " "))
	}
	return out
}
