package parse

import (
	"strings"
	"testing"

	"github.com/rivers-church/propresenter-toolkit/internal/metrics"
	"github.com/rivers-church/propresenter-toolkit/internal/pdftext"
)

// A layout like the youth-service notes: 20pt body, 27pt line spacing,
// blocks 54pt apart, a small page header and 28pt section headings.
type fakePage struct {
	page  int
	y     float64
	lines []pdftext.Line
}

func (p *fakePage) add(text string, size, x0, x1, gapBefore float64) {
	p.y -= gapBefore
	var runs []pdftext.Run
	for i, w := range strings.Fields(text) {
		runs = append(runs, pdftext.Run{Text: w, SpaceBefore: i > 0})
	}
	p.lines = append(p.lines, pdftext.Line{Page: p.page, Runs: runs, X0: x0, X1: x1, Y: p.y, Size: size})
}

var testBox = metrics.Box{Width: 1920, FontSize: 135}

func slideTexts(slides []PromptSlide) []string {
	var out []string
	for _, s := range slides {
		var ls []string
		for _, l := range s {
			ls = append(ls, pdftext.Line{Runs: l}.Text())
		}
		out = append(out, strings.Join(ls, " / "))
	}
	return out
}

func TestFreeformBlocks(t *testing.T) {
	p := &fakePage{page: 1, y: 842}
	p.add("No Exits", 12, 490, 538, 60) // header: dropped
	p.add("22 November 2024", 14, 417, 538, 20)
	p.add("INTRO", 28, 252, 342, 57) // section heading joins the next block
	p.add("WHAT GOD DOES", 20, 91, 300, 39.5)
	p.add("HE IS THE GOD OF PROCESS", 20, 158, 436, 27)
	// A wrapped paragraph: wide lines, mixed case -> joined.
	p.add("Daniel 6:3 ESV Then this Daniel became", 20, 57, 500, 54)
	p.add("distinguished above all the others.", 20, 57, 300, 27)
	p.add("THERE'S NO EXIT", 28, 175, 420, 54)
	// A heading at the foot of a page belongs with what follows it.
	p2 := &fakePage{page: 2, y: 842}
	p2.add("LION'S DENS DON'T HAVE EXITS", 20, 150, 440, 60)
	p2.add("ALTAR CALL", 20, 200, 400, 54)

	got := slideTexts(Freeform(append(p.lines, p2.lines...), FreeformOptions{Box: testBox, MaxLines: 7}))
	want := []string{
		"INTRO / WHAT GOD DOES / HE IS THE GOD OF PROCESS",
		"Daniel 6:3 ESV Then this Daniel became distinguished above all the others.",
		"THERE'S NO EXIT / LION'S DENS DON'T HAVE EXITS",
		"ALTAR CALL",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("slides:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestFreeformSplitsLongParagraphAtSentences(t *testing.T) {
	sentence := "The king went to his palace and spent the night fasting and no one came near him."
	p := &fakePage{page: 1, y: 842}
	p.add("Heading", 20, 250, 300, 60)
	var text []string
	for i := 0; i < 6; i++ {
		text = append(text, sentence)
	}
	// Lay the paragraph out as wide lines of roughly 8 words.
	words := strings.Fields(strings.Join(text, " "))
	for i := 0; i < len(words); i += 8 {
		end := min(i+8, len(words))
		p.add(strings.Join(words[i:end], " "), 20, 57, 520, map[bool]float64{true: 54, false: 27}[i == 0])
	}

	slides := Freeform(p.lines, FreeformOptions{Box: testBox, MaxLines: 7})
	texts := slideTexts(slides)
	if len(texts) < 3 {
		t.Fatalf("expected the paragraph split over several slides, got %v", texts)
	}
	for i, s := range slides[1:] {
		n := testBox.Lines(texts[i+1])
		if n > 7 {
			t.Errorf("slide %d has %d lines, over the limit", i+2, n)
		}
		if !strings.HasSuffix(texts[i+1], ".") {
			t.Errorf("slide %d doesn't end at a sentence: %q", i+2, texts[i+1])
		}
		if len(s) != 1 {
			t.Errorf("flowing text should be one paragraph per slide, got %d lines", len(s))
		}
	}
	// Balanced: no tiny leftover slide.
	first, last := testBox.Lines(texts[1]), testBox.Lines(texts[len(texts)-1])
	if last < first-3 {
		t.Errorf("unbalanced split: first %d lines, last %d", first, last)
	}
}

func TestFreeformKeepsCapsLinesSeparate(t *testing.T) {
	// Wide ALL-CAPS lines are headings broken on purpose, not wrapped text.
	p := &fakePage{page: 1, y: 842}
	p.add("I HOPE PEOPLE WOULD FIND NO FAULT", 20, 60, 530, 60)
	p.add("CONTEXT OF DANIEL AND KING", 20, 139, 456, 27)
	got := Freeform(p.lines, FreeformOptions{Box: testBox})
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("want one slide with two lines, got %v", slideTexts(got))
	}
}

func TestPack(t *testing.T) {
	sizes := []int{3, 3, 3, 3, 3}
	sum := func(from, to int) int {
		n := 0
		for _, s := range sizes[from:to] {
			n += s
		}
		return n
	}
	got := pack(len(sizes), 7, sum)
	// Greedy to 7 needs 3 chunks (6,6,3); the balanced result keeps 3
	// chunks with the smallest maximum: [3+3][3+3][3].
	if len(got) != 3 {
		t.Fatalf("chunks = %v", got)
	}
	for _, c := range got {
		if sum(c[0], c[1]) > 7 {
			t.Errorf("chunk %v over limit", c)
		}
	}
}
