package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

var signOutAction = regexp.MustCompile(`action="[^"]*/account/sessions/([0-9a-f]+)/signout"`)

func TestAccountListsSessionsAndSignsOthersOut(t *testing.T) {
	app := newApp(t)
	email := app.owner("1401")
	laptop := app.login(email)
	phone := app.login(email)
	stranger := app.login(app.owner("1402"))

	page := body(laptop.get("/account"))
	if !strings.Contains(page, "Where you are signed in") || strings.Count(page, "(this browser)") != 1 {
		t.Fatalf("sessions section missing or current not marked once:\n%s", page)
	}
	ids := signOutAction.FindAllStringSubmatch(page, -1)
	if len(ids) != 1 {
		t.Fatalf("want exactly one other session with a sign-out button, got %d", len(ids))
	}

	// Another user cannot end it, even knowing the id.
	if got := stranger.post("/account/sessions/"+ids[0][1]+"/signout", url.Values{}).StatusCode; got != http.StatusNotFound {
		t.Errorf("stranger sign-out = %d, want 404", got)
	}
	if got := phone.get("/").StatusCode; got != http.StatusOK {
		t.Fatalf("phone session was ended by a stranger (%d)", got)
	}

	// A token-less POST is refused and changes nothing.
	if got := laptop.postRaw("/account/sessions/"+ids[0][1]+"/signout", url.Values{}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("no CSRF token = %d, want 400", got)
	}
	if got := phone.get("/").StatusCode; got != http.StatusOK {
		t.Fatal("phone signed out without a CSRF token")
	}

	// The owner ends it, and the phone is then signed out.
	if got := laptop.post("/account/sessions/"+ids[0][1]+"/signout", url.Values{}).StatusCode; got != http.StatusSeeOther {
		t.Fatalf("sign out = %d, want 303", got)
	}
	if got := phone.get("/").StatusCode; got == http.StatusOK {
		t.Error("phone still signed in after being signed out")
	}
	if got := laptop.get("/").StatusCode; got != http.StatusOK {
		t.Error("the current browser was signed out")
	}
}

func TestSignOutEverywhereElse(t *testing.T) {
	app := newApp(t)
	email := app.owner("1403")
	a, b, c := app.login(email), app.login(email), app.login(email)

	if got := a.postRaw("/account/sessions/signout-others", url.Values{}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("no CSRF token = %d, want 400", got)
	}
	if got := a.post("/account/sessions/signout-others", url.Values{}).StatusCode; got != http.StatusSeeOther {
		t.Fatalf("sign out others = %d, want 303", got)
	}
	for name, cl := range map[string]*client{"b": b, "c": c} {
		if cl.get("/").StatusCode == http.StatusOK {
			t.Errorf("session %s survived 'sign out everywhere else'", name)
		}
	}
	if a.get("/").StatusCode != http.StatusOK {
		t.Error("the current browser was signed out")
	}
	page := body(a.get("/account"))
	if strings.Contains(page, "Sign out everywhere else") {
		t.Error("offers to sign out others when there are none")
	}
}

// loginWithAgent signs in through the real form with a given User-Agent.
func loginWithAgent(t *testing.T, app *testApp, email, agent string) *client {
	t.Helper()
	c := app.anon()
	tok := scrapeCSRF(t, c.get("/login"))
	req, _ := http.NewRequest("POST", c.base+"/login", strings.NewReader(url.Values{"csrf": {tok}, "email": {email}, "password": {testPassword}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", agent)
	if resp := c.do(req); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login = %d", resp.StatusCode)
	}
	for _, ck := range c.hc.Jar.Cookies(mustURL(c.base)) {
		if ck.Name == sessionCookieName {
			c.csrf = app.auth.CSRFToken(ck.Value)
		}
	}
	return c
}

func TestSessionListShowsACoarseBrowserLabelNeverTheRawHeader(t *testing.T) {
	app := newApp(t)
	email := app.owner("1404")
	loginWithAgent(t, app, email, "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/130.0 Safari/537.36 secret-build-id-12345")
	here := loginWithAgent(t, app, email, "Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0")

	page := body(here.get("/account"))
	if !strings.Contains(page, "Chrome on Windows") || !strings.Contains(page, "Firefox on Linux") {
		t.Errorf("browser labels missing:\n%s", page)
	}
	if strings.Contains(page, "secret-build-id-12345") || strings.Contains(page, "Mozilla/5.0") {
		t.Error("the raw User-Agent header is shown on the page")
	}
}

func TestMasterAdminCookieExpiresInHoursOthersInDays(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	expiry := func(c *client) time.Duration {
		for _, ck := range c.hc.Jar.Cookies(mustURL(c.base)) {
			if ck.Name == sessionCookieName {
				var e time.Time
				ts.a.db.QueryRow(`SELECT expires_at FROM sessions WHERE token = ?`, ck.Value).Scan(&e)
				return time.Until(e)
			}
		}
		t.Fatal("no session cookie")
		return 0
	}
	if d := expiry(master); d > 9*time.Hour || d < 7*time.Hour {
		t.Errorf("master admin session lasts %v, want about 8h", d)
	}
	if d := expiry(ts.ownerA); d < 29*24*time.Hour {
		t.Errorf("ordinary session lasts %v, want 30 days", d)
	}
}
