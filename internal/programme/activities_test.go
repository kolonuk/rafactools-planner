package programme

import (
	"errors"
	"testing"
)

func TestActivityList(t *testing.T) {
	st, d := newStore(t)
	if got := DefaultActivities(); len(got) != 15 || got[0] != "First Class - Flying Operation" || got[14] != "Guest Speaker - RAF Police" {
		t.Fatalf("defaults = %v", got)
	}
	if err := st.AddActivity(1, "  Fieldcraft  "); err != nil {
		t.Fatal(err)
	}
	for _, dup := range []string{"fieldcraft", "FIELDCRAFT ", "bowling", "  laser   quest "} {
		if err := st.AddActivity(1, dup); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("AddActivity(%q) = %v, want an invalid-input error", dup, err)
		}
	}
	if err := st.AddActivity(1, ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("a blank activity: %v", err)
	}
	if err := st.AddActivity(2, "Fieldcraft"); err != nil { // another squadron may have the same
		t.Fatal(err)
	}
	opts, err := st.ActivityOptions(1)
	if err != nil || len(opts) != 16 || opts[15] != "Fieldcraft" {
		t.Fatalf("options = %v, %v", opts, err)
	}
	custom, _ := st.ListCustomActivities(1)
	if err := st.UpdateActivity(2, custom[0].ID, "Hijack"); !errors.Is(err, ErrNotFound) {
		t.Errorf("another squadron renamed it: %v", err)
	}
	if err := st.UpdateActivity(1, custom[0].ID, "Bowling"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("renamed to a built-in title: %v", err)
	}
	if err := st.UpdateActivity(1, custom[0].ID, "Fieldcraft weekend"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteActivity(2, custom[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another squadron deleted it: %v", err)
	}

	t.Run("start-up clean-up removes own activities that match a built-in one", func(t *testing.T) {
		// Rows that got in before the title was in the built-in list.
		for _, v := range [][]any{{1, "Climatic injuries"}, {2, " Annual  Briefings"}, {2, "Parachuting"}} {
			if _, err := d.Exec(`INSERT INTO squadron_activities (squadron_id, title) VALUES (?, ?)`, v...); err != nil {
				t.Fatal(err)
			}
		}
		n, err := st.RemoveDuplicatesOfDefaults()
		if err != nil || n != 2 {
			t.Fatalf("removed %d, %v", n, err)
		}
		a, _ := st.ListCustomActivities(1)
		b, _ := st.ListCustomActivities(2)
		if len(a) != 1 || a[0].Title != "Fieldcraft weekend" || len(b) != 2 {
			t.Errorf("after clean-up: %+v and %+v", a, b)
		}
		if n, _ := st.RemoveDuplicatesOfDefaults(); n != 0 {
			t.Errorf("a second run removed %d", n)
		}
	})
}
