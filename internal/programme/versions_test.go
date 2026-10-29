package programme

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func parade(fp *FullProgramme) Day {
	for _, d := range fp.Days {
		if d.IsParade {
			return d
		}
	}
	return Day{}
}

func TestVersionsAreKeptDedupedAndListedNewestFirst(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 8)

	v, made, err := st.RecordVersion(1, p.ID, 7, "a@example.org", "Created the month")
	if err != nil || !made || v != 1 {
		t.Fatalf("first: %d %v %v", v, made, err)
	}
	// Nothing changed: no new entry, and the existing number is returned.
	if v, made, _ := st.RecordVersion(1, p.ID, 7, "a@example.org", "Saved"); made || v != 1 {
		t.Errorf("an unchanged save made version %d (made=%v)", v, made)
	}
	fp, _ := st.GetFullProgramme(1, p.ID)
	d := parade(fp)
	st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: d.ID, IsParade: true, Notes: "bring water", Rows: []RowInput{{ID: d.Rows[0].ID, Texts: []string{"Drill", "", "", ""}}}}})
	if v, made, _ := st.RecordVersion(1, p.ID, 8, "b@example.org", "Saved the programme"); !made || v != 2 {
		t.Errorf("a changed save = version %d made=%v, want 2", v, made)
	}

	list, err := st.ListVersions(1, p.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %v, %v", list, err)
	}
	if list[0].Version != 2 || list[0].UserEmail != "b@example.org" || list[0].Action != "Saved the programme" || time.Since(list[0].At) > time.Minute || list[1].Version != 1 {
		t.Errorf("list = %+v", list)
	}
	if !st.HasVersions(p.ID) {
		t.Error("HasVersions false")
	}
	if _, err := st.ListVersions(2, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another squadron lists the history: %v", err)
	}
	if _, _, err := st.RecordVersion(2, p.ID, 1, "x", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("another squadron records a version: %v", err)
	}
}

func TestRestoreBringsBackEverythingTheMonthHolds(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 9)
	fp, _ := st.GetFullProgramme(1, p.ID)
	d := parade(fp)

	// State A: text, notes, uniform, a closure, a weekend note, a tracker entry, second half.
	st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: d.ID, IsParade: true, UniformKey: "green", Notes: "State A", Rows: []RowInput{
		{ID: d.Rows[0].ID, Texts: []string{"Activity A", "", "", ""}}, {ID: d.Rows[1].ID, Texts: []string{"Sgt A", "", "", ""}}}}})
	st.AddHalf(1, p.ID, d.ID, 4)
	st.AddClosure(p.ID, "2030-09-20", "2030-09-21", "Closed A")
	st.AddWeekendNote(p.ID, "2030-09-07", "Camp A")
	cat, _ := st.AddRequirementCategory(p.ID, "Fieldcraft", "by Oct")
	st.AddRequirementEntry(p.ID, cat, "Cdt A", "note A")
	vA, _, _ := st.RecordVersion(1, p.ID, 1, "a@example.org", "State A")

	// Then everything is changed (and the month reset).
	st.ResetProgramme(1, p.ID)
	st.AddClosure(p.ID, "2030-09-25", "2030-09-26", "Closed B")
	fp2, _ := st.GetFullProgramme(1, p.ID)
	d2 := parade(fp2)
	st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: d2.ID, IsParade: false, Notes: "State B"}})
	st.RecordVersion(1, p.ID, 2, "b@example.org", "State B")

	if err := st.RestoreVersion(1, p.ID, vA); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetFullProgramme(1, p.ID)
	var night Day
	for _, d := range got.Days {
		if d.Date == d2.Date {
			night = d
		}
	}
	act, _ := night.RowByHalfType(1, "activity")
	ins, _ := night.RowByHalfType(1, "instructor")
	if !night.IsParade || night.Notes != "State A" || night.UniformKey != "green" || night.Halves != 2 || act.Texts[0] != "Activity A" || ins.Texts[0] != "Sgt A" {
		t.Errorf("night after restore: %+v", night)
	}
	if len(got.Days) != len(fp.Days) {
		t.Errorf("the month has %d days, want %d", len(got.Days), len(fp.Days))
	}
	if len(got.Closures) != 1 || got.Closures[0].Message != "Closed A" || got.Closures[0].StartDate != "2030-09-20" {
		t.Errorf("closures = %+v", got.Closures)
	}
	if len(got.WeekendNotes) != 1 || got.WeekendNotes[0].Text != "Camp A" {
		t.Errorf("weekend notes = %+v", got.WeekendNotes)
	}
	if len(got.Requirements) != 1 || got.Requirements[0].Name != "Fieldcraft" || len(got.Requirements[0].Entries) != 1 || got.Requirements[0].Entries[0].CadetName != "Cdt A" {
		t.Errorf("requirements = %+v", got.Requirements)
	}
	// Nothing else about the month changed.
	if got.ID != p.ID || got.Year != 2030 || got.Month != 9 {
		t.Errorf("identity changed: %+v", got.Programme)
	}
}

func TestRestoreFitsTheBoxesToTheCurrentClassifications(t *testing.T) {
	st, d := newStore(t)
	p := mustProg(t, st, 1, 2030, 10)
	fp, _ := st.GetFullProgramme(1, p.ID)
	day := parade(fp)
	st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: day.ID, IsParade: true, Rows: []RowInput{{ID: day.Rows[0].ID, Texts: []string{"A", "B", "C", "D"}}}}})
	v, _, _ := st.RecordVersion(1, p.ID, 1, "a@example.org", "four boxes")

	// The squadron later has six classifications (existing rows are widened by that change).
	d.Exec(`UPDATE squadrons SET classifications = '["1","2","3","4","5","6"]' WHERE id = 1`)
	if err := st.RestoreVersion(1, p.ID, v); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetFullProgramme(1, p.ID)
	for _, n := range got.Days {
		for _, r := range n.Rows {
			if len(r.Texts) != 6 {
				t.Fatalf("a restored row has %d boxes, want 6", len(r.Texts))
			}
		}
	}
	night := parade(got)
	a, _ := night.RowByHalfType(1, "activity")
	if strings.Join(a.Texts, "|") != "A|B|C|D||" {
		t.Errorf("texts = %q", a.Texts)
	}
}

func TestRestoreRefusesUnknownVersionsAndOtherSquadrons(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 11)
	v, _, _ := st.RecordVersion(1, p.ID, 1, "a", "x")
	if err := st.RestoreVersion(1, p.ID, 99); !errors.Is(err, ErrVersionNotFound) {
		t.Errorf("unknown version = %v", err)
	}
	if err := st.RestoreVersion(2, p.ID, v); !errors.Is(err, ErrNotFound) {
		t.Errorf("another squadron = %v", err)
	}
	if _, err := st.GetVersionInfo(2, p.ID, v); !errors.Is(err, ErrNotFound) {
		t.Errorf("another squadron's info = %v", err)
	}
	if _, err := st.GetVersionInfo(1, p.ID, 99); !errors.Is(err, ErrVersionNotFound) {
		t.Errorf("unknown info = %v", err)
	}
}

func TestOnlyTheNewestVersionsAreKept(t *testing.T) {
	st, d := newStore(t)
	p := mustProg(t, st, 1, 2031, 1)
	for i := 0; i < maxVersionsKept+10; i++ {
		d.Exec(`UPDATE programme_days SET notes = ? WHERE id = (SELECT MIN(id) FROM programme_days WHERE programme_id = ?)`, strings.Repeat("n", i+1), p.ID)
		if _, made, err := st.RecordVersion(1, p.ID, 1, "a", "edit"); err != nil || !made {
			t.Fatalf("version %d: made=%v err=%v", i, made, err)
		}
	}
	list, _ := st.ListVersions(1, p.ID)
	if len(list) != maxVersionsKept || list[0].Version != maxVersionsKept+10 || list[len(list)-1].Version != 11 {
		t.Errorf("kept %d versions, newest %d oldest %d", len(list), list[0].Version, list[len(list)-1].Version)
	}
}

func TestVersionsGoWhenTheProgrammeGoes(t *testing.T) {
	st, d := newStore(t)
	p := mustProg(t, st, 1, 2031, 2)
	st.RecordVersion(1, p.ID, 1, "a", "x")
	st.DeleteProgramme(1, p.ID)
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM programme_versions`).Scan(&n)
	if n != 0 {
		t.Errorf("%d versions left behind", n)
	}
}

func TestProgrammesHaveNoStatusColumnAnyMore(t *testing.T) {
	_, d := newStore(t)
	if _, err := d.Exec(`SELECT status FROM programmes`); err == nil {
		t.Error("the draft/published column still exists")
	}
}
