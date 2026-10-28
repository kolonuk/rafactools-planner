package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func mailForm(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

func TestOnlyTheMasterAdminReachesTheMailSettings(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	// Signed out: sent to the login page. Signed in but not the master admin: refused.
	for name, c := range map[string]*client{"anonymous": ts.a.anon(), "owner": ts.ownerA} {
		want := http.StatusForbidden
		if name == "anonymous" {
			want = http.StatusSeeOther
		}
		if got := c.get("/admin/mail").StatusCode; got != want {
			t.Errorf("%s GET = %d, want %d", name, got, want)
		}
		for _, p := range []string{"/admin/mail", "/admin/mail/reset", "/admin/mail/test"} {
			if got := c.post(p, mailForm("smtp_host", "evil.example.org")).StatusCode; got != want {
				t.Errorf("%s POST %s = %d, want %d", name, p, got, want)
			}
		}
	}
	if all, _ := ts.a.settings.All(); len(all) != 0 {
		t.Errorf("a non-master changed settings: %v", all)
	}
	if got := master.get("/admin/mail").StatusCode; got != http.StatusOK {
		t.Errorf("master GET = %d, want 200", got)
	}
	if !strings.Contains(body(master.get("/account")), "/admin/mail") {
		t.Error("the master admin's page does not link to the email settings")
	}
	if strings.Contains(body(ts.ownerA.get("/account")), "/admin/mail") {
		t.Error("an ordinary user is shown a link to the email settings")
	}
}

func TestSavedSettingsOverrideTheEnvironmentAndBlankFallsBack(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	ts.a.env["SMTP_HOST"] = "env-host.example.org"
	ts.a.env["SMTP_PORT"] = "587"
	ts.a.env["MAIL_FROM"] = "env@example.org"

	page := body(master.get("/admin/mail"))
	if !strings.Contains(page, "env-host.example.org:587") || !strings.Contains(page, "now from the environment") {
		t.Fatalf("environment defaults not shown:\n%s", page)
	}

	wantRedirect(t, master.post("/admin/mail", mailForm("smtp_host", "db-host.example.org", "mail_from", "")), "/admin/mail?done=saved")
	page = body(master.get("/admin/mail"))
	if !strings.Contains(page, "db-host.example.org:587") {
		t.Errorf("the saved host does not win:\n%s", page)
	}
	if !strings.Contains(page, "env@example.org") {
		t.Error("an empty field did not fall back to the environment")
	}

	// Blanking it again returns to the environment's value.
	wantRedirect(t, master.post("/admin/mail", mailForm("smtp_host", "")), "/admin/mail?done=saved")
	if !strings.Contains(body(master.get("/admin/mail")), "env-host.example.org:587") {
		t.Error("clearing the saved host did not restore the environment's")
	}
}

func TestMailPasswordIsWriteOnlyAndEncrypted(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	ts.a.env["SMTP_HOST"] = "h.example.org"
	wantRedirect(t, master.post("/admin/mail", mailForm("smtp_pass", "s3cret-password-value")), "/admin/mail?done=saved")

	if page := body(master.get("/admin/mail")); strings.Contains(page, "s3cret-password-value") {
		t.Fatal("the saved password was sent back to the browser")
	}
	var stored string
	ts.a.db.QueryRow(`SELECT value FROM site_settings WHERE key = 'SMTP_PASS'`).Scan(&stored)
	if stored == "" || strings.Contains(stored, "s3cret") {
		t.Errorf("the password is stored as %q", stored)
	}
	if all, _ := ts.a.settings.All(); all["SMTP_PASS"] != "s3cret-password-value" {
		t.Error("the password does not round-trip")
	}

	// Blank keeps it; the tickbox removes it.
	wantRedirect(t, master.post("/admin/mail", mailForm("smtp_pass", "", "smtp_user", "me")), "/admin/mail?done=saved")
	if all, _ := ts.a.settings.All(); all["SMTP_PASS"] == "" {
		t.Error("a blank password box deleted the saved password")
	}
	wantRedirect(t, master.post("/admin/mail", mailForm("clear_smtp_pass", "1")), "/admin/mail?done=saved")
	if all, _ := ts.a.settings.All(); all["SMTP_PASS"] != "" {
		t.Error("the remove-password tickbox did nothing")
	}
}

func TestBadMailSettingsAreRejectedAsAWholeAndNothingIsWritten(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	ts.a.env["SMTP_HOST"] = "good.example.org"
	cases := map[string]url.Values{
		"unknown provider":     mailForm("mail_provider", "carrier-pigeon"),
		"ses without a region": mailForm("mail_provider", "ses", "smtp_user", "u", "smtp_pass", "p", "mail_from", "a@example.org"),
		"bad port":             mailForm("smtp_port", "99999"),
		"host with spaces":     mailForm("smtp_host", "a b"),
		"from not an address":  mailForm("mail_from", "Bob <b@example.org>"),
		"too long":             mailForm("smtp_user", strings.Repeat("x", 300)),
	}
	for name, form := range cases {
		resp := master.post("/admin/mail", form)
		if resp.StatusCode == http.StatusSeeOther {
			t.Errorf("%s was accepted", name)
		}
	}
	if all, _ := ts.a.settings.All(); len(all) != 0 {
		t.Errorf("rejected input was partly written: %v", all)
	}

	// Removing the only mail server would leave the site unable to send.
	ts.a.env["SMTP_HOST"] = ""
	wantRedirect(t, master.post("/admin/mail", mailForm("smtp_host", "db.example.org")), "/admin/mail?done=saved")
	ts2 := master.post("/admin/mail", mailForm("smtp_host", ""))
	if ts2.StatusCode != http.StatusBadRequest {
		t.Errorf("clearing the only mail server = %d, want 400", ts2.StatusCode)
	}
	if all, _ := ts.a.settings.All(); all["SMTP_HOST"] != "db.example.org" {
		t.Error("the working host was removed")
	}
}

func TestPresetNeedsWhatItCannotGuess(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	_ = ts
	resp := master.post("/admin/mail", mailForm("mail_provider", "cloudflare", "mail_from", "no-reply@example.org"))
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body(resp), "SMTP_PASS") {
		t.Errorf("cloudflare without a token = %d", resp.StatusCode)
	}
	wantRedirect(t, master.post("/admin/mail", mailForm("mail_provider", "cloudflare", "smtp_pass", "cf-token", "mail_from", "no-reply@example.org")), "/admin/mail?done=saved")
	page := body(master.get("/admin/mail"))
	if !strings.Contains(page, "smtp.mx.cloudflare.net:465") || !strings.Contains(page, "api_token") {
		t.Errorf("preset not shown as in use:\n%s", page)
	}
}

func TestTestEmailGoesToTheMasterAndReportsFailure(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	ts.a.env["SMTP_HOST"] = "h.example.org"

	resp := master.post("/admin/mail/test", url.Values{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("test email = %d", resp.StatusCode)
	}
	if got := ts.a.mail.tests; len(got) != 1 || got[0] != "master@example.org" {
		t.Errorf("test email sent to %v, want only the master admin", got)
	}

	ts.a.mail.testErr = errors.New("535 5.7.8 authentication failed for someone@example.org")
	resp = master.post("/admin/mail/test", url.Values{})
	msg := body(resp)
	if resp.StatusCode != http.StatusBadGateway || !strings.Contains(msg, "was not sent") || !strings.Contains(msg, "authentication failed") {
		t.Errorf("failure not reported (%d):\n%s", resp.StatusCode, msg)
	}
	if strings.Contains(msg, "someone@example.org") {
		t.Error("an email address in the server's error was shown unredacted")
	}
	if got := master.postRaw("/admin/mail/test", url.Values{}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("test without a CSRF token = %d, want 400", got)
	}
}

func TestResetRemovesSavedSettingsAndIsAudited(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	ts.a.env["SMTP_HOST"] = "env.example.org"
	wantRedirect(t, master.post("/admin/mail", mailForm("smtp_host", "db.example.org", "smtp_pass", "pw-value-123")), "/admin/mail?done=saved")
	wantRedirect(t, master.post("/admin/mail/reset", url.Values{}), "/admin/mail?done=reset")
	if all, _ := ts.a.settings.All(); len(all) != 0 {
		t.Errorf("settings left after reset: %v", all)
	}
	page := body(master.get("/admin/mail"))
	if !strings.Contains(page, "env.example.org") {
		t.Error("the environment's host is not back in use")
	}
	for _, want := range []string{"Recent changes", "SMTP_HOST", "removed all database mail settings"} {
		if !strings.Contains(page, want) {
			t.Errorf("audit trail lacks %q", want)
		}
	}
	if strings.Contains(page, "pw-value-123") || strings.Contains(page, "db.example.org\"") && false {
		t.Error("a secret appears in the page")
	}

	// Reset is refused when the environment alone could not send mail.
	ts.a.env["SMTP_HOST"] = ""
	wantRedirect(t, master.post("/admin/mail", mailForm("smtp_host", "db.example.org")), "/admin/mail?done=saved")
	if got := master.post("/admin/mail/reset", url.Values{}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("reset that would strand the site = %d, want 400", got)
	}
}
