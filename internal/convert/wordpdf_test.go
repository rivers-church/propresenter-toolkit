package convert

import (
	"os"
	"strings"
	"testing"

	"github.com/rivers-church/propresenter-toolkit/internal/style"
)

// A Microsoft Print to PDF export: CID fonts with no glyph widths, so the
// text is only recoverable in content-stream order.
func TestParseWordPrintToPDFNotes(t *testing.T) {
	pdf, err := os.ReadFile("testdata/word-print-to-pdf-notes.pdf")
	if err != nil {
		// Real sermon PDFs are git-ignored; drop one at this path to run it.
		t.Skipf("sample PDF not present: %v", err)
	}
	job, err := Parse(pdf, style.KindSlides, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Unrecognized) != 0 {
		t.Errorf("unrecognized lines: %q", job.Unrecognized)
	}
	rows := job.Rows()
	if len(rows) != 19 {
		t.Fatalf("got %d rows, want 19: %+v", len(rows), rows)
	}
	if rows[1].Kind != "Title" || rows[1].Text != "The journey is too much for you" {
		t.Errorf("title row = %+v", rows[1])
	}
	for _, r := range rows {
		if strings.ContainsRune(r.Text, '�') {
			t.Errorf("row %+v contains a replacement character", r)
		}
	}
}
