package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func displayNameOf(t *testing.T, app *testApp, email string) string {
	t.Helper()
	var name string
	if err := app.db.QueryRow(`SELECT display_name FROM users WHERE email = ?`, email).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestClaimAndInviteFormsTakeAnOptionalName(t *testing.T) {
	app := newApp(t)

	anon := app.anon()
	tok := scrapeCSRF(t, anon.get("/claim"))
	if !strings.Contains(body(anon.get("/claim")), `name="display_name"`) {
		t.Error("the claim form has no name box")
	}
	resp := anon.postRaw("/claim", url.Values{"csrf": {tok}, "squadron_name": {"7001 Sqn"}, "email": {"oc.7001@rafac.mod.gov.uk"},
		"password": {testPassword}, "display_name": {"  Fg Off   Smith "}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("claim -> %d", resp.StatusCode)
	}
	if got := displayNameOf(t, app, "oc.7001@rafac.mod.gov.uk"); got != "Fg Off Smith" {
		t.Errorf("name = %q, want it tidied", got)
	}

	// A name that is too long is refused before anything is created.
	anon = app.anon()
	tok = scrapeCSRF(t, anon.get("/claim"))
	resp = anon.postRaw("/claim", url.Values{"csrf": {tok}, "squadron_name": {"7002 Sqn"}, "email": {"oc.7002@rafac.mod.gov.uk"},
		"password": {testPassword}, "display_name": {strings.Repeat("x", 81)}})
	if !strings.Contains(body(resp), "at most 80 characters") {
		t.Error("a too-long name was not refused")
	}
	if n := count(t, app.db, `SELECT COUNT(*) FROM users WHERE email = 'oc.7002@rafac.mod.gov.uk'`); n != 0 {
		t.Error("an account was made for a refused name")
	}

	// The name box is optional on an invitation too.
	owner := "oc.7001@rafac.mod.gov.uk"
	var sqID, ownerID int64
	if err := app.db.QueryRow(`SELECT squadron_id, id FROM users WHERE email = ?`, owner).Scan(&sqID, &ownerID); err != nil {
		t.Fatal(err)
	}
	invite, err := app.auth.CreateInvite(sqID, ownerID, "named@example.org", "editor")
	if err != nil {
		t.Fatal(err)
	}
	c := app.anon()
	page := body(c.get("/invite/" + invite))
	if !strings.Contains(page, `name="display_name"`) {
		t.Error("the invite form has no name box")
	}
	csrf := scrapeCSRF(t, c.get("/invite/"+invite))
	wantRedirect(t, c.postRaw("/invite/"+invite, url.Values{"csrf": {csrf}, "password": {testPassword}, "display_name": {"Mr Jones"}}), "/")
	if got := displayNameOf(t, app, "named@example.org"); got != "Mr Jones" {
		t.Errorf("invited name = %q", got)
	}
}

func TestInstructorsPageListsMembersAsFixedEntries(t *testing.T) {
	app := newApp(t)
	owner := app.owner("7003")
	oc := app.login(owner)
	app.member(owner, "named-ed@example.org", "editor")
	app.member(owner, "gone@example.org", "viewer")
	var id int64
	app.db.QueryRow(`SELECT id FROM users WHERE email = 'named-ed@example.org'`).Scan(&id)
	if err := app.auth.SetDisplayName(id, "Flt Lt Editor"); err != nil {
		t.Fatal(err)
	}
	var goneID int64
	app.db.QueryRow(`SELECT id FROM users WHERE email = 'gone@example.org'`).Scan(&goneID)
	if err := app.auth.SetMemberActive(app.squadronID(owner), goneID, ownerIDOf(t, app, owner), false); err != nil {
		t.Fatal(err)
	}

	page := body(oc.get("/instructors"))
	section := page[strings.Index(page, "People with a login"):]
	section = section[:strings.Index(section, "Your list")]
	if !strings.Contains(section, "Flt Lt Editor") || !strings.Contains(section, owner) {
		t.Errorf("members are not listed as fixed entries:\n%s", section)
	}
	if strings.Contains(section, "gone@example.org") {
		t.Error("an inactive member is listed")
	}
	if strings.Contains(section, "<form") || strings.Contains(section, "<button") || strings.Contains(section, "<a ") {
		t.Errorf("fixed entries offer edit or remove:\n%s", section)
	}
}

func TestAClosureLocksItsDatesInTheGrid(t *testing.T) {
	app := newApp(t)
	owner := app.owner("7004")
	oc := app.login(owner)
	sq := app.squadronID(owner)
	var userID int64
	app.db.QueryRow(`SELECT id FROM users WHERE email = ?`, owner).Scan(&userID)
	p, err := app.store.CreateProgramme(sq, 2031, 5, userID)
	if err != nil {
		t.Fatal(err)
	}
	fp, _ := app.store.GetFullProgramme(sq, p.ID)
	var closed, open int64
	for _, d := range fp.Days {
		if d.IsParade && closed == 0 {
			closed = d.ID
		} else if d.IsParade && open == 0 {
			open = d.ID
		}
	}
	var closedDate string
	for _, d := range fp.Days {
		if d.ID == closed {
			closedDate = d.Date
		}
	}
	if err := app.store.AddClosure(p.ID, closedDate, closedDate, "Stand-down"); err != nil {
		t.Fatal(err)
	}

	page := body(oc.get("/programmes/" + strconv.FormatInt(p.ID, 10)))
	cid := strconv.FormatInt(closed, 10)
	if !strings.Contains(page, `class="closed-row"`) || !strings.Contains(page, "Stand-down") {
		t.Error("the closed date is not shown as closed")
	}
	if strings.Contains(page, `name="day_`+cid+`_`) {
		t.Error("the closed date still has inputs in the grid")
	}
	if strings.Contains(page, `<option value="`+cid+`">`) {
		t.Error("the closed date is offered as a place to move to")
	}
	oid := strconv.FormatInt(open, 10)
	if !strings.Contains(page, `name="day_`+oid+`_uniform"`) {
		t.Error("an open date lost its inputs")
	}

	// A forged save cannot change the closed date, and a move to or from it is refused.
	form := url.Values{"day_" + cid + "_present": {"1"}, "day_" + cid + "_parade": {"1"}, "day_" + cid + "_notes": {"sneaky"}}
	oc.post("/programmes/"+strconv.FormatInt(p.ID, 10), form)
	if n := count(t, app.db, `SELECT COUNT(*) FROM programme_days WHERE id = ? AND notes = 'sneaky'`, closed); n != 0 {
		t.Error("a forged save changed a closed date")
	}
	resp := oc.post("/programmes/"+strconv.FormatInt(p.ID, 10)+"/days/"+oid+"/move", url.Values{"target_day": {cid}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("move onto a closed date -> %d, want 400", resp.StatusCode)
	}
}

func ownerIDOf(t *testing.T, app *testApp, email string) int64 {
	t.Helper()
	var id int64
	if err := app.db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
