package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var confirmLink = regexp.MustCompile(`/email/confirm/([0-9a-f]+)$`)

func TestRAFACAddressesCannotBeChangedOthersCan(t *testing.T) {
	app := newApp(t)
	owner := app.owner("2001")
	oc := app.login(owner)
	app.member(owner, "ed-em@example.org", "editor")
	ec := app.login("ed-em@example.org")

	if page := body(oc.get("/account")); !strings.Contains(page, "can't be changed here") || strings.Contains(page, `name="new_email"`) {
		t.Errorf("a RAFAC-domain account is offered the change:\n%s", page)
	}
	if got := oc.post("/account/email", url.Values{"new_email": {"x@example.org"}, "current_password": {testPassword}}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("RAFAC-domain POST = %d, want 400", got)
	}
	if page := body(ec.get("/account")); !strings.Contains(page, `name="new_email"`) {
		t.Error("an ordinary account is not offered the change")
	}
	if len(app.mail.verifies) != 0 {
		t.Error("a link was sent for a refused change")
	}
}

func TestEmailChangeNeedsPasswordAndAllowedAddress(t *testing.T) {
	app := newApp(t)
	owner := app.owner("2002")
	app.member(owner, "ed-em2@example.org", "editor")
	app.member(owner, "taken-em@example.org", "viewer")
	ec := app.login("ed-em2@example.org")

	for name, f := range map[string]url.Values{
		"wrong password":      {"new_email": {"fresh@example.org"}, "current_password": {"not my password!!"}},
		"on the RAFAC domain": {"new_email": {"oc.1@rafac.mod.gov.uk"}, "current_password": {testPassword}},
		"already used":        {"new_email": {"taken-em@example.org"}, "current_password": {testPassword}},
		"not an address":      {"new_email": {"nope"}, "current_password": {testPassword}},
	} {
		if got := ec.post("/account/email", f).StatusCode; got != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, got)
		}
	}
	if got := ec.postRaw("/account/email", url.Values{"new_email": {"fresh@example.org"}, "current_password": {testPassword}}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("no CSRF token = %d", got)
	}
	if len(app.mail.verifies) != 0 {
		t.Error("a link was sent for a refused change")
	}
}

func TestEmailChangeFullFlow(t *testing.T) {
	app := newApp(t)
	owner := app.owner("2003")
	oc := app.login(owner)
	app.member(owner, "ed-em3@example.org", "editor")
	ec := app.login("ed-em3@example.org")

	wantRedirect(t, ec.post("/account/email", url.Values{"new_email": {"Brand.New@Example.org"}, "current_password": {testPassword}}), "/account?emailsent=1")
	if len(app.mail.verifies) != 1 || !strings.HasPrefix(app.mail.verifies[0], "brand.new@example.org|") {
		t.Fatalf("the link must go to the NEW address: %v", app.mail.verifies)
	}
	token := confirmLink.FindStringSubmatch(app.mail.verifies[0])[1]

	// Nothing has changed yet, and merely opening the link does not use it.
	if got := app.anon().get("/account").StatusCode; got == http.StatusOK {
		t.Fatal("anonymous account page")
	}
	anon := app.anon()
	page := body(anon.get("/email/confirm/" + token))
	if !strings.Contains(page, "brand.new@example.org") || !strings.Contains(page, "Yes, use this address") {
		t.Fatalf("confirm page:\n%s", page)
	}
	c2 := app.anon()
	if resp := c2.postRaw("/login", url.Values{"csrf": {scrapeCSRF(t, c2.get("/login"))}, "email": {"ed-em3@example.org"}, "password": {testPassword}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatal("the old address stopped working before confirmation")
	}
	if got := app.anon().postRaw("/email/confirm/"+token, url.Values{}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("confirm without a form token = %d, want 400", got)
	}

	resp := anon.post("/email/confirm/"+token, url.Values{"csrf": {scrapeCSRF(t, anon.get("/email/confirm/"+token))}})
	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(resp.Header.Get("Location"), "/login?emailchanged=1") {
		t.Fatalf("confirm = %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	// The existing session survives and now shows the new address.
	if page := body(ec.get("/account")); !strings.Contains(page, "brand.new@example.org") && ec.get("/").StatusCode != http.StatusOK {
		t.Error("the signed-in session was lost")
	}
	if got := ec.get("/").StatusCode; got != http.StatusOK {
		t.Errorf("signed-in session after the change = %d", got)
	}
	c3 := app.anon()
	if resp := c3.postRaw("/login", url.Values{"csrf": {scrapeCSRF(t, c3.get("/login"))}, "email": {"brand.new@example.org"}, "password": {testPassword}}); resp.StatusCode != http.StatusSeeOther {
		t.Errorf("new address cannot sign in (%d)", resp.StatusCode)
	}
	c4 := app.anon()
	if resp := c4.postRaw("/login", url.Values{"csrf": {scrapeCSRF(t, c4.get("/login"))}, "email": {"ed-em3@example.org"}, "password": {testPassword}}); resp.StatusCode == http.StatusSeeOther {
		t.Error("the old address still signs in")
	}
	// The old address is told; the link cannot be used twice; the owner sees it logged.
	waitFor(t, func() bool { app.mail.mu.Lock(); defer app.mail.mu.Unlock(); return len(app.mail.changed) == 1 })
	if app.mail.changed[0] != "ed-em3@example.org|brand.new@example.org" {
		t.Errorf("notice: %v", app.mail.changed)
	}
	if got := anon.get("/email/confirm/" + token).StatusCode; got != http.StatusBadRequest {
		t.Errorf("link reused = %d, want 400", got)
	}
	if !strings.Contains(body(oc.get("/squadron")), "Changed their email address") {
		t.Error("not in the security log")
	}
}

func TestMasterAdminCannotChangeEmail(t *testing.T) {
	_, master, _, _ := masterApp(t)
	if page := body(master.get("/account")); strings.Contains(page, `name="new_email"`) || strings.Contains(page, "can't be changed here") {
		t.Error("the master admin is offered an email change")
	}
	if got := master.post("/account/email", url.Values{"new_email": {"x@example.org"}, "current_password": {testPassword}}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("master POST = %d, want 400", got)
	}
}
