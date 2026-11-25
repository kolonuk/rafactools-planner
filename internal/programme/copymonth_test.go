package programme

import (
	"errors"
	"fmt"
	"testing"
)

// fillNights puts "A<n>" and "I<n>" in the boxes of a month's parade nights, in
// order, with the first uniform colour and a note.
func fillNights(t *testing.T, st *Store, sq, pid int64, prefix string) {
	t.Helper()
	fp, err := st.GetFullProgramme(sq, pid)
	if err != nil {
		t.Fatal(err)
	}
	uniform := ""
	if s, err := st.GetSquadron(sq); err == nil && len(s.UniformColours) > 0 {
		uniform = s.UniformColours[0].Key
	}
	var in []DayInput
	n := 0
	for _, d := range fp.Days {
		if !d.IsParade {
			continue
		}
		n++
		act, _ := d.RowByHalfType(1, "activity")
		ins, _ := d.RowByHalfType(1, "instructor")
		in = append(in, DayInput{ID: d.ID, IsParade: true, UniformKey: uniform, Notes: prefix + " note",
			Rows: []RowInput{
				{ID: act.ID, Texts: []string{fmt.Sprintf("%sA%d", prefix, n), "", "", ""}},
				{ID: ins.ID, Texts: []string{fmt.Sprintf("%sI%d", prefix, n), "", "", ""}},
			}})
	}
	if err := st.SaveDaysAndRows(sq, pid, in); err != nil {
		t.Fatal(err)
	}
}

func nights(t *testing.T, st *Store, sq, pid int64) (dates, acts, ins []string, fp *FullProgramme) {
	t.Helper()
	fp, err := st.GetFullProgramme(sq, pid)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range fp.Days {
		if !d.IsParade {
			continue
		}
		a, _ := d.RowByHalfType(1, "activity")
		i, _ := d.RowByHalfType(1, "instructor")
		dates = append(dates, d.Date)
		acts = append(acts, a.Texts[0])
		ins = append(ins, i.Texts[0])
	}
	return
}

func TestCopyMonthLinesUpParadeNights(t *testing.T) {
	st, _ := newStore(t)
	sq, _ := st.GetSquadron(1)
	sq.ParadeWeekdays = []int{1} // Mondays
	if err := st.UpdateSquadronSettings(sq); err != nil {
		t.Fatal(err)
	}
	apr := mustProg(t, st, 1, 2030, 4) // Mondays: 1 8 15 22 29
	may := mustProg(t, st, 1, 2030, 5) // 6 13 20 27
	jul := mustProg(t, st, 1, 2030, 7) // 1 8 15 22 29
	fillNights(t, st, 1, apr.ID, "Apr")
	fillNights(t, st, 1, jul.ID, "Jul") // already has entries
	if err := st.AddWeekendNote(may.ID, "2030-05-04", "May camp"); err != nil {
		t.Fatal(err)
	}

	t.Run("a 5-night month into a 4-night month drops the extra night", func(t *testing.T) {
		plan, err := st.CopyMonth(1, may.ID, apr.ID)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Copied != 4 || plan.Dropped != 1 || plan.Untouched != 0 || plan.Replaced != 0 {
			t.Errorf("plan = %+v", plan)
		}
		dates, acts, ins, fp := nights(t, st, 1, may.ID)
		if fmt.Sprint(dates) != "[2030-05-06 2030-05-13 2030-05-20 2030-05-27]" {
			t.Fatalf("dates = %v", dates)
		}
		if fmt.Sprint(acts) != "[AprA1 AprA2 AprA3 AprA4]" || fmt.Sprint(ins) != "[AprI1 AprI2 AprI3 AprI4]" {
			t.Errorf("acts %v ins %v", acts, ins)
		}
		for _, d := range fp.Days {
			if d.IsParade && d.UniformKey == "" {
				t.Errorf("%s has no uniform", d.Date)
			}
			if d.Notes != "" {
				t.Errorf("%s: notes were copied (%q)", d.Date, d.Notes)
			}
		}
		if len(fp.WeekendNotes) != 1 || fp.WeekendNotes[0].Text != "May camp" {
			t.Errorf("weekend notes touched: %+v", fp.WeekendNotes)
		}
	})

	t.Run("a 4-night month into a 5-night month leaves the last night as it was", func(t *testing.T) {
		plan, err := st.CopyMonth(1, jul.ID, may.ID)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Copied != 4 || plan.Untouched != 1 || plan.Replaced != 4 {
			t.Errorf("plan = %+v", plan)
		}
		_, acts, _, _ := nights(t, st, 1, jul.ID)
		if fmt.Sprint(acts) != "[AprA1 AprA2 AprA3 AprA4 JulA5]" {
			t.Errorf("acts = %v", acts)
		}
	})

	t.Run("a night under a closure is not counted", func(t *testing.T) {
		aug := mustProg(t, st, 1, 2030, 8) // Mondays 5 12 19 26
		if err := st.AddClosure(aug.ID, "2030-08-12", "2030-08-12", "Bank holiday"); err != nil {
			t.Fatal(err)
		}
		plan, err := st.CopyMonth(1, aug.ID, apr.ID)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Copied != 3 || plan.Dropped != 2 {
			t.Errorf("plan = %+v", plan)
		}
		fp, _ := st.GetFullProgramme(1, aug.ID)
		for _, d := range fp.Days {
			a, ok := d.RowByHalfType(1, "activity")
			if d.Date == "2030-08-12" && ok && a.Texts[0] != "" {
				t.Error("the closed night was filled in")
			}
			if d.Date == "2030-08-19" && a.Texts[0] != "AprA2" {
				t.Errorf("19 Aug has %q, want AprA2", a.Texts[0])
			}
		}
	})

	t.Run("second halves come across, and the copy is refused for the wrong months", func(t *testing.T) {
		sep := mustProg(t, st, 1, 2030, 9)
		oct := mustProg(t, st, 1, 2030, 10)
		fp, _ := st.GetFullProgramme(1, sep.ID)
		var first Day
		for _, d := range fp.Days {
			if d.IsParade {
				first = d
				break
			}
		}
		if err := st.AddHalf(1, sep.ID, first.ID, 4); err != nil {
			t.Fatal(err)
		}
		if _, err := st.CopyMonth(1, oct.ID, sep.ID); err != nil {
			t.Fatal(err)
		}
		o, _ := st.GetFullProgramme(1, oct.ID)
		for _, d := range o.Days {
			if d.IsParade {
				if d.Halves != 2 {
					t.Errorf("first night has %d halves, want 2", d.Halves)
				}
				break
			}
		}
		if _, err := st.CopyMonth(1, oct.ID, oct.ID); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("a month copied into itself: %v", err)
		}
		other := mustProg(t, st, 2, 2030, 4)
		if _, err := st.CopyMonth(1, oct.ID, other.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("copied from another squadron's month: %v", err)
		}
	})
}

func TestCopyableNights(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 4)
	fp, _ := st.GetFullProgramme(1, p.ID)
	if CopyableNights(fp) != 0 {
		t.Error("a blank month has entries")
	}
	fillNights(t, st, 1, p.ID, "X")
	fp, _ = st.GetFullProgramme(1, p.ID)
	if CopyableNights(fp) == 0 {
		t.Error("a filled month has no entries")
	}
}
