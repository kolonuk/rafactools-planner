package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"rafactools-planner/internal/config"
)

func turnstileApp(t *testing.T) *testApp {
	a := newApp(t, func(c *config.Config) { c.TurnstileSiteKey = "site-key"; c.TurnstileSecret = "secret" })
	a.turnstileOK = func(tok string) bool { return tok == "good" }
	return a
}

func TestTurnstileGuardsLoginClaimAndDelete(t *testing.T) {
	app := turnstileApp(t)
	email := app.owner("501")
	owner := app.login(email)
	anonPost := func(path string, v url.Values) *http.Response {
		c := app.anon()
		v.Set("csrf", scrapeCSRF(t, c.get(path)))
		return c.postRaw(path, v)
	}

	for _, path := range []string{"/login", "/claim"} {
		page := body(app.anon().get(path))
		if !strings.Contains(page, `data-sitekey="site-key"`) {
			t.Errorf("%s has no Turnstile widget", path)
		}
	}
	if page := body(owner.get("/squadron")); !strings.Contains(page, `data-sitekey="site-key"`) {
		t.Error("delete form has no Turnstile widget")
	}
	resp := app.anon().get("/login")
	resp.Body.Close()
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' https://challenges.cloudflare.com") || !strings.Contains(csp, "frame-src https://challenges.cloudflare.com") {
		t.Errorf("CSP does not allow Turnstile: %s", csp)
	}

	login := func(tok string) url.Values {
		v := url.Values{"email": {email}, "password": {testPassword}}
		if tok != "" {
			v.Set("cf-turnstile-response", tok)
		}
		return v
	}
	if got := anonPost("/login", login("")).StatusCode; got != http.StatusBadRequest {
		t.Errorf("login without token = %d, want 400", got)
	}
	if got := anonPost("/login", login("bad")).StatusCode; got != http.StatusBadRequest {
		t.Errorf("login with bad token = %d, want 400", got)
	}
	if got := anonPost("/login", login("good")).StatusCode; got != http.StatusSeeOther {
		t.Errorf("login with good token = %d, want 303", got)
	}

	claim := url.Values{"email": {"snco.777@rafac.mod.gov.uk"}, "password": {testPassword}}
	if !strings.Contains(body(anonPost("/claim", claim)), "human check") {
		t.Error("claim without a Turnstile token was not refused")
	}
	var n int
	app.db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, "snco.777@rafac.mod.gov.uk").Scan(&n)
	if n != 0 {
		t.Error("claim created an account without a Turnstile token")
	}

	del := url.Values{"confirm_number": {"501"}, "password": {testPassword}}
	if got := owner.post("/squadron/delete", del).StatusCode; got != http.StatusBadRequest {
		t.Errorf("delete without token = %d, want 400", got)
	}
	del.Set("cf-turnstile-response", "good")
	if got := owner.post("/squadron/delete", del).StatusCode; got != http.StatusSeeOther {
		t.Errorf("delete with good token = %d, want 303", got)
	}
}

func TestTurnstileOffByDefault(t *testing.T) {
	app := newApp(t)
	resp := app.anon().get("/login")
	page := body(resp)
	if strings.Contains(page, "challenges.cloudflare.com") || strings.Contains(resp.Header.Get("Content-Security-Policy"), "cloudflare") {
		t.Error("Turnstile leaked into a site that has not configured it")
	}
}
