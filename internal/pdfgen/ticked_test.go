package pdfgen

import (
	"bytes"
	"github.com/go-pdf/fpdf"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestCalendarQRIsPrintedOnlyWhenAskedFor(t *testing.T) {
	sq, fp := testSquadron(), testProgramme()
	compressOutput = false
	defer func() { compressOutput = true }()

	plain, err := Render(sq, fp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "Add this programme to your calendar") {
		t.Error("no QR code was asked for, but one was printed")
	}

	exp := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	with, err := RenderWith(sq, fp, Options{CalendarURL: "webcal://example.test/share/tok/calendar.ics", CalendarExpires: exp})
	if err != nil {
		t.Fatal(err)
	}
	out := string(with)
	for _, want := range []string{"Add this programme to your calendar", "This link works until 1 March 2030."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !strings.Contains(out, "/Subtype /Image") {
		t.Error("the QR code image is missing")
	}
	if n := strings.Count(out, "/Type /Page\n"); n != 1 {
		t.Errorf("%d pages, want 1", n)
	}
}

// heavyMonth is a month that cannot fit on one A4 page at normal size: two
// parts a night, long titles and a few notes.
func heavyMonth(nights, halves int, long bool, notes int) (*programme.Squadron, *programme.FullProgramme) {
	sq := testSquadron()
	sq.Classifications = []string{"Junior", "1st Class", "Leading", "Senior"}
	fp := &programme.FullProgramme{Programme: programme.Programme{ID: 1, Year: 2026, Month: 11}}
	for d := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC); d.Month() == 11; d = d.AddDate(0, 0, 1) {
		day := programme.Day{ID: int64(d.Day()), Date: d.Format("2006-01-02"), Halves: 1}
		if wd := int(d.Weekday()+6) % 7; wd < nights {
			day.IsParade, day.UniformKey, day.Halves = true, "green", halves
			txt := "Drill"
			if long {
				txt = "First Class - Initial Expedition training with a long title"
			}
			for h := 1; h <= halves; h++ {
				day.Rows = append(day.Rows,
					programme.Row{Half: h, RowType: "activity", Texts: []string{txt, txt + " b", txt + " c", txt + " d"}},
					programme.Row{Half: h, RowType: "instructor", Texts: []string{"Sgt A", "Sgt B", "Sgt C", "Sgt D"}})
			}
		}
		fp.Days = append(fp.Days, day)
	}
	for i := 0; i < notes; i++ {
		c := programme.ReqCategory{Name: "Category", DueLabel: "(TBC)"}
		for j := 0; j < 8; j++ {
			c.Entries = append(c.Entries, programme.ReqEntry{CadetName: "Example Cadet", Note: "1 x Exercise"})
		}
		fp.Requirements = append(fp.Requirements, c)
	}
	return sq, fp
}

func TestACrowdedMonthIsShrunkToFitOnePage(t *testing.T) {
	opts := Options{CalendarURL: "webcal://example.test/share/t/calendar.ics", CalendarExpires: time.Date(2026, 12, 6, 0, 0, 0, 0, time.UTC)}

	// A month that already fits is drawn at full size, exactly as before.
	sq, fp := heavyMonth(2, 1, false, 0)
	if _, scale, err := renderFit(sq, fp, opts); err != nil || scale != 1 {
		t.Fatalf("a light month: scale %v, err %v (want 1)", scale, err)
	}

	for name, c := range map[string]struct {
		nights, halves, notes int
		long                  bool
	}{
		"two nights, two parts, long titles, notes": {2, 2, 5, true},
		"three nights, two parts, long titles":      {3, 2, 0, true},
		"four nights, one part, notes":              {4, 1, 5, true},
	} {
		sq, fp := heavyMonth(c.nights, c.halves, c.long, c.notes)
		_, atFull, err := renderAt(sq, fp, opts, 1)
		if err != nil {
			t.Fatal(err)
		}
		if atFull+footerRoom+qrRoom <= pageHeight {
			t.Fatalf("%s: expected to overflow at full size, ends at %.0f mm", name, atFull)
		}
		data, scale, err := renderFit(sq, fp, opts)
		if err != nil {
			t.Fatal(err)
		}
		if scale <= 1 {
			t.Errorf("%s: not shrunk (scale %v)", name, scale)
		}
		_, end, _ := renderAt(sq, fp, opts, scale)
		if need := end + (footerRoom+qrRoom)*scale; need > pageHeight*scale+0.5 && scale < maxShrink {
			t.Errorf("%s: still runs %.1f mm past the page at scale %.2f", name, need-pageHeight*scale, scale)
		}
		if !bytes.Contains(data, []byte("/Count 1")) {
			t.Errorf("%s: not a single page", name)
		}
	}
}

// textY finds where a piece of text was drawn on the page (points up from the
// bottom), in a PDF that was made with compression off.
func textY(t *testing.T, pdf, text string) float64 {
	t.Helper()
	m := regexp.MustCompile(`([0-9.]+) ([0-9.]+) Td \(` + regexp.QuoteMeta(text) + `\)Tj`).FindStringSubmatch(pdf)
	if m == nil {
		t.Fatalf("%q was not drawn", text)
	}
	y, _ := strconv.ParseFloat(m[2], 64)
	return y
}

// A closure early in the month used to leave the cursor high on the page, so the
// uniform key and the notes table were drawn on top of the nights below it.
func TestTheKeyAndNotesAreBelowTheWholeGridEvenWithAnEarlyClosure(t *testing.T) {
	compressOutput = false
	defer func() { compressOutput = true }()
	sq, fp := heavyMonth(2, 1, false, 3)
	fp.Closures = []programme.Closure{{StartDate: "2026-11-09", EndDate: "2026-11-12", Message: "STAND DOWN"}}
	data, err := Render(sq, fp)
	if err != nil {
		t.Fatal(err)
	}
	pdf := string(data)
	lastNight := textY(t, pdf, "30") // the last row of the grid
	for _, below := range []string{"Uniform Key", "NOTES"} {
		if y := textY(t, pdf, below); y >= lastNight {
			t.Errorf("%q is drawn at %.0f, level with or above the last night (%.0f): it overlaps the grid", below, y, lastNight)
		}
	}
	// And with no closure the key is still the same distance below the grid.
	fp.Closures = nil
	plain, _ := Render(sq, fp)
	if y := textY(t, string(plain), "Uniform Key"); y >= textY(t, string(plain), "30") {
		t.Error("the key overlaps the grid without a closure too")
	}
}
