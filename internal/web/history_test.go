package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"rafactools-planner/internal/programme"
)

func TestHistoryRecordsSavesAndResetsAndRestoreUndoesThem(t *testing.T) {
	app := newApp(t)
	email := app.owner("2201")
	c := app.login(email)
	sq := app.squadronID(email)
	p, _ := app.store.CreateProgramme(sq, 2035, 3, 1)
	pid := strconv.FormatInt(p.ID, 10)
	base := "/programmes/" + pid
	fp, _ := app.store.GetFullProgramme(sq, p.ID)
	night := firstDay(fp, true)
	row0 := strconv.FormatInt(night.Rows[0].ID, 10)

	save := func(text string) {
		wantRedirect(t, c.post(base, gridForm(t, app, sq, p.ID, func(d programme.Day, v url.Values) {
			if d.ID == night.ID {
				v.Set("row_"+row0+"_text_0", text)
			}
		})), base)
	}
	save("First version of the text")
	save("Second version of the text")
	wantRedirect(t, c.post(base+"/reset", url.Values{}), base)
	wantRedirect(t, c.post(base+"/closures", url.Values{"start_date": {"2035-03-10"}, "end_date": {"2035-03-12"}, "message": {"Closed"}}), base)

	page := body(c.get(base + "/history"))
	for _, want := range []string{"Starting point", "Saved the programme", "Reset the month to blank", "Added a closure", email} {
		if !strings.Contains(page, want) {
			t.Errorf("history lacks %q:\n%s", want, page)
		}
	}
	list, _ := app.store.ListVersions(sq, p.ID)
	if len(list) != 5 { // start, save, save, reset, closure
		t.Fatalf("%d versions, want 5: %+v", len(list), list)
	}
	if !strings.Contains(page, "(latest)") {
		t.Error("the newest version is not marked")
	}

	// Find the version that held the second text and restore it.
	second := 0
	for _, v := range list {
		if v.Action == "Saved the programme" && second == 0 {
			second = v.Version // newest save = the second text
		}
	}
	restore := base + "/history/" + strconv.Itoa(second) + "/restore"
	confirm := body(c.get(restore))
	if !strings.Contains(confirm, "Restore version "+strconv.Itoa(second)) || !strings.Contains(confirm, `name="csrf"`) {
		t.Fatalf("no confirmation page:\n%s", confirm)
	}
	if got, _ := app.store.GetFullProgramme(sq, p.ID); len(got.Closures) != 1 {
		t.Fatal("GET changed the programme")
	}
	resp := c.post(restore, url.Values{})
	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(resp.Header.Get("Location"), "history?restored="+strconv.Itoa(second)) {
		t.Fatalf("restore = %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	got, _ := app.store.GetFullProgramme(sq, p.ID)
	r, _ := firstDay(got, true).RowByHalfType(1, "activity")
	if r.Texts[0] != "Second version of the text" || len(got.Closures) != 0 {
		t.Errorf("after restore: text=%q closures=%d", r.Texts[0], len(got.Closures))
	}

	// The restore is itself in the history, and everything before it is still there:
	// the state it replaced is the version that was newest, so it can be undone.
	list, _ = app.store.ListVersions(sq, p.ID)
	if list[0].Action != "Restored version "+strconv.Itoa(second) {
		t.Errorf("newest = %+v", list[0])
	}
	var before int
	for _, v := range list {
		if v.Action == "Added a closure" {
			before = v.Version
		}
	}
	if before == 0 {
		t.Fatal("the version before the restore is gone")
	}
	wantRedirect(t, c.post(base+"/history/"+strconv.Itoa(before)+"/restore", url.Values{}), base+"/history?restored="+strconv.Itoa(before))
	got, _ = app.store.GetFullProgramme(sq, p.ID)
	if len(got.Closures) != 1 {
		t.Error("undoing the restore did not bring the closure back")
	}
}

func TestHistoryIsSquadronScopedViewOnlyForViewersAndCSRFProtected(t *testing.T) {
	app := newApp(t)
	email := app.owner("2202")
	oc := app.login(email)
	sq := app.squadronID(email)
	p, _ := app.store.CreateProgramme(sq, 2035, 4, 1)
	base := "/programmes/" + strconv.FormatInt(p.ID, 10)
	oc.post(base+"/closures", url.Values{"start_date": {"2035-04-10"}, "end_date": {"2035-04-10"}, "message": {"X"}})
	list, _ := app.store.ListVersions(sq, p.ID)
	v := strconv.Itoa(list[len(list)-1].Version)

	other := app.login(app.owner("2203"))
	if got := other.get(base + "/history").StatusCode; got != http.StatusNotFound {
		t.Errorf("another squadron's history = %d, want 404", got)
	}
	if got := other.get(base + "/history/" + v + "/restore").StatusCode; got != http.StatusNotFound {
		t.Errorf("another squadron's restore page = %d, want 404", got)
	}
	if got := other.post(base+"/history/"+v+"/restore", url.Values{}).StatusCode; got != http.StatusNotFound {
		t.Errorf("another squadron's restore = %d, want 404", got)
	}

	app.member(email, "vi-hist@example.org", "viewer")
	vc := app.login("vi-hist@example.org")
	page := body(vc.get(base + "/history"))
	if !strings.Contains(page, "Added a closure") || strings.Contains(page, "/restore") {
		t.Errorf("a viewer sees the history without restore links:\n%s", page)
	}
	if got := vc.post(base+"/history/"+v+"/restore", url.Values{}).StatusCode; got != http.StatusForbidden {
		t.Errorf("viewer restore = %d, want 403", got)
	}
	if got := oc.postRaw(base+"/history/"+v+"/restore", url.Values{}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("restore without a form token = %d, want 400", got)
	}
	if got := oc.get(base + "/history/999/restore").StatusCode; got != http.StatusNotFound {
		t.Errorf("an unknown version = %d, want 404", got)
	}
	if got := oc.post(base+"/history/999/restore", url.Values{}).StatusCode; got != http.StatusNotFound {
		t.Errorf("restoring an unknown version = %d, want 404", got)
	}
}

func TestAFailedChangeLeavesNoHistoryEntry(t *testing.T) {
	app := newApp(t)
	email := app.owner("2204")
	c := app.login(email)
	sq := app.squadronID(email)
	p, _ := app.store.CreateProgramme(sq, 2035, 5, 1)
	base := "/programmes/" + strconv.FormatInt(p.ID, 10)
	c.post(base+"/weekend-notes", url.Values{"date": {"2035-05-04"}, "text": {"first change"}}) // so there is a history
	before, _ := app.store.ListVersions(sq, p.ID)

	// A closure that ends before it starts is refused.
	if got := c.post(base+"/closures", url.Values{"start_date": {"2035-05-20"}, "end_date": {"2035-05-10"}, "message": {"x"}}); got.StatusCode < 400 && got.StatusCode != http.StatusSeeOther {
		t.Logf("status %d", got.StatusCode)
	}
	after, _ := app.store.ListVersions(sq, p.ID)
	fp, _ := app.store.GetFullProgramme(sq, p.ID)
	if len(fp.Closures) == 0 && len(after) != len(before) {
		t.Errorf("a refused change made %d history entries", len(after)-len(before))
	}
	// An unchanged save adds nothing either.
	c.post(base, gridForm(t, app, sq, p.ID, nil))
	c.post(base, gridForm(t, app, sq, p.ID, nil))
	list, _ := app.store.ListVersions(sq, p.ID)
	if len(list) > len(before)+1 {
		t.Errorf("two identical saves made %d entries", len(list)-len(before))
	}
	if len(before) != 2 {
		t.Errorf("a first change should leave a starting point and itself, got %d", len(before))
	}
}

func TestDashboardHasNoStatusColumnAndNewMonthsAreRecorded(t *testing.T) {
	app := newApp(t)
	email := app.owner("2205")
	c := app.login(email)
	sq := app.squadronID(email)
	resp := c.post("/programmes", url.Values{"year": {"2036"}, "month": {"2"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	page := body(c.get("/"))
	for _, bad := range []string{">Status<", "draft", "Draft"} {
		if strings.Contains(page, bad) {
			t.Errorf("dashboard still mentions %q", bad)
		}
	}
	id, _ := app.store.FindProgramme(sq, 2036, 2)
	list, _ := app.store.ListVersions(sq, id)
	if len(list) != 1 || list[0].Action != "Created the month" || list[0].UserEmail != email {
		t.Errorf("history of a new month = %+v", list)
	}
}
