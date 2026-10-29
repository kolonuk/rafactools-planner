package programme

import (
	"errors"
	"strings"
	"testing"
)

func TestInstructorsAddListDelete(t *testing.T) {
	st, _ := newStore(t)
	for _, in := range []struct{ name, rank string }{
		{"Smith", "Fg Off"}, {"alice Jones", ""}, {"Brown", "Sgt"}, {"  Lee   Park  ", "  Cpl "},
	} {
		if err := st.AddInstructor(1, in.name, in.rank); err != nil {
			t.Fatalf("%+v: %v", in, err)
		}
	}
	got, err := st.ListInstructors(1)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, i := range got {
		names = append(names, i.Display())
	}
	want := "alice Jones|Sgt Brown|Cpl Lee Park|Fg Off Smith" // alphabetical by name, case-insensitive; whitespace tidied
	if strings.Join(names, "|") != want {
		t.Errorf("list = %q, want %q", strings.Join(names, "|"), want)
	}

	if err := st.DeleteInstructor(1, got[0].ID); err != nil {
		t.Fatal(err)
	}
	if after, _ := st.ListInstructors(1); len(after) != 3 {
		t.Errorf("%d left after delete, want 3", len(after))
	}
}

func TestInstructorsAreScopedToTheSquadron(t *testing.T) {
	st, _ := newStore(t)
	st.AddInstructor(1, "Smith", "")
	st.AddInstructor(2, "Smith", "") // same name in another squadron is fine
	one, _ := st.ListInstructors(1)
	two, _ := st.ListInstructors(2)
	if len(one) != 1 || len(two) != 1 {
		t.Fatalf("lists: %d and %d, want 1 and 1", len(one), len(two))
	}
	if err := st.DeleteInstructor(2, one[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting another squadron's entry: %v", err)
	}
	if still, _ := st.ListInstructors(1); len(still) != 1 {
		t.Error("another squadron deleted this squadron's instructor")
	}
}

func TestInstructorValidation(t *testing.T) {
	st, _ := newStore(t)
	st.AddInstructor(1, "Smith", "Fg Off")
	for name, tc := range map[string]struct{ n, r string }{
		"empty name":        {"", ""},
		"blank name":        {"   ", "Sgt"},
		"duplicate":         {"Smith", "Fg Off"},
		"duplicate spacing": {" Smith ", " Fg   Off "},
		"name too long":     {strings.Repeat("a", 81), ""},
		"rank too long":     {"Jones", strings.Repeat("r", 41)},
		"newline in name":   {"Jones\nSmith", ""},
		"control char":      {"Jo\x00nes", ""},
		"tab in rank":       {"Jones", "Fg\tOff\x07"},
	} {
		err := st.AddInstructor(1, tc.n, tc.r)
		if tc.n == "Jones\nSmith" {
			// whitespace including newlines is collapsed to a space, so this is accepted as "Jones Smith"
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
	// A different rank makes it a different entry.
	if err := st.AddInstructor(1, "Smith", "Sgt"); err != nil {
		t.Errorf("same name, different rank: %v", err)
	}
}

func TestInstructorListIsCapped(t *testing.T) {
	st, _ := newStore(t)
	for i := 0; i < maxInstructors; i++ {
		if err := st.AddInstructor(1, "Person "+strings.Repeat("x", i%30)+string(rune('a'+i%26))+strings.Repeat("y", i/26), ""); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	if err := st.AddInstructor(1, "One too many", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("past the cap: %v", err)
	}
}

func TestInstructorsCascadeWithTheSquadron(t *testing.T) {
	st, d := newStore(t)
	st.AddInstructor(1, "Smith", "")
	d.Exec(`DELETE FROM squadrons WHERE id = 1`)
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM instructors`).Scan(&n)
	if n != 0 {
		t.Errorf("%d instructors survived their squadron", n)
	}
}

func TestUpdateInstructorRulesAndTenancy(t *testing.T) {
	st, _ := newStore(t)
	st.AddInstructor(1, "Smith", "Fg Off")
	st.AddInstructor(1, "Jones", "")
	st.AddInstructor(2, "Other", "")
	list, _ := st.ListInstructors(1)
	var smith, jones int64
	for _, i := range list {
		if i.Name == "Smith" {
			smith = i.ID
		} else {
			jones = i.ID
		}
	}
	other, _ := st.ListInstructors(2)

	if err := st.UpdateInstructor(1, smith, "  Smythe ", " Flt Lt "); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetInstructor(1, smith); got.Name != "Smythe" || got.Rank != "Flt Lt" {
		t.Errorf("after rename: %+v", got)
	}
	for name, err := range map[string]error{
		"blank name":    st.UpdateInstructor(1, smith, "   ", ""),
		"too long":      st.UpdateInstructor(1, smith, strings.Repeat("x", 81), ""),
		"control char":  st.UpdateInstructor(1, smith, "Sm\x00ith", ""),
		"duplicate":     st.UpdateInstructor(1, smith, "Jones", ""),
		"rank too long": st.UpdateInstructor(1, smith, "Smith", strings.Repeat("r", 41)),
	} {
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
	if got, _ := st.GetInstructor(1, smith); got.Name != "Smythe" {
		t.Errorf("a refused rename changed the entry: %+v", got)
	}
	_ = jones
	// Another squadron's entry can be neither read nor changed.
	if err := st.UpdateInstructor(1, other[0].ID, "Hijacked", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant update = %v, want ErrNotFound", err)
	}
	if _, err := st.GetInstructor(1, other[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant get = %v, want ErrNotFound", err)
	}
	if got, _ := st.GetInstructor(2, other[0].ID); got.Name != "Other" {
		t.Errorf("the other squadron's entry changed: %+v", got)
	}
}
