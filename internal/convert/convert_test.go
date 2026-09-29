package convert

import (
	"strings"
	"testing"

	"github.com/rivers-church/propresenter-toolkit/internal/pro"
	"github.com/rivers-church/propresenter-toolkit/internal/style"
	"github.com/rivers-church/propresenter-toolkit/internal/testpdf"
)

func messageStyle(t *testing.T) *style.Profile {
	t.Helper()
	for _, data := range style.Defaults() {
		p, err := style.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if p.Name == "Message (Prompts)" {
			return p
		}
	}
	t.Fatal("no Message (Prompts) style")
	return nil
}

// freeformPDF mimics the youth-service notes: black pages, coloured
// centred headings, italic scripture and a black-on-white callout.
func freeformPDF() []byte {
	red, yellow := [3]float64{1, 0.39, 0.31}, [3]float64{0.98, 0.89, 0.2}
	white := [3]float64{1, 1, 1}
	span := func(text string, c [3]float64) testpdf.Span { return testpdf.Span{Text: text, Color: c} }
	line := func(y float64, s ...testpdf.Span) testpdf.Placed {
		return testpdf.Placed{Spans: s, X: 297, Y: y, Size: 20, Center: true}
	}
	return testpdf.BuildDarkPages([][]testpdf.Placed{{
		{Spans: []testpdf.Span{span("No Exits", white)}, X: 490, Y: 50, Size: 12},
		line(120, span("WHAT GOD DOES IN OUR LIFE", white)),
		line(147, span("SPACE | TIME | OPPORTUNITY", red)),
		line(174, span("HE IS THE GOD OF PROCESS", yellow)),
		{Spans: []testpdf.Span{{Text: "STOP TRYING TO PRAY OUT OF IT"}}, X: 297, Y: 228, Size: 20, Center: true, Box: true},
		{Spans: []testpdf.Span{{Text: "Daniel 6:3 ESV ", Bold: true, Color: white}, {Text: "Then this Daniel became distinguished above", Italic: true, Color: white}}, X: 57, Y: 282, Size: 20},
		{Spans: []testpdf.Span{{Text: "all the other high officials and satraps.", Italic: true, Color: white}}, X: 57, Y: 309, Size: 20},
	}})
}

func rtfOf(t *testing.T, data []byte, cue int) string {
	t.Helper()
	p, err := pro.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return string(pro.SlideAction(p.GetCues()[cue]).GetSlide().GetPresentation().GetBaseSlide().GetElements()[0].GetElement().GetText().GetRtfData())
}

func TestFreeformPrompts(t *testing.T) {
	st := messageStyle(t)
	job, err := Parse(freeformPDF(), style.KindPrompts, Options{Style: st})
	if err != nil {
		t.Fatal(err)
	}
	if !job.Freeform || !job.PDFColors() {
		t.Fatalf("freeform=%v pdfColors=%v", job.Freeform, job.PDFColors())
	}
	rows := job.Rows()
	want := []string{
		"WHAT GOD DOES IN OUR LIFE SPACE | TIME | OPPORTUNITY HE IS THE GOD OF PROCESS",
		"STOP TRYING TO PRAY OUT OF IT",
		"Daniel 6:3 ESV Then this Daniel became distinguished above all the other high officials and satraps.",
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for i, w := range want {
		if rows[i].Text != w {
			t.Errorf("slide %d = %q, want %q", i+1, rows[i].Text, w)
		}
	}
	if len(job.Warnings()) == 0 || !strings.Contains(job.Warnings()[0], "paragraphs") {
		t.Errorf("expected a note about paragraph splitting, got %v", job.Warnings())
	}

	job.Name = "Youth"
	data, _, err := job.Build(st)
	if err != nil {
		t.Fatal(err)
	}
	headings := rtfOf(t, data, 0)
	for _, c := range []string{`\red255\green99\blue79`, `\red250\green227\blue51`} {
		if !strings.Contains(headings, c) {
			t.Errorf("heading slide missing PDF colour %s: %s", c, headings)
		}
	}
	if strings.Count(headings, `\par\pard`) != 2 {
		t.Errorf("heading lines should stay separate paragraphs: %s", headings)
	}
	// Colour 1 is opaque white (the regular colour), colour 2 black.
	callout := rtfOf(t, data, 1)
	if !strings.Contains(callout, `{\colortbl;\red255\green255\blue255;\red0\green0\blue0;`) ||
		!strings.Contains(callout, `\cf2\highlight1\cb1 STOP`) {
		t.Errorf("callout should be black on a white highlight: %s", callout)
	}
	scripture := rtfOf(t, data, 2)
	if !strings.Contains(scripture, `\i Then this Daniel`) {
		t.Errorf("scripture should be italic: %s", scripture)
	}
}

func TestFreeformWithStyleColors(t *testing.T) {
	st := messageStyle(t)
	job, err := Parse(freeformPDF(), style.KindPrompts, Options{Style: st, Colors: ColorsStyle})
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := job.Build(st)
	if err != nil {
		t.Fatal(err)
	}
	s := rtfOf(t, data, 0)
	if strings.Contains(s, `\red255\green99\blue79`) || strings.Contains(s, `\i `) {
		t.Errorf("style colours shouldn't carry PDF colours or italics: %s", s)
	}
}

func TestSlideMarkersStillWin(t *testing.T) {
	pdf := testpdf.Build([]testpdf.Line{
		{testpdf.Regular("SLIDE 1:")},
		{testpdf.Regular("Hello")},
	})
	job, err := Parse(pdf, style.KindPrompts, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if job.Freeform || job.PDFColors() || len(job.Prompts) != 1 {
		t.Errorf("freeform=%v pdfColors=%v slides=%d", job.Freeform, job.PDFColors(), len(job.Prompts))
	}
}
