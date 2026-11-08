package pro

import (
	"math"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/rivers-church/propresenter-toolkit/internal/parse"
	"github.com/rivers-church/propresenter-toolkit/internal/pb"
	"github.com/rivers-church/propresenter-toolkit/internal/pdftext"
	"github.com/rivers-church/propresenter-toolkit/internal/rtf"
	"github.com/rivers-church/propresenter-toolkit/internal/style"
)

func defaultStyle(t *testing.T, kind style.Kind) *style.Profile {
	t.Helper()
	if kind == style.KindPrompts {
		return namedStyle(t, "Message (Prompts)")
	}
	return namedStyle(t, "2026-09-27 (Slides)")
}

func namedStyle(t *testing.T, name string) *style.Profile {
	t.Helper()
	for _, data := range style.Defaults() {
		p, err := style.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no built-in style %q", name)
	return nil
}

func promptsRTF(t *testing.T, st *style.Profile) string {
	t.Helper()
	slides := []parse.PromptSlide{{{{Text: "We"}, {Text: "are", SpaceBefore: true}, {Text: "SAVED", Bold: true, SpaceBefore: true}}}}
	data, err := BuildPrompts(st, slides, "x", PromptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return string(SlideAction(p.GetCues()[0]).GetSlide().GetPresentation().GetBaseSlide().GetElements()[0].GetElement().GetText().GetRtfData())
}

func TestPromptsAllYellowAndPlain(t *testing.T) {
	for name, colour := range map[string]string{
		"Message (Prompts, all yellow)": `\red255\green255\blue0`,
		"Message (Prompts, plain)":      `\red255\green255\blue255`,
	} {
		s := promptsRTF(t, namedStyle(t, name))
		// One text colour plus the transparent highlight entry, and the bold
		// word is not given a different colour.
		if !strings.Contains(s, `{\colortbl;`+colour+`;\red255\green255\blue255;}`) {
			t.Errorf("%s: wrong colour table: %s", name, s)
		}
		if strings.Contains(s, `\cf2`) || !strings.Contains(s, `\cf1 We are SAVED`) {
			t.Errorf("%s: bold word should not be highlighted: %s", name, s)
		}
	}
}

// checkIdentities verifies every cue has a unique UUID and the cue group
// lists them in order - ProPresenter relies on both.
func checkIdentities(t *testing.T, p *pb.Presentation) {
	t.Helper()
	seen := map[string]bool{}
	ids := p.GetCueGroups()[0].GetCueIdentifiers()
	if len(ids) != len(p.GetCues()) {
		t.Fatalf("cue group has %d ids for %d cues", len(ids), len(p.GetCues()))
	}
	for i, c := range p.GetCues() {
		u := c.GetUuid().GetString_()
		if u == "" || seen[u] {
			t.Errorf("cue %d has empty or duplicate uuid %q", i, u)
		}
		seen[u] = true
		if ids[i].GetString_() != u {
			t.Errorf("cue group id %d = %q, want %q", i, ids[i].GetString_(), u)
		}
		for _, a := range c.GetActions() {
			if seen[a.GetUuid().GetString_()] {
				t.Errorf("cue %d reuses action uuid", i)
			}
			seen[a.GetUuid().GetString_()] = true
		}
	}
}

func TestBuildPrompts(t *testing.T) {
	st := defaultStyle(t, style.KindPrompts)
	slides := []parse.PromptSlide{
		{{{Text: "We"}, {Text: "are", SpaceBefore: true}, {Text: "SAVED", Bold: true, SpaceBefore: true}}},
		{{{Text: "Second"}}, {{Text: "slide"}}},
	}
	data, err := BuildPrompts(st, slides, "Test Message", PromptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.GetName() != "Test Message" || len(p.GetCues()) != 2 {
		t.Fatalf("name %q, %d cues", p.GetName(), len(p.GetCues()))
	}
	checkIdentities(t, p)

	cue := p.GetCues()[0]
	if cue.GetName() != "Slide 1" {
		t.Errorf("cue name = %q", cue.GetName())
	}
	el := SlideAction(cue).GetSlide().GetPresentation().GetBaseSlide().GetElements()[0].GetElement()
	if el.GetText().GetAttributes().GetCapitalization() != pb.Graphics_Text_Attributes_CAPITALIZATION_NONE {
		t.Error("forced caps should be cleared")
	}
	rtfData := string(el.GetText().GetRtfData())
	if !strings.Contains(rtfData, `\cf1 We are \cf2 SAVED`) {
		t.Errorf("bold word not highlighted: %s", rtfData)
	}
	if got := rtf.PlainText(el.GetText().GetRtfData()); got != "We are SAVED" {
		t.Errorf("text = %q", got)
	}
}

func TestBuildSlides(t *testing.T) {
	st := defaultStyle(t, style.KindSlides)
	entries := []parse.Entry{
		{Kind: parse.KindTitle, Text: "The Condition"},
		{Kind: parse.KindScripture, Reference: "Mark 7:21-23 NIV", Verse: "For it is from within..."},
		{Kind: parse.KindPoint, Label: "Point 1", Text: "The Condition"},
		{Kind: parse.KindPoint, Label: "Condition 1", Text: "Dead"},
		{Kind: parse.KindImage, Text: "A sculpture"},
		{Kind: parse.KindBackTo, Ref: "Point 1", Trigger: "What does Paul mean?"},
		{Kind: parse.KindBackTo, Ref: "Point 9", Trigger: "Nowhere"},
	}
	data, rep, err := BuildSlides(st, entries, "Slides Test", SlideOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	checkIdentities(t, p)

	type want struct{ name, text, look string }
	wants := []want{
		{"Title", "The Condition", st.Settings.AudienceLooks[style.RoleTitle].Name},
		{"Mark 7:21-23 NIV", "Mark 7:21-23 NIV | For it is from within...", st.Settings.AudienceLooks[style.RoleScripture].Name},
		{"Point 1", "The Condition", st.Settings.AudienceLooks[style.RolePoint].Name},
		{"Condition 1", "Dead", st.Settings.AudienceLooks[style.RoleKeyword].Name},
		{"What does Paul mean?", "The Condition", st.Settings.AudienceLooks[style.RoleBackTo].Name},
	}
	cues := ListCues(p)
	if len(cues) != len(wants) {
		t.Fatalf("got %d cues, want %d", len(cues), len(wants))
	}
	for i, w := range wants {
		c := cues[i]
		if c.Name != w.name || c.Label != w.name || c.Preview != w.text {
			t.Errorf("cue %d = %q / %q / %q, want %q / %q", i, c.Name, c.Label, c.Preview, w.name, w.text)
		}
		if c.Look == nil || c.Look.Name != w.look {
			t.Errorf("cue %d look = %+v, want %q", i, c.Look, w.look)
		}
		if n := len(p.GetCues()[i].GetActions()); n != 2 {
			t.Errorf("cue %d has %d actions, want slide + look", i, n)
		}
	}
	if len(rep.SkippedImages) != 1 || len(rep.UnresolvedBackTo) != 1 {
		t.Errorf("report = %+v", rep)
	}
}

func TestBuildPromptsWrongTemplate(t *testing.T) {
	st := &style.Profile{Name: "empty", Kind: style.KindPrompts, Templates: map[string][]byte{}}
	if _, err := BuildPrompts(st, nil, "x", PromptOptions{}); err == nil {
		t.Error("expected an error for a style with no body template")
	}
}

// Make sure the parse->build pipeline holds together on a real PDF.
func TestPromptsFromPDF(t *testing.T) {
	st := defaultStyle(t, style.KindPrompts)
	lines := []pdftext.Line{
		{Runs: []pdftext.Run{{Text: "SLIDE"}, {Text: "1:", SpaceBefore: true}}},
		{Runs: []pdftext.Run{{Text: "Hello"}}},
	}
	data, err := BuildPrompts(st, parse.Prompts(lines, ""), "x", PromptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(data); err != nil {
		t.Fatal(err)
	}
}

func TestBuildSlidesDefaultCopies(t *testing.T) {
	st := defaultStyle(t, style.KindSlides)
	entries := []parse.Entry{
		{Kind: parse.KindTitle, Text: "T"},
		{Kind: parse.KindScripture, Reference: "John 1:1 NIV", Verse: "In the beginning"},
		{Kind: parse.KindPoint, Label: "Point 1", Text: "P"},
		{Kind: parse.KindPoint, Label: "Subpoint 1", Text: "S"},
	}
	data, _, err := BuildSlides(st, entries, "x", SlideOptions{DefaultCopies: true, DisableCopies: true})
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	checkIdentities(t, p)

	def := st.Settings.AudienceLooks[style.RoleBackTo].Name
	point := st.Settings.AudienceLooks[style.RolePoint].Name
	type want struct{ name, text, look string }
	wants := []want{
		{"Title", "T", st.Settings.AudienceLooks[style.RoleTitle].Name},
		{"Title", "T", def},
		{"John 1:1 NIV", "John 1:1 NIV | In the beginning", st.Settings.AudienceLooks[style.RoleScripture].Name},
		{"Point 1", "P", point},
		{"Point 1", "P", def},
		{"Subpoint 1", "S", point}, // subpoints use the Point look and template
		{"Subpoint 1", "S", def},
	}
	cues := ListCues(p)
	if len(cues) != len(wants) {
		t.Fatalf("got %d cues, want %d", len(cues), len(wants))
	}
	for i, w := range wants {
		c := cues[i]
		if c.Name != w.name || c.Label != w.name || c.Preview != w.text || c.Look == nil || c.Look.Name != w.look {
			t.Errorf("cue %d = %q / %q / %q / %+v, want %+v", i, c.Name, c.Label, c.Preview, c.Look, w)
		}
		if got, want := p.GetCues()[i].GetIsEnabled(), w.look != def; got != want {
			t.Errorf("cue %d enabled = %v, want %v", i, got, want)
		}
	}
}

func TestBuildSlidesKeepsTemplateLabelColors(t *testing.T) {
	st := defaultStyle(t, style.KindSlides)
	green := &pb.Color{Red: 0, Green: 0.502, Blue: 0, Alpha: 1}
	cue, err := st.TemplateCue(style.RoleScripture)
	if err != nil {
		t.Fatal(err)
	}
	SlideAction(cue).Label.Color = green
	if st.Templates[style.RoleScripture], err = proto.Marshal(cue); err != nil {
		t.Fatal(err)
	}
	data, _, err := BuildSlides(st, []parse.Entry{
		{Kind: parse.KindTitle, Text: "T"},
		{Kind: parse.KindScripture, Reference: "John 1:1 NIV", Verse: "In the beginning"},
		{Kind: parse.KindBackTo, Ref: "John 1:1 NIV"},
		{Kind: parse.KindBackTo, Ref: "Title"},
	}, "Colors", SlideOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	// title (red from its template), scripture (green), and "Back to"
	// copies inheriting their source's colour.
	want := []*pb.Color{labelRed, green, green, labelRed}
	cues := p.GetCues()
	if len(cues) != len(want) {
		t.Fatalf("got %d cues, want %d", len(cues), len(want))
	}
	for i, c := range cues {
		if got := SlideAction(c).GetLabel().GetColor(); !proto.Equal(got, want[i]) {
			t.Errorf("cue %d label colour = %v, want %v", i, got, want[i])
		}
	}
}

func TestBuildSlidesCentersScripturesVertically(t *testing.T) {
	st := defaultStyle(t, style.KindSlides)
	short := "For God so loved the world."
	long := strings.Repeat("For God so loved the world that he gave his one and only Son. ", 6)
	build := func(center bool) *pb.Presentation {
		data, _, err := BuildSlides(st, []parse.Entry{
			{Kind: parse.KindScripture, Reference: "John 3:16 NIV", Verse: short},
			{Kind: parse.KindScripture, Reference: "John 3:16 NIV", Verse: long},
			{Kind: parse.KindScripture, Reference: "John 3", Verse: strings.Repeat(long, 8)},
		}, "V", SlideOptions{CenterScriptures: center})
		if err != nil {
			t.Fatal(err)
		}
		p, err := Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	geom := func(c *pb.Cue) (slideH, top, bottom, verseH float64) {
		bs := SlideAction(c).GetSlide().GetPresentation().GetBaseSlide()
		els := bs.GetElements()
		top, bottom = 1e9, -1e9
		for _, e := range els[:2] {
			b := e.GetElement().GetBounds()
			top = math.Min(top, b.GetOrigin().GetY())
			bottom = math.Max(bottom, b.GetOrigin().GetY()+b.GetSize().GetHeight())
		}
		return bs.GetSize().GetHeight(), top, bottom, els[1].GetElement().GetBounds().GetSize().GetHeight()
	}

	var heights []float64
	for i, c := range build(true).GetCues() {
		slideH, top, bottom, h := geom(c)
		if d := (top + bottom) / 2; math.Abs(d-slideH/2) > 0.5 {
			t.Errorf("cue %d: group centre %.1f, want %.1f", i, d, slideH/2)
		}
		heights = append(heights, h)
	}
	for i, c := range build(true).GetCues()[2:] {
		slideH, top, bottom, _ := geom(c)
		if top < -0.01 || bottom > slideH+0.01 {
			t.Errorf("overlong passage %d runs off the slide: %.1f..%.1f of %.1f", i, top, bottom, slideH)
		}
	}
	if heights[1] <= heights[0] {
		t.Errorf("a longer verse should get a taller box: %v", heights)
	}

	// Off by default: the template's own geometry is kept.
	_, _, _, h0 := geom(build(false).GetCues()[0])
	_, _, _, h1 := geom(build(false).GetCues()[1])
	if h0 != h1 {
		t.Errorf("without the option both boxes keep the template height, got %.1f and %.1f", h0, h1)
	}
}

func TestBuildSlidesSplitsLongScripture(t *testing.T) {
	st := defaultStyle(t, style.KindSlides)
	box, err := scriptureBox(st)
	if err != nil {
		t.Fatal(err)
	}
	verse := strings.Repeat("Jesus took the loaves and gave thanks to God for the food; ", 12) + "They all ate."
	data, _, err := BuildSlides(st, []parse.Entry{
		{Kind: parse.KindScripture, Reference: "John 6:1-13 NIV", Verse: verse},
		{Kind: parse.KindBackTo, Ref: "John 6:1-13 NIV", Trigger: "again"},
	}, "Split", SlideOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	cues := p.GetCues()
	if len(cues) < 3 {
		t.Fatalf("expected a split passage plus a Back to, got %d cues", len(cues))
	}
	var parts []string
	for _, c := range cues[:len(cues)-1] {
		els := SlideAction(c).GetSlide().GetPresentation().GetBaseSlide().GetElements()
		if ref := rtf.PlainText(els[0].GetElement().GetText().GetRtfData()); ref != "John 6:1-13 NIV" {
			t.Errorf("each slide should carry the reference, got %q", ref)
		}
		text := rtf.PlainText(els[1].GetElement().GetText().GetRtfData())
		if n := box.Lines(text); n > 5 {
			t.Errorf("slide has %d lines", n)
		}
		parts = append(parts, text)
	}
	if got := strings.Join(parts, " "); got != strings.Join(strings.Fields(verse), " ") {
		t.Errorf("split changed the text: %q", got)
	}
	// "Back to" repeats the first part.
	last := cues[len(cues)-1]
	els := SlideAction(last).GetSlide().GetPresentation().GetBaseSlide().GetElements()
	if got := rtf.PlainText(els[1].GetElement().GetText().GetRtfData()); got != parts[0] {
		t.Errorf("Back to should copy the first slide: %q", got)
	}
}
