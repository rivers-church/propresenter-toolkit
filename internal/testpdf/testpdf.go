// Package testpdf writes tiny PDFs for tests, so the test suite doesn't
// depend on real (private) sermon documents.
package testpdf

import (
	"bytes"
	"fmt"
	"strings"
)

// Span is a piece of text in regular or bold weight.
type Span struct {
	Text string
	Bold bool
}

// Line is one line of text; spans are drawn left to right.
type Line []Span

// Regular and Bold are shorthands for building lines.
func Regular(s string) Span { return Span{Text: s} }
func Bold(s string) Span    { return Span{Text: s, Bold: true} }

// Build returns a single-page A4 PDF with one line of 12pt text per entry,
// using the standard Helvetica and Helvetica-Bold fonts.
func Build(lines []Line) []byte {
	var content strings.Builder
	y := 800.0
	for _, line := range lines {
		content.WriteString("BT\n")
		fmt.Fprintf(&content, "1 0 0 1 40 %.1f Tm\n", y)
		for _, sp := range line {
			font := "F1"
			if sp.Bold {
				font = "F2"
			}
			fmt.Fprintf(&content, "/%s 12 Tf (%s) Tj\n", font, escape(sp.Text))
		}
		content.WriteString("ET\n")
		y -= 20
	}

	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R /F2 6 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>",
	}
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
