package web

import (
	"net/http"
	"net/url"
	"rafactools-planner/internal/config"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	a := newApp(t)
	resp := a.anon().get("/health")
	wantStatus(t, resp, 200)
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
}

func TestProtectedRoutesRedirectAnonymousUsers(t *testing.T) {
	a := newApp(t)
	c := a.anon()
	for _, path := range []string{"/", "/squadron", "/programmes/1", "/programmes/1/pdf"} {
		wantRedirect(t, c.get(path), "/login")
	}
	for _, path := range []string{"/programmes", "/squadron", "/squadron/invites", "/programmes/1/closures", "/logout"} {
		wantRedirect(t, c.postRaw(path, url.Values{}), "/login")
	}
}

func TestLoginFlow(t *testing.T) {
	a := newApp(t)
	email := a.owner("101")

	t.Run("wrong password and unknown user get the same generic answer", func(t *testing.T) {
		var bodies []string
		for _, e := range []string{email, "nobody@rafac.mod.gov.uk"} {
			c := a.anon()
			tok := scrapeCSRF(t, c.get("/login"))
			resp := c.postRaw("/login", url.Values{"csrf": {tok}, "email": {e}, "password": {"wrong-password-123"}})
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status %d, want 401", resp.StatusCode)
			}
			bodies = append(bodies, body(resp))
		}
		if bodies[0] != bodies[1] {
			// CSRF tokens differ per client, so compare with them stripped.
			strip := func(s string) string { return csrfInput.ReplaceAllString(s, "") }
			if strip(bodies[0]) != strip(bodies[1]) {
				t.Error("responses differ between known and unknown email: account enumeration")
			}
		}
	})

	t.Run("missing or forged CSRF is rejected before credentials are checked", func(t *testing.T) {
		c := a.anon()
		c.get("/login") // gets the anon cookie
		for _, tok := range []string{"", "deadbeef"} {
			resp := c.postRaw("/login", url.Values{"csrf": {tok}, "email": {email}, "password": {testPassword}})
			wantStatus(t, resp, http.StatusBadRequest)
		}
		if n := count(t, a.db, `SELECT COUNT(*) FROM sessions`); n != 0 {
			t.Errorf("%d sessions created by CSRF-less logins", n)
		}
	})

	t.Run("success sets a hardened session cookie", func(t *testing.T) {
		c := a.anon()
		tok := scrapeCSRF(t, c.get("/login"))
		resp := c.postRaw("/login", url.Values{"csrf": {tok}, "email": {email}, "password": {testPassword}})
		wantRedirect(t, resp, "/")
		var session *http.Cookie
		for _, ck := range resp.Cookies() {
			if ck.Name == sessionCookieName {
				session = ck
			}
		}
		if session == nil {
			t.Fatal("no session cookie")
		}
		if !session.HttpOnly || session.SameSite != http.SameSiteLaxMode || session.Path != "/" {
			t.Errorf("weak cookie attrs: %+v", session)
		}
		if len(session.Value) < 64 {
			t.Errorf("session token only %d chars", len(session.Value))
		}
		if time.Until(session.Expires) <= 0 && session.MaxAge <= 0 {
			t.Error("session cookie already expired")
		}
		wantStatus(t, c.get("/"), 200)
	})

	t.Run("email is case-insensitive", func(t *testing.T) {
		c := a.anon()
		tok := scrapeCSRF(t, c.get("/login"))
		wantRedirect(t, c.postRaw("/login", url.Values{"csrf": {tok}, "email": {" " + strings.ToUpper(email) + " "}, "password": {testPassword}}), "/")
	})
}

func TestLogoutInvalidatesSessionServerSide(t *testing.T) {
	a := newApp(t)
	c := a.login(a.owner("102"))
	stolen := c.hc.Jar.Cookies(mustURL(c.base))

	wantRedirect(t, c.post("/logout", nil), "/login")
	wantRedirect(t, c.get("/"), "/login")

	// Replaying the old token (as a thief would) must fail: logout deletes
	// the row, it doesn't merely clear the browser cookie.
	thief := a.anon()
	thief.hc.Jar.SetCookies(mustURL(c.base), stolen)
	wantRedirect(t, thief.get("/"), "/login")
}

func TestLogoutNeedsCSRF(t *testing.T) {
	a := newApp(t)
	c := a.login(a.owner("103"))
	wantRedirect(t, c.postRaw("/logout", url.Values{"csrf": {"forged"}}), "/")
	wantStatus(t, c.get("/"), 200) // still logged in
}

func TestExpiredSessionIsRejectedAndPurged(t *testing.T) {
	a := newApp(t)
	c := a.login(a.owner("104"))
	if _, err := a.db.Exec(`UPDATE sessions SET expires_at = ?`, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	wantRedirect(t, c.get("/"), "/login")
	if n := count(t, a.db, `SELECT COUNT(*) FROM sessions`); n != 0 {
		t.Errorf("expired session still in DB (%d rows)", n)
	}
}

func TestClaimFlow(t *testing.T) {
	a := newApp(t)
	c := a.anon()
	post := func(email, pw string) *http.Response {
		tok := scrapeCSRF(t, c.get("/claim"))
		return c.postRaw("/claim", url.Values{"csrf": {tok}, "email": {email}, "password": {pw}, "squadron_name": {"Test Sqn"}})
	}

	for _, tc := range map[string]struct{ email, pw string }{
		"wrong domain":   {"oc.555@gmail.com", testPassword},
		"wrong prefix":   {"cadet.555@rafac.mod.gov.uk", testPassword},
		"short password": {"oc.555@rafac.mod.gov.uk", "short"},
	} {
		resp := post(tc.email, tc.pw)
		wantStatus(t, resp, 200) // re-rendered form with an error, not a redirect
	}
	if n := count(t, a.db, `SELECT COUNT(*) FROM squadrons`); n != 0 {
		t.Fatalf("invalid claims created %d squadrons", n)
	}

	resp := post("oc.555@rafac.mod.gov.uk", testPassword)
	wantRedirect(t, resp, "/check-email?email=oc.555%40rafac.mod.gov.uk")

	// Unverified owner cannot log in yet.
	tok := scrapeCSRF(t, c.get("/login"))
	wantStatus(t, c.postRaw("/login", url.Values{"csrf": {tok}, "email": {"oc.555@rafac.mod.gov.uk"}, "password": {testPassword}}), 401)

	// Following the emailed link verifies, and the token is single use.
	var token string
	if err := a.db.QueryRow(`SELECT token FROM email_verifications`).Scan(&token); err != nil {
		t.Fatal(err)
	}
	wantRedirect(t, c.get("/verify?token="+token), "/login?verified=1")
	wantStatus(t, c.get("/verify?token="+token), 400)
	wantStatus(t, c.get("/verify?token=nonsense"), 400)

	tok = scrapeCSRF(t, c.get("/login"))
	wantRedirect(t, c.postRaw("/login", url.Values{"csrf": {tok}, "email": {"oc.555@rafac.mod.gov.uk"}, "password": {testPassword}}), "/")

	// A squadron number can only be claimed once, even from another address.
	resp = post("snco.555@rafac.mod.gov.uk", testPassword)
	if body := body(resp); !strings.Contains(body, "already been claimed") {
		t.Errorf("duplicate claim not refused: %.200s", body)
	}
}

func TestClaimRedirectEscapesEmail(t *testing.T) {
	// The email is reflected into a redirect query string; nothing in it may
	// be able to start a new parameter. (Only well-formed claim addresses
	// get this far, so assert the escaping on the one that does.)
	a := newApp(t)
	c := a.anon()
	tok := scrapeCSRF(t, c.get("/claim"))
	resp := c.postRaw("/claim", url.Values{"csrf": {tok}, "email": {"oc.7@rafac.mod.gov.uk"}, "password": {testPassword}})
	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "@") {
		t.Errorf("redirect %q contains an unescaped @", loc)
	}
}

func TestSkipEmailVerificationDevMode(t *testing.T) {
	a := newApp(t, func(c *config.Config) { c.SkipEmailVerification = true })
	c := a.anon()
	tok := scrapeCSRF(t, c.get("/claim"))
	wantRedirect(t, c.postRaw("/claim", url.Values{"csrf": {tok}, "email": {"oc.9@rafac.mod.gov.uk"}, "password": {testPassword}}), "/login?claimed=1")
	if n := count(t, a.db, `SELECT COUNT(*) FROM email_verifications`); n != 0 {
		t.Errorf("dev mode created %d verification tokens", n)
	}
}

func TestInviteFlow(t *testing.T) {
	a := newApp(t)
	owner := a.login(a.owner("201"))

	wantRedirect(t, owner.post("/squadron/invites", url.Values{"email": {"New.Person@example.org"}, "role": {"viewer"}}), "/squadron")
	var token, storedEmail, role string
	if err := a.db.QueryRow(`SELECT token, email, role FROM invites`).Scan(&token, &storedEmail, &role); err != nil {
		t.Fatal(err)
	}
	if storedEmail != "new.person@example.org" || role != "viewer" {
		t.Fatalf("stored invite = %q/%q", storedEmail, role)
	}

	t.Run("pending invite is listed to the owner", func(t *testing.T) {
		resp := owner.get("/squadron")
		if b := body(resp); !strings.Contains(b, "new.person@example.org") {
			t.Error("invite not shown on settings page")
		}
	})

	t.Run("accepting creates a verified user with the invited role and logs them in", func(t *testing.T) {
		c := a.anon()
		resp := c.get("/invite/" + token)
		if b := body(resp); !strings.Contains(b, "new.person@example.org") {
			t.Fatal("invite page does not show the invited address")
		}
		tok := scrapeCSRF(t, c.get("/invite/"+token))
		// Weak password rejected, invite not consumed.
		wantStatus(t, c.postRaw("/invite/"+token, url.Values{"csrf": {tok}, "password": {"short"}}), 400)
		wantRedirect(t, c.postRaw("/invite/"+token, url.Values{"csrf": {tok}, "password": {testPassword}}), "/")
		wantStatus(t, c.get("/"), 200)
		if got := count(t, a.db, `SELECT COUNT(*) FROM users WHERE email = 'new.person@example.org' AND role = 'viewer' AND email_verified_at IS NOT NULL`); got != 1 {
			t.Errorf("invited user row wrong (matched %d)", got)
		}
	})

	t.Run("invite is single use", func(t *testing.T) {
		c := a.anon()
		wantStatus(t, c.get("/invite/"+token), 400)
	})

	t.Run("unknown and expired tokens", func(t *testing.T) {
		wantStatus(t, a.anon().get("/invite/not-a-token"), 400)
		wantRedirect(t, owner.post("/squadron/invites", url.Values{"email": {"late@example.org"}, "role": {"editor"}}), "/squadron")
		a.db.Exec(`UPDATE invites SET expires_at = ? WHERE email = 'late@example.org'`, time.Now().Add(-time.Hour))
		var tk string
		a.db.QueryRow(`SELECT token FROM invites WHERE email = 'late@example.org'`).Scan(&tk)
		wantStatus(t, a.anon().get("/invite/"+tk), 400)
	})
}

func TestInviteValidation(t *testing.T) {
	a := newApp(t)
	owner := a.login(a.owner("202"))
	for name, form := range map[string]url.Values{
		"bad role":        {"email": {"a@example.org"}, "role": {"superadmin"}},
		"not an address":  {"email": {"nope"}, "role": {"viewer"}},
		"header injected": {"email": {"a@example.org\r\nBcc: evil@example.org"}, "role": {"viewer"}},
		"display name":    {"email": {"Evil <evil@example.org>"}, "role": {"viewer"}},
		"empty":           {"email": {""}, "role": {"viewer"}},
	} {
		resp := owner.post("/squadron/invites", form)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
	if n := count(t, a.db, `SELECT COUNT(*) FROM invites`); n != 0 {
		t.Errorf("%d invites created from invalid input", n)
	}
}

func TestCannotRevokeAnotherSquadronsInvite(t *testing.T) {
	a := newApp(t)
	ownerA := a.login(a.owner("301"))
	ownerB := a.login(a.owner("302"))
	wantRedirect(t, ownerA.post("/squadron/invites", url.Values{"email": {"x@example.org"}, "role": {"viewer"}}), "/squadron")
	var token string
	a.db.QueryRow(`SELECT token FROM invites`).Scan(&token)

	wantRedirect(t, ownerB.post("/squadron/invites/"+token+"/revoke", nil), "/squadron")
	if n := count(t, a.db, `SELECT COUNT(*) FROM invites`); n != 1 {
		t.Fatal("squadron B revoked squadron A's invite")
	}
	wantRedirect(t, ownerA.post("/squadron/invites/"+token+"/revoke", nil), "/squadron")
	if n := count(t, a.db, `SELECT COUNT(*) FROM invites`); n != 0 {
		t.Fatal("owner could not revoke their own invite")
	}
}

func TestRolesAreEnforced(t *testing.T) {
	a := newApp(t)
	ownerEmail := a.owner("401")
	a.member(ownerEmail, "ed@example.org", "editor")
	a.member(ownerEmail, "vi@example.org", "viewer")
	owner, editor, viewer := a.login(ownerEmail), a.login("ed@example.org"), a.login("vi@example.org")

	prog, err := a.store.CreateProgramme(a.squadronID(ownerEmail), 2030, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	pid := strconv.FormatInt(prog.ID, 10)

	type attempt struct {
		name   string
		method string
		path   string
		form   url.Values
		allow  map[string]bool // who gets past the role check
	}
	any3 := map[string]bool{"owner": true, "editor": true, "viewer": true}
	edit := map[string]bool{"owner": true, "editor": true}
	own := map[string]bool{"owner": true}
	attempts := []attempt{
		{"dashboard", "GET", "/", nil, any3},
		{"view programme", "GET", "/programmes/" + pid, nil, any3},
		{"pdf", "GET", "/programmes/" + pid + "/pdf", nil, any3},
		{"create programme", "POST", "/programmes", url.Values{"year": {"2031"}, "month": {"1"}}, edit},
		{"save programme", "POST", "/programmes/" + pid, nil, edit},
		{"add closure", "POST", "/programmes/" + pid + "/closures", url.Values{"start_date": {"2030-03-01"}, "end_date": {"2030-03-02"}}, edit},
		{"add weekend note", "POST", "/programmes/" + pid + "/weekend-notes", url.Values{"date": {"2030-03-02"}, "text": {"x"}}, edit},
		{"add requirement", "POST", "/programmes/" + pid + "/requirements", url.Values{"name": {"x"}}, edit},
		{"squadron settings", "GET", "/squadron", nil, own},
		{"save squadron", "POST", "/squadron", url.Values{"name": {"N"}}, own},
		{"create invite", "POST", "/squadron/invites", url.Values{"email": {"z@example.org"}, "role": {"viewer"}}, own},
	}
	for _, at := range attempts {
		for who, c := range map[string]*client{"owner": owner, "editor": editor, "viewer": viewer} {
			var resp *http.Response
			if at.method == "GET" {
				resp = c.get(at.path)
			} else {
				resp = c.post(at.path, cloneForm(at.form))
			}
			forbidden := resp.StatusCode == http.StatusForbidden
			resp.Body.Close()
			if forbidden == at.allow[who] {
				t.Errorf("%s as %s: status %d (allowed=%v)", at.name, who, resp.StatusCode, at.allow[who])
			}
		}
	}
}

func cloneForm(v url.Values) url.Values {
	out := url.Values{}
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
