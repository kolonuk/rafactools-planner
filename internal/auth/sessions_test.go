package auth

import (
	"strings"
	"testing"
	"time"
)

func TestDeviceLabelIsFromAFixedVocabulary(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0":                                                         "Firefox on Linux",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36 Edg/130.0":          "Edge on Windows",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile Safari/604": "Safari on iOS",
		"Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/130.0 Mobile Safari/537.36":                                           "Chrome on Android",
		"":                                 "Unknown browser",
		"<script>alert(1)</script> curl/8": "Unknown browser",
	} {
		if got := DeviceLabel(ua); got != want {
			t.Errorf("DeviceLabel(%q) = %q, want %q", ua, got, want)
		}
	}
}

func TestSessionsListSignOutAndOwnership(t *testing.T) {
	svc := newTestService(t)
	aID, _, err := svc.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "A")
	if err != nil {
		t.Fatal(err)
	}
	bID, _, err := svc.ClaimSquadron("snco.4321@rafac.mod.gov.uk", "ownerpassword1", "B")
	if err != nil {
		t.Fatal(err)
	}
	cur, _, _ := svc.CreateSessionFor(aID, "Mozilla/5.0 (X11; Linux x86_64) Firefox/130.0")
	other, _, _ := svc.CreateSessionFor(aID, "Mozilla/5.0 (Windows NT 10.0) Chrome/130.0 Safari/537.36")
	bTok, _, _ := svc.CreateSessionFor(bID, "")

	list, err := svc.ListSessions(aID, cur)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %v, %v", list, err)
	}
	var currentCount int
	var otherID string
	for _, si := range list {
		if strings.Contains(si.ID, cur) || strings.Contains(si.ID, other) {
			t.Errorf("session id %q contains the cookie value", si.ID)
		}
		if si.Current {
			currentCount++
			if si.Device != "Firefox on Linux" {
				t.Errorf("current device = %q", si.Device)
			}
		} else {
			otherID = si.ID
		}
	}
	if currentCount != 1 || otherID == "" {
		t.Fatalf("current=%d other=%q", currentCount, otherID)
	}

	// Another person's session id, the current session, and nonsense are all refused.
	bList, _ := svc.ListSessions(bID, bTok)
	if err := svc.RevokeSession(aID, bList[0].ID, cur); err != ErrNoSuchSession {
		t.Errorf("revoking another user's session = %v, want ErrNoSuchSession", err)
	}
	if u, _ := svc.UserBySession(bTok); u == nil {
		t.Error("another user's session was ended")
	}
	curID := ""
	for _, si := range list {
		if si.Current {
			curID = si.ID
		}
	}
	if err := svc.RevokeSession(aID, curID, cur); err != ErrNoSuchSession {
		t.Errorf("revoking the current session = %v, want ErrNoSuchSession (use Log out)", err)
	}
	if err := svc.RevokeSession(aID, "nonsense", cur); err != ErrNoSuchSession {
		t.Errorf("revoking nonsense = %v", err)
	}

	if err := svc.RevokeSession(aID, otherID, cur); err != nil {
		t.Fatal(err)
	}
	if u, _ := svc.UserBySession(other); u != nil {
		t.Error("revoked session still works")
	}
	if u, _ := svc.UserBySession(cur); u == nil {
		t.Error("current session was ended")
	}

	svc.CreateSessionFor(aID, "")
	svc.CreateSessionFor(aID, "")
	n, err := svc.RevokeOtherSessions(aID, cur)
	if err != nil || n != 2 {
		t.Errorf("RevokeOtherSessions = %d, %v, want 2", n, err)
	}
	if left, _ := svc.ListSessions(aID, cur); len(left) != 1 || !left[0].Current {
		t.Errorf("after sign out everywhere else: %+v", left)
	}
	if u, _ := svc.UserBySession(bTok); u == nil {
		t.Error("signing out everywhere ended another user's session")
	}
}

func TestMasterAdminSessionsAreShort(t *testing.T) {
	s, _ := newService(t, false)
	if _, err := s.EnsureMasterAdmin(masterEmail, masterPW); err != nil {
		t.Fatal(err)
	}
	m, err := s.Authenticate(masterEmail, masterPW)
	if err != nil {
		t.Fatal(err)
	}
	_, exp, err := s.CreateSessionFor(m.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(exp); d > 9*time.Hour || d < 7*time.Hour {
		t.Errorf("master session lasts %v, want about 8h", d)
	}

	ownerID, _, _ := s.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "A")
	_, exp, _ = s.CreateSessionFor(ownerID, "")
	if d := time.Until(exp); d < 29*24*time.Hour {
		t.Errorf("ordinary session lasts %v, want 30 days", d)
	}
}

func TestOverlongUserAgentIsCapped(t *testing.T) {
	svc := newTestService(t)
	id, _, _ := svc.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "A")
	tok, _, _ := svc.CreateSessionFor(id, strings.Repeat("A", 5000))
	var n int
	svc.db.QueryRow(`SELECT LENGTH(user_agent) FROM sessions WHERE token = ?`, tok).Scan(&n)
	if n != maxUserAgent {
		t.Errorf("stored %d bytes, want %d", n, maxUserAgent)
	}
}
