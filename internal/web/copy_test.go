package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"rafactools-planner/internal/programme"
)

func TestCopyAnEarlierMonth(t *testing.T) {
	app := newApp(t)
	email := app.owner("1601")
	app.member(email, "viewer1601@example.org", "viewer")
	sq := app.squadronID(email)
	c := app.login(email)
	apr, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	may, _ := app.store.CreateProgramme(sq, 2030, 5, 1)
	parade, _, _ := firstParadeAndPlain(t, app, sq, apr.ID)
	if err := app.store.SaveDaysAndRows(sq, apr.ID, []programme.DayInput{{ID: parade.ID, IsParade: true, Rows: []programme.RowInput{{ID: parade.Rows[0].ID, Texts: []string{"Shooting", "", "", ""}}}}}); err != nil {
		t.Fatal(err)
	}
	mayID, aprID := strconv.FormatInt(may.ID, 10), strconv.FormatInt(apr.ID, 10)

	page := body(c.get("/programmes/" + mayID + "/copy"))
	if !strings.Contains(page, "April 2030") || strings.Contains(page, "May 2030 <span") {
		t.Errorf("the list of months is wrong:\n%.800s", page)
	}
	if !strings.Contains(body(c.get("/programmes/"+mayID)), "/copy") {
		t.Error("the month page has no Copy link")
	}
	// An earlier month cannot be copied from a later one.
	wantStatus(t, c.post("/programmes/"+aprID+"/copy", url.Values{"source": {mayID}}), http.StatusBadRequest)
	wantStatus(t, c.post("/programmes/"+mayID+"/copy", url.Values{}), http.StatusBadRequest)
	wantStatus(t, app.login("viewer1601@example.org").post("/programmes/"+mayID+"/copy", url.Values{"source": {aprID}}), http.StatusForbidden)
	stranger := app.login(app.owner("1602"))
	wantStatus(t, stranger.post("/programmes/"+mayID+"/copy", url.Values{"source": {aprID}}), http.StatusNotFound)

	wantRedirect(t, c.post("/programmes/"+mayID+"/copy", url.Values{"source": {aprID}}), "/programmes/"+mayID)
	if !strings.Contains(body(c.get("/programmes/"+mayID)), `value="Shooting"`) {
		t.Error("the activity did not arrive")
	}

	// Now May has entries: a second copy needs the box ticked.
	resp := c.post("/programmes/"+mayID+"/copy", url.Values{"source": {aprID}})
	wantStatus(t, resp, http.StatusBadRequest)
	if b := body(c.get("/programmes/" + mayID + "/copy")); !strings.Contains(b, "already has entries") {
		t.Error("the page does not warn that entries will be replaced")
	}
	wantRedirect(t, c.post("/programmes/"+mayID+"/copy", url.Values{"source": {aprID}, "confirm": {"1"}}), "/programmes/"+mayID)

	versions, _ := app.store.ListVersions(sq, may.ID)
	found := false
	for _, v := range versions {
		found = found || strings.HasPrefix(v.Action, "Copied an earlier month")
	}
	if !found {
		t.Error("the copy is not in the history")
	}
}
