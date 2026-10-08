// Package convert ties PDF extraction, parsing and .pro building together,
// so the web UI and the command line share one pipeline.
package convert

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/rivers-church/propresenter-toolkit/internal/metrics"
	"github.com/rivers-church/propresenter-toolkit/internal/parse"
	"github.com/rivers-church/propresenter-toolkit/internal/pdftext"
	"github.com/rivers-church/propresenter-toolkit/internal/pro"
	"github.com/rivers-church/propresenter-toolkit/internal/style"
)

// ColorMode chooses where Prompts text colours come from.
type ColorMode string

const (
	// ColorsAuto uses the PDF's colours for free-form notes and the style's
	// colours for SLIDE-marker scripts.
	ColorsAuto  ColorMode = "auto"
	ColorsStyle ColorMode = "style" // style's text colour; bold words highlighted
	ColorsPDF   ColorMode = "pdf"   // keep the PDF's own colours and italics
)

// Options tune parsing.
type Options struct {
	// Style is the style that will be used, so free-form notes can be split
	// to fit its text box. Optional.
	Style *style.Profile
	// MaxLines is the soft limit of lines per slide for free-form notes
	// (default parse.DefaultMaxLines).
	MaxLines int
	Colors   ColorMode
}

// Job is a parsed PDF waiting to be reviewed and turned into a .pro.
type Job struct {
	Mode          style.Kind
	StyleName     string
	Name          string
	Prompts       []parse.PromptSlide // Mode == prompts
	Freeform      bool                // prompts: no SLIDE markers, split by layout
	MaxLines      int                 // prompts: soft line limit used for freeform
	Colors        ColorMode           // prompts
	DisableCopies bool                // slides: mark those copies disabled
	DefaultCopies bool                // slides: also add Default-look copies of title/point slides
	Entries       []parse.Entry       // Mode == slides
	Unrecognized  []string            // slides: lines that matched no label
}

// Row is one line of the review table.
type Row struct {
	Kind string
	Text string
}

// defaultBox matches the Message template: Arial Bold 135pt in a
// 1920pt-wide box.
var defaultBox = metrics.Box{Width: 1920, FontSize: 135}

// Parse reads a PDF in the given mode.
func Parse(pdf []byte, mode style.Kind, opt Options) (*Job, error) {
	switch mode {
	case style.KindPrompts:
		lines, err := pdftext.ReadLines(bytes.NewReader(pdf), int64(len(pdf)), pdftext.Options{})
		if err != nil {
			return nil, err
		}
		job := &Job{Mode: mode, Colors: opt.Colors, MaxLines: opt.MaxLines}
		if job.Colors == "" {
			job.Colors = ColorsAuto
		}
		if job.MaxLines <= 0 {
			job.MaxLines = parse.DefaultMaxLines
		}
		job.Prompts = parse.Prompts(lines, parse.DefaultSlideMarker)
		if len(job.Prompts) == 0 {
			// No SLIDE markers: split by the document's own layout.
			box := defaultBox
			if opt.Style != nil {
				if b, err := pro.TextBox(opt.Style); err == nil && b.Width > 0 && b.FontSize > 0 {
					box = b
				}
			}
			job.Freeform = true
			job.Prompts = parse.Freeform(lines, parse.FreeformOptions{Box: box, MaxLines: job.MaxLines})
		}
		if len(job.Prompts) == 0 {
			return nil, fmt.Errorf("no text found in that PDF - is it a scanned image?")
		}
		return job, nil
	case style.KindSlides:
		lines, err := pdftext.ReadLines(bytes.NewReader(pdf), int64(len(pdf)), pdftext.Options{DropSuperscriptDigits: true})
		if err != nil {
			return nil, err
		}
		var text []string
		for _, l := range lines {
			text = append(text, l.Text())
		}
		entries, unrec := parse.Notes(text)
		if len(entries) == 0 {
			return nil, fmt.Errorf("no Title:/POINT N:/scripture labels found - is this a Notes PDF?")
		}
		return &Job{Mode: mode, Entries: entries, Unrecognized: unrec}, nil
	}
	return nil, fmt.Errorf("unknown mode %q", mode)
}

// Rows describes the job for review.
func (j *Job) Rows() []Row {
	var rows []Row
	if j.Mode == style.KindPrompts {
		for i, s := range j.Prompts {
			rows = append(rows, Row{Kind: fmt.Sprintf("Slide %d", i+1), Text: truncate(s.Text(), 160)})
		}
		return rows
	}
	for _, e := range j.Entries {
		k, t := e.Summary()
		rows = append(rows, Row{Kind: k, Text: t})
	}
	return rows
}

// Remove drops review row i.
func (j *Job) Remove(i int) {
	if j.Mode == style.KindPrompts && i >= 0 && i < len(j.Prompts) {
		j.Prompts = append(j.Prompts[:i], j.Prompts[i+1:]...)
	}
	if j.Mode == style.KindSlides && i >= 0 && i < len(j.Entries) {
		j.Entries = append(j.Entries[:i], j.Entries[i+1:]...)
	}
}

// PDFColors reports whether Prompts slides keep the PDF's own colours.
func (j *Job) PDFColors() bool {
	return j.Colors == ColorsPDF || (j.Colors != ColorsStyle && j.Freeform)
}

// Warnings lists what the user should check before generating.
func (j *Job) Warnings() []string {
	var w []string
	if j.Freeform {
		w = append(w, fmt.Sprintf("No “SLIDE 1:” markers found, so slides were split using the PDF's own paragraphs "+
			"(each block of lines is a slide, long ones split to about %d lines). Remove or tidy any you don't want.", j.MaxLines))
	}
	if len(j.Unrecognized) > 0 {
		w = append(w, "Lines not recognised (skipped): "+strings.Join(j.Unrecognized, "; "))
	}
	if j.Mode == style.KindSlides {
		// Dry-run the "Back to" and image logic so problems show up here.
		seen := map[string]bool{"title": false}
		var images, unresolved []string
		for _, e := range j.Entries {
			switch e.Kind {
			case parse.KindTitle:
				seen["title"] = true
			case parse.KindPoint:
				seen[strings.ToLower(e.Label)] = true
			case parse.KindScripture:
				seen[strings.ToLower(e.Reference)] = true
			case parse.KindImage:
				images = append(images, e.Text)
			case parse.KindBackTo:
				if !seen[strings.ToLower(e.Ref)] {
					unresolved = append(unresolved, "Back to "+e.Ref)
				}
			}
		}
		w = append(w, pro.Report{SkippedImages: images, UnresolvedBackTo: unresolved}.Warnings()...)
	}
	return w
}

// Build produces the .pro file.
func (j *Job) Build(st *style.Profile) ([]byte, pro.Report, error) {
	if st.Kind != j.Mode {
		return nil, pro.Report{}, fmt.Errorf("style %q is a %s style, but this is a %s conversion", st.Name, st.Kind, j.Mode)
	}
	if missing := st.Missing(); len(missing) > 0 {
		return nil, pro.Report{}, fmt.Errorf("style %q is missing templates: %s", st.Name, strings.Join(missing, ", "))
	}
	if j.Mode == style.KindPrompts {
		data, err := pro.BuildPrompts(st, j.Prompts, j.Name, pro.PromptOptions{PDFColors: j.PDFColors()})
		return data, pro.Report{}, err
	}
	return pro.BuildSlides(st, j.Entries, j.Name, pro.SlideOptions{DefaultCopies: j.DefaultCopies, DisableCopies: j.DisableCopies})
}

// FileName turns a presentation name into a safe .pro file name.
func FileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Presentation"
	}
	r := strings.NewReplacer("/", "-", `\`, "-", ":", "-", "*", "", "?", "", `"`, "", "<", "", ">", "", "|", "-")
	return r.Replace(name) + ".pro"
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
