package programme

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rafactools-planner/internal/db"
)

// newStore returns a store with two squadrons (ids 1 and 2) and one user in
// each. Tests build everything else through the Store API.
func newStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	for _, n := range []string{"1", "2"} {
		if _, err := d.Exec(`INSERT INTO squadrons (number, name) VALUES (?, ?)`, n, n); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Exec(`INSERT INTO users (squadron_id, email, password_hash, role) VALUES (?, ?, 'x', 'owner')`, n, n+"@x"); err != nil {
			t.Fatal(err)
		}
	}
	return NewStore(d), d
}

func mustProg(t *testing.T, st *Store, sq int64, y, m int) *Programme {
	t.Helper()
	p, err := st.CreateProgramme(sq, y, m, sq)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCreateProgrammeGeneratesTheMonth(t *testing.T) {
	st, _ := newStore(t)
	for _, tc := range []struct {
		year, month, days int
	}{
		{2030, 1, 31}, {2030, 2, 28}, {2028, 2, 29}, {2100, 2, 28}, {2000, 2, 29}, {2030, 4, 30}, {2030, 12, 31},
	} {
		p := mustProg(t, st, 1, tc.year, tc.month)
		fp, err := st.GetFullProgramme(1, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(fp.Days) != tc.days {
			t.Errorf("%d-%02d: %d days, want %d", tc.year, tc.month, len(fp.Days), tc.days)
		}
		for i, d := range fp.Days {
			want := time.Date(tc.year, time.Month(tc.month), i+1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
			if d.Date != want {
				t.Fatalf("day %d is %s, want %s (days must be contiguous and ordered)", i, d.Date, want)
			}
		}
	}
}

func TestParadeNightsFollowSquadronWeekdays(t *testing.T) {
	st, _ := newStore(t)
	// Default is Mon+Thu. March 2030 has 5 Fridays, 4 Mondays, 4 Thursdays.
	fp, _ := st.GetFullProgramme(1, mustProg(t, st, 1, 2030, 3).ID)
	var parades int
	for _, d := range fp.Days {
		tm, _ := time.Parse("2006-01-02", d.Date)
		wantParade := tm.Weekday() == time.Monday || tm.Weekday() == time.Thursday
		if d.IsParade != wantParade {
			t.Errorf("%s (%s): IsParade=%v", d.Date, tm.Weekday(), d.IsParade)
		}
		if d.IsParade {
			parades++
			wantLabel := map[time.Weekday]string{time.Monday: "M", time.Thursday: "T"}[tm.Weekday()]
			if d.DayLabel != wantLabel {
				t.Errorf("%s label %q, want %q", d.Date, d.DayLabel, wantLabel)
			}
			if len(d.Rows) != 2 || d.Halves != 1 {
				t.Errorf("%s: %d rows / %d halves, want activity+instructor / 1", d.Date, len(d.Rows), d.Halves)
			}
			for _, r := range d.Rows {
				if len(r.Texts) != 4 {
					t.Errorf("row has %d text slots, want one per classification (4)", len(r.Texts))
				}
			}
		} else if len(d.Rows) != 0 {
			t.Errorf("%s: non-parade day has rows", d.Date)
		}
	}
	if parades != 8 {
		t.Errorf("%d parade nights, want 8", parades)
	}

	t.Run("Sunday=7 maps correctly and settings changes apply to new programmes only", func(t *testing.T) {
		sq, _ := st.GetSquadron(2)
		sq.ParadeWeekdays = []int{7}
		sq.Classifications = []string{"One", "Two"}
		if err := st.UpdateSquadronSettings(sq); err != nil {
			t.Fatal(err)
		}
		fp, _ := st.GetFullProgramme(2, mustProg(t, st, 2, 2030, 3).ID)
		for _, d := range fp.Days {
			tm, _ := time.Parse("2006-01-02", d.Date)
			if d.IsParade != (tm.Weekday() == time.Sunday) {
				t.Errorf("%s IsParade=%v", d.Date, d.IsParade)
			}
			for _, r := range d.Rows {
				if len(r.Texts) != 2 {
					t.Errorf("row has %d slots, want 2", len(r.Texts))
				}
			}
		}
	})
}

func TestCreateProgrammeRejectsDuplicateMonthPerSquadronOnly(t *testing.T) {
	st, _ := newStore(t)
	mustProg(t, st, 1, 2030, 6)
	if _, err := st.CreateProgramme(1, 2030, 6, 1); err == nil {
		t.Fatal("duplicate month accepted")
	}
	if _, err := st.CreateProgramme(2, 2030, 6, 2); err != nil {
		t.Fatalf("other squadron blocked by the first's programme: %v", err)
	}
	if _, err := st.CreateProgramme(1, 2031, 6, 1); err != nil {
		t.Fatalf("same month next year rejected: %v", err)
	}
	// A failed create must not leave a half-built programme behind.
	list, _ := st.ListProgrammes(1)
	if len(list) != 2 {
		t.Errorf("%d programmes, want 2", len(list))
	}
}

func TestListProgrammesOrderAndScope(t *testing.T) {
	st, _ := newStore(t)
	mustProg(t, st, 1, 2030, 1)
	mustProg(t, st, 1, 2031, 3)
	mustProg(t, st, 1, 2031, 11)
	mustProg(t, st, 2, 2040, 1)
	got, err := st.ListProgrammes(1)
	if err != nil {
		t.Fatal(err)
	}
	var order [][2]int
	for _, p := range got {
		order = append(order, [2]int{p.Year, p.Month})
	}
	want := [][2]int{{2031, 11}, {2031, 3}, {2030, 1}}
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Errorf("order = %v, want newest first %v", order, want)
	}
}

func TestProgrammeAccessIsScopedToSquadron(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 7)

	if _, err := st.GetFullProgramme(2, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetFullProgramme cross-tenant: %v", err)
	}
	if _, err := st.GetFullProgramme(1, 99999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing programme: %v", err)
	}
	if err := st.SetProgrammeStatus(2, p.ID, "final"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetProgrammeStatus cross-tenant: %v", err)
	}
	if err := st.DeleteProgramme(2, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteProgramme cross-tenant: %v", err)
	}
	if err := st.SaveDaysAndRows(2, p.ID, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("SaveDaysAndRows cross-tenant: %v", err)
	}
	if err := st.AddHalf(2, p.ID, 1, 4); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddHalf cross-tenant: %v", err)
	}
	// Nothing above may have had an effect.
	fp, _ := st.GetFullProgramme(1, p.ID)
	if fp.Status != "draft" {
		t.Errorf("status changed to %q", fp.Status)
	}

	if err := st.SetProgrammeStatus(1, p.ID, "final"); err != nil {
		t.Fatal(err)
	}
	if fp, _ := st.GetFullProgramme(1, p.ID); fp.Status != "final" {
		t.Errorf("status = %q", fp.Status)
	}
}

func TestDeleteProgrammeCascades(t *testing.T) {
	st, d := newStore(t)
	p := mustProg(t, st, 1, 2030, 8)
	st.AddClosure(p.ID, "2030-08-01", "2030-08-02", "x")
	st.AddWeekendNote(p.ID, "2030-08-03", "x")
	cat, _ := st.AddRequirementCategory(p.ID, "c", "")
	st.AddRequirementEntry(p.ID, cat, "n", "")

	if err := st.DeleteProgramme(1, p.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"programmes", "programme_days", "programme_rows", "programme_closures", "programme_weekend_notes", "programme_requirement_categories", "programme_requirement_entries"} {
		var n int
		d.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
		if n != 0 {
			t.Errorf("%s: %d orphan rows after delete (foreign keys on?)", table, n)
		}
	}
}

func TestSaveDaysAndRows(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 9)
	other := mustProg(t, st, 2, 2030, 9)
	fp, _ := st.GetFullProgramme(1, p.ID)
	ofp, _ := st.GetFullProgramme(2, other.ID)

	var parade Day
	for _, d := range fp.Days {
		if d.IsParade {
			parade = d
			break
		}
	}
	good := DayInput{ID: parade.ID, IsParade: true, UniformKey: "green", Notes: "bring water",
		Rows: []RowInput{{ID: parade.Rows[0].ID, Texts: []string{"Drill", "Drill", "Drill", "Drill"}}}}

	t.Run("round trip", func(t *testing.T) {
		if err := st.SaveDaysAndRows(1, p.ID, []DayInput{good}); err != nil {
			t.Fatal(err)
		}
		got, _ := st.GetFullProgramme(1, p.ID)
		for _, d := range got.Days {
			if d.ID == parade.ID {
				if d.Notes != "bring water" || d.UniformKey != "green" {
					t.Errorf("day not saved: %+v", d)
				}
				if r, _ := d.RowByHalfType(1, parade.Rows[0].RowType); r.Texts[2] != "Drill" {
					t.Errorf("row not saved: %+v", r)
				}
			}
		}
	})

	t.Run("foreign day or row IDs are rejected and nothing is partially applied", func(t *testing.T) {
		foreignDay := DayInput{ID: ofp.Days[0].ID, Notes: "hijack"}
		// A valid first day followed by a foreign second day: all-or-nothing.
		err := st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: parade.ID, IsParade: true, Notes: "partial?"}, foreignDay})
		if err == nil {
			t.Fatal("foreign day accepted")
		}
		got, _ := st.GetFullProgramme(1, p.ID)
		for _, d := range got.Days {
			if d.ID == parade.ID && d.Notes != "bring water" {
				t.Errorf("first day was committed despite the later failure: notes=%q", d.Notes)
			}
		}
		og, _ := st.GetFullProgramme(2, other.ID)
		if og.Days[0].Notes == "hijack" {
			t.Error("other squadron's day modified")
		}

		var otherRowID int64
		for _, d := range og.Days {
			if len(d.Rows) > 0 {
				otherRowID = d.Rows[0].ID
				break
			}
		}
		err = st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: parade.ID, IsParade: true, Rows: []RowInput{{ID: otherRowID, Texts: []string{"hijack", "", "", ""}}}}})
		if err == nil {
			t.Fatal("foreign row accepted under own day")
		}
		og, _ = st.GetFullProgramme(2, other.ID)
		for _, d := range og.Days {
			for _, r := range d.Rows {
				if r.Texts[0] == "hijack" {
					t.Error("other squadron's row text modified")
				}
			}
		}
	})
}

func TestAddHalf(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 10)
	fp, _ := st.GetFullProgramme(1, p.ID)
	var day Day
	for _, d := range fp.Days {
		if d.IsParade {
			day = d
			break
		}
	}
	for i := 0; i < 2; i++ { // idempotent
		if err := st.AddHalf(1, p.ID, day.ID, 4); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := st.GetFullProgramme(1, p.ID)
	for _, d := range got.Days {
		if d.ID == day.ID {
			if d.Halves != 2 || len(d.Rows) != 4 {
				t.Errorf("halves=%d rows=%d, want 2/4 (no duplicates after repeating)", d.Halves, len(d.Rows))
			}
			if _, ok := d.RowByHalfType(2, "instructor"); !ok {
				t.Error("half-2 instructor row missing")
			}
		}
	}
	// A day from a different programme of the same squadron is not addressable.
	p2 := mustProg(t, st, 1, 2030, 11)
	if err := st.AddHalf(1, p2.ID, day.ID, 4); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddHalf with a day from another programme: %v", err)
	}
}

func TestClosureAndNoteValidation(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 12)

	bad := []struct{ name, start, end string }{
		{"garbage", "a", "b"},
		{"empty", "", ""},
		{"wrong format", "01/12/2030", "02/12/2030"},
		{"impossible date", "2030-02-30", "2030-03-01"},
		{"reversed", "2030-12-10", "2030-12-01"},
		{"too long", "2030-01-01", "2032-01-01"},
		{"sql", "2030-01-01'; DROP TABLE programmes;--", "2030-01-02"},
	}
	for _, tc := range bad {
		err := st.AddClosure(p.ID, tc.start, tc.end, "m")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", tc.name, err)
		}
	}
	for _, tc := range []struct{ start, end string }{
		{"2030-12-01", "2030-12-01"}, {"2030-12-24", "2031-01-02"}, {"2028-02-28", "2028-03-01"}, {"2030-01-01", "2031-01-01"},
	} {
		if err := st.AddClosure(p.ID, tc.start, tc.end, "ok"); err != nil {
			t.Errorf("%v rejected: %v", tc, err)
		}
	}
	if err := st.AddWeekendNote(p.ID, "tomorrow", "x"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad note date: %v", err)
	}
	if err := st.AddWeekendNote(p.ID, "2030-12-07", "x"); err != nil {
		t.Errorf("good note: %v", err)
	}

	fp, _ := st.GetFullProgramme(1, p.ID)
	if len(fp.Closures) != 4 || len(fp.WeekendNotes) != 1 {
		t.Errorf("closures=%d notes=%d, want 4/1 — invalid rows must not be stored", len(fp.Closures), len(fp.WeekendNotes))
	}
	if _, err := st.ListProgrammes(1); err != nil {
		t.Errorf("programmes table damaged: %v", err)
	}
}

func TestWeekendNoteOrdering(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 5)
	for _, txt := range []string{"first", "second", "third"} {
		st.AddWeekendNote(p.ID, "2030-05-04", txt)
	}
	st.AddWeekendNote(p.ID, "2030-05-03", "earlier day")
	fp, _ := st.GetFullProgramme(1, p.ID)
	var got []string
	for _, n := range fp.WeekendNotes {
		got = append(got, n.Text)
	}
	want := []string{"earlier day", "first", "second", "third"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("notes = %v, want %v", got, want)
		}
	}
	st.DeleteWeekendNote(p.ID+1, fp.WeekendNotes[0].ID) // wrong programme: no effect
	if fp2, _ := st.GetFullProgramme(1, p.ID); len(fp2.WeekendNotes) != 4 {
		t.Error("note deleted via the wrong programme ID")
	}
}

func TestRequirementsLifecycle(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	c1, _ := st.AddRequirementCategory(p.ID, "Duty Cadets", "due 1 Apr")
	c2, _ := st.AddRequirementCategory(p.ID, "Subs", "")
	for _, n := range []string{"A", "B", "C"} {
		if err := st.AddRequirementEntry(p.ID, c1, n, ""); err != nil {
			t.Fatal(err)
		}
	}
	fp, _ := st.GetFullProgramme(1, p.ID)
	if len(fp.Requirements) != 2 || fp.Requirements[0].ID != c1 || fp.Requirements[1].ID != c2 {
		t.Fatalf("categories out of order: %+v", fp.Requirements)
	}
	ents := fp.Requirements[0].Entries
	if len(ents) != 3 || ents[0].CadetName != "A" || ents[2].CadetName != "C" || ents[0].SortOrder >= ents[2].SortOrder {
		t.Errorf("entries = %+v", ents)
	}

	// Entry deletion needs the right category AND programme.
	p2 := mustProg(t, st, 1, 2030, 3)
	st.DeleteRequirementEntry(p2.ID, c1, ents[0].ID)
	st.DeleteRequirementEntry(p.ID, c2, ents[0].ID)
	if fp, _ := st.GetFullProgramme(1, p.ID); len(fp.Requirements[0].Entries) != 3 {
		t.Error("entry deleted with a mismatched programme/category")
	}
	st.DeleteRequirementEntry(p.ID, c1, ents[0].ID)
	if fp, _ := st.GetFullProgramme(1, p.ID); len(fp.Requirements[0].Entries) != 2 {
		t.Error("legit entry delete failed")
	}
	// Deleting a category removes its entries.
	st.DeleteRequirementCategory(p.ID, c1)
	if fp, _ := st.GetFullProgramme(1, p.ID); len(fp.Requirements) != 1 {
		t.Error("category not deleted")
	}
	// Adding to a deleted/foreign category is refused.
	if err := st.AddRequirementEntry(p.ID, c1, "late", ""); err == nil {
		t.Error("entry added to a deleted category")
	}
}

func TestSquadronSettingsRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	sq, err := st.GetSquadron(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sq.UniformColours) != 5 || sq.UniformColours[0].Hex == "" || len(sq.Classifications) != 4 || len(sq.ParadeWeekdays) != 2 {
		t.Fatalf("defaults wrong: %+v", sq)
	}
	sq.Name, sq.Subtitle, sq.ProtectiveMarking = `Ünï "quoted" <Sqn>`, "sub", "OFFICIAL"
	sq.ParadeWeekdays = []int{2, 3}
	sq.Classifications = []string{"A", "B", "C"}
	if err := st.UpdateSquadronSettings(sq); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetSquadron(1)
	if got.Name != sq.Name || got.ProtectiveMarking != "OFFICIAL" || len(got.Classifications) != 3 || got.ParadeWeekdays[1] != 3 {
		t.Errorf("round trip lost data: %+v", got)
	}
	if o, _ := st.GetSquadron(2); o.Name != "2" {
		t.Error("update leaked to another squadron")
	}
	if _, err := st.GetSquadron(999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing squadron: %v", err)
	}
}

// Ticking "Parade" on a night that wasn't generated as one must give it
// activity/instructor rows, otherwise the editor shows a parade night with
// nowhere to type.
func TestMakingANightAParadeNightSeedsRows(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 3)
	fp, _ := st.GetFullProgramme(1, p.ID)
	var plain Day
	for _, d := range fp.Days {
		if !d.IsParade {
			plain = d
			break
		}
	}
	in := DayInput{ID: plain.ID, IsParade: true}
	for i := 0; i < 2; i++ { // saving again must not duplicate rows
		if err := st.SaveDaysAndRows(1, p.ID, []DayInput{in}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := st.GetFullProgramme(1, p.ID)
	for _, d := range got.Days {
		if d.ID == plain.ID {
			if len(d.Rows) != 2 {
				t.Fatalf("%d rows after ticking Parade, want 2", len(d.Rows))
			}
			for _, r := range d.Rows {
				if len(r.Texts) != 4 {
					t.Errorf("row has %d text slots, want 4", len(r.Texts))
				}
			}
		}
	}

	// Unticking and re-ticking keeps what was typed and does not add rows.
	got, _ = st.GetFullProgramme(1, p.ID)
	var firstRow int64
	for _, d := range got.Days {
		if d.ID == plain.ID {
			firstRow = d.Rows[0].ID
		}
	}
	st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: plain.ID, IsParade: true, Rows: []RowInput{{ID: firstRow, Texts: []string{"Kept", "", "", ""}}}}})
	st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: plain.ID, IsParade: false}})
	st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: plain.ID, IsParade: true}})
	got, _ = st.GetFullProgramme(1, p.ID)
	for _, d := range got.Days {
		if d.ID != plain.ID {
			continue
		}
		if len(d.Rows) != 2 {
			t.Fatalf("%d rows after untick/tick, want 2", len(d.Rows))
		}
		var kept bool
		for _, r := range d.Rows {
			kept = kept || (r.ID == firstRow && r.Texts[0] == "Kept")
		}
		if !kept {
			t.Error("typed text lost across untick/tick")
		}
	}
}

// --- moving activities between dates -------------------------------------

// paradeDays returns the programme's parade nights and its non-parade days.
func splitDays(t *testing.T, st *Store, sq, pid int64) (parade, plain []Day) {
	t.Helper()
	fp, err := st.GetFullProgramme(sq, pid)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range fp.Days {
		if d.IsParade {
			parade = append(parade, d)
		} else {
			plain = append(plain, d)
		}
	}
	return
}

func setTexts(t *testing.T, st *Store, sq, pid int64, day Day, activity, instructor string) {
	t.Helper()
	var in []RowInput
	for _, r := range day.Rows {
		if r.Half != 1 {
			continue
		}
		txt := make([]string, len(r.Texts))
		for i := range txt {
			if r.RowType == "activity" {
				txt[i] = activity
			} else {
				txt[i] = instructor
			}
		}
		in = append(in, RowInput{ID: r.ID, Texts: txt})
	}
	if err := st.SaveDaysAndRows(sq, pid, []DayInput{{ID: day.ID, IsParade: true, Rows: in}}); err != nil {
		t.Fatal(err)
	}
}

func dayByID(t *testing.T, st *Store, sq, pid, id int64) Day {
	t.Helper()
	fp, _ := st.GetFullProgramme(sq, pid)
	for _, d := range fp.Days {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("day %d not found", id)
	return Day{}
}

func text(d Day, half int, rt string) string {
	r, ok := d.RowByHalfType(half, rt)
	if !ok || len(r.Texts) == 0 {
		return ""
	}
	return r.Texts[0]
}

func TestMoveDayContentSwapsTwoParadeNights(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	parade, _ := splitDays(t, st, 1, p.ID)
	a, b := parade[0], parade[1]
	setTexts(t, st, 1, p.ID, a, "Drill", "Sgt A")
	setTexts(t, st, 1, p.ID, b, "First Aid", "Cpl B")

	if err := st.MoveDayContent(1, p.ID, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	ga, gb := dayByID(t, st, 1, p.ID, a.ID), dayByID(t, st, 1, p.ID, b.ID)
	if text(ga, 1, "activity") != "First Aid" || text(ga, 1, "instructor") != "Cpl B" ||
		text(gb, 1, "activity") != "Drill" || text(gb, 1, "instructor") != "Sgt A" {
		t.Errorf("not swapped: a=%q/%q b=%q/%q", text(ga, 1, "activity"), text(ga, 1, "instructor"), text(gb, 1, "activity"), text(gb, 1, "instructor"))
	}
	// Everything about the night itself stays put: date, label, notes, uniform.
	if ga.Date != a.Date || ga.DayLabel != a.DayLabel || gb.Date != b.Date {
		t.Error("dates or labels moved")
	}

	// Swapping again restores the original: nothing was lost.
	if err := st.MoveDayContent(1, p.ID, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	ga, gb = dayByID(t, st, 1, p.ID, a.ID), dayByID(t, st, 1, p.ID, b.ID)
	if text(ga, 1, "activity") != "Drill" || text(gb, 1, "activity") != "First Aid" {
		t.Error("second swap did not restore the original")
	}
}

func TestMoveDayContentToANonParadeDayMakesItAParadeNight(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	parade, plain := splitDays(t, st, 1, p.ID)
	src, dst := parade[0], plain[0]
	setTexts(t, st, 1, p.ID, src, "Camp prep", "Fg Off C")

	if err := st.MoveDayContent(1, p.ID, src.ID, dst.ID); err != nil {
		t.Fatal(err)
	}
	gd, gs := dayByID(t, st, 1, p.ID, dst.ID), dayByID(t, st, 1, p.ID, src.ID)
	if !gd.IsParade {
		t.Error("target was not made a parade night")
	}
	if text(gd, 1, "activity") != "Camp prep" || text(gd, 1, "instructor") != "Fg Off C" {
		t.Errorf("target content = %q/%q", text(gd, 1, "activity"), text(gd, 1, "instructor"))
	}
	if gd.DayLabel != plain[0].DayLabel || gd.DayLabel == "" {
		t.Errorf("day label changed or empty: %q (was %q); labels come from the calendar date and must not move", gd.DayLabel, plain[0].DayLabel)
	}
	// The source stays a parade night, now empty, ready to be unticked or refilled.
	if !gs.IsParade || text(gs, 1, "activity") != "" {
		t.Errorf("source: parade=%v activity=%q", gs.IsParade, text(gs, 1, "activity"))
	}
	// Per-classification text slots are intact on the new night.
	if r, _ := gd.RowByHalfType(1, "activity"); len(r.Texts) != 4 {
		t.Errorf("target row has %d text slots, want 4", len(r.Texts))
	}
}

func TestMoveDayContentCarriesTheSecondHalf(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	parade, _ := splitDays(t, st, 1, p.ID)
	a, b := parade[0], parade[1]
	if err := st.AddHalf(1, p.ID, a.ID, 4); err != nil {
		t.Fatal(err)
	}
	a = dayByID(t, st, 1, p.ID, a.ID)
	for _, r := range a.Rows {
		if r.Half == 2 && r.RowType == "activity" {
			st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: a.ID, IsParade: true, Rows: []RowInput{{ID: r.ID, Texts: []string{"Second half", "", "", ""}}}}})
		}
	}

	if err := st.MoveDayContent(1, p.ID, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	ga, gb := dayByID(t, st, 1, p.ID, a.ID), dayByID(t, st, 1, p.ID, b.ID)
	if gb.Halves != 2 || text(gb, 2, "activity") != "Second half" {
		t.Errorf("target: halves=%d half2=%q, want 2 / Second half", gb.Halves, text(gb, 2, "activity"))
	}
	if ga.Halves != 1 {
		t.Errorf("source: halves=%d, want 1 (it received a one-half night)", ga.Halves)
	}
}

func TestMoveDayContentRefusals(t *testing.T) {
	st, d := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	other := mustProg(t, st, 2, 2030, 4)
	p2 := mustProg(t, st, 1, 2030, 5)
	parade, plain := splitDays(t, st, 1, p.ID)
	oparade, _ := splitDays(t, st, 2, other.ID)
	p2parade, _ := splitDays(t, st, 1, p2.ID)
	a, b := parade[0], parade[1]
	setTexts(t, st, 1, p.ID, a, "Keep me", "x")

	snapshot := func() string {
		var s string
		d.QueryRow(`SELECT group_concat(day_id||half||row_type||texts||is_parade||halves, '|') FROM programme_rows r JOIN programme_days pd ON pd.id = r.day_id ORDER BY 1`).Scan(&s)
		return s
	}
	before := snapshot()

	for name, tc := range map[string]struct {
		sq, prog, from, to int64
		want               error
	}{
		"same day":                 {1, p.ID, a.ID, a.ID, ErrInvalidInput},
		"source is not a parade":   {1, p.ID, plain[0].ID, b.ID, ErrInvalidInput},
		"other squadron's day":     {1, p.ID, a.ID, oparade[0].ID, ErrNotFound},
		"other squadron as caller": {2, p.ID, a.ID, b.ID, ErrNotFound},
		"day of a different month": {1, p.ID, a.ID, p2parade[0].ID, ErrNotFound},
		"nonexistent target":       {1, p.ID, a.ID, 999999, ErrNotFound},
		"nonexistent source":       {1, p.ID, 999999, b.ID, ErrNotFound},
		"programme of another":     {1, other.ID, a.ID, b.ID, ErrNotFound},
	} {
		err := st.MoveDayContent(tc.sq, tc.prog, tc.from, tc.to)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if after := snapshot(); after != before {
		t.Error("a refused move changed data")
	}
}

// A one-half night must not be left with spare half-2 rows after a move:
// AddHalf only copies half 1 into rows that do not exist yet, so leftovers
// would make a later "add second half" start empty instead of copying.
func TestMoveLeavesNoSpareHalfTwoRowsAndAddHalfStillClones(t *testing.T) {
	st, d := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	parade, plain := splitDays(t, st, 1, p.ID)
	src, dst := parade[0], plain[0]
	setTexts(t, st, 1, p.ID, src, "Drill", "Sgt A")

	if err := st.MoveDayContent(1, p.ID, src.ID, dst.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{src.ID, dst.ID} {
		var n int
		d.QueryRow("SELECT COUNT(*) FROM programme_rows WHERE day_id = ? AND half = 2", id).Scan(&n)
		if n != 0 {
			t.Errorf("day %d has %d spare half-2 rows after a move", id, n)
		}
	}

	if err := st.AddHalf(1, p.ID, dst.ID, 4); err != nil {
		t.Fatal(err)
	}
	if got := text(dayByID(t, st, 1, p.ID, dst.ID), 2, "activity"); got != "Drill" {
		t.Errorf("second half after a move = %q, want Drill (copied from half 1)", got)
	}
}

// --- changing the classification list --------------------------------------

func rowCounts(t *testing.T, d *sql.DB, sqID int64) map[int]int {
	t.Helper()
	rows, err := d.Query(`SELECT r.texts FROM programme_rows r JOIN programme_days pd ON pd.id = r.day_id
		JOIN programmes p ON p.id = pd.programme_id WHERE p.squadron_id = ?`, sqID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var j string
		rows.Scan(&j)
		var a []string
		json.Unmarshal([]byte(j), &a)
		out[len(a)]++
	}
	return out
}

func TestChangingClassificationsKeepsEveryRowInStep(t *testing.T) {
	st, d := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	other := mustProg(t, st, 2, 2030, 4)
	parade, _ := splitDays(t, st, 1, p.ID)
	setTexts(t, st, 1, p.ID, parade[0], "Drill", "Sgt A") // 4 columns, all filled

	sq, _ := st.GetSquadron(1)
	if err := st.UpdateSquadronSettings(&Squadron{ID: 1, Name: sq.Name, Subtitle: sq.Subtitle, ParadeWeekdays: sq.ParadeWeekdays,
		Classifications: append(append([]string{}, sq.Classifications...), "Staff Cadets", "Cadet NCOs"), UniformColours: sq.UniformColours}); err != nil {
		t.Fatalf("growing the list: %v", err)
	}
	if got := rowCounts(t, d, 1); len(got) != 1 || got[6] == 0 {
		t.Fatalf("after growing to 6, row sizes = %v, want all 6", got)
	}
	fp, _ := st.GetFullProgramme(1, p.ID)
	for _, day := range fp.Days {
		if day.ID == parade[0].ID {
			r, _ := day.RowByHalfType(1, "activity")
			if r.Texts[0] != "Drill" || r.Texts[3] != "Drill" || r.Texts[4] != "" || r.Texts[5] != "" {
				t.Errorf("existing text moved or lost, new boxes not blank: %q", r.Texts)
			}
		}
	}
	if got := rowCounts(t, d, 2); len(got) != 1 || got[4] == 0 {
		t.Errorf("another squadron's rows were resized: %v", got)
	}
	_ = other

	// Shrinking over empty columns is fine and trims the rows.
	sq, _ = st.GetSquadron(1)
	sq.Classifications = sq.Classifications[:4]
	if err := st.UpdateSquadronSettings(sq); err != nil {
		t.Fatalf("shrinking over empty columns: %v", err)
	}
	if got := rowCounts(t, d, 1); len(got) != 1 || got[4] == 0 {
		t.Errorf("after shrinking to 4, row sizes = %v", got)
	}
}

func TestShrinkingClassificationsNeverDiscardsText(t *testing.T) {
	st, d := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	parade, _ := splitDays(t, st, 1, p.ID)
	setTexts(t, st, 1, p.ID, parade[0], "Drill", "Sgt A") // fills all 4 columns

	sq, _ := st.GetSquadron(1)
	before := rowCounts(t, d, 1)
	sq.Classifications = sq.Classifications[:2] // would drop two filled columns
	err := st.UpdateSquadronSettings(sq)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	for _, name := range []string{"Leading Cadets", "Senior Cadets"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("message does not name the column %q that has text: %v", name, err)
		}
	}
	if got := st.mustClassificationCount(t, 1); got != 4 {
		t.Errorf("classification list changed to %d columns despite the refusal", got)
	}
	if after := rowCounts(t, d, 1); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Errorf("rows changed despite the refusal: %v -> %v", before, after)
	}
}

func (s *Store) mustClassificationCount(t *testing.T, id int64) int {
	t.Helper()
	sq, err := s.GetSquadron(id)
	if err != nil {
		t.Fatal(err)
	}
	return len(sq.Classifications)
}

// Rows that already disagree with the list (created before this was enforced)
// are brought back into line the next time settings are saved.
func TestSavingSettingsHealsMismatchedRows(t *testing.T) {
	st, d := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	d.Exec(`UPDATE programme_rows SET texts = '["a","b"]'`) // legacy damage: 2 boxes, 4 headings
	sq, _ := st.GetSquadron(1)
	if err := st.UpdateSquadronSettings(sq); err != nil {
		t.Fatal(err)
	}
	if got := rowCounts(t, d, 1); len(got) != 1 || got[4] == 0 {
		t.Errorf("row sizes after healing = %v, want all 4", got)
	}
	_ = p
}

func TestResetProgrammeEmptiesContentKeepsStructure(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 10)
	other := mustProg(t, st, 2, 2030, 10)
	fp, _ := st.GetFullProgramme(1, p.ID)
	var parade Day
	for _, d := range fp.Days {
		if d.IsParade {
			parade = d
			break
		}
	}
	st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: parade.ID, IsParade: true, UniformKey: "green", Notes: "bring water",
		Rows: []RowInput{{ID: parade.Rows[0].ID, Texts: []string{"Drill", "Drill", "Drill", "Drill"}}}}})
	st.SaveDaysAndRows(2, other.ID, nil)
	st.AddClosure(p.ID, "2030-10-01", "2030-10-02", "x")
	st.AddWeekendNote(p.ID, "2030-10-05", "camp")
	cat, _ := st.AddRequirementCategory(p.ID, "c", "")
	st.AddRequirementEntry(p.ID, cat, "n", "")
	st.AddClosure(other.ID, "2030-10-01", "2030-10-02", "keep")

	if err := st.ResetProgramme(2, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant reset = %v, want ErrNotFound", err)
	}
	if got, _ := st.GetFullProgramme(1, p.ID); len(got.Closures) != 1 {
		t.Fatal("another squadron's reset changed the programme")
	}

	if err := st.ResetProgramme(1, p.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetFullProgramme(1, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Closures) != 0 || len(got.WeekendNotes) != 0 || len(got.Requirements) != 0 {
		t.Errorf("closures/weekend notes/requirements not cleared: %d/%d/%d", len(got.Closures), len(got.WeekendNotes), len(got.Requirements))
	}
	if len(got.Days) != len(fp.Days) {
		t.Errorf("days = %d, want %d (the month stays)", len(got.Days), len(fp.Days))
	}
	for i, d := range got.Days {
		if d.Notes != "" {
			t.Errorf("day %d notes %q not cleared", d.ID, d.Notes)
		}
		if d.IsParade != fp.Days[i].IsParade || d.Halves != fp.Days[i].Halves || len(d.Rows) != len(fp.Days[i].Rows) {
			t.Errorf("day %d structure changed", d.ID)
		}
		for _, r := range d.Rows {
			if len(r.Texts) != 4 {
				t.Errorf("row has %d cells, want 4", len(r.Texts))
			}
			for _, c := range r.Texts {
				if c != "" {
					t.Errorf("cell %q not cleared", c)
				}
			}
		}
	}
	if o, _ := st.GetFullProgramme(2, other.ID); len(o.Closures) != 1 {
		t.Error("reset leaked into another squadron's programme")
	}
}

func TestCopyRequirementCategoriesCopiesNamesNeverCadets(t *testing.T) {
	st, _ := newStore(t)
	jan := mustProg(t, st, 1, 2031, 1)
	mar := mustProg(t, st, 1, 2031, 3)
	otherSq := mustProg(t, st, 2, 2031, 2)
	apr := mustProg(t, st, 1, 2031, 4)

	// Nothing to copy from yet.
	if n, err := st.CopyRequirementCategories(1, jan.ID); err != nil || n != 0 {
		t.Fatalf("first month: %d, %v", n, err)
	}

	c1, _ := st.AddRequirementCategory(jan.ID, "Old category", "by Jan")
	st.AddRequirementEntry(jan.ID, c1, "Cdt Old", "old note")
	c2, _ := st.AddRequirementCategory(mar.ID, "Fieldcraft", "by Apr")
	c3, _ := st.AddRequirementCategory(mar.ID, "First aid", "")
	st.AddRequirementEntry(mar.ID, c2, "Cdt Secret", "secret note")
	_ = c3
	oc, _ := st.AddRequirementCategory(otherSq.ID, "Other squadron's", "")
	st.AddRequirementEntry(otherSq.ID, oc, "Cdt Other", "")

	n, err := st.CopyRequirementCategories(1, apr.ID)
	if err != nil || n != 2 {
		t.Fatalf("copy into April: %d, %v (want the 2 categories of March, the nearest earlier month)", n, err)
	}
	got, _ := st.GetFullProgramme(1, apr.ID)
	if len(got.Requirements) != 2 || got.Requirements[0].Name != "Fieldcraft" || got.Requirements[0].DueLabel != "by Apr" || got.Requirements[1].Name != "First aid" {
		t.Fatalf("copied = %+v", got.Requirements)
	}
	for _, c := range got.Requirements {
		if len(c.Entries) != 0 {
			t.Errorf("category %q copied %d cadet entries, want none", c.Name, len(c.Entries))
		}
	}
	// The source is untouched, and another squadron's months are never used.
	if mar2, _ := st.GetFullProgramme(1, mar.ID); len(mar2.Requirements[0].Entries) != 1 {
		t.Error("the source month lost its entries")
	}
	for _, c := range got.Requirements {
		if c.Name == "Other squadron's" {
			t.Error("a category was copied from another squadron")
		}
	}
	// Another squadron cannot copy into this programme.
	if _, err := st.CopyRequirementCategories(2, apr.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant copy = %v, want ErrNotFound", err)
	}
}
