package web

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"rafactools-planner/internal/programme"
)

// The in-place edits (closures, weekend notes, requirement categories and
// entries): they change what they should, refuse blank text, keep to the
// squadron and the month they belong to, need an editor, and leave a line in
// the history.
func TestInPlaceEditsOfClosuresNotesAndRequirements(t *testing.T) {
	app := newApp(t)
	email := app.owner("1401")
	app.member(email, "viewer1401@example.org", "viewer")
	sq := app.squadronID(email)
	c := app.login(email)

	p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	other, _ := app.store.CreateProgramme(sq, 2030, 5, 1)
	pid, oid := strconv.FormatInt(p.ID, 10), strconv.FormatInt(other.ID, 10)
	if err := app.store.AddClosure(p.ID, "2030-04-08", "2030-04-08", "Stand-down"); err != nil {
		t.Fatal(err)
	}
	if err := app.store.AddWeekendNote(p.ID, "2030-04-13", "Camp"); err != nil {
		t.Fatal(err)
	}
	catID, err := app.store.AddRequirementCategory(p.ID, "Fitness", "due June")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.AddRequirementEntry(p.ID, catID, "Cdt Smith", "needs test"); err != nil {
		t.Fatal(err)
	}
	full := func() *programme.FullProgramme {
		fp, err := app.store.GetFullProgramme(sq, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return fp
	}
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	closure, note := full().Closures[0], full().WeekendNotes[0]
	entry := full().Requirements[0].Entries[0]

	base := "/programmes/" + pid
	closureURL := base + "/closures/" + id(closure.ID) + "/edit"
	noteURL := base + "/weekend-notes/" + id(note.ID) + "/edit"
	catURL := base + "/requirements/" + id(catID) + "/edit"
	entryURL := base + "/requirements/" + id(catID) + "/entries/" + id(entry.ID) + "/edit"

	t.Run("a good edit is saved and recorded in the history", func(t *testing.T) {
		wantRedirect(t, c.post(closureURL, url.Values{"start_date": {"2030-04-09"}, "end_date": {"2030-04-10"}, "message": {" Flood "}}), base)
		wantRedirect(t, c.post(noteURL, url.Values{"date": {"2030-04-14"}, "text": {"Camp (moved)"}}), base)
		wantRedirect(t, c.post(catURL, url.Values{"name": {"Fitness test"}, "due_label": {"due July"}}), base)
		wantRedirect(t, c.post(entryURL, url.Values{"cadet_name": {"Cdt Jones"}, "note": {"passed"}}), base)

		fp := full()
		if got := fp.Closures[0]; got.StartDate != "2030-04-09" || got.EndDate != "2030-04-10" || got.Message != "Flood" {
			t.Errorf("closure = %+v", got)
		}
		if got := fp.WeekendNotes[0]; got.Date != "2030-04-14" || got.Text != "Camp (moved)" {
			t.Errorf("weekend note = %+v", got)
		}
		if got := fp.Requirements[0]; got.Name != "Fitness test" || got.DueLabel != "due July" {
			t.Errorf("category = %+v", got)
		}
		if got := fp.Requirements[0].Entries[0]; got.CadetName != "Cdt Jones" || got.Note != "passed" {
			t.Errorf("entry = %+v", got)
		}
		versions, err := app.store.ListVersions(sq, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, v := range versions {
			seen[v.Action] = true
		}
		for _, want := range []string{"Edited a closure", "Edited a weekend note", "Edited a requirement category", "Edited a requirement entry"} {
			if !seen[want] {
				t.Errorf("the history has no %q line", want)
			}
		}
	})

	t.Run("blank or invalid text is refused and nothing changes", func(t *testing.T) {
		before := full()
		wantStatus(t, c.post(closureURL, url.Values{"start_date": {"2030-04-09"}, "end_date": {"2030-04-10"}, "message": {"   "}}), http.StatusBadRequest)
		wantStatus(t, c.post(closureURL, url.Values{"start_date": {"2030-04-10"}, "end_date": {"2030-04-09"}, "message": {"Back to front"}}), http.StatusBadRequest)
		wantStatus(t, c.post(noteURL, url.Values{"date": {"2030-04-14"}, "text": {""}}), http.StatusBadRequest)
		wantStatus(t, c.post(noteURL, url.Values{"date": {"not a date"}, "text": {"x"}}), http.StatusBadRequest)
		wantStatus(t, c.post(catURL, url.Values{"name": {" "}, "due_label": {"x"}}), http.StatusBadRequest)
		wantStatus(t, c.post(entryURL, url.Values{"cadet_name": {""}, "note": {"x"}}), http.StatusBadRequest)
		after := full()
		if after.Closures[0] != before.Closures[0] || after.WeekendNotes[0] != before.WeekendNotes[0] ||
			after.Requirements[0].Name != before.Requirements[0].Name || after.Requirements[0].Entries[0] != before.Requirements[0].Entries[0] {
			t.Error("a refused edit still changed something")
		}
	})

	t.Run("an entry cannot be edited through a different month", func(t *testing.T) {
		wrong := "/programmes/" + oid
		wantStatus(t, c.post(wrong+"/closures/"+id(closure.ID)+"/edit", url.Values{"start_date": {"2030-05-01"}, "end_date": {"2030-05-01"}, "message": {"Hijack"}}), http.StatusNotFound)
		wantStatus(t, c.post(wrong+"/weekend-notes/"+id(note.ID)+"/edit", url.Values{"date": {"2030-05-04"}, "text": {"Hijack"}}), http.StatusNotFound)
		wantStatus(t, c.post(wrong+"/requirements/"+id(catID)+"/edit", url.Values{"name": {"Hijack"}}), http.StatusNotFound)
		wantStatus(t, c.post(wrong+"/requirements/"+id(catID)+"/entries/"+id(entry.ID)+"/edit", url.Values{"cadet_name": {"Hijack"}}), http.StatusNotFound)
		fp := full()
		if fp.Closures[0].Message == "Hijack" || fp.WeekendNotes[0].Text == "Hijack" || fp.Requirements[0].Name == "Hijack" || fp.Requirements[0].Entries[0].CadetName == "Hijack" {
			t.Error("an entry was changed through another month")
		}
	})

	t.Run("another squadron cannot edit them", func(t *testing.T) {
		stranger := app.login(app.owner("1402"))
		wantStatus(t, stranger.post(closureURL, url.Values{"start_date": {"2030-04-09"}, "end_date": {"2030-04-10"}, "message": {"Hijack"}}), http.StatusNotFound)
		wantStatus(t, stranger.post(entryURL, url.Values{"cadet_name": {"Hijack"}}), http.StatusNotFound)
		if full().Closures[0].Message == "Hijack" || full().Requirements[0].Entries[0].CadetName == "Hijack" {
			t.Error("another squadron changed this month")
		}
	})

	t.Run("a viewer cannot edit", func(t *testing.T) {
		v := app.login("viewer1401@example.org")
		wantStatus(t, v.post(closureURL, url.Values{"start_date": {"2030-04-09"}, "end_date": {"2030-04-10"}, "message": {"Hijack"}}), http.StatusForbidden)
		wantStatus(t, v.post(noteURL, url.Values{"date": {"2030-04-14"}, "text": {"Hijack"}}), http.StatusForbidden)
		if full().Closures[0].Message == "Hijack" {
			t.Error("a viewer changed a closure")
		}
	})
}
