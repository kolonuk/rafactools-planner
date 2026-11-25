package programme

import (
	"errors"
	"testing"
)

func TestAbsencesAreKeptPerSquadronAndValidated(t *testing.T) {
	st, _ := newStore(t)
	if err := st.AddAbsence(1, "Fg Off Smith", 0, 1, "2030-04-08", "2030-04-12", "leave"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][3]string{{"", "2030-04-08", "2030-04-09"}, {"X", "nope", "2030-04-09"}, {"X", "2030-04-10", "2030-04-09"}, {"X", "2030-01-01", "2032-01-01"}} {
		if err := st.AddAbsence(1, bad[0], 0, 1, bad[1], bad[2], ""); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	list, err := st.ListAbsences(1, "")
	if err != nil || len(list) != 1 || list[0].Name != "Fg Off Smith" || list[0].Note != "leave" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if other, _ := st.ListAbsences(2, ""); len(other) != 0 {
		t.Error("another squadron sees them")
	}
	if over, _ := st.ListAbsences(1, "2030-04-13"); len(over) != 0 {
		t.Error("a finished entry is listed as upcoming")
	}
	if err := st.UpdateAbsence(2, list[0].ID, "", "2030-04-08", "2030-04-12", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("another squadron edited it: %v", err)
	}
	if err := st.UpdateAbsence(1, list[0].ID, "", "2030-04-08", "2030-04-14", "longer"); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetAbsence(1, list[0].ID)
	if got.End != "2030-04-14" || got.Name != "Fg Off Smith" || got.Note != "longer" {
		t.Errorf("after edit = %+v", got)
	}
	if err := st.DeleteAbsence(2, list[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another squadron deleted it: %v", err)
	}
	if err := st.DeleteAbsence(1, list[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestAwayMatcher(t *testing.T) {
	abs := []Absence{
		{ID: 1, Name: "Fg Off Smith", Start: "2030-04-08", End: "2030-04-12"},
		{ID: 2, Name: "Jones", Start: "2030-04-15", End: "2030-04-15", Note: "course"},
	}
	m := NewAwayMatcher(abs, map[string][]string{"Fg Off Smith": {"Smith"}})
	for _, tc := range []struct {
		text, date string
		want       int
	}{
		{"Fg Off Smith", "2030-04-09", 1},
		{"fg off  smith,", "2030-04-12", 1},
		{"Smith", "2030-04-09", 1},    // by the alias
		{"Smith", "2030-04-13", 0},    // not away that day
		{"Smithson", "2030-04-09", 0}, // whole words only
		{"Smith / Jones", "2030-04-15", 1},
		{"Flt Lt Jones and Smith", "2030-04-15", 1},
		{"All Staff", "2030-04-09", 0},
		{"", "2030-04-09", 0},
	} {
		if got := len(m.Away(tc.text, tc.date)); got != tc.want {
			t.Errorf("Away(%q, %s) = %d, want %d", tc.text, tc.date, got, tc.want)
		}
	}
	if got := abs[1].Describe(); got != "Jones is away 15 Apr (course)" {
		t.Errorf("Describe = %q", got)
	}
	var none *AwayMatcher
	if none.Away("x", "2030-04-09") != nil {
		t.Error("a nil matcher found someone")
	}
}
