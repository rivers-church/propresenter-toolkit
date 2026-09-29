package metrics

import "testing"

// Calibrated against a real slide made in ProPresenter (Arial Bold 135pt,
// 1920pt box): "If we were to consider" fits on one line but adding
// "CURRENT" wraps, and "CURRENT STATE of humanity," wraps after "of".
func TestLinesMatchProPresenter(t *testing.T) {
	box := Box{Width: 1920, FontSize: 135}
	for text, want := range map[string]int{
		"If we were to consider":         1,
		"If we were to consider CURRENT": 2,
		"CURRENT STATE of":               1,
		"CURRENT STATE of humanity,":     2,
		"":                               1,
	} {
		if got := box.Lines(text); got != want {
			t.Errorf("Lines(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestAllCapsIsWider(t *testing.T) {
	text := "the king went to his palace and spent the night fasting"
	plain := Box{Width: 1920, FontSize: 135}
	caps := Box{Width: 1920, FontSize: 135, AllCaps: true}
	if caps.Lines(text) <= plain.Lines(text) {
		t.Errorf("ALL CAPS should need more lines: %d vs %d", caps.Lines(text), plain.Lines(text))
	}
}
