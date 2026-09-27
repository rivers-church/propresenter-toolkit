package pro

import (
	"strings"
	"testing"

	"github.com/rivers-church/propresenter-toolkit/internal/parse"
	"github.com/rivers-church/propresenter-toolkit/internal/pb"
	"github.com/rivers-church/propresenter-toolkit/internal/pdftext"
	"github.com/rivers-church/propresenter-toolkit/internal/rtf"
	"github.com/rivers-church/propresenter-toolkit/internal/style"
)

func defaultStyle(t *testing.T, kind style.Kind) *style.Profile {
	t.Helper()
	for _, data := range style.Defaults() {
		p, err := style.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if p.Kind == kind {
			return p
		}
	}
	t.Fatalf("no default %s style", kind)
	return nil
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
	data, err := BuildPrompts(st, slides, "Test Message")
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
	data, rep, err := BuildSlides(st, entries, "Slides Test")
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
	if _, err := BuildPrompts(st, nil, "x"); err == nil {
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
	data, err := BuildPrompts(st, parse.Prompts(lines, ""), "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(data); err != nil {
		t.Fatal(err)
	}
}
