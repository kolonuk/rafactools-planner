package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func requestReset(t *testing.T, c *client, email string) string {
	t.Helper()
	tok := scrapeCSRF(t, c.get("/forgot"))
	resp := c.postRaw("/forgot", url.Values{"csrf": {tok}, "email": {email}})
	wantStatus2xx(t, resp)
	return csrfInput.ReplaceAllString(body(resp), "")
}

func wantStatus2xx(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.StatusCode != 200 {
		t.Fatalf("%s %s -> %d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode)
	}
}

func TestForgotPasswordPageIsLinkedFromLogin(t *testing.T) {
	app := newApp(t)
	if !strings.Contains(body(app.anon().get("/login")), `href="/forgot"`) {
		t.Error("login page has no 'Forgot your password?' link")
	}
	wantStatus(t, app.anon().get("/forgot"), 200)
}

// The page, the status and the email must not reveal whether an address has
// an account.
func TestForgotPasswordDoesNotRevealWhichAddressesHaveAccounts(t *testing.T) {
	app := newApp(t)
	known := app.owner("1301")
	knownPage := requestReset(t, app.anon(), known)
	unknownPage := requestReset(t, app.anon(), "nobody@example.org")
	garbagePage := requestReset(t, app.anon(), "not even an address")

	knownPage, unknownPage, garbagePage = stripRequestID(knownPage), stripRequestID(unknownPage), stripRequestID(garbagePage)
	if knownPage != unknownPage || knownPage != garbagePage {
		t.Errorf("responses differ:\nknown:   %.300s\nunknown: %.300s", knownPage, unknownPage)
	}
	if !strings.Contains(knownPage, "If that address has an account") {
		t.Error("response does not use the neutral wording")
	}
	if _, ok := app.mail.waitFor(t, "reset", known); !ok {
		t.Fatal("known address was not emailed")
	}
	if n := app.mail.count("reset"); n != 1 {
		t.Errorf("%d reset emails sent, want exactly 1 (none for unknown addresses)", n)
	}
}

func TestResetPasswordEndToEnd(t *testing.T) {
	app := newApp(t)
	email := app.owner("1302")
	logged := app.login(email) // an existing session that the reset must kill
	requestReset(t, app.anon(), email)
	mail, ok := app.mail.waitFor(t, "reset", email)
	if !ok {
		t.Fatal("no reset email")
	}

	c := app.anon()
	form := body(c.get("/reset/" + mail.Token))
	if !strings.Contains(form, `name="password"`) || !strings.Contains(form, `action="/reset/`+mail.Token+`"`) {
		t.Fatalf("reset form missing:\n%.600s", form)
	}
	// Opening the link again does not use it up.
	wantStatus(t, c.get("/reset/"+mail.Token), 200)
	tok := scrapeCSRF(t, c.get("/reset/"+mail.Token))

	t.Run("form token is required", func(t *testing.T) {
		resp := c.postRaw("/reset/"+mail.Token, url.Values{"csrf": {"forged"}, "password": {"a brand new password"}})
		wantStatus(t, resp, 400)
	})
	t.Run("a weak password re-shows the form and keeps the link alive", func(t *testing.T) {
		for _, pw := range []string{"short", strings.Repeat("a", 73)} {
			resp := c.postRaw("/reset/"+mail.Token, url.Values{"csrf": {tok}, "password": {pw}})
			page := body(resp)
			if resp.StatusCode != 400 || !strings.Contains(page, `name="password"`) || !strings.Contains(page, "password must be") {
				t.Errorf("%d-byte password: %d\n%.300s", len(pw), resp.StatusCode, page)
			}
		}
		wantStatus(t, c.get("/reset/"+mail.Token), 200)
	})

	wantRedirect(t, c.postRaw("/reset/"+mail.Token, url.Values{"csrf": {tok}, "password": {"a brand new password"}}), "/login?reset=1")

	t.Run("old session is dead and the old password no longer works", func(t *testing.T) {
		wantRedirect(t, logged.get("/"), "/login")
		cl := app.anon()
		lt := scrapeCSRF(t, cl.get("/login"))
		wantStatus(t, cl.postRaw("/login", url.Values{"csrf": {lt}, "email": {email}, "password": {testPassword}}), 401)
	})
	t.Run("new password logs in", func(t *testing.T) {
		cl := app.anon()
		lt := scrapeCSRF(t, cl.get("/login"))
		wantRedirect(t, cl.postRaw("/login", url.Values{"csrf": {lt}, "email": {email}, "password": {"a brand new password"}}), "/")
		if !strings.Contains(body(cl.get("/login?reset=1")), "Password changed") {
			t.Error("no confirmation message after reset")
		}
	})
	t.Run("link is single use", func(t *testing.T) {
		wantStatus(t, app.anon().get("/reset/"+mail.Token), 400)
		resp := c.postRaw("/reset/"+mail.Token, url.Values{"csrf": {tok}, "password": {"another new password!"}})
		wantStatus(t, resp, 400)
	})
}

func TestResetLinksThatAreInvalid(t *testing.T) {
	app := newApp(t)
	for _, p := range []string{"/reset/nope", "/reset/" + strings.Repeat("0", 64), "/reset/%27%20OR%20%271%27=%271"} {
		resp := app.anon().get(p)
		if resp.StatusCode != 400 {
			t.Errorf("GET %s -> %d, want 400", p, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestPasswordResetSharesTheAuthRateLimit(t *testing.T) {
	app := newApp(t)
	c := app.anon()
	c.xff = "203.0.113.77"
	last := 0
	for i := 0; i < 25; i++ {
		resp := c.postRaw("/forgot", url.Values{"csrf": {"x"}, "email": {"a@b.c"}})
		last = resp.StatusCode
		resp.Body.Close()
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("25th request -> %d, want 429", last)
	}
}
