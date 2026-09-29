// Package testpdf writes tiny PDFs for tests, so the test suite doesn't
// depend on real (private) sermon documents.
package testpdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/rivers-church/propresenter-toolkit/internal/metrics"
)

// Span is a piece of text in one style.
type Span struct {
	Text   string
	Bold   bool
	Italic bool
	Color  [3]float64 // fill colour, 0..1 per component; zero value is black
}

// Colored returns s drawn in the given RGB colour.
func Colored(s Span, r, g, b float64) Span {
	s.Color = [3]float64{r, g, b}
	return s
}

// Line is one line of text; spans are drawn left to right.
type Line []Span

// Regular, Bold and Italic are shorthands for building lines.
func Regular(s string) Span { return Span{Text: s} }
func Bold(s string) Span    { return Span{Text: s, Bold: true} }
func Italic(s string) Span  { return Span{Text: s, Italic: true} }

// Placed is a line at an exact position: X is the left edge (or the centre
// when Center is set), Y the baseline from the top of the page, both in
// points. Size defaults to 12.
type Placed struct {
	Spans  []Span
	X, Y   float64
	Size   float64
	Center bool
	Box    bool // draw a white box behind the line
}

// Build returns a single-page A4 PDF with one line of 12pt text per entry,
// 20pt apart, left-aligned at x=40.
func Build(lines []Line) []byte {
	var page []Placed
	for i, l := range lines {
		page = append(page, Placed{Spans: l, X: 40, Y: 42 + float64(i)*20})
	}
	return BuildPages([][]Placed{page})
}

const pageW, pageH = 595.0, 842.0

// LineWidth is the drawn width of spans at a font size (all fonts here share
// the Helvetica-Bold width table, declared via /Widths).
func LineWidth(spans []Span, size float64) float64 {
	w := 0.0
	for _, sp := range spans {
		w += metrics.TextWidth(sp.Text, size)
	}
	return w
}

// BuildPages returns a PDF with one page per entry.
func BuildPages(pages [][]Placed) []byte { return build(pages, nil) }

// BuildDarkPages is BuildPages on black pages (painted with a full-page
// path fill, as some note apps export). A line with Box set gets a white
// box drawn behind it.
func BuildDarkPages(pages [][]Placed) []byte {
	black := [3]float64{}
	return build(pages, &black)
}

func build(pages [][]Placed, background *[3]float64) []byte {
	var widths strings.Builder
	for _, w := range metrics.Widths() {
		fmt.Fprintf(&widths, "%d ", w)
	}
	font := func(base string) string {
		return fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /%s /Encoding /WinAnsiEncoding /FirstChar 32 /LastChar 126 /Widths [%s] >>", base, widths.String())
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"", // pages, filled in below
		font("Helvetica"),
		font("Helvetica-Bold"),
		font("Helvetica-Oblique"),
		font("Helvetica-BoldOblique"),
	}
	var kids []string
	for _, page := range pages {
		var content strings.Builder
		if background != nil {
			bg := *background
			fmt.Fprintf(&content, "%g %g %g rg 0 %g m %g %g l %g 0 l 0 0 l h f\n", bg[0], bg[1], bg[2], pageH, pageW, pageH, pageW)
		}
		for _, l := range page {
			if l.Box {
				size := l.Size
				if size == 0 {
					size = 12
				}
				x := l.X
				if l.Center {
					x -= LineWidth(l.Spans, size) / 2
				}
				fmt.Fprintf(&content, "1 1 1 rg %.2f %.2f %.2f %.2f re f\n", x-2, pageH-l.Y-4, LineWidth(l.Spans, size)+4, size+4)
			}
		}
		for _, l := range page {
			size := l.Size
			if size == 0 {
				size = 12
			}
			x := l.X
			if l.Center {
				x -= LineWidth(l.Spans, size) / 2
			}
			fmt.Fprintf(&content, "BT\n1 0 0 1 %.2f %.2f Tm\n", x, pageH-l.Y)
			for _, sp := range l.Spans {
				f := 1
				if sp.Bold {
					f++
				}
				if sp.Italic {
					f += 2
				}
				fmt.Fprintf(&content, "%g %g %g rg /F%d %g Tf (%s) Tj\n",
					sp.Color[0], sp.Color[1], sp.Color[2], f, size, escape(sp.Text))
			}
			content.WriteString("ET\n")
		}
		contentObj := len(objs) + 1
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()))
		pageObj := len(objs) + 1
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %g %g] /Contents %d 0 R /Resources << /Font << /F1 3 0 R /F2 4 0 R /F3 5 0 R /F4 6 0 R >> >> >>", pageW, pageH, contentObj))
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObj))
	}
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids))

	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(s)
}
