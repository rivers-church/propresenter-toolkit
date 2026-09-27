package parse

import (
	"reflect"
	"strings"
	"testing"

	"github.com/rivers-church/propresenter-toolkit/internal/pdftext"
)

func line(words string, bold ...string) pdftext.Line {
	isBold := map[string]bool{}
	for _, b := range bold {
		isBold[b] = true
	}
	var runs []pdftext.Run
	for i, w := range strings.Fields(words) {
		runs = append(runs, pdftext.Run{Text: w, Bold: isBold[w], SpaceBefore: i > 0})
	}
	return pdftext.Line{Runs: runs}
}

func TestPrompts(t *testing.T) {
	lines := []pdftext.Line{
		line("THE SERMON TITLE"), // heading before the first marker: ignored
		line("SLIDE 1:"),
		line("We are SAVED", "SAVED"),
		line("by grace."),
		line("SLIDE 2:"), // marker with no content: skipped
		line("SLIDE 3"),  // missing colon still counts
		line("Hello SLIDE 4: world"),
	}
	got := Prompts(lines, "")
	if len(got) != 3 {
		t.Fatalf("got %d slides, want 3: %v", len(got), got)
	}
	if s := got[0].Text(); s != "We are SAVED by grace." {
		t.Errorf("slide 1 = %q", s)
	}
	if !got[0][0][2].Bold || got[0][0][0].Bold {
		t.Errorf("bold flags wrong: %+v", got[0][0])
	}
	if s := got[1].Text(); s != "Hello" {
		t.Errorf("slide 3 = %q", s)
	}
	if s := got[2].Text(); s != "world" {
		t.Errorf("slide 4 = %q", s)
	}
	if got[2][0][0].SpaceBefore {
		t.Error("first run of a line should not have SpaceBefore")
	}
}

func TestNotes(t *testing.T) {
	lines := []string{
		"THE CONDITION, THE CURE,",
		"AND THE CALLING",
		"Ephesians 2:1-10 (NIV)",
		"As for you, you were dead in your transgressions",
		"2",
		"and sins.",
		"Title: The Condition, The Cure,",
		"and The Calling",
		"POINT 1: The Condition",
		"CONDITION 1: Dead",
		"Back to Point 1 when I say: What does Paul mean",
		"when he says we're spiritually dead?",
		"DRIVEN 2: The Devil",
		"Image: 17-foot-tall sculpture",
		"1 John 1:9 NLT",
		"If we confess our sins...",
		"Back to Point 3 when I say: You are HIS handiwork",
	}
	entries, unrec := Notes(lines)
	want := []Entry{
		{Kind: KindScripture, Reference: "Ephesians 2:1-10 NIV", Verse: "As for you, you were dead in your transgressions and sins."},
		{Kind: KindTitle, Text: "The Condition, The Cure, and The Calling"},
		{Kind: KindPoint, Label: "Point 1", Text: "The Condition"},
		{Kind: KindPoint, Label: "Condition 1", Text: "Dead"},
		{Kind: KindBackTo, Ref: "Point 1", Trigger: "What does Paul mean when he says we're spiritually dead?"},
		{Kind: KindPoint, Label: "Driven 2", Text: "The Devil"},
		{Kind: KindImage, Text: "17-foot-tall sculpture"},
		{Kind: KindScripture, Reference: "1 John 1:9 NLT", Verse: "If we confess our sins..."},
		{Kind: KindBackTo, Ref: "Point 3", Trigger: "You are HIS handiwork"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries mismatch\n got: %+v\nwant: %+v", entries, want)
	}
	if len(unrec) != 0 {
		t.Errorf("unrecognized = %v", unrec)
	}
}

func TestNotesUnrecognized(t *testing.T) {
	entries, unrec := Notes([]string{"POINT 1: A", "some text", "", "Title: X"})
	// Text after a label is a continuation, not unrecognized.
	if len(entries) != 2 || entries[0].Text != "A some text" {
		t.Errorf("entries = %+v", entries)
	}
	if len(unrec) != 0 {
		t.Errorf("unrecognized = %v", unrec)
	}
}
