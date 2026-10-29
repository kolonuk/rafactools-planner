package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSecurityLogRecordsSignInsFailuresAndPasswordChanges(t *testing.T) {
	app := newApp(t)
	email := app.owner("1501")
	owner := app.login(email)

	// A wrong password for a real account is logged (once, however often it is repeated).
	for i := 0; i < 3; i++ {
		c := app.anon()
		c.post("/login", url.Values{"csrf": {scrapeCSRF(t, c.get("/login"))}, "email": {email}, "password": {"not the password at all"}})
		c.xff = "198.51.100." + strconv.Itoa(i+1)
	}
	// An unknown address leaves no trace: there is no squadron to show it to.
	c := app.anon()
	c.postRaw("/login", url.Values{"csrf": {scrapeCSRF(t, c.get("/login"))}, "email": {"nobody@example.org"}, "password": {"whatever whatever"}})

	wantRedirect(t, owner.post("/account/password", url.Values{"current_password": {testPassword}, "new_password": {"a brand new long password"}}), "/account?changed=1")
	owner2 := app.anon()
	if resp := owner2.postRaw("/login", url.Values{"csrf": {scrapeCSRF(t, owner2.get("/login"))}, "email": {email}, "password": {"a brand new long password"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login with the new password = %d", resp.StatusCode)
	}

	page := body(owner.get("/squadron"))
	if got := strings.Count(page, "Wrong password entered"); got != 1 {
		t.Errorf("%d failed-login lines, want exactly 1 (repeats within a minute are collapsed)", got)
	}
	for _, want := range []string{"Signed in", "Changed their password"} {
		if !strings.Contains(page, want) {
			t.Errorf("log lacks %q", want)
		}
	}
	if strings.Contains(page, "nobody@example.org") {
		t.Error("an unknown address appears in a squadron's log")
	}
	if strings.Contains(page, "not the password at all") || strings.Contains(page, "a brand new long password") {
		t.Error("a password appears in the log")
	}
}

func TestPasswordResetAndSignOutOthersAndSettingsAreLogged(t *testing.T) {
	app := newApp(t)
	email := app.owner("1502")
	a, b := app.login(email), app.login(email)

	token, send, err := app.auth.RequestPasswordReset(email)
	if err != nil || !send {
		t.Fatalf("reset request: %v %v", send, err)
	}
	r := app.anon()
	wantRedirect(t, r.postRaw("/reset/"+token, url.Values{"csrf": {scrapeCSRF(t, r.get("/reset/"+token))}, "password": {testPassword}}), "/login?reset=1")

	owner := app.login(email) // the reset signed everyone out
	owner.post("/squadron", url.Values{"name": {"Renamed"}, "subtitle": {"x"}, "protective_marking": {"OFFICIAL"}, "classifications": {"A\nB"}})
	app.login(email)
	wantRedirect(t, owner.post("/account/sessions/signout-others", url.Values{}), "/account?signedout=1")
	_ = a
	_ = b

	page := body(owner.get("/squadron"))
	for _, want := range []string{"Reset their password from the emailed link", "Saved the squadron settings", "Signed out their other sessions"} {
		if !strings.Contains(page, want) {
			t.Errorf("log lacks %q", want)
		}
	}
}

func TestOwnersCanDownloadTheSecurityLogAsASafeCSV(t *testing.T) {
	ts, master, sqA, _ := masterApp(t)
	master.post("/account/switch", url.Values{"squadron_id": {strconv.FormatInt(sqA, 10)}})
	// A hostile-looking actor address and detail must not become a formula.
	if err := ts.a.auth.Audit(sqA, 0, "=HYPERLINK(\"http://evil.example\")@example.org", "invite.sent", "+cmd|' /C calc'!A0"); err != nil {
		t.Fatal(err)
	}

	resp := ts.ownerA.get("/squadron/security-log.csv")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/csv") || !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download = %d %q %q", resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Disposition"))
	}
	csv := body(resp)
	if !strings.HasPrefix(csv, "when_utc,who,action,detail") || !strings.Contains(csv, "Site administrator viewed this squadron") {
		t.Errorf("csv:\n%s", csv)
	}
	for _, line := range strings.Split(csv, "\n") {
		for _, cell := range strings.Split(line, ",") {
			c := strings.Trim(cell, `"`)
			if c != "" && strings.ContainsRune("=+-@", rune(c[0])) {
				t.Errorf("a cell starts with a formula character: %q", cell)
			}
		}
	}

	// Only owners, and only their own squadron's log.
	if got := ts.ownerB.get("/squadron/security-log.csv"); !strings.Contains(body(got), "Site administrator viewed") == false {
		t.Error("squadron B's download contains squadron A's entries")
	}
	ts.a.member(ts.ownerAEmail, "editor-csv@example.org", "editor")
	if got := ts.a.login("editor-csv@example.org").get("/squadron/security-log.csv").StatusCode; got != http.StatusForbidden {
		t.Errorf("editor download = %d, want 403", got)
	}
	if got := ts.a.anon().get("/squadron/security-log.csv").StatusCode; got != http.StatusSeeOther {
		t.Errorf("anonymous download = %d, want a redirect to log in", got)
	}
}

func TestOwnersAreEmailedWhenTheMasterAdminViewsTheirSquadronAtMostHourly(t *testing.T) {
	ts, master, sqA, _ := masterApp(t)
	idA := strconv.FormatInt(sqA, 10)
	ts.a.member(ts.ownerAEmail, "editor-n@example.org", "editor")

	wait := func(n int) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			ts.a.mail.mu.Lock()
			got := len(ts.a.mail.notices)
			ts.a.mail.mu.Unlock()
			if got >= n {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("expected %d notice(s)", n)
	}
	master.post("/account/switch", url.Values{"squadron_id": {idA}})
	wait(1)
	master.post("/account/switch", url.Values{"back": {"1"}})
	master.post("/account/switch", url.Values{"squadron_id": {idA}})
	time.Sleep(200 * time.Millisecond)

	ts.a.mail.mu.Lock()
	defer ts.a.mail.mu.Unlock()
	if len(ts.a.mail.notices) != 1 {
		t.Fatalf("%d notices, want 1 (throttled to one an hour): %v", len(ts.a.mail.notices), ts.a.mail.notices)
	}
	if !strings.HasPrefix(ts.a.mail.notices[0], ts.ownerAEmail+"|") {
		t.Errorf("notice went to %q, want the owner only (not the editor)", ts.a.mail.notices[0])
	}
}

func TestSignInsCannotPushSecurityEventsOffTheLog(t *testing.T) {
	ts, master, sqA, _ := masterApp(t)
	master.post("/account/switch", url.Values{"squadron_id": {strconv.FormatInt(sqA, 10)}})
	var uid int64
	ts.a.db.QueryRow(`SELECT id FROM users WHERE email = ?`, ts.ownerAEmail).Scan(&uid)
	for i := 0; i < 150; i++ {
		ts.a.auth.AuditUser(uid, "login", "")
	}
	page := body(ts.ownerA.get("/squadron"))
	main, signIns, ok := strings.Cut(page, "Recent sign-ins")
	if !ok {
		t.Fatal("no sign-ins section")
	}
	if !strings.Contains(main, "Site administrator viewed this squadron") {
		t.Error("150 sign-ins pushed the master admin's visit off the security log")
	}
	if strings.Count(main, ">Signed in<") != 0 {
		t.Error("sign-ins appear in the security table")
	}
	if got := strings.Count(signIns, ">Signed in<"); got != 30 {
		t.Errorf("%d sign-ins listed, want the newest 30", got)
	}
	// The download still has everything.
	csv := body(ts.ownerA.get("/squadron/security-log.csv"))
	if got := strings.Count(csv, "Signed in"); got < 150 {
		t.Errorf("csv holds %d sign-ins, want all (>=150)", got)
	}
}
