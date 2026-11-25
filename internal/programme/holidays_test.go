package programme

import (
	"testing"
	"time"
)

func mustDate(t *testing.T, d string) time.Time {
	t.Helper()
	v, err := time.Parse("2006-01-02", d)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func closuresOf(t *testing.T, st *Store, sq, id int64) []Closure {
	t.Helper()
	fp, err := st.GetFullProgramme(sq, id)
	if err != nil {
		t.Fatal(err)
	}
	return fp.Closures
}

var testHolidays = []Holiday{
	{RegionEnglandWales, "2030-12-25", "Christmas Day"},
	{RegionEnglandWales, "2030-12-26", "Boxing Day"},
	{RegionScotland, "2030-12-25", "Christmas Day"},
	{RegionScotland, "2030-12-26", "Boxing Day"},
	{RegionScotland, "2031-01-02", "2nd January"},
	{RegionNorthernIE, "2030-07-12", "Battle of the Boyne (Orangemen’s Day)"},
}

func TestNewMonthsGetBankHolidayClosures(t *testing.T) {
	st, _ := newStore(t)
	if _, err := st.ReplaceBankHolidays(testHolidays); err != nil {
		t.Fatal(err)
	}
	p := mustProg(t, st, 1, 2030, 12)
	got := closuresOf(t, st, 1, p.ID)
	if len(got) != 2 || got[0].StartDate != "2030-12-25" || got[0].EndDate != "2030-12-25" || got[0].Message != "Christmas Day" || got[0].Source != "bank" {
		t.Fatalf("December closures = %+v", got)
	}
	if n := len(closuresOf(t, st, 1, mustProg(t, st, 1, 2030, 11).ID)); n != 0 {
		t.Errorf("November got %d closures", n)
	}
	// The other region's dates are not used.
	if n := len(closuresOf(t, st, 1, mustProg(t, st, 1, 2030, 7).ID)); n != 0 {
		t.Errorf("July got %d closures for England and Wales", n)
	}
}

func TestBankHolidayRegionAndNone(t *testing.T) {
	st, _ := newStore(t)
	st.ReplaceBankHolidays(testHolidays)
	p := mustProg(t, st, 1, 2030, 7)
	jan := mustProg(t, st, 1, 2031, 1)
	if err := st.SetHolidayRegion(1, RegionNorthernIE); err != nil {
		t.Fatal(err)
	}
	if got := closuresOf(t, st, 1, p.ID); len(got) != 1 || got[0].StartDate != "2030-07-12" {
		t.Errorf("after choosing Northern Ireland, July = %+v", got)
	}
	if err := st.SetHolidayRegion(1, RegionScotland); err != nil {
		t.Fatal(err)
	}
	if n := len(closuresOf(t, st, 1, p.ID)); n != 0 {
		t.Errorf("the old region's closure stayed (%d)", n)
	}
	if got := closuresOf(t, st, 1, jan.ID); len(got) != 1 || got[0].Message != "2nd January" {
		t.Errorf("Scotland's January = %+v", got)
	}
	if err := st.SetHolidayRegion(1, RegionNone); err != nil {
		t.Fatal(err)
	}
	if n := len(closuresOf(t, st, 1, jan.ID)); n != 0 {
		t.Errorf("closures remain with the list turned off (%d)", n)
	}
	if err := st.SetHolidayRegion(1, "mars"); err == nil {
		t.Error("an unknown region was accepted")
	}
	// Another squadron is not touched.
	if region, _ := st.SquadronHolidayRegion(2); region != RegionEnglandWales {
		t.Errorf("squadron 2 follows %q", region)
	}
}

func TestUpdatedListChangesExistingMonths(t *testing.T) {
	st, _ := newStore(t)
	st.ReplaceBankHolidays(testHolidays)
	p := mustProg(t, st, 1, 2030, 12)
	manual := mustProg(t, st, 2, 2030, 12)
	if err := st.AddClosure(manual.ID, "2030-12-26", "2030-12-30", "Stand-down"); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyBankHolidays(); err != nil {
		t.Fatal(err)
	}

	same, err := st.ReplaceBankHolidays(testHolidays)
	if err != nil || same {
		t.Fatalf("an identical list reported a change (%v, %v)", same, err)
	}
	// Boxing Day is moved to the 27th, Christmas is renamed and a new day is added.
	changed, err := st.ReplaceBankHolidays([]Holiday{
		{RegionEnglandWales, "2030-12-25", "Christmas"},
		{RegionEnglandWales, "2030-12-27", "Substitute day"},
		{RegionEnglandWales, "2030-12-31", "Extra"},
	})
	if err != nil || !changed {
		t.Fatalf("a changed list reported no change (%v, %v)", changed, err)
	}
	if err := st.ApplyBankHolidays(); err != nil {
		t.Fatal(err)
	}
	got := closuresOf(t, st, 1, p.ID)
	want := map[string]string{"2030-12-25": "Christmas", "2030-12-27": "Substitute day", "2030-12-31": "Extra"}
	if len(got) != len(want) {
		t.Fatalf("closures = %+v", got)
	}
	for _, c := range got {
		if want[c.StartDate] != c.Message {
			t.Errorf("closure %+v unexpected", c)
		}
	}
	// A closure a person made is left alone, and it keeps a bank holiday off its days.
	got = closuresOf(t, st, 2, manual.ID)
	if len(got) != 3 {
		t.Fatalf("squadron 2's closures = %+v", got)
	}
	for _, c := range got {
		if c.StartDate == "2030-12-27" && c.Source == "bank" {
			t.Error("a bank holiday was added inside a closure a person made")
		}
	}
	if _, err := st.ReplaceBankHolidays(nil); err == nil {
		t.Error("an empty list was accepted")
	}
}

func TestDeletedOrEditedBankHolidayClosuresStayThatWay(t *testing.T) {
	st, _ := newStore(t)
	st.ReplaceBankHolidays(testHolidays)
	p := mustProg(t, st, 1, 2030, 12)
	cs := closuresOf(t, st, 1, p.ID)
	if err := st.DeleteClosure(p.ID, cs[0].ID); err != nil { // Christmas Day, taken out
		t.Fatal(err)
	}
	if err := st.UpdateClosure(p.ID, cs[1].ID, "2030-12-26", "2030-12-28", "Our own break"); err != nil {
		t.Fatal(err)
	}
	st.ReplaceBankHolidays(append([]Holiday{{RegionEnglandWales, "2030-12-31", "New"}}, testHolidays...))
	if err := st.ApplyBankHolidays(); err != nil {
		t.Fatal(err)
	}
	got := closuresOf(t, st, 1, p.ID)
	for _, c := range got {
		if c.StartDate == "2030-12-25" {
			t.Error("a deleted bank holiday closure came back")
		}
		if c.StartDate == "2030-12-26" && (c.Source != "" || c.Message != "Our own break" || c.EndDate != "2030-12-28") {
			t.Errorf("an edited closure was changed back: %+v", c)
		}
	}
	if len(got) != 2 {
		t.Errorf("closures = %+v", got)
	}
}

func TestPastMonthsAreNotChangedByAnUpdate(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2030, 12)
	setClock(t, mustDate(t, "2031-03-01"))
	st.ReplaceBankHolidays(testHolidays)
	if err := st.ApplyBankHolidays(); err != nil {
		t.Fatal(err)
	}
	if n := len(closuresOf(t, st, 1, p.ID)); n != 0 {
		t.Errorf("a month in the past got %d closures", n)
	}
}
