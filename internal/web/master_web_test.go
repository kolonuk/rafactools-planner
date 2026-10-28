package web

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// masterApp seeds two ordinary squadrons and a master admin who signs in
// through the real login form. testPassword is long enough for the rules.
func masterApp(t *testing.T) (ts twoSquadrons, master *client, sqA, sqB int64) {
	t.Helper()
	ts = seedTwoSquadrons(t)
	if _, err := ts.a.auth.EnsureMasterAdmin("master@example.org", testPassword); err != nil {
		t.Fatal(err)
	}
	master = ts.a.login("master@example.org")
	return ts, master, ts.a.squadronID(ts.ownerAEmail), ts.a.squadronID("oc.502@rafac.mod.gov.uk")
}

func TestOnlyTheMasterAdminSeesOrUsesTheSwitcher(t *testing.T) {
	ts, master, sqA, _ := masterApp(t)

	if strings.Contains(body(ts.ownerA.get("/account")), "Switch squadron") {
		t.Error("an ordinary owner is shown the squadron switcher")
	}
	resp := ts.ownerA.post("/account/switch", url.Values{"squadron_id": {strconv.FormatInt(ts.a.squadronID("oc.502@rafac.mod.gov.uk"), 10)}})
	wantStatus(t, resp, 403)
	if page := body(ts.ownerA.get("/")); strings.Contains(page, "Master admin:") {
		t.Error("an ordinary owner was switched")
	}
	// the forged post did not touch their session either
	if strings.Contains(body(ts.ownerA.get("/")), "502") {
		t.Error("an ordinary owner now sees another squadron")
	}

	page := body(master.get("/account"))
	for _, want := range []string{"Switch squadron", "501 – ", "502 – "} {
		if !strings.Contains(page, want) {
			t.Errorf("master's page lacks %q", want)
		}
	}
	if strings.Contains(page, "Master admin</option>") || strings.Contains(page, "master –") {
		t.Error("the hidden home squadron is offered in the list")
	}
	_ = sqA
}

func TestMasterAdminViewsAnotherSquadronReadOnly(t *testing.T) {
	ts, master, sqA, sqB := masterApp(t)
	app := ts.a
	idA := strconv.FormatInt(sqA, 10)

	wantRedirect(t, master.post("/account/switch", url.Values{"squadron_id": {idA}}), "/")

	t.Run("sees that squadron's programmes, with a banner", func(t *testing.T) {
		page := body(master.get("/"))
		if !strings.Contains(page, "501 Sqn") || !strings.Contains(page, "May 2030") {
			t.Errorf("dashboard is not squadron A's:\n%.600s", page)
		}
		if !strings.Contains(page, "Master admin: you are viewing") || !strings.Contains(page, "read-only") {
			t.Error("no banner while viewing another squadron")
		}
		if strings.Contains(page, "502") {
			t.Error("squadron B leaks into A's view")
		}
		wantStatus(t, master.get("/programmes/"+ts.progA), 200)
		resp := master.get("/programmes/" + ts.progA + "/pdf")
		wantStatus(t, resp, 200)
		if strings.Contains(body(master.get("/programmes/"+ts.progA)), `class="grid-form"`) {
			t.Error("the edit grid is shown to a read-only viewer")
		}
	})
	t.Run("cannot see squadron B's data from A's view", func(t *testing.T) {
		wantStatus(t, master.get("/programmes/"+ts.progB), 404)
	})
	t.Run("cannot change anything", func(t *testing.T) {
		before := count(t, app.db, `SELECT (SELECT COUNT(*) FROM programmes)+(SELECT COUNT(*) FROM programme_closures)+(SELECT COUNT(*) FROM instructors)+(SELECT COUNT(*) FROM invites)+(SELECT COUNT(*) FROM users)`)
		for _, p := range []struct {
			path string
			form url.Values
		}{
			{"/programmes", url.Values{"year": {"2031"}, "month": {"1"}}},
			{"/programmes/" + ts.progA, nil},
			{"/programmes/" + ts.progA + "/closures", url.Values{"start_date": {"2030-05-01"}, "end_date": {"2030-05-02"}, "message": {"x"}}},
			{"/instructors", url.Values{"name": {"Intruder"}}},
			{"/squadron", url.Values{"name": {"Renamed"}}},
			{"/squadron/invites", url.Values{"email": {"x@example.org"}, "role": {"owner"}}},
			{"/squadron/members/1/role", url.Values{"role": {"viewer"}}},
			{"/squadron/members/1/remove", nil},
		} {
			resp := master.post(p.path, p.form)
			if resp.StatusCode != 403 {
				t.Errorf("POST %s -> %d, want 403", p.path, resp.StatusCode)
			}
			resp.Body.Close()
		}
		wantStatus(t, master.get("/squadron"), 403)
		wantStatus(t, master.get("/instructors"), 403)
		after := count(t, app.db, `SELECT (SELECT COUNT(*) FROM programmes)+(SELECT COUNT(*) FROM programme_closures)+(SELECT COUNT(*) FROM instructors)+(SELECT COUNT(*) FROM invites)+(SELECT COUNT(*) FROM users)`)
		if before != after {
			t.Errorf("rows changed: %d -> %d", before, after)
		}
		if sq, _ := app.store.GetSquadron(sqA); sq.Name == "Renamed" {
			t.Error("squadron renamed by a viewer")
		}
	})
	t.Run("switch to the other squadron, then back", func(t *testing.T) {
		wantRedirect(t, master.post("/account/switch", url.Values{"squadron_id": {strconv.FormatInt(sqB, 10)}}), "/")
		if page := body(master.get("/")); !strings.Contains(page, "502 Sqn") || strings.Contains(page, "501 Sqn") {
			t.Error("did not switch to squadron B")
		}
		wantRedirect(t, master.post("/account/switch", url.Values{"back": {"1"}}), "/account")
		if strings.Contains(body(master.get("/")), "Master admin: you are viewing") {
			t.Error("banner still shown after switching back")
		}
	})
}

func TestMasterAdminSwitchValidation(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	wantStatus(t, master.post("/account/switch", url.Values{"squadron_id": {"99999"}}), 404)
	wantStatus(t, master.post("/account/switch", url.Values{"squadron_id": {"abc"}}), 400)
	wantStatus(t, master.post("/account/switch", url.Values{}), 400)
	wantStatus(t, master.postRaw("/account/switch", url.Values{"csrf": {"forged"}, "squadron_id": {"1"}}), 400)
	wantStatus(t, ts.a.anon().postRaw("/account/switch", url.Values{"squadron_id": {"1"}}), 303)

	// the hidden home squadron is not a valid target
	var home int64
	ts.a.db.QueryRow(`SELECT id FROM squadrons WHERE number = 'master'`).Scan(&home)
	wantStatus(t, master.post("/account/switch", url.Values{"squadron_id": {strconv.FormatInt(home, 10)}}), 404)
}

func TestMasterAdminDoesNotAppearInAnyMemberList(t *testing.T) {
	ts, master, sqA, _ := masterApp(t)
	wantRedirect(t, master.post("/account/switch", url.Values{"squadron_id": {strconv.FormatInt(sqA, 10)}}), "/")
	// The security log (below the members) is meant to name the master admin
	// as the one who looked; the member and invite tables must not.
	page := body(ts.ownerA.get("/squadron"))
	members, _, _ := strings.Cut(page, "Security log")
	if strings.Contains(members, "master@example.org") {
		t.Error("the master admin is listed as a member of an ordinary squadron")
	}
	// the master admin cannot be invited-into or claimed via the normal flows
	resp := ts.ownerA.post("/squadron/invites", url.Values{"email": {"master@example.org"}, "role": {"owner"}})
	if resp.StatusCode == 303 {
		// the invite itself may be accepted for sending, but accepting it must fail
		var tok string
		ts.a.db.QueryRow(`SELECT token FROM invites WHERE email = 'master@example.org'`).Scan(&tok)
		cl := ts.a.anon()
		csrf := scrapeCSRF(t, cl.get("/invite/"+tok))
		r2 := cl.postRaw("/invite/"+tok, url.Values{"csrf": {csrf}, "password": {"a long enough password"}})
		if r2.StatusCode != 400 {
			t.Errorf("accepting an invite for the master admin's email -> %d, want 400", r2.StatusCode)
		}
		r2.Body.Close()
	}
	resp.Body.Close()
	if count(t, ts.a.db, `SELECT COUNT(*) FROM users WHERE is_master_admin = 1`) != 1 {
		t.Error("master admin count changed")
	}
}
