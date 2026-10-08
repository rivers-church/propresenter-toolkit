package parse

import (
	"regexp"
	"strings"
)

// Kind is the type of a parsed notes entry.
type Kind string

const (
	KindTitle     Kind = "title"
	KindPoint     Kind = "point"
	KindScripture Kind = "scripture"
	KindImage     Kind = "image"
	KindBackTo    Kind = "backto"
)

// Entry is one recognised item from a notes outline.
type Entry struct {
	Kind Kind

	// Title, Point, Image: the text. Point also has a Label ("Point 1",
	// "Condition 2", ...).
	Text  string
	Label string

	// Scripture
	Reference string // e.g. "Ephesians 2:1-10 NIV"
	Verse     string

	// Back to <Ref> when I say: <Trigger>
	Ref     string
	Trigger string
}

// Summary is a one-line description for review screens.
func (e Entry) Summary() (kind, text string) {
	switch e.Kind {
	case KindTitle:
		return "Title", e.Text
	case KindPoint:
		return e.Label, e.Text
	case KindScripture:
		return "Scripture", e.Reference + ": " + truncate(e.Verse, 100)
	case KindImage:
		return "Image (skipped)", e.Text
	case KindBackTo:
		return "Back to " + e.Ref, e.Trigger
	}
	return string(e.Kind), e.Text
}

var (
	scriptureRE = regexp.MustCompile(`^([1-3]?\s?[A-Za-z][A-Za-z ]*)\s+(\d+:\d+(?:-\d+)?)\s*\(?([A-Za-z]{2,6})?\)?$`)
	pointRE     = regexp.MustCompile(`(?i)^(SUB\s?POINT|POINT|CONDITION|DRIVEN)\s+(\d+)\s*(?::\s*(.*)|\s+(.+))$`)
	// "Scripture:" on a line of its own introduces a reference on the next
	// line (which may be a non-biblical source such as "African Proverb").
	scriptureMarkerRE = regexp.MustCompile(`(?i)^Scripture\s*:\s*$`)
	titleRE           = regexp.MustCompile(`(?i)^Title\s*:\s*(.+)$`)
	imageRE           = regexp.MustCompile(`(?i)^Image\s*:\s*(.+)$`)
	backToRE          = regexp.MustCompile(`(?i)^Back to (.+?) when I say:\s*(.+)$`)
	leadingVerseNumRE = regexp.MustCompile(`^\d{1,3}\s+`)
	verseNumRE        = regexp.MustCompile(`^\d{1,3}$`)
	spacesRE          = regexp.MustCompile(`\s+`)
)

func isLabelLine(s string) bool {
	return scriptureMarkerRE.MatchString(s) || pointRE.MatchString(s) || titleRE.MatchString(s) || imageRE.MatchString(s) ||
		backToRE.MatchString(s) || scriptureRE.MatchString(s)
}

// Notes parses a labelled outline. Lines that don't match any known label
// are returned as unrecognized rather than guessed at, so nothing silently
// goes missing.
func Notes(rawLines []string) (entries []Entry, unrecognized []string) {
	var lines []string
	for _, l := range rawLines {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	n := len(lines)
	i := 0
	for i < n && !isLabelLine(lines[i]) {
		i++ // skip the document heading before the first real label
	}

	// continuation gathers following non-label lines (a label's text can
	// wrap onto several lines), dropping stray verse numbers.
	continuation := func(start int) (string, int) {
		j := start
		var extra []string
		for j < n && !isLabelLine(lines[j]) {
			if !verseNumRE.MatchString(lines[j]) {
				extra = append(extra, lines[j])
			}
			j++
		}
		return strings.TrimSpace(strings.Join(extra, " ")), j
	}
	join := func(a, b string) string { return strings.TrimSpace(a + " " + b) }

	for i < n {
		line := lines[i]
		if m := titleRE.FindStringSubmatch(line); m != nil {
			extra, next := continuation(i + 1)
			entries = append(entries, Entry{Kind: KindTitle, Text: join(m[1], extra)})
			i = next
			continue
		}
		if m := imageRE.FindStringSubmatch(line); m != nil {
			extra, next := continuation(i + 1)
			entries = append(entries, Entry{Kind: KindImage, Text: join(m[1], extra)})
			i = next
			continue
		}
		if m := backToRE.FindStringSubmatch(line); m != nil {
			extra, next := continuation(i + 1)
			entries = append(entries, Entry{Kind: KindBackTo, Ref: strings.TrimSpace(m[1]), Trigger: join(m[2], extra)})
			i = next
			continue
		}
		if m := pointRE.FindStringSubmatch(line); m != nil {
			extra, next := continuation(i + 1)
			label := capitalize(strings.ReplaceAll(m[1], " ", "")) + " " + m[2]
			entries = append(entries, Entry{Kind: KindPoint, Label: label, Text: join(m[3]+m[4], extra)})
			i = next
			continue
		}
		if scriptureMarkerRE.MatchString(line) {
			if i+1 < n && !scriptureMarkerRE.MatchString(lines[i+1]) {
				i++
				line = lines[i]
				ref := spacesRE.ReplaceAllString(line, " ")
				if m := scriptureRE.FindStringSubmatch(line); m != nil {
					ref = scriptureRef(m)
				}
				verse, next := continuation(i + 1)
				verse = cleanVerse(verse)
				entries = append(entries, Entry{Kind: KindScripture, Reference: ref, Verse: verse})
				i = next
				continue
			}
			i++ // a "Scripture:" label with nothing after it
			continue
		}
		if m := scriptureRE.FindStringSubmatch(line); m != nil {
			ref := scriptureRef(m)
			verse, next := continuation(i + 1)
			verse = cleanVerse(verse)
			entries = append(entries, Entry{Kind: KindScripture, Reference: ref, Verse: verse})
			i = next
			continue
		}
		unrecognized = append(unrecognized, line)
		i++
	}
	return entries, unrecognized
}

// cleanVerse collapses whitespace and drops a leading verse number
// ("16 But Jesus..."), which isn't part of the text to display.
func cleanVerse(v string) string {
	v = strings.TrimSpace(spacesRE.ReplaceAllString(v, " "))
	return leadingVerseNumRE.ReplaceAllString(v, "")
}

func scriptureRef(m []string) string {
	ref := strings.TrimSpace(m[1]) + " " + m[2]
	if m[3] != "" {
		ref += " " + strings.ToUpper(m[3])
	}
	return ref
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToLower(s)
	return strings.ToUpper(s[:1]) + s[1:]
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
