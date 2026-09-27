package pdftext

import (
	"bytes"
	"testing"

	"github.com/rivers-church/propresenter-toolkit/internal/testpdf"
)

func TestReadLines(t *testing.T) {
	pdf := testpdf.Build([]testpdf.Line{
		{testpdf.Regular("SLIDE 1:")},
		{testpdf.Regular("We are "), testpdf.Bold("SAVED"), testpdf.Regular(" by grace.")},
	})
	lines, err := ReadLines(bytes.NewReader(pdf), int64(len(pdf)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %+v", len(lines), lines)
	}
	if got := lines[0].Text(); got != "SLIDE 1:" {
		t.Errorf("line 0 = %q", got)
	}
	want := []Run{
		{Text: "We"}, {Text: "are", SpaceBefore: true},
		{Text: "SAVED", Bold: true, SpaceBefore: true},
		{Text: "by", SpaceBefore: true}, {Text: "grace.", SpaceBefore: true},
	}
	got := lines[1].Runs
	if len(got) != len(want) {
		t.Fatalf("runs = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestIsBoldFont(t *testing.T) {
	for name, want := range map[string]bool{
		"AAAAAD+Avenir-Heavy":   true,
		"AAAAAB+Avenir-Black":   true,
		"Helvetica-Bold":        true,
		"AAAAAC+Avenir-Book":    false,
		"Avenir-BookOblique":    false,
		"AAAAAF+Avenir-HeavyOb": true,
	} {
		if got := IsBoldFont(name); got != want {
			t.Errorf("IsBoldFont(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestBuildRunsSplitsWeightChangeWithoutSpace(t *testing.T) {
	row := []glyph{
		{s: "h", font: "X-Heavy", x: 0, w: 5},
		{s: "i", font: "X-Heavy", x: 5, w: 5},
		{s: ",", font: "X-Book", x: 10, w: 3},
		{s: " ", font: "X-Book", x: 13, w: 3},
		{s: "y", font: "X-Book", x: 16, w: 5},
	}
	got := buildRuns(row)
	want := []Run{{Text: "hi", Bold: true}, {Text: ","}, {Text: "y", SpaceBefore: true}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDropSuperscriptDigits(t *testing.T) {
	row := []glyph{
		{s: "a", size: 23}, {s: "2", size: 14.4}, {s: "b", size: 23}, {s: "3", size: 23},
	}
	got := dropSuperscriptDigits(row)
	if len(got) != 3 || got[1].s != "b" || got[2].s != "3" {
		t.Errorf("got %+v", got)
	}
}
