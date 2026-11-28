package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestResendingAnInviteAndLastUsedOnSessions(t *testing.T) {
	app := newApp(t)
	owner := app.owner("1901")
	app.member(owner, "editor1901@example.org", "editor")
	sq := app.squadronID(owner)
	var ownerID int64
	app.db.QueryRow(`SELECT id FROM users WHERE email = ?`, owner).Scan(&ownerID)
	token, err := app.auth.CreateInvite(sq, ownerID, "newbie@example.org", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	o := app.login(owner)
	page := body(o.get("/squadron"))
	if !strings.Contains(page, "Send again") || !strings.Contains(page, "Last sent") {
		t.Fatal("the pending invite has no Send again button")
	}
	// Just sent: too soon.
	wantStatus(t, o.post("/squadron/invites/"+token+"/resend", url.Values{}), http.StatusTooManyRequests)
	// Sent a while ago: goes out again, with the same link.
	app.db.Exec(`UPDATE invites SET sent_at = datetime('now', '-1 hour'), expires_at = datetime('now', '+1 hour') WHERE token = ?`, token)
	wantRedirect(t, o.post("/squadron/invites/"+token+"/resend", url.Values{}), "/squadron#invites")
	app.mail.mu.Lock()
	var got []sentMail
	for _, m := range app.mail.sent {
		if m.Kind == "invite" && m.To == "newbie@example.org" {
			got = append(got, m)
		}
	}
	app.mail.mu.Unlock()
	if len(got) != 1 || got[0].Token != token {
		t.Errorf("resent mail = %+v", got)
	}
	pending, _ := app.auth.PendingInvites(sq)
	if len(pending) != 1 || time.Until(pending[0].ExpiresAt) < 6*24*time.Hour {
		t.Errorf("the resent invite was not given a fresh week: %+v", pending)
	}
	// Only an owner of that squadron, and only a pending invite.
	other := app.login(app.owner("1902"))
	app.db.Exec(`UPDATE invites SET sent_at = datetime('now', '-1 hour') WHERE token = ?`, token)
	wantStatus(t, other.post("/squadron/invites/"+token+"/resend", url.Values{}), http.StatusNotFound)
	wantStatus(t, app.login("editor1901@example.org").post("/squadron/invites/"+token+"/resend", url.Values{}), http.StatusForbidden)
	wantStatus(t, o.post("/squadron/invites/nope/resend", url.Values{}), http.StatusNotFound)

	// Sessions record when they were last used.
	var last any
	app.db.QueryRow(`SELECT last_used_at FROM sessions WHERE user_id = ?`, ownerID).Scan(&last)
	o.get("/account")
	app.db.QueryRow(`SELECT last_used_at FROM sessions WHERE user_id = ?`, ownerID).Scan(&last)
	if last == nil {
		t.Error("a session that made requests has no last-used time")
	}
	if p := body(o.get("/account")); !strings.Contains(p, "Last used (UTC)") || strings.Contains(p, "not recorded") {
		t.Errorf("the sessions list does not show when it was last used:\n%.300s", p)
	}
}

func TestOwnersEditUniformColours(t *testing.T) {
	app := newApp(t)
	owner := app.owner("1903")
	app.member(owner, "editor1903@example.org", "editor")
	sq := app.squadronID(owner)
	o := app.login(owner)
	before, _ := app.store.GetSquadron(sq)
	first := before.UniformColours[0]
	form := url.Values{"label_" + first.Key: {"Smart dress"}, "hex_" + first.Key: {"#123456"}, "new_label": {"Flying suit"}, "new_hex": {"#abcdef"}}
	for _, c := range before.UniformColours[1:] {
		form.Set("label_"+c.Key, c.Label)
		form.Set("hex_"+c.Key, c.Hex)
	}
	wantRedirect(t, o.post("/squadron/uniforms", form), "/squadron#uniforms")
	after, _ := app.store.GetSquadron(sq)
	if len(after.UniformColours) != len(before.UniformColours)+1 || after.UniformColours[0].Label != "Smart dress" || after.UniformColours[0].Hex != "#123456" || after.UniformColours[0].Key != first.Key {
		t.Errorf("colours = %+v", after.UniformColours)
	}
	if !strings.Contains(body(o.get("/squadron")), "Smart dress") {
		t.Error("the settings page does not show the new name")
	}
	wantStatus(t, o.post("/squadron/uniforms", url.Values{"label_" + first.Key: {"X"}, "hex_" + first.Key: {"nothex"}}), http.StatusBadRequest)
	wantStatus(t, app.login("editor1903@example.org").post("/squadron/uniforms", form), http.StatusForbidden)
	// A form that lists only some colours leaves the rest alone.
	wantRedirect(t, o.post("/squadron/uniforms", url.Values{"label_" + first.Key: {"Smart dress"}, "hex_" + first.Key: {"#123456"}}), "/squadron#uniforms")
	if again, _ := app.store.GetSquadron(sq); len(again.UniformColours) != len(after.UniformColours) {
		t.Errorf("a partial form changed the number of colours: %d", len(again.UniformColours))
	}
}
