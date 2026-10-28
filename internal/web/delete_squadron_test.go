package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDeleteSquadronNeedsOwnerPasswordAndTypedNumber(t *testing.T) {
	ts := seedTwoSquadrons(t)
	app := ts.a
	sqA := app.squadronID(ts.ownerAEmail)
	sqB := app.squadronID("oc.502@rafac.mod.gov.uk")
	app.member(ts.ownerAEmail, "editor-del@example.org", "editor")
	editor := app.login("editor-del@example.org")

	if page := body(ts.ownerA.get("/squadron")); !strings.Contains(page, "Delete this squadron") {
		t.Error("owner's settings page has no delete section")
	}
	// post() adds the CSRF token to the map it is given, so build a fresh one each time.
	good := func() url.Values { return url.Values{"confirm_number": {"501"}, "password": {testPassword}} }

	// Not an editor, not without the password, not with the wrong number.
	if got := editor.post("/squadron/delete", good()).StatusCode; got != http.StatusForbidden {
		t.Errorf("editor = %d, want 403", got)
	}
	for name, v := range map[string]url.Values{
		"wrong password": {"confirm_number": {"501"}, "password": {"not the password at all"}},
		"wrong number":   {"confirm_number": {"502"}, "password": {testPassword}},
		"no number":      {"password": {testPassword}},
	} {
		if got := ts.ownerA.post("/squadron/delete", v).StatusCode; got != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, got)
		}
	}
	if got := ts.ownerA.postRaw("/squadron/delete", good()).StatusCode; got != http.StatusBadRequest {
		t.Errorf("no CSRF token = %d, want 400", got)
	}
	if _, err := app.store.GetSquadron(sqA); err != nil {
		t.Fatalf("a refused request deleted the squadron: %v", err)
	}

	// The owner of A succeeds; only A goes.
	resp := ts.ownerA.post("/squadron/delete", good())
	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(resp.Header.Get("Location"), "deleted=1") {
		t.Fatalf("delete = %d to %q: %s", resp.StatusCode, resp.Header.Get("Location"), body(resp))
	}
	if _, err := app.store.GetSquadron(sqA); err == nil {
		t.Error("squadron A still exists")
	}
	if _, err := app.store.GetSquadron(sqB); err != nil {
		t.Errorf("squadron B was affected: %v", err)
	}
	var users, programmes int
	app.db.QueryRow(`SELECT COUNT(*) FROM users WHERE squadron_id = ?`, sqA).Scan(&users)
	app.db.QueryRow(`SELECT COUNT(*) FROM programmes WHERE squadron_id = ?`, sqA).Scan(&programmes)
	if users != 0 || programmes != 0 {
		t.Errorf("left behind: %d users, %d programmes", users, programmes)
	}
	if got := editor.get("/").StatusCode; got == http.StatusOK {
		t.Error("a deleted squadron's member is still signed in")
	}
	if ts.ownerB.get("/").StatusCode != http.StatusOK {
		t.Error("squadron B's owner was signed out")
	}
}

func TestMasterHomeSquadronCannotBeDeleted(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	got := master.post("/squadron/delete", url.Values{"confirm_number": {"master"}, "password": {testPassword}})
	if got.StatusCode == http.StatusSeeOther {
		t.Error("the master admin's home squadron was deleted")
	}
	if n := masterCountWeb(ts); n != 1 {
		t.Errorf("%d master admins left, want 1", n)
	}
}

func masterCountWeb(ts twoSquadrons) int {
	var n int
	ts.a.db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_master_admin = 1`).Scan(&n)
	return n
}
