package programme

import (
	"fmt"
	"strings"
	"testing"
)

func TestDescribeChanges(t *testing.T) {
	classes := []string{"First Class", "Leading"}
	uniform := map[string]string{"blue": "Blue", "wd": "Working dress"}
	day := func(date string, parade bool, uni string, texts ...string) snapDay {
		return snapDay{Date: date, IsParade: parade, UniformKey: uni, Halves: 1, Rows: []snapRow{
			{Half: 1, RowType: "activity", Texts: texts}, {Half: 1, RowType: "instructor", Texts: []string{"", ""}}}}
	}
	before := snapshot{
		Days:         []snapDay{day("2030-11-17", true, "blue", "Drill", ""), day("2030-11-24", false, "")},
		Closures:     []snapClosure{{Start: "2030-12-22", End: "2030-12-26", Message: "Christmas"}},
		WeekendNotes: []snapWeekend{{Date: "2030-11-23", Text: "Camp"}},
		Requirements: []snapCategory{{Name: "Fitness", Entries: []snapEntry{{Cadet: "Cdt Smith", Note: "x"}}}},
	}
	after := snapshot{
		Days:         []snapDay{day("2030-11-17", true, "wd", "Shooting", "Map reading"), day("2030-11-24", true, "")},
		Closures:     nil,
		WeekendNotes: []snapWeekend{{Date: "2030-11-23", Text: "Camp"}, {Date: "2030-11-30", Text: "Trip"}},
		Requirements: []snapCategory{{Name: "Fitness", Entries: []snapEntry{{Cadet: "Cdt Jones", Note: "y"}, {Cadet: "Cdt Smith", Note: "x"}}}, {Name: "Briefings"}},
	}
	after.Days[0].Notes = "bring a pen"
	got := strings.Join(describeChanges(before, after, uniform, classes), "\n")
	for _, want := range []string{
		"Changed uniform for 17 Nov from Blue to Working dress",
		"Changed activity on 17 Nov (First Class) from “Drill” to “Shooting”",
		"Set activity on 17 Nov (Leading) to “Map reading”",
		"Changed the notes on 17 Nov",
		"Made 24 Nov a parade night",
		"Removed closure 22 Dec to 26 Dec",
		"Added weekend note for 30 Nov “Trip”",
		"Changed the entries in “Fitness” (1 added, 0 removed)",
		"Added the tracker category “Briefings”",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Smith") || strings.Contains(got, "Jones") {
		t.Errorf("a cadet's name is in the history:\n%s", got)
	}
	if lines := describeChanges(before, before, uniform, classes); len(lines) != 0 {
		t.Errorf("no change reported as %v", lines)
	}
	var many []string
	for i := 0; i < 12; i++ {
		many = append(many, fmt.Sprintf("line %d", i))
	}
	if c := capLines(many); len(c) != maxChangeLines+1 || c[maxChangeLines] != "and 4 more changes" {
		t.Errorf("capLines = %v", c)
	}
}

func TestHistoryChangesFromKeptVersions(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	fp, _ := st.GetFullProgramme(1, p.ID)
	if _, _, err := st.RecordVersion(1, p.ID, 1, "o@x", "Starting point"); err != nil {
		t.Fatal(err)
	}
	var d Day
	for _, x := range fp.Days {
		if x.IsParade {
			d = x
			break
		}
	}
	act, _ := d.RowByHalfType(1, "activity")
	if err := st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: d.ID, IsParade: true, Rows: []RowInput{{ID: act.ID, Texts: []string{"Drill", "", "", ""}}}}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.RecordVersion(1, p.ID, 1, "o@x", "Saved"); err != nil {
		t.Fatal(err)
	}
	got, err := st.HistoryChanges(1, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[1]) != 0 || len(got[2]) != 1 || !strings.Contains(got[2][0], "“Drill”") {
		t.Errorf("changes = %v", got)
	}
	if _, err := st.HistoryChanges(2, p.ID); err == nil {
		t.Error("another squadron read the history")
	}
}
