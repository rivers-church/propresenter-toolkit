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

func TestNotesLenientLabels(t *testing.T) {
	entries, unrec := Notes([]string{
		"Title: Run",
		"Point 1 Slow down",
		"subPoint 1: SLOW DOWN",
		"Subpoint 2:",
		"WITH People",
		"Scripture:",
		"African Proverb",
		"“Go far, go together”",
		"Scripture:",
		"Ephesians 6:16",
		"16 Take up the shield",
	})
	if len(unrec) != 0 {
		t.Errorf("unrecognized = %q", unrec)
	}
	want := []Entry{
		{Kind: KindTitle, Text: "Run"},
		{Kind: KindPoint, Label: "Point 1", Text: "Slow down"},
		{Kind: KindPoint, Label: "Subpoint 1", Text: "SLOW DOWN"},
		{Kind: KindPoint, Label: "Subpoint 2", Text: "WITH People"},
		{Kind: KindScripture, Reference: "African Proverb", Verse: "“Go far, go together”"},
		{Kind: KindScripture, Reference: "Ephesians 6:16", Verse: "Take up the shield"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries %+v, want %d", len(entries), entries, len(want))
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, entries[i], want[i])
		}
	}
}

// Formats from "The Momentum Effect" notes: comma verse lists, "says:",
// short Back-to forms and other ALL-CAPS labels.
func TestNotesLooserFormats(t *testing.T) {
	lines := []string{
		"THE MOMENTUM EFFECT",
		"1 Samuel 17:1-5,8-16, 24-26 (NIV)",
		"Now the Philistines gathered.",
		"Title: The Momentum Effect",
		"POINT 1: Momentum makes hard things easier",
		"2 Samuel 3:1 (HCSB) says:",
		"The war between the house of Saul",
		"Back to Point 1",
		"TRUTH: Sin is a momentum killer",
		"Back to Point 1: “Not only that”",
		"Back to Title: “David’s life serves as a powerful",
		"reminder”",
		"So they went back to Jerusalem.",
		"Back to Title",
	}
	entries, unrec := Notes(lines)
	want := []Entry{
		{Kind: KindScripture, Reference: "1 Samuel 17:1-5,8-16,24-26 NIV", Verse: "Now the Philistines gathered."},
		{Kind: KindTitle, Text: "The Momentum Effect"},
		{Kind: KindPoint, Label: "Point 1", Text: "Momentum makes hard things easier"},
		{Kind: KindScripture, Reference: "2 Samuel 3:1 HCSB", Verse: "The war between the house of Saul"},
		{Kind: KindBackTo, Ref: "Point 1"},
		{Kind: KindPoint, Label: "Truth", Text: "Sin is a momentum killer"},
		{Kind: KindBackTo, Ref: "Point 1", Trigger: "Not only that"},
		// A sentence containing "back to" is continuation text, not a label.
		{Kind: KindBackTo, Ref: "Title", Trigger: "David’s life serves as a powerful reminder” So they went back to Jerusalem."},
		{Kind: KindBackTo, Ref: "Title"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries mismatch\n got: %+v\nwant: %+v", entries, want)
	}
	if len(unrec) != 0 {
		t.Errorf("unrecognized = %v", unrec)
	}
}
