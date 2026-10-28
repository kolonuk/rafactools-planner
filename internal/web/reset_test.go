package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestResetProgrammeNeedsConfirmationAndEditorAndSameSquadron(t *testing.T) {
	app := newApp(t)
	email := app.owner("1204")
	sq := app.squadronID(email)
	owner := app.login(email)
	p, _ := app.store.CreateProgramme(sq, 2030, 5, 1)
	app.store.AddClosure(p.ID, "2030-05-01", "2030-05-02", "Bank holiday")
	id := strconv.FormatInt(p.ID, 10)

	if page := body(owner.get("/programmes/" + id)); !strings.Contains(page, "/programmes/"+id+"/reset") {
		t.Error("edit page has no reset link")
	}
	confirm := body(owner.get("/programmes/" + id + "/reset"))
	if !strings.Contains(confirm, "May 2030") || !strings.Contains(confirm, `name="csrf"`) {
		t.Errorf("confirmation page incomplete: %s", confirm)
	}
	if fp, _ := app.store.GetFullProgramme(sq, p.ID); len(fp.Closures) != 1 {
		t.Fatal("GET reset the programme")
	}

	other := app.login(app.owner("1205"))
	if got := other.get("/programmes/" + id + "/reset").StatusCode; got != http.StatusNotFound {
		t.Errorf("other squadron GET = %d, want 404", got)
	}
	if got := other.post("/programmes/"+id+"/reset", url.Values{}).StatusCode; got != http.StatusNotFound {
		t.Errorf("other squadron POST = %d, want 404", got)
	}

	app.member(email, "viewer-reset@example.org", "viewer")
	viewer := app.login("viewer-reset@example.org")
	if got := viewer.post("/programmes/"+id+"/reset", url.Values{}).StatusCode; got != http.StatusForbidden {
		t.Errorf("viewer POST = %d, want 403", got)
	}
	if fp, _ := app.store.GetFullProgramme(sq, p.ID); len(fp.Closures) != 1 {
		t.Fatal("a refused request changed the programme")
	}

	if got := owner.postRaw("/programmes/"+id+"/reset", url.Values{}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("POST without CSRF token = %d, want 400", got)
	}
	resp := owner.post("/programmes/"+id+"/reset", url.Values{})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("owner POST = %d, want 303", resp.StatusCode)
	}
	if fp, err := app.store.GetFullProgramme(sq, p.ID); err != nil || len(fp.Closures) != 0 || len(fp.Days) == 0 {
		t.Errorf("after reset: err=%v closures=%d days=%d", err, len(fp.Closures), len(fp.Days))
	}
}
