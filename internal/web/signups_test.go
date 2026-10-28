package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"rafactools-planner/internal/config"
)

func TestDisableSignupsClosesTheClaimFormOnly(t *testing.T) {
	app := newApp(t, func(c *config.Config) { c.DisableSignups = true })
	email := app.owner("1301") // created directly: existing accounts keep working

	anon := app.anon()
	if got := anon.get("/claim").StatusCode; got != http.StatusForbidden {
		t.Errorf("GET /claim = %d, want 403", got)
	}
	resp := anon.post("/claim", url.Values{"email": {"snco.1302@rafac.mod.gov.uk"}, "password": {"averylongpassword1"}, "squadron_name": {"X"}})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST /claim = %d, want 403", resp.StatusCode)
	}
	if login := body(anon.get("/login")); strings.Contains(login, "/claim") {
		t.Error("login page still links to the closed claim form")
	}
	if got := app.login(email).get("/").StatusCode; got != http.StatusOK {
		t.Errorf("existing user dashboard = %d, want 200", got)
	}
}

func TestSignupsOpenByDefault(t *testing.T) {
	app := newApp(t)
	if got := app.anon().get("/claim").StatusCode; got != http.StatusOK {
		t.Errorf("GET /claim = %d, want 200", got)
	}
	if login := body(app.anon().get("/login")); !strings.Contains(login, "/claim") {
		t.Error("login page lacks the claim link")
	}
}

func TestPrivacyNoticeIsPublicAndLinkedEverywhere(t *testing.T) {
	app := newApp(t)
	page := body(app.anon().get("/privacy"))
	for _, want := range []string{"Privacy notice", "cadets' names", "No analytics", "delete the whole squadron"} {
		if !strings.Contains(page, want) {
			t.Errorf("privacy notice lacks %q", want)
		}
	}
	for _, path := range []string{"/login", "/claim", "/privacy"} {
		if !strings.Contains(body(app.anon().get(path)), `/privacy">Privacy</a>`) {
			t.Errorf("%s has no footer link to the privacy notice", path)
		}
	}
}
