package rtf

import (
	"fmt"
	"strings"
	"testing"
)

var testStyle = FontStyle{FontName: "Avenir-Heavy", SizePt: 60, Bold: true, LineHeightMultiple: 0.9, ParagraphSpacingPt: 24, BoxWidthPt: 1920}

func TestBuild(t *testing.T) {
	got, err := Build(testStyle, []string{"#FFFFFF", "#FFFF00"}, []Line{
		{{Text: "We", Color: 0}, {Text: "are", SpaceBefore: true}, {Text: "SAVED", Color: 1, SpaceBefore: true}},
		{{Text: "by grace"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{
		`{\rtf0\ansi\ansicpg1252{\fonttbl\f0\fnil Avenir-Heavy;}`,
		`{\colortbl;\red255\green255\blue255;\red255\green255\blue0;\red255\green255\blue255;}`,
		`\csgenericrgb\c100000\c100000\c0\c100000`,
		`\paperw38400`,
		`\sa480\sl216\slmult1`,
		`\f0\b\i0`,
		`\fs120`,
		`\cf1 We are \cf2 SAVED\par`,
		`\highlight3\cb3 \cf1 by grace}`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("RTF missing %q\n%s", want, s)
		}
	}
}

// Regression: \highlight and \cb must point at a fully transparent colour
// entry. Pointing at a text colour gave every line a yellow highlight in
// Prompts; pointing past the table rendered a black box in Slides.
func TestHighlightIsTransparent(t *testing.T) {
	for _, colors := range [][]string{{"#FFFFFF"}, {"#FFFFFF", "#FFFF00"}} {
		data, err := Build(testStyle, colors, []Line{{{Text: "x"}}})
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		n := len(colors) + 1
		if !strings.Contains(s, fmt.Sprintf(`\highlight%d\cb%d `, n, n)) {
			t.Errorf("%d colours: highlight should use entry %d\n%s", len(colors), n, s)
		}
		// The nth expandedcolortbl entry must have alpha 0.
		start := strings.Index(s, `{\*\expandedcolortbl;`)
		end := strings.Index(s[start:], "}")
		entries := strings.Split(strings.TrimSuffix(s[start+len(`{\*\expandedcolortbl;`):start+end], ";"), ";")
		if len(entries) != n || !strings.HasSuffix(entries[n-1], `\c0`) {
			t.Errorf("%d colours: expanded table %q should end with an alpha-0 entry", len(colors), entries)
		}
	}
}

func TestBuildItalicAndHighlight(t *testing.T) {
	got, err := Build(testStyle, []string{"#FFFFFF", "#000000"}, []Line{{
		{Text: "Daniel"},
		{Text: "Then", Italic: true, SpaceBefore: true},
		{Text: "the", Italic: true, SpaceBefore: true},
		{Text: "STOP", Color: 1, Highlight: 1, SpaceBefore: true},
		{Text: "end", SpaceBefore: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := `\cf1 Daniel \i Then the \i0\cf2\highlight1\cb1 STOP \cf1\highlight3\cb3 end}`
	// Control words may come in any order, so compare the pieces we care about.
	for _, piece := range []string{`\cf1 Daniel \i Then the `, `\highlight1\cb1 STOP `, `\cf2`, `\i0`, `\highlight3\cb3 end}`} {
		if !strings.Contains(string(got), piece) {
			t.Errorf("RTF missing %q (want something like %s)\n%s", piece, want, got)
		}
	}
}

func TestBuildBadColor(t *testing.T) {
	if _, err := Build(testStyle, []string{"white"}, nil); err == nil {
		t.Error("expected an error for a non-hex color")
	}
}

func TestEscape(t *testing.T) {
	const bs = `\`
	for in, want := range map[string]string{
		"we’re":  "we" + bs + "u8217 ?re",
		`a{b}\c`: `a\{b\}\\c`,
		"poiēma": `poi\u275 ?ma`,
		"😀":      `\u-10179 ?\u-8704 ?`,
	} {
		if got := Escape(in); got != want {
			t.Errorf("Escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlainTextRoundTrip(t *testing.T) {
	data, _ := Build(testStyle, []string{"#FFFFFF", "#FFFF00"}, []Line{
		{{Text: "God’s"}, {Text: "{work}", Color: 1, SpaceBefore: true}},
		{{Text: "of art"}},
	})
	if got, want := PlainText(data), "God’s {work} of art"; got != want {
		t.Errorf("PlainText = %q, want %q", got, want)
	}
}

func TestPlainTextHexEscape(t *testing.T) {
	in := []byte(`{\rtf1\ansi{\fonttbl\f0 Arial;}{\colortbl;\red0\green0\blue0;}\f0 caf\'e9\par ok}`)
	if got, want := PlainText(in), "café ok"; got != want {
		t.Errorf("PlainText = %q, want %q", got, want)
	}
}
