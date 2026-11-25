package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestActivityListPageAndGridSuggestions(t *testing.T) {
	app := newApp(t)
	email := app.owner("1701")
	app.member(email, "viewer1701@example.org", "viewer")
	sq := app.squadronID(email)
	c := app.login(email)
	p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	pid := strconv.FormatInt(p.ID, 10)

	wantStatus(t, app.login("viewer1701@example.org").get("/activities"), http.StatusForbidden)
	wantStatus(t, app.login("viewer1701@example.org").post("/activities", url.Values{"title": {"Nope"}}), http.StatusForbidden)

	wantRedirect(t, c.post("/activities", url.Values{"title": {"Fieldcraft"}}), "/activities")
	wantStatus(t, c.post("/activities", url.Values{"title": {"bowling"}}), http.StatusBadRequest)
	wantStatus(t, c.post("/activities", url.Values{"title": {"fieldcraft"}}), http.StatusBadRequest)

	page := body(c.get("/programmes/" + pid))
	for _, want := range []string{`<datalist id="activity-options">`, `value="Laser Quest"`, `value="Fieldcraft"`, `list="activity-options"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the grid lacks %q", want)
		}
	}
	// Another squadron does not get this squadron's own activity.
	other := app.login(app.owner("1702"))
	op, _ := app.store.CreateProgramme(app.squadronID("oc.1702@rafac.mod.gov.uk"), 2030, 4, 1)
	if strings.Contains(body(other.get("/programmes/"+strconv.FormatInt(op.ID, 10))), `value="Fieldcraft"`) {
		t.Error("another squadron is offered this squadron's activity")
	}

	list, _ := app.store.ListCustomActivities(sq)
	id := strconv.FormatInt(list[0].ID, 10)
	wantRedirect(t, c.post("/activities/"+id+"/edit", url.Values{"title": {"Fieldcraft weekend"}}), "/activities")
	wantStatus(t, other.post("/activities/"+id+"/delete", url.Values{}), http.StatusNotFound)
	if !strings.Contains(body(c.get("/activities")), "Fieldcraft weekend") {
		t.Error("the rename is not shown")
	}
	wantRedirect(t, c.post("/activities/"+id+"/delete", url.Values{}), "/activities")
	if left, _ := app.store.ListCustomActivities(sq); len(left) != 0 {
		t.Error("not deleted")
	}
}
