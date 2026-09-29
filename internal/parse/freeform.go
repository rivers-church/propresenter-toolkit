package parse

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/rivers-church/propresenter-toolkit/internal/metrics"
	"github.com/rivers-church/propresenter-toolkit/internal/pdftext"
)

// FreeformOptions control how free-form notes are split into slides.
type FreeformOptions struct {
	Box      metrics.Box // the slide's text box, for estimating wrapped lines
	MaxLines int         // soft limit of wrapped lines per slide
}

// DefaultMaxLines is the usual soft limit for a Prompts slide.
const DefaultMaxLines = 7

// Freeform turns notes with no "SLIDE N:" markers into slides, using the
// document's own layout:
//
//   - a block of lines separated from the next by extra space is a unit, and
//     each block starts a new slide; larger section headings start a block;
//   - text set smaller than the body (page headers, dates) is dropped;
//   - wide, mixed-case blocks (e.g. scripture) are joined into one flowing
//     paragraph; other blocks (centred headings) keep their line breaks;
//   - a block too long for one slide is split across several, at sentence
//     ends where possible, keeping each slide near MaxLines.
func Freeform(lines []pdftext.Line, opt FreeformOptions) []PromptSlide {
	if opt.MaxLines <= 0 {
		opt.MaxLines = DefaultMaxLines
	}
	body := bodySize(lines)
	var kept []pdftext.Line
	for _, l := range lines {
		if l.Size >= 0.8*body {
			kept = append(kept, l)
		}
	}
	if len(kept) == 0 {
		return nil
	}

	gap := lineGap(kept)
	column := 0.0
	for _, l := range kept {
		column = math.Max(column, l.X1-l.X0)
	}

	// Group into blocks.
	var blocks [][]pdftext.Line
	for i, l := range kept {
		newBlock := i == 0
		if i > 0 {
			prev := kept[i-1]
			isHeading := l.Size > 1.2*body
			prevHeading := prev.Size > 1.2*body
			switch {
			case l.Page != prev.Page:
				newBlock = true
			case prev.Y-l.Y > 1.5*gap:
				newBlock = true
			case isHeading && !prevHeading:
				newBlock = true
			}
		}
		if newBlock {
			blocks = append(blocks, nil)
		}
		blocks[len(blocks)-1] = append(blocks[len(blocks)-1], l)
	}

	var slides []PromptSlide
	var heading [][]pdftext.Run // a heading-only block waiting for the next block
	for _, b := range blocks {
		var bs []PromptSlide
		if flowing(b, column) {
			bs = splitParagraph(joinLines(b), opt)
		} else {
			var ls [][]pdftext.Run
			for _, l := range b {
				ls = append(ls, withoutLeadingSpace(l.Runs))
			}
			if headingOnly(b, body) {
				slides = flushHeading(slides, heading)
				heading = ls
				continue
			}
			bs = splitLines(ls, opt)
		}
		// Put a waiting section heading at the top of this block's first
		// slide if it fits; otherwise give it a slide of its own.
		if heading != nil {
			first := append(append([][]pdftext.Run{}, heading...), bs[0]...)
			if measureLines(first, opt.Box) <= opt.MaxLines {
				bs[0] = first
			} else {
				slides = flushHeading(slides, heading)
			}
			heading = nil
		}
		slides = append(slides, bs...)
	}
	return flushHeading(slides, heading)
}

func flushHeading(slides []PromptSlide, heading [][]pdftext.Run) []PromptSlide {
	if heading == nil {
		return slides
	}
	return append(slides, PromptSlide(heading))
}

// headingOnly reports whether every line of the block is a larger heading.
func headingOnly(b []pdftext.Line, body float64) bool {
	for _, l := range b {
		if l.Size <= 1.2*body {
			return false
		}
	}
	return true
}

func measureLines(ls [][]pdftext.Run, box metrics.Box) int {
	n := 0
	for _, l := range ls {
		n += box.Lines(runsText(l))
	}
	return n
}

// bodySize is the font size carrying the most text.
func bodySize(lines []pdftext.Line) float64 {
	weight := map[float64]int{}
	for _, l := range lines {
		weight[math.Round(l.Size*2)/2] += len(l.Text())
	}
	best, bestW := 0.0, -1
	for s, w := range weight {
		if w > bestW || (w == bestW && s < best) {
			best, bestW = s, w
		}
	}
	return best
}

// lineGap is the typical baseline-to-baseline distance within a paragraph.
func lineGap(lines []pdftext.Line) float64 {
	var gaps []float64
	for i := 1; i < len(lines); i++ {
		if lines[i].Page == lines[i-1].Page {
			if g := lines[i-1].Y - lines[i].Y; g > 0 {
				gaps = append(gaps, g)
			}
		}
	}
	if len(gaps) == 0 {
		return 1.2 * lines[0].Size
	}
	sort.Float64s(gaps)
	// The most common spacing is within paragraphs; take the lower quartile
	// so paragraph gaps don't pull it up.
	return gaps[len(gaps)/4]
}

// flowing reports whether a block is running text that was wrapped to the
// page (so its line breaks mean nothing), as opposed to deliberate lines.
func flowing(b []pdftext.Line, column float64) bool {
	if len(b) < 2 {
		return false
	}
	hasLower := false
	for _, l := range b {
		for _, r := range l.Text() {
			if unicode.IsLower(r) {
				hasLower = true
			}
		}
	}
	if !hasLower {
		return false // ALL-CAPS lines are headings, broken on purpose
	}
	// Wrapped text runs close to the full width on most lines (the last
	// line, and a short reference line at the start, can be shorter).
	wide := 0
	for _, l := range b[:len(b)-1] {
		if l.X1-l.X0 >= 0.75*column {
			wide++
		}
	}
	return 3*wide >= 2*(len(b)-1)
}

func withoutLeadingSpace(runs []pdftext.Run) []pdftext.Run {
	out := append([]pdftext.Run(nil), runs...)
	if len(out) > 0 {
		out[0].SpaceBefore = false
	}
	return out
}

// joinLines turns a wrapped paragraph back into one line of runs.
func joinLines(b []pdftext.Line) []pdftext.Run {
	var out []pdftext.Run
	for i, l := range b {
		for j, r := range l.Runs {
			if j == 0 {
				r.SpaceBefore = i > 0
			}
			out = append(out, r)
		}
	}
	return out
}

func runsText(runs []pdftext.Run) string {
	return pdftext.Line{Runs: runs}.Text()
}

// splitLines packs a block's deliberate lines into slides.
func splitLines(lines [][]pdftext.Run, opt FreeformOptions) []PromptSlide {
	measure := func(ls [][]pdftext.Run) int {
		n := 0
		for _, l := range ls {
			n += opt.Box.Lines(runsText(l))
		}
		return n
	}
	// A single line too long for a slide on its own is split by words.
	var units [][]pdftext.Run
	for _, l := range lines {
		if measure([][]pdftext.Run{l}) > opt.MaxLines {
			units = append(units, splitWords(l, opt)...)
		} else {
			units = append(units, l)
		}
	}
	var slides []PromptSlide
	for _, chunk := range pack(len(units), opt.MaxLines, func(from, to int) int {
		return measure(units[from:to])
	}) {
		slides = append(slides, PromptSlide(units[chunk[0]:chunk[1]]))
	}
	return slides
}

// splitParagraph splits one flowing paragraph into slides at sentence ends.
func splitParagraph(para []pdftext.Run, opt FreeformOptions) []PromptSlide {
	if opt.Box.Lines(runsText(para)) <= opt.MaxLines {
		return []PromptSlide{{para}}
	}
	var units [][]pdftext.Run
	for _, s := range sentences(para) {
		if opt.Box.Lines(runsText(s)) > opt.MaxLines {
			units = append(units, splitWords(s, opt)...)
		} else {
			units = append(units, s)
		}
	}
	join := func(from, to int) []pdftext.Run {
		var out []pdftext.Run
		for i, u := range units[from:to] {
			for j, r := range u {
				if j == 0 {
					r.SpaceBefore = i > 0
				}
				out = append(out, r)
			}
		}
		return out
	}
	var slides []PromptSlide
	for _, chunk := range pack(len(units), opt.MaxLines, func(from, to int) int {
		return opt.Box.Lines(runsText(join(from, to)))
	}) {
		slides = append(slides, PromptSlide{join(chunk[0], chunk[1])})
	}
	return slides
}

// sentences splits runs after words ending a sentence (. ! ? ; optionally
// followed by closing quotes or brackets).
func sentences(runs []pdftext.Run) [][]pdftext.Run {
	var out [][]pdftext.Run
	start := 0
	for i, r := range runs {
		endOfWord := i+1 == len(runs) || runs[i+1].SpaceBefore
		t := strings.TrimRight(r.Text, "\"'”’)]")
		if endOfWord && t != "" && strings.ContainsRune(".!?;", rune(t[len(t)-1])) {
			out = append(out, withoutLeadingSpace(runs[start:i+1]))
			start = i + 1
		}
	}
	if start < len(runs) {
		out = append(out, withoutLeadingSpace(runs[start:]))
	}
	return out
}

// splitWords breaks runs into pieces that each fit within MaxLines.
func splitWords(runs []pdftext.Run, opt FreeformOptions) [][]pdftext.Run {
	var out [][]pdftext.Run
	start := 0
	for i := range runs {
		if i > start && runs[i].SpaceBefore && opt.Box.Lines(runsText(runs[start:i+1])) > opt.MaxLines {
			out = append(out, withoutLeadingSpace(runs[start:i]))
			start = i
		}
	}
	return append(out, withoutLeadingSpace(runs[start:]))
}

// pack groups n units, in order, into chunks whose measured size is at most
// limit, spreading them evenly: it uses as few chunks as a greedy fill to
// limit would, but with the smallest per-chunk size that still achieves that.
func pack(n, limit int, size func(from, to int) int) [][2]int {
	greedy := func(max int) [][2]int {
		var chunks [][2]int
		start := 0
		for i := 1; i <= n; i++ {
			if i-start > 1 && size(start, i) > max {
				chunks = append(chunks, [2]int{start, i - 1})
				start = i - 1
			}
		}
		if start < n {
			chunks = append(chunks, [2]int{start, n})
		}
		return chunks
	}
	best := greedy(limit)
	for max := 1; max < limit; max++ {
		if c := greedy(max); len(c) <= len(best) {
			return c
		}
	}
	return best
}
