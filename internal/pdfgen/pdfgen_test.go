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
