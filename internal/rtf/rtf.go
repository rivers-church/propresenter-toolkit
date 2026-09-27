// Package rtf builds the narrow RTF dialect ProPresenter itself writes into
// a text element's rtf_data. It is not a general RTF library: the goal is
// only that a generated slide looks byte-for-byte plausible next to a
// human-authored one.
package rtf

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/thatguycleeb/propresenter-toolkit/internal/pb"
)

// FontStyle is the font/paragraph parameters read back from a template
// text element, so generated text matches it without hardcoding anything.
type FontStyle struct {
	FontName           string
	SizePt             float64
	Bold               bool
	LineHeightMultiple float64
	ParagraphSpacingPt float64
	BoxWidthPt         float64
}

// Run is a span of text drawn in one color. SpaceBefore inserts a single
// space between this run and the previous one on the same line.
type Run struct {
	Text        string
	Color       int // index into the colors passed to Build
	SpaceBefore bool
}

// Line is one paragraph of the text box.
type Line []Run

// StyleFromElement reads the font parameters a template element already uses.
func StyleFromElement(el *pb.Graphics_Element) FontStyle {
	attrs := el.GetText().GetAttributes()
	lh := attrs.GetParagraphStyle().GetLineHeightMultiple()
	if lh == 0 {
		lh = 1.0
	}
	return FontStyle{
		FontName:           attrs.GetFont().GetName(),
		SizePt:             attrs.GetFont().GetSize(),
		Bold:               attrs.GetFont().GetBold(),
		LineHeightMultiple: lh,
		ParagraphSpacingPt: attrs.GetParagraphStyle().GetParagraphSpacing(),
		BoxWidthPt:         el.GetBounds().GetSize().GetWidth(),
	}
}

// Build returns RTF for the given lines. colorsHex[0] becomes \cf1,
// colorsHex[1] becomes \cf2, and so on.
func Build(style FontStyle, colorsHex []string, lines []Line) ([]byte, error) {
	var tbl, expanded []string
	for _, hex := range colorsHex {
		r, g, b, err := hexToRGB(hex)
		if err != nil {
			return nil, err
		}
		tbl = append(tbl, fmt.Sprintf(`\red%d\green%d\blue%d`, r, g, b))
		expanded = append(expanded, fmt.Sprintf(`\csgenericrgb\c%d\c%d\c%d\c100000`,
			round(float64(r)/255*100000), round(float64(g)/255*100000), round(float64(b)/255*100000)))
	}

	bold := `\b0`
	if style.Bold {
		bold = `\b`
	}
	pard := fmt.Sprintf(`\pard\li0\fi0\ri0\qc\sb0\sa%d\sl%d\slmult1\slleading0`+
		`\f0%s\i0\ul0\strike0\fs%d\expnd0\expndtw0\CocoaLigature1`+
		`\cf1\strokewidth0\strokec1\nosupersub\ulc0\highlight2\cb2 `,
		round(style.ParagraphSpacingPt*20), round(style.LineHeightMultiple*240),
		bold, round(style.SizePt*2))

	paragraphs := make([]string, 0, len(lines))
	for _, line := range lines {
		var sb strings.Builder
		sb.WriteString(pard)
		current := -1
		for i, run := range line {
			leading := ""
			if i > 0 && run.SpaceBefore {
				leading = " "
			}
			if run.Color != current {
				fmt.Fprintf(&sb, `%s\cf%d %s`, leading, run.Color+1, Escape(run.Text))
			} else {
				sb.WriteString(leading + Escape(run.Text))
			}
			current = run.Color
		}
		paragraphs = append(paragraphs, sb.String())
	}

	out := `{\rtf0\ansi\ansicpg1252{\fonttbl\f0\fnil ` + style.FontName + `;}` +
		`{\colortbl;` + strings.Join(tbl, ";") + `;}` +
		`{\*\expandedcolortbl;` + strings.Join(expanded, ";") + `;}` +
		`{\*\listtable}{\*\listoverridetable}` +
		fmt.Sprintf(`\uc1\paperw%d\margl0\margr0\margt0\margb0`, round(style.BoxWidthPt*20)) +
		strings.Join(paragraphs, `\par`) + `}`
	return []byte(out), nil
}

// Plain is a convenience for a single run of single-colored text.
func Plain(style FontStyle, colorHex, text string) ([]byte, error) {
	return Build(style, []string{colorHex}, []Line{{{Text: text}}})
}

// Escape escapes RTF control characters and encodes everything outside
// ASCII as \uN (signed 16-bit) with a '?' fallback, as ProPresenter does.
func Escape(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r == '\\':
			sb.WriteString(`\\`)
		case r == '{':
			sb.WriteString(`\{`)
		case r == '}':
			sb.WriteString(`\}`)
		case r < 128:
			sb.WriteRune(r)
		default:
			// Characters above the BMP become a UTF-16 surrogate pair.
			for _, u := range utf16Units(r) {
				fmt.Fprintf(&sb, `\u%d ?`, int16(u))
			}
		}
	}
	return sb.String()
}

func utf16Units(r rune) []uint16 {
	if r < 0x10000 {
		return []uint16{uint16(r)}
	}
	r -= 0x10000
	return []uint16{uint16(0xD800 + (r >> 10)), uint16(0xDC00 + (r & 0x3FF))}
}

func hexToRGB(hex string) (r, g, b int, err error) {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) != 6 {
		return 0, 0, 0, fmt.Errorf("bad color %q: want #RRGGBB", hex)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("bad color %q: %w", hex, err)
	}
	return int(v >> 16 & 0xFF), int(v >> 8 & 0xFF), int(v & 0xFF), nil
}

// round matches Python's round() (banker's rounding) so output lines up
// with files produced by the original Python tool.
func round(f float64) int {
	return int(math.RoundToEven(f))
}
