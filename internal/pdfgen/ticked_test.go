package pdfgen

import (
	"bytes"
	"github.com/go-pdf/fpdf"
	"strings"
	"testing"

	"rafactools-planner/internal/programme"
)

// renderText renders with compression off, so the text drawn on the page can be
// found in the file.
func renderText(t *testing.T, sq *programme.Squadron, fp *programme.FullProgramme) string {
	t.Helper()
	compressOutput = false
	defer func() { compressOutput = true }()
	return string(render(t, sq, fp))
}

func dayCell(date string) string { return "(" + dateDay(date) + ")" }

func TestOnlyTickedNightsGetARow(t *testing.T) {
	fp := testProgramme() // Mondays are the parade nights
	weeks := groupIntoWeeks(fp.Days)
	rows := 0
	for _, w := range weeks {
		for _, d := range w.rows {
			if !d.IsParade {
				t.Errorf("an unticked night %s has a row", d.Date)
			}
			rows++
		}
		if len(w.days) == 0 {
			t.Error("a week lost its days (the weekend dates come from them)")
		}
	}
	if rows != 4 { // 4, 11, 18, 25 February 2030
		t.Errorf("%d rows, want the 4 Mondays", rows)
	}

	text := renderText(t, testSquadron(), fp)
	for _, d := range []string{"2030-02-04", "2030-02-11", "2030-02-18", "2030-02-25"} {
		if !strings.Contains(text, dayCell(d)) {
			t.Errorf("ticked night %s is not in the PDF", d)
		}
	}
	for _, d := range []string{"2030-02-05", "2030-02-12", "2030-02-19", "2030-02-27", "2030-02-28"} {
		if strings.Contains(text, dayCell(d)) {
			t.Errorf("unticked day %s is in the PDF", d)
		}
	}
}

func TestUntickingANightRemovesItsRow(t *testing.T) {
	fp := testProgramme()
	for i := range fp.Days {
		if fp.Days[i].Date == "2030-02-11" {
			fp.Days[i].IsParade = false
		}
	}
	if strings.Contains(renderText(t, testSquadron(), fp), dayCell("2030-02-11")) {
		t.Error("an unticked night is still drawn")
	}
}

func TestAClosureAcrossSeveralNightsIsOneCell(t *testing.T) {
	fp := testProgramme()
	fp.Closures = []programme.Closure{{ID: 7, StartDate: "2030-02-04", EndDate: "2030-02-18", Message: "CLOSED FOR HALF TERM"}}
	text := renderText(t, testSquadron(), fp)

	if n := strings.Count(text, "(CLOSED FOR HALF TERM)"); n != 1 {
		t.Errorf("the message is drawn %d times, want once (one merged cell)", n)
	}
	// All three closed nights keep their own date and day cells.
	for _, d := range []string{"2030-02-04", "2030-02-11", "2030-02-18"} {
		if !strings.Contains(text, dayCell(d)) {
			t.Errorf("closed night %s lost its date", d)
		}
	}
	// The night after the closure is an ordinary one again.
	if !strings.Contains(text, dayCell("2030-02-25")) {
		t.Error("the night after the closure is missing")
	}
}

func TestClosureBlocksShareOneHeightAndSpan(t *testing.T) {
	pdf := newTestPDF(t)
	cols := newColumns(194, 4)
	fp := testProgramme()
	c := programme.Closure{ID: 3, StartDate: "2030-02-04", EndDate: "2030-02-18", Message: "Closed"}
	byDate := map[string]programme.Closure{"2030-02-04": c, "2030-02-11": c, "2030-02-18": c}
	cb := newClosureBlocks(pdf, cols, groupIntoWeeks(fp.Days), byDate)

	if cb.rows[3] != 3 {
		t.Errorf("rows = %d, want 3", cb.rows[3])
	}
	if h := cb.rowHeight(c); h < lineH {
		t.Errorf("row height %.2f is thinner than a line of text", h)
	}
	cb.note(c, 10, 20)
	cb.note(c, 20, 30)
	cb.note(c, 30, 40)
	if s := cb.spans[3]; s.y0 != 10 || s.y1 != 40 {
		t.Errorf("span = %+v, want 10 to 40", *s)
	}

	// A long message makes the rows taller, shared between them.
	long := programme.Closure{ID: 4, Message: strings.Repeat("closed for refurbishment of the hall ", 12)}
	cb2 := newClosureBlocks(pdf, cols, groupIntoWeeks(fp.Days), map[string]programme.Closure{"2030-02-04": long, "2030-02-11": long})
	if cb2.rowHeight(long)*2 < cb2.needed[4]-0.001 {
		t.Errorf("two rows of %.2f cannot hold a message needing %.2f", cb2.rowHeight(long), cb2.needed[4])
	}
}

func TestWeekendNotesSurviveAWeekWithNoTickedNights(t *testing.T) {
	fp := testProgramme()
	for i := range fp.Days {
		if fp.Days[i].Date == "2030-02-25" {
			fp.Days[i].IsParade = false // the last week now has no row
		}
	}
	fp.WeekendNotes = []programme.WeekendNote{{Date: "2030-02-02", Text: "Camp"}, {Date: "2030-02-26", Text: "Tuesday note is not a weekend"}}
	text := renderText(t, testSquadron(), fp)
	if !strings.Contains(text, "(Camp)") {
		t.Error("a weekend note in a week with no ticked night was lost")
	}

	// A week with no ticked night and no weekend notes draws nothing at all.
	empty := testProgramme()
	for i := range empty.Days {
		if empty.Days[i].Date == "2030-02-25" {
			empty.Days[i].IsParade = false
		}
	}
	withGap := renderText(t, testSquadron(), empty)
	if bytes.Count([]byte(withGap), []byte("(SAT)")) > 4 {
		t.Errorf("an empty week still draws a weekend block: %d SAT cells", bytes.Count([]byte(withGap), []byte("(SAT)")))
	}
}

func newTestPDF(t *testing.T) *fpdf.Fpdf {
	t.Helper()
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	return pdf
}
