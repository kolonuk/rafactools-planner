package web

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestSecurityLogShowsMasterViewsAndMemberChangesToOwnersOnly(t *testing.T) {
	ts, master, sqA, _ := masterApp(t)
	idA := strconv.FormatInt(sqA, 10)

	// Signing in is itself recorded now, but nobody has looked yet.
	if page := body(ts.ownerA.get("/squadron")); strings.Contains(page, "Site administrator viewed") {
		t.Fatalf("a master admin visit is listed before there was one:\n%s", page)
	}

	// The master admin looks at squadron A: the owner can see that they did.
	master.post("/account/switch", url.Values{"squadron_id": {idA}})
	page := body(ts.ownerA.get("/squadron"))
	if !strings.Contains(page, "Site administrator viewed this squadron") || !strings.Contains(page, "master@example.org") {
		t.Errorf("owner's log lacks the master admin's visit:\n%s", page)
	}

	// The owner's own changes appear too: invite, role change, removal.
	ts.a.member(ts.ownerAEmail, "newbie@example.org", "viewer")
	var memberID int64
	members, _ := ts.a.auth.ListMembers(sqA)
	for _, m := range members {
		if m.Email == "newbie@example.org" {
			memberID = m.ID
		}
	}
	mid := strconv.FormatInt(memberID, 10)
	ts.ownerA.post("/squadron/members/"+mid+"/role", url.Values{"role": {"editor"}})
	ts.ownerA.post("/squadron/invites", url.Values{"email": {"another@example.org"}, "role": {"viewer"}})
	ts.ownerA.post("/squadron/members/"+mid+"/remove", url.Values{})
	page = body(ts.ownerA.get("/squadron"))
	for _, want := range []string{"Changed a member", "newbie@example.org to editor", "Sent an invitation", "another@example.org as viewer", "Removed a member"} {
		if !strings.Contains(page, want) {
			t.Errorf("log lacks %q", want)
		}
	}

	// Another squadron's owner sees none of it.
	if other := body(ts.ownerB.get("/squadron")); strings.Contains(other, "master@example.org") || strings.Contains(other, "newbie@example.org") {
		t.Error("squadron B's log leaks squadron A's entries")
	}

	// The master admin sees which squadrons they have looked at.
	if acct := body(master.get("/account")); !strings.Contains(acct, "Squadrons you have looked at") || !strings.Contains(acct, "501") {
		t.Errorf("master's account lacks the views list:\n%s", acct)
	}
}

func TestSwitchingBackIsNotLoggedAsAView(t *testing.T) {
	ts, master, sqA, _ := masterApp(t)
	master.post("/account/switch", url.Values{"squadron_id": {strconv.FormatInt(sqA, 10)}})
	master.post("/account/switch", url.Values{"back": {"1"}})
	entries, _ := ts.a.auth.ListAuditAll("master.view", 50)
	if len(entries) != 1 {
		t.Errorf("%d view entries, want 1", len(entries))
	}
}
