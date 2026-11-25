package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"rafactools-planner/internal/auth"
)

var transferLink = regexp.MustCompile(`/transfer/([A-Za-z0-9_-]+)`)

func (a *testApp) notesTo(to string) []sentNotice {
	a.mail.mu.Lock()
	defer a.mail.mu.Unlock()
	var out []sentNotice
	for _, n := range a.mail.notes {
		if n.To == to {
			out = append(out, n)
		}
	}
	return out
}

func (a *testApp) memberID(t *testing.T, email string) int64 {
	t.Helper()
	var id int64
	if err := a.db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (a *testApp) signInStatus(email string) (int, string) {
	c := a.anon()
	resp := c.postRaw("/login", url.Values{"csrf": {scrapeCSRF(a.t, c.get("/login"))}, "email": {email}, "password": {testPassword}})
	return resp.StatusCode, body(resp)
}

func TestMovingALoginWhenThePersonAsks(t *testing.T) {
	app := newApp(t)
	ownerA, ownerB := app.owner("1801"), app.owner("1802")
	app.member(ownerA, "mover1801@example.org", "editor")
	sqA, sqB := app.squadronID(ownerA), app.squadronID(ownerB)
	mover := app.login("mover1801@example.org")
	a, b := app.login(ownerA), app.login(ownerB)

	if page := body(mover.get("/account/transfer")); !strings.Contains(page, "Number of the squadron to join") {
		t.Fatalf("the page has no form:\n%.500s", page)
	}
	wantStatus(t, mover.post("/account/transfer", url.Values{"number": {"1802"}, "password": {"wrong password!"}}), http.StatusBadRequest)
	wantStatus(t, mover.post("/account/transfer", url.Values{"number": {"9999"}, "password": {testPassword}}), http.StatusBadRequest)
	wantStatus(t, mover.post("/account/transfer", url.Values{"number": {"1801"}, "password": {testPassword}}), http.StatusBadRequest)
	// A claim-domain address cannot move.
	if page := body(a.get("/account/transfer")); !strings.Contains(page, "claim-domain") {
		t.Errorf("an official address is offered the form:\n%.500s", page)
	}
	wantStatus(t, a.post("/account/transfer", url.Values{"number": {"1802"}, "password": {testPassword}}), http.StatusBadRequest)

	wantRedirect(t, mover.post("/account/transfer", url.Values{"number": {"1802"}, "password": {testPassword}}), "/login?transfer=1")

	t.Run("the account is switched off and the right people are told", func(t *testing.T) {
		status, page := app.signInStatus("mover1801@example.org")
		if status == http.StatusSeeOther || !strings.Contains(page, "switched off") {
			t.Errorf("signing in: %d\n%.400s", status, page)
		}
		personal := app.notesTo("mover1801@example.org")
		if len(personal) != 1 || !transferLink.MatchString(personal[0].Body) || !strings.Contains(personal[0].Body, "switched off") {
			t.Fatalf("the person's email: %+v", personal)
		}
		if got := app.notesTo(ownerB); len(got) != 1 || !strings.Contains(got[0].Subject, "join your squadron") || !strings.Contains(got[0].Body, "/squadron#transfers") {
			t.Errorf("the target owner's email: %+v", got)
		}
		if got := app.notesTo(ownerA); len(got) != 1 || !strings.Contains(got[0].Body, "switched off") {
			t.Errorf("the old owner's email: %+v", got)
		}
		for _, sq := range []int64{sqA, sqB} {
			entries, _ := app.auth.ListAuditSecurity(sq, 20)
			found := false
			for _, e := range entries {
				found = found || (e.Action == auth.AuditTransfer && strings.HasPrefix(e.Detail, "requested"))
			}
			if !found {
				t.Errorf("squadron %d's security log has no line for the request", sq)
			}
		}
		if page := body(b.get("/squadron")); !strings.Contains(page, "mover1801@example.org") || !strings.Contains(page, "/transfers/") {
			t.Error("the target's owner is not offered the request")
		}
		if page := body(a.get("/squadron")); !strings.Contains(page, "Waiting for an owner there") {
			t.Error("the old squadron's owner cannot see the move")
		}
	})

	t.Run("only an owner of the target accepts, and chooses the role", func(t *testing.T) {
		in, _ := app.auth.IncomingTransfers(sqB)
		id := strconv.FormatInt(in[0].ID, 10)
		wantStatus(t, a.post("/squadron/transfers/"+id+"/accept", url.Values{"role": {"owner"}}), http.StatusForbidden)
		wantStatus(t, b.post("/squadron/transfers/"+id+"/accept", url.Values{"role": {"emperor"}}), http.StatusBadRequest)
		wantRedirect(t, b.post("/squadron/transfers/"+id+"/accept", url.Values{"role": {"viewer"}}), "/squadron#transfers")

		c := app.login("mover1801@example.org")
		if page := body(c.get("/")); !strings.Contains(page, "1802") {
			t.Errorf("the dashboard does not show the new squadron:\n%.400s", page)
		}
		wantStatus(t, c.post("/programmes/1/copy", url.Values{"source": {"2"}}), http.StatusForbidden) // a viewer now
		if got := app.notesTo("mover1801@example.org"); len(got) != 2 || !strings.Contains(got[1].Body, "accepted you as viewer") {
			t.Errorf("the person's second email: %+v", got)
		}
		if _, _, active := func() (int64, string, bool) {
			var sq int64
			var role string
			var act int
			app.db.QueryRow(`SELECT squadron_id, role, active FROM users WHERE email = 'mover1801@example.org'`).Scan(&sq, &role, &act)
			return sq, role, act == 1
		}(); !active {
			t.Error("the account is still switched off after the move")
		}
		if got := app.squadronID("mover1801@example.org"); got != sqB {
			t.Errorf("squadron = %d, want %d", got, sqB)
		}
	})
}

func TestMovingALoginWhenAnOwnerAsksThePersonMustAgree(t *testing.T) {
	app := newApp(t)
	ownerA, ownerB := app.owner("1803"), app.owner("1804")
	app.member(ownerA, "member1803@example.org", "editor")
	app.member(ownerA, "official-viewer@example.org", "viewer")
	sqA := app.squadronID(ownerA)
	a, b := app.login(ownerA), app.login(ownerB)
	member := app.memberID(t, "member1803@example.org")
	mid := strconv.FormatInt(member, 10)

	wantStatus(t, app.login("official-viewer@example.org").post("/squadron/members/"+mid+"/transfer", url.Values{"number": {"1804"}}), http.StatusForbidden)
	wantStatus(t, b.post("/squadron/members/"+mid+"/transfer", url.Values{"number": {"1803"}}), http.StatusNotFound) // not B's member
	wantStatus(t, a.post("/squadron/members/"+mid+"/transfer", url.Values{"number": {"9999"}}), http.StatusBadRequest)
	ownerAID := strconv.FormatInt(app.memberID(t, ownerA), 10)
	wantStatus(t, a.post("/squadron/members/"+ownerAID+"/transfer", url.Values{"number": {"1804"}}), http.StatusBadRequest) // their own, and an official address

	wantRedirect(t, a.post("/squadron/members/"+mid+"/transfer", url.Values{"number": {"1804"}}), "/squadron#transfers")
	personal := app.notesTo("member1803@example.org")
	if len(personal) != 1 || !strings.Contains(personal[0].Body, "Nothing happens unless you agree") {
		t.Fatalf("the person's email: %+v", personal)
	}
	token := transferLink.FindStringSubmatch(personal[0].Body)[1]
	if got := app.notesTo(ownerB); len(got) != 0 {
		t.Error("the target's owners were told before the person agreed")
	}
	if status, _ := app.signInStatus("member1803@example.org"); status != http.StatusSeeOther {
		t.Errorf("the account was switched off before the person agreed (%d)", status)
	}
	if in, _ := app.auth.IncomingTransfers(app.squadronID(ownerB)); len(in) != 0 {
		t.Error("the target can see a move that is not agreed")
	}

	anon := app.anon()
	if page := body(anon.get("/transfer/" + token)); !strings.Contains(page, "Agree to the move") || !strings.Contains(page, "member1803@example.org") {
		t.Fatalf("the link page:\n%.600s", page)
	}
	wantStatus(t, anon.get("/transfer/not-a-token"), http.StatusNotFound)
	post := func(c *client, action string) *http.Response {
		return c.postRaw("/transfer/"+token+"/"+action, url.Values{"csrf": {scrapeCSRF(t, c.get("/transfer/"+token))}})
	}
	wantStatus(t, post(app.anon(), "burn"), http.StatusNotFound)
	wantRedirect(t, post(app.anon(), "agree"), "/transfer/"+token)
	if status, page := app.signInStatus("member1803@example.org"); status == http.StatusSeeOther || !strings.Contains(page, "switched off") {
		t.Errorf("after agreeing the account is still on (%d)", status)
	}
	if got := app.notesTo(ownerB); len(got) != 1 {
		t.Errorf("the target's owners were told %d times", len(got))
	}
	wantStatus(t, post(app.anon(), "agree"), http.StatusNotFound) // already agreed

	t.Run("a decline by the target brings the account back in the old squadron", func(t *testing.T) {
		in, _ := app.auth.IncomingTransfers(app.squadronID(ownerB))
		wantRedirect(t, b.post("/squadron/transfers/"+strconv.FormatInt(in[0].ID, 10)+"/decline", url.Values{}), "/squadron#transfers")
		if status, _ := app.signInStatus("member1803@example.org"); status != http.StatusSeeOther {
			t.Errorf("the account is not back on (%d)", status)
		}
		if app.squadronID("member1803@example.org") != sqA {
			t.Error("the member left the squadron although the move was declined")
		}
	})

	t.Run("the person can cancel by their link while switched off", func(t *testing.T) {
		app.member(ownerA, "member1803b@example.org", "editor")
		mid2 := strconv.FormatInt(app.memberID(t, "member1803b@example.org"), 10)
		wantRedirect(t, a.post("/squadron/members/"+mid2+"/transfer", url.Values{"number": {"1804"}}), "/squadron#transfers")
		notes := app.notesTo("member1803b@example.org")
		tok2 := transferLink.FindStringSubmatch(notes[len(notes)-1].Body)[1]
		c := app.anon()
		wantRedirect(t, c.postRaw("/transfer/"+tok2+"/agree", url.Values{"csrf": {scrapeCSRF(t, c.get("/transfer/"+tok2))}}), "/transfer/"+tok2)
		c2 := app.anon()
		wantRedirect(t, c2.postRaw("/transfer/"+tok2+"/cancel", url.Values{"csrf": {scrapeCSRF(t, c2.get("/transfer/"+tok2))}}), "/transfer/"+tok2)
		if status, _ := app.signInStatus("member1803b@example.org"); status != http.StatusSeeOther {
			t.Errorf("not switched back on after cancelling (%d)", status)
		}
	})

	t.Run("an owner cannot switch the account on while the move waits", func(t *testing.T) {
		app.member(ownerA, "member1803c@example.org", "editor")
		id := app.memberID(t, "member1803c@example.org")
		wantRedirect(t, a.post("/squadron/members/"+strconv.FormatInt(id, 10)+"/transfer", url.Values{"number": {"1804"}}), "/squadron#transfers")
		notes := app.notesTo("member1803c@example.org")
		tok := transferLink.FindStringSubmatch(notes[len(notes)-1].Body)[1]
		c := app.anon()
		wantRedirect(t, c.postRaw("/transfer/"+tok+"/agree", url.Values{"csrf": {scrapeCSRF(t, c.get("/transfer/"+tok))}}), "/transfer/"+tok)
		wantStatus(t, a.post("/squadron/members/"+strconv.FormatInt(id, 10)+"/active", url.Values{"active": {"1"}}), http.StatusBadRequest)
	})
}
