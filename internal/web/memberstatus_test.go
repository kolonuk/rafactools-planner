package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func memberID(t *testing.T, app *testApp, email string) string {
	t.Helper()
	var id int64
	app.db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&id)
	if id == 0 {
		t.Fatalf("no user %s", email)
	}
	return strconv.FormatInt(id, 10)
}

func TestOwnerCanMarkAMemberInactiveAndActive(t *testing.T) {
	app := newApp(t)
	owner := app.owner("1901")
	oc := app.login(owner)
	app.member(owner, "ed-act@example.org", "editor")
	editor := app.login("ed-act@example.org")
	id := memberID(t, app, "ed-act@example.org")

	page := body(oc.get("/squadron"))
	if !strings.Contains(page, "/squadron/members/"+id+"/active") || !strings.Contains(page, "Send temporary password") {
		t.Fatalf("the member row has no actions:\n%s", page)
	}
	if strings.Contains(page, "/squadron/members/"+memberID(t, app, owner)+"/active") {
		t.Error("the owner is offered actions on their own row")
	}

	wantRedirect(t, oc.post("/squadron/members/"+id+"/active", url.Values{"active": {"0"}}), "/squadron")
	if got := editor.get("/").StatusCode; got == http.StatusOK {
		t.Error("an inactive member's session still works")
	}
	// Signing in: the right password says why, a wrong one says nothing special.
	signIn := func(pw string) (int, string) {
		c := app.anon()
		resp := c.postRaw("/login", url.Values{"csrf": {scrapeCSRF(t, c.get("/login"))}, "email": {"ed-act@example.org"}, "password": {pw}})
		return resp.StatusCode, body(resp)
	}
	if code, msg := signIn(testPassword); code != http.StatusUnauthorized || !strings.Contains(msg, "inactive") {
		t.Errorf("inactive, right password = %d %q", code, msg)
	}
	if code, msg := signIn("wrong password here"); code != http.StatusUnauthorized || strings.Contains(msg, "inactive") {
		t.Errorf("inactive, wrong password = %d: it must not reveal the status", code)
	}
	page = body(oc.get("/squadron"))
	if !strings.Contains(page, "<strong>Inactive</strong>") || !strings.Contains(page, "Mark ed-act@example.org active again") {
		t.Error("the row does not show Inactive with a way back")
	}
	// No reset email for an inactive member.
	before := len(app.mail.sent)
	requestReset(t, app.anon(), "ed-act@example.org")
	if len(app.mail.sent) != before {
		t.Error("a password reset email was sent to an inactive member")
	}

	wantRedirect(t, oc.post("/squadron/members/"+id+"/active", url.Values{"active": {"1"}}), "/squadron")
	if code, _ := signIn(testPassword); code != http.StatusSeeOther {
		t.Errorf("after reactivation = %d, want 303", code)
	}
	log := body(oc.get("/squadron"))
	for _, want := range []string{"Marked a member inactive", "Made a member active again"} {
		if !strings.Contains(log, want) {
			t.Errorf("security log lacks %q", want)
		}
	}
}

func TestMemberStatusActionsAreOwnerOnlySquadronScopedAndGuarded(t *testing.T) {
	app := newApp(t)
	owner := app.owner("1902")
	oc := app.login(owner)
	app.member(owner, "ed-g@example.org", "editor")
	ec := app.login("ed-g@example.org")
	other := app.login(app.owner("1903"))
	eid, oid := memberID(t, app, "ed-g@example.org"), memberID(t, app, owner)

	for _, p := range []string{"/active", "/temp-password"} {
		if got := ec.post("/squadron/members/"+oid+p, url.Values{"active": {"0"}}).StatusCode; got != http.StatusForbidden {
			t.Errorf("editor POST %s = %d, want 403", p, got)
		}
		if got := other.post("/squadron/members/"+eid+p, url.Values{"active": {"0"}}).StatusCode; got != http.StatusNotFound {
			t.Errorf("another squadron's owner POST %s = %d, want 404 (not a member)", p, got)
		}
	}
	// Self, and the last owner.
	for name, resp := range map[string]*http.Response{
		"self inactive": oc.post("/squadron/members/"+oid+"/active", url.Values{"active": {"0"}}),
		"self temp":     oc.post("/squadron/members/"+oid+"/temp-password", url.Values{}),
	} {
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, resp.StatusCode)
		}
	}
	if got := oc.postRaw("/squadron/members/"+eid+"/active", url.Values{"active": {"0"}}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("no CSRF token = %d, want 400", got)
	}
	if got := ec.get("/").StatusCode; got != http.StatusOK {
		t.Error("a refused request deactivated the editor")
	}
}

func TestTemporaryPasswordIsEmailedNeverShownAndForcesAChange(t *testing.T) {
	app := newApp(t)
	owner := app.owner("1904")
	oc := app.login(owner)
	app.member(owner, "ed-tmp@example.org", "editor")
	old := app.login("ed-tmp@example.org")
	id := memberID(t, app, "ed-tmp@example.org")

	resp := oc.post("/squadron/members/"+id+"/temp-password", url.Values{})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("send = %d", resp.StatusCode)
	}
	if len(app.mail.temps) != 1 || app.mail.temps[0].To != "ed-tmp@example.org" {
		t.Fatalf("emails: %+v", app.mail.temps)
	}
	mailed := app.mail.temps[0]
	if len(mailed.Password) < 20 || !strings.HasSuffix(mailed.URL, "/login") || mailed.Expires == "" {
		t.Errorf("mail: %+v", mailed)
	}
	if page := body(oc.get("/squadron")); strings.Contains(page, mailed.Password) {
		t.Error("the owner can see the temporary password")
	}
	if got := old.get("/").StatusCode; got == http.StatusOK {
		t.Error("the member's old session survived")
	}
	// The old password is gone.
	c := app.anon()
	bad := c.postRaw("/login", url.Values{"csrf": {scrapeCSRF(t, c.get("/login"))}, "email": {"ed-tmp@example.org"}, "password": {testPassword}})
	if bad.StatusCode != http.StatusUnauthorized {
		t.Errorf("old password = %d", bad.StatusCode)
	}

	// Sign in with the temporary one: everything redirects to the change page.
	c = app.anon()
	good := c.postRaw("/login", url.Values{"csrf": {scrapeCSRF(t, c.get("/login"))}, "email": {"ed-tmp@example.org"}, "password": {mailed.Password}})
	if good.StatusCode != http.StatusSeeOther {
		t.Fatalf("temp login = %d", good.StatusCode)
	}
	for _, path := range []string{"/", "/instructors", "/account", "/docs", "/programmes/1"} {
		if r := c.get(path); r.StatusCode != http.StatusSeeOther || !strings.HasSuffix(r.Header.Get("Location"), "/account/new-password") {
			t.Errorf("GET %s while on a temporary password = %d to %q", path, r.StatusCode, r.Header.Get("Location"))
		}
	}
	for _, ck := range c.hc.Jar.Cookies(mustURL(c.base)) { // the form token for this browser
		if ck.Name == sessionCookieName {
			c.csrf = app.auth.CSRFToken(ck.Value)
		}
	}
	form := body(c.get("/account/new-password"))
	if !strings.Contains(form, "Choose your own password") {
		t.Errorf("no change page:\n%s", form)
	}
	// Posting a weak password re-shows this page, not the full account page.
	bad = c.post("/account/password", url.Values{"current_password": {mailed.Password}, "new_password": {"short"}})
	if bad.StatusCode != http.StatusBadRequest || !strings.Contains(body(bad), "Choose your own password") {
		t.Errorf("weak password = %d", bad.StatusCode)
	}
	r2 := c.post("/account/password", url.Values{"current_password": {mailed.Password}, "new_password": {"my very own password"}})
	if r2.StatusCode != http.StatusSeeOther {
		t.Fatalf("choosing a password = %d: %s", r2.StatusCode, body(r2))
	}
	if got := c.get("/").StatusCode; got != http.StatusOK {
		t.Errorf("after choosing their own, / = %d", got)
	}
	if got := c.get("/account/new-password").StatusCode; got != http.StatusSeeOther {
		t.Errorf("the change page is still offered after the change (%d)", got)
	}
	if !strings.Contains(body(oc.get("/squadron")), "Sent a member a temporary password") {
		t.Error("not in the security log")
	}
}

func TestFailedTemporaryPasswordEmailChangesNothing(t *testing.T) {
	app := newApp(t)
	owner := app.owner("1905")
	oc := app.login(owner)
	app.member(owner, "ed-fail@example.org", "editor")
	app.mail.tempErr = http.ErrHandlerTimeout
	id := memberID(t, app, "ed-fail@example.org")

	if got := oc.post("/squadron/members/"+id+"/temp-password", url.Values{}).StatusCode; got != http.StatusBadGateway {
		t.Errorf("send with a failing mailer = %d, want 502", got)
	}
	c := app.anon()
	if resp := c.postRaw("/login", url.Values{"csrf": {scrapeCSRF(t, c.get("/login"))}, "email": {"ed-fail@example.org"}, "password": {testPassword}}); resp.StatusCode != http.StatusSeeOther {
		t.Errorf("the member's own password stopped working although no email went out (%d)", resp.StatusCode)
	}
}

func TestNoTemporaryPasswordForAnInactiveMember(t *testing.T) {
	app := newApp(t)
	owner := app.owner("1906")
	oc := app.login(owner)
	app.member(owner, "ed-ina@example.org", "editor")
	id := memberID(t, app, "ed-ina@example.org")
	oc.post("/squadron/members/"+id+"/active", url.Values{"active": {"0"}})
	resp := oc.post("/squadron/members/"+id+"/temp-password", url.Values{})
	if resp.StatusCode != http.StatusBadRequest || len(app.mail.temps) != 0 {
		t.Errorf("temp password for an inactive member = %d, emails %d", resp.StatusCode, len(app.mail.temps))
	}
}

func TestInactiveMembersAreNotSuggestedAsInstructors(t *testing.T) {
	app := newApp(t)
	owner := app.owner("1907")
	oc := app.login(owner)
	app.member(owner, "ghost-ed@example.org", "editor")
	p, _ := app.store.CreateProgramme(app.squadronID(owner), 2033, 1, 1)
	path := "/programmes/" + strconv.FormatInt(p.ID, 10)
	if !strings.Contains(body(oc.get(path)), "ghost-ed@example.org") {
		t.Fatal("an active member is not suggested")
	}
	oc.post("/squadron/members/"+memberID(t, app, "ghost-ed@example.org")+"/active", url.Values{"active": {"0"}})
	if strings.Contains(body(oc.get(path)), "ghost-ed@example.org") {
		t.Error("an inactive member is still suggested")
	}
}
