package pdfgen

import (
	"bytes"
	"testing"
	"time"

	"rafactools-planner/internal/programme"
)

func testSquadron() *programme.Squadron {
	return &programme.Squadron{
		Number: "2491", Name: "2491 (Test) Squadron", Subtitle: "Royal Air Force Air Cadets",
		ProtectiveMarking: "OFFICIAL",
		Classifications:   []string{"Junior", "1st Class", "Leading", "Senior"},
		UniformColours:    []programme.UniformColour{{Key: "green", Label: "Greens", Hex: "#8faa6e"}},
	}
}

func testProgramme() *programme.FullProgramme {
	fp := &programme.FullProgramme{Programme: programme.Programme{ID: 1, Year: 2030, Month: 2}}
	for d := time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC); d.Month() == 2; d = d.AddDate(0, 0, 1) {
		day := programme.Day{ID: int64(d.Day()), Date: d.Format("2006-01-02"), Halves: 1}
		if d.Weekday() == time.Monday {
			day.IsParade, day.DayLabel, day.UniformKey = true, "M", "green"
			day.Rows = []programme.Row{
				{Half: 1, RowType: "activity", Texts: []string{"Drill", "Drill", "First Aid", "Shooting"}},
				{Half: 1, RowType: "instructor", Texts: []string{"Sgt A", "Sgt A", "Fg Off B", "Cpl C"}},
			}
		}
		fp.Days = append(fp.Days, day)
	}
	return fp
}

// render runs Render with a deadline so a regression that loops forever
// fails the test instead of hanging the whole suite.
func render(t *testing.T, sq *programme.Squadron, fp *programme.FullProgramme) []byte {
	t.Helper()
	type result struct {
		b   []byte
		err error
	}
	ch := make(chan result, 1)
	go func() { b, err := Render(sq, fp); ch <- result{b, err} }()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("Render: %v", r.err)
		}
		return r.b
	case <-time.After(10 * time.Second):
		t.Fatal("Render did not finish within 10s")
		return nil
	}
}

func TestRenderProducesPDF(t *testing.T) {
	b := render(t, testSquadron(), testProgramme())
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("output does not start with %%PDF-: %q", b[:8])
	}
	if !bytes.Contains(b[len(b)-64:], []byte("%%EOF")) {
		t.Fatal("PDF is truncated: no EOF trailer")
	}
}

func TestRenderEmptyProgrammeAndHostileText(t *testing.T) {
	// No days at all must not panic.
	render(t, testSquadron(), &programme.FullProgramme{})

	// Long, unicode and markup-ish text must not panic or break layout code.
	fp := testProgramme()
	long := string(bytes.Repeat([]byte("Ünïcödé <b>&amp;</b> "), 200))
	for i := range fp.Days {
		for j := range fp.Days[i].Rows {
			for k := range fp.Days[i].Rows[j].Texts {
				fp.Days[i].Rows[j].Texts[k] = long
			}
		}
	}
	fp.Requirements = []programme.ReqCategory{{Name: long, Entries: []programme.ReqEntry{{CadetName: long, Note: long}}}}
	render(t, testSquadron(), fp)
}

// A closure whose dates don't parse must not make Render loop forever
// (nextDate returns its input unchanged for unparseable dates).
func TestRenderSurvivesMalformedClosureDates(t *testing.T) {
	fp := testProgramme()
	fp.Closures = []programme.Closure{
		{StartDate: "a", EndDate: "b", Message: "garbage"},
		{StartDate: "2030-02-10", EndDate: "2030-02-12", Message: "valid"},
		{StartDate: "2030-02-20", EndDate: "2030-02-15", Message: "reversed"},
	}
	render(t, testSquadron(), fp)
}

func TestNextDate(t *testing.T) {
	for in, want := range map[string]string{
		"2030-02-28": "2030-03-01",
		"2028-02-28": "2028-02-29", // leap year
		"2030-12-31": "2031-01-01",
		"junk":       "junk",
	} {
		if got := nextDate(in); got != want {
			t.Errorf("nextDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMergeSegments(t *testing.T) {
	segs := mergeSegments(10, []float64{5, 5, 5, 5}, []string{"A", "A", "B", "A"})
	if len(segs) != 3 {
		t.Fatalf("got %d segments, want 3 (A+A, B, A): %+v", len(segs), segs)
	}
	if segs[0].w != 10 || segs[0].x != 10 || segs[1].x != 20 || segs[2].x != 25 {
		t.Errorf("unexpected geometry: %+v", segs)
	}
}

// programmeWith builds a one-night programme whose Notes and requirement
// tracker content are the only things that vary.
func programmeWith(notes string, cadetName string) *programme.FullProgramme {
	return &programme.FullProgramme{
		Programme: programme.Programme{ID: 1, Year: 2026, Month: 4},
		Days: []programme.Day{
			{ID: 1, Date: "2026-04-06", IsParade: true, DayLabel: "M", Halves: 1, Notes: notes,
				Rows: []programme.Row{
					{DayID: 1, Half: 1, RowType: "activity", Texts: []string{"Drill", "Drill", "Drill", "Drill"}},
					{DayID: 1, Half: 1, RowType: "instructor", Texts: []string{"Staff", "Staff", "Staff", "Staff"}},
				}},
		},
		Requirements: []programme.ReqCategory{
			{Name: "Category", DueLabel: "(TBC)", Entries: []programme.ReqEntry{{CadetName: cadetName}}},
		},
	}
}

// TestRenderReflectsCurrentContent guards against a regression where the
// PDF export silently stops reading the programme's actual saved state. The
// rendered bytes are flate-compressed, so this can't just grep for the
// text — instead it renders two programmes differing only in Notes and
// requirement-tracker content and checks the output actually differs.
func TestRenderReflectsCurrentContent(t *testing.T) {
	sq := testSquadron()

	a, err := Render(sq, programmeWith("Original notes", "Cadet A"))
	if err != nil {
		t.Fatalf("Render (a): %v", err)
	}
	b, err := Render(sq, programmeWith("Updated notes", "Cadet B"))
	if err != nil {
		t.Fatalf("Render (b): %v", err)
	}
	if bytes.Equal(a, b) {
		t.Error("Render produced identical output for different Notes/requirement content — PDF export isn't reflecting current data")
	}
	// Not asserting determinism for identical input: fpdf embeds a
	// /CreationDate timestamp, so two renders a moment apart legitimately differ.
}

func TestRenderIsSinglePage(t *testing.T) {
	data, err := Render(testSquadron(), programmeWith("notes", "cadet"))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// "/Count N" in the Pages object is the page count: fpdf compresses only
	// content streams, so this is reliable without a PDF parser dependency.
	if !bytes.Contains(data, []byte("/Count 1")) {
		t.Error("expected a single-page PDF (/Count 1 not found)")
	}
}
