// Package pdftext pulls visual lines of text out of a PDF, keeping enough
// font information per word to tell bold words from regular ones.
package pdftext

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// BoldFontMarkers are substrings of a PDF font name that mark it as bold.
// This matched every sample seen so far (Avenir-Heavy / Avenir-Black); if a
// different export uses other naming, add to this list.
var BoldFontMarkers = []string{"Heavy", "Black", "Bold"}

// IsBoldFont reports whether a PDF font name looks like a bold weight.
func IsBoldFont(name string) bool {
	for _, m := range BoldFontMarkers {
		if strings.Contains(name, m) {
			return true
		}
	}
	return false
}

// Run is one word (or part of a word, where its weight changes mid-word).
// SpaceBefore is true when a space separates it from the previous run.
type Run struct {
	Text        string
	Bold        bool
	SpaceBefore bool
}

// Line is one visual line of text on a page.
type Line struct {
	Page int
	Runs []Run
}

// Text returns the line as a plain string.
func (l Line) Text() string {
	var sb strings.Builder
	for i, r := range l.Runs {
		if i > 0 && r.SpaceBefore {
			sb.WriteByte(' ')
		}
		sb.WriteString(r.Text)
	}
	return sb.String()
}

// Options tune extraction.
type Options struct {
	// DropSuperscriptDigits removes digits set noticeably smaller than the
	// rest of their line - i.e. inline verse numbers in scripture.
	DropSuperscriptDigits bool
}

const (
	// Glyphs whose baselines are within this many points share a line.
	// Real line spacing in the samples is ~30pt; a word nudged 2pt off the
	// baseline is still the same line.
	yTolerance = 3.0
	// A horizontal gap wider than this between glyphs counts as a space.
	xTolerance = 3.0
	// A glyph this much smaller than its line's largest font is superscript.
	superscriptRatio = 0.8
)

// Typographic ligatures some PDF exports use; expanded so slide text is
// plain, editable letters.
var ligatures = strings.NewReplacer(
	"ﬀ", "ff", "ﬁ", "fi", "ﬂ", "fl", "ﬃ", "ffi", "ﬄ", "ffl", "ﬅ", "st", "ﬆ", "st",
)

type glyph struct {
	s          string
	font       string
	size, x, y float64
	w          float64
	seq        int // position in the content stream, for stable ordering
}

// ReadLines parses a PDF and returns its visual lines in reading order.
func ReadLines(r io.ReaderAt, size int64, opt Options) ([]Line, error) {
	doc, err := pdf.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("reading PDF: %w", err)
	}
	var out []Line
	for p := 1; p <= doc.NumPage(); p++ {
		page := doc.Page(p)
		if page.V.IsNull() {
			continue
		}
		lines, err := pageLines(page, p, opt)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", p, err)
		}
		out = append(out, lines...)
	}
	return out, nil
}

func pageLines(page pdf.Page, pageNum int, opt Options) (lines []Line, err error) {
	// The PDF library panics on some malformed content streams.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("could not read text: %v", r)
		}
	}()

	var glyphs []glyph
	for i, t := range page.Content().Text {
		// The library emits zero-width newline glyphs at text-object
		// boundaries; they aren't real characters, and line breaks are
		// recovered from positions below.
		if t.S == "\n" || t.S == "\r" || t.S == "" {
			continue
		}
		s := ligatures.Replace(t.S)
		glyphs = append(glyphs, glyph{s: s, font: t.Font, size: t.FontSize, x: t.X, y: t.Y, w: t.W, seq: i})
	}

	// Cluster by baseline (top of page first), then order each line by x.
	sort.SliceStable(glyphs, func(i, j int) bool { return glyphs[i].y > glyphs[j].y })
	var rows [][]glyph
	for _, g := range glyphs {
		n := len(rows)
		if n > 0 && math.Abs(rows[n-1][0].y-g.y) <= yTolerance {
			rows[n-1] = append(rows[n-1], g)
		} else {
			rows = append(rows, []glyph{g})
		}
	}

	for _, row := range rows {
		sort.Slice(row, func(i, j int) bool {
			if row[i].x != row[j].x {
				return row[i].x < row[j].x
			}
			return row[i].seq < row[j].seq
		})
		if opt.DropSuperscriptDigits {
			row = dropSuperscriptDigits(row)
		}
		if runs := buildRuns(row); len(runs) > 0 {
			lines = append(lines, Line{Page: pageNum, Runs: runs})
		}
	}
	return lines, nil
}

func dropSuperscriptDigits(row []glyph) []glyph {
	maxSize := 0.0
	for _, g := range row {
		if strings.TrimSpace(g.s) != "" && g.size > maxSize {
			maxSize = g.size
		}
	}
	var kept []glyph
	for _, g := range row {
		if g.size < maxSize*superscriptRatio && isDigits(g.s) {
			continue
		}
		kept = append(kept, g)
	}
	return kept
}

func isDigits(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// buildRuns turns x-ordered glyphs into one run per word, breaking at
// explicit spaces or visible gaps. A word whose weight changes part-way
// through (e.g. a bold word followed by a regular comma) is split into
// runs with SpaceBefore=false, so no stray space appears.
func buildRuns(row []glyph) []Run {
	var runs []Run
	pendingSpace := false
	prevEnd := math.Inf(-1)

	for _, g := range row {
		if strings.TrimSpace(g.s) == "" {
			pendingSpace = true
			prevEnd = g.x + g.w
			continue
		}
		if g.x-prevEnd > xTolerance {
			pendingSpace = true
		}
		prevEnd = g.x + g.w
		bold := IsBoldFont(g.font)

		n := len(runs)
		if n > 0 && !pendingSpace && runs[n-1].Bold == bold {
			runs[n-1].Text += g.s
			continue
		}
		runs = append(runs, Run{Text: g.s, Bold: bold, SpaceBefore: pendingSpace && n > 0})
		pendingSpace = false
	}
	return runs
}
