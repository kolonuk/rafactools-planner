package programme

import (
	"errors"
	"testing"
	"time"
)

func TestPastMonthsCannotBeCreated(t *testing.T) {
	st, _ := newStore(t)
	setClock(t, time.Date(2031, 3, 20, 12, 0, 0, 0, time.UTC))

	for name, ym := range map[string][2]int{"last month": {2031, 2}, "last year": {2030, 12}, "long ago": {2025, 6}} {
		if _, err := st.CreateProgramme(1, ym[0], ym[1], 1); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s = %v, want ErrInvalidInput", name, err)
		}
	}
	for name, ym := range map[string][2]int{"this month": {2031, 3}, "next month": {2031, 4}, "next year": {2032, 1}} {
		if _, err := st.CreateProgramme(1, ym[0], ym[1], 1); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, ym := range map[string][2]int{"month 0": {2031, 0}, "month 13": {2031, 13}, "year 1900": {1900, 5}, "year 3000": {3000, 5}} {
		if _, err := st.CreateProgramme(1, ym[0], ym[1], 1); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestTheMonthTurnsOverInUKTime(t *testing.T) {
	// 23:30 UTC on 31 July is 00:30 on 1 August in London (summer time).
	setClock(t, time.Date(2031, 7, 31, 23, 30, 0, 0, time.UTC))
	if y, m := CurrentMonth(); y != 2031 || m != 8 {
		t.Errorf("CurrentMonth = %d-%d, want 2031-8", y, m)
	}
	// In winter London is on UTC.
	setClock(t, time.Date(2031, 1, 31, 23, 30, 0, 0, time.UTC))
	if y, m := CurrentMonth(); y != 2031 || m != 1 {
		t.Errorf("CurrentMonth = %d-%d, want 2031-1", y, m)
	}
}

func TestEnsureCurrentProgrammeMakesTheMonthOnceAndCopiesCategoryNames(t *testing.T) {
	st, _ := newStore(t)
	setClock(t, time.Date(2031, 5, 10, 12, 0, 0, 0, time.UTC))
	// An earlier month (made while it was still current) with a tracker.
	setClock(t, time.Date(2031, 4, 10, 12, 0, 0, 0, time.UTC))
	apr := mustProg(t, st, 1, 2031, 4)
	cat, _ := st.AddRequirementCategory(apr.ID, "Fieldcraft", "by June")
	st.AddRequirementEntry(apr.ID, cat, "Cdt Private", "note")

	setClock(t, time.Date(2031, 5, 10, 12, 0, 0, 0, time.UTC))
	made, err := st.EnsureCurrentProgramme(1, 1)
	if err != nil || !made {
		t.Fatalf("first call: made=%v err=%v", made, err)
	}
	id, err := st.FindProgramme(1, 2031, 5)
	if err != nil {
		t.Fatal(err)
	}
	fp, _ := st.GetFullProgramme(1, id)
	if len(fp.Requirements) != 1 || fp.Requirements[0].Name != "Fieldcraft" || len(fp.Requirements[0].Entries) != 0 {
		t.Errorf("tracker = %+v: want the category, never the cadets", fp.Requirements)
	}
	if made, err := st.EnsureCurrentProgramme(1, 1); err != nil || made {
		t.Errorf("second call: made=%v err=%v, want it left alone", made, err)
	}
	// Another squadron is independent.
	if made, _ := st.EnsureCurrentProgramme(2, 2); !made {
		t.Error("the other squadron got no programme")
	}
	var n int
	st.db.QueryRow(`SELECT COUNT(*) FROM programmes WHERE year = 2031 AND month = 5`).Scan(&n)
	if n != 2 {
		t.Errorf("%d May programmes, want one per squadron", n)
	}
	// It never goes back for earlier months.
	st.db.QueryRow(`SELECT COUNT(*) FROM programmes WHERE year*12+month < 2031*12+4`).Scan(&n)
	if n != 0 {
		t.Errorf("%d historic programmes were made", n)
	}
}
