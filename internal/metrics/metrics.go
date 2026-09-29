// Package metrics estimates how text wraps in a ProPresenter text box, so
// long passages can be split across slides before they overflow.
//
// It uses Helvetica-Bold / Arial Bold character widths (the two share
// metrics). For other fonts the estimate is rougher, which is fine: the
// line limit is a soft target and slides can be tidied by hand.
package metrics

import (
	"strings"
	"unicode"
)

// boldWidths are Helvetica-Bold advance widths in 1/1000 em for ASCII
// 32 (space) through 126 (~).
var boldWidths = [95]int{
	278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278, // space ! " # $ % & ' ( ) * + , - . /
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, // 0-9
	333, 333, 584, 584, 584, 611, 975, // : ; < = > ? @
	722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833, // A-M
	722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, // N-Z
	333, 278, 333, 584, 556, 333, // [ \ ] ^ _ `
	556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889, // a-m
	611, 611, 611, 611, 389, 556, 333, 611, 556, 778, 556, 556, 500, // n-z
	389, 280, 389, 584, // { | } ~
}

// Width returns the advance width of r in 1/1000 em.
func Width(r rune) int {
	switch {
	case r >= 32 && r <= 126:
		return boldWidths[r-32]
	case r == '’' || r == '‘':
		return 278
	case r == '“' || r == '”':
		return 500
	case r == '—':
		return 1000
	case r == '–':
		return 556
	case r == '…':
		return 1000
	}
	if unicode.IsUpper(r) {
		return 722
	}
	return 556
}

// Widths returns the ASCII width table, for writing test PDFs.
func Widths() []int { return boldWidths[:] }

// TextWidth is the width of s in points at the given font size.
func TextWidth(s string, size float64) float64 {
	total := 0
	for _, r := range s {
		total += Width(r)
	}
	return float64(total) / 1000 * size
}

// Box describes a text box for wrapping.
type Box struct {
	Width    float64 // usable width in points
	FontSize float64 // points
	AllCaps  bool    // the template forces ALL CAPS
}

// Lines estimates how many lines text occupies when word-wrapped in the box.
// Empty text counts as one (blank) line.
func (b Box) Lines(text string) int {
	if b.AllCaps {
		text = strings.ToUpper(text)
	}
	words := strings.Fields(text)
	if len(words) == 0 || b.Width <= 0 || b.FontSize <= 0 {
		return 1
	}
	space := TextWidth(" ", b.FontSize)
	lines, cur := 1, 0.0
	for _, w := range words {
		ww := TextWidth(w, b.FontSize)
		switch {
		case cur == 0:
			cur = ww
		case cur+space+ww <= b.Width:
			cur += space + ww
		default:
			lines++
			cur = ww
		}
		// A single word wider than the box wraps mid-word.
		for cur > b.Width {
			lines++
			cur -= b.Width
		}
	}
	return lines
}
