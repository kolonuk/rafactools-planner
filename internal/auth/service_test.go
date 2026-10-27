package auth

import (
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"rafactools-planner/internal/db"
)

const (
	testDomain = "rafac.mod.gov.uk"
	goodPW     = "twelve-chars!"
)

func newService(t *testing.T, skipVerify bool) (*Service, *sql.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return NewService(d, "unit-test-session-key-0123456789abcdef", false, testDomain, skipVerify), d
}

func claimVerified(t *testing.T, s *Service, number string) (email string, userID int64) {
	t.Helper()
	email = "oc." + number + "@" + testDomain
	id, tok, err := s.ClaimSquadron(email, goodPW, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyEmail(tok); err != nil {
		t.Fatal(err)
	}
	return email, id
}

func TestParseClaimEmailEdgeCases(t *testing.T) {
	for _, tc := range []struct {
		in     string
		number string
		ok     bool
	}{
		{"  OC.12@RAFAC.MOD.GOV.UK  ", "12", true},     // trimmed + case-folded
		{"oc.0@rafac.mod.gov.uk", "0", true},           // zero is a number
		{"oc.12@rafac.mod.gov.uk.evil.com", "", false}, // suffix attack
		{"oc.12@evil.com@rafac.mod.gov.uk", "", false}, // last @ wins, local part has @
		{"x@oc.12@rafac.mod.gov.uk", "", false},        // local part "x@oc.12"
		{"oc.12\n@rafac.mod.gov.uk", "", false},        // newline in local part
		{"oc.12\r\nBcc:x@rafac.mod.gov.uk", "", false}, // header injection shape
		{"oc.１２@rafac.mod.gov.uk", "", false},          // fullwidth digits are not [0-9]
		{"oc.-1@rafac.mod.gov.uk", "", false},
		{"oc..12@rafac.mod.gov.uk", "", false},
		{"snco.12.13@rafac.mod.gov.uk", "", false},
		{"@rafac.mod.gov.uk", "", false},
		{"", "", false},
	} {
		n, ok := ParseClaimEmail(tc.in, testDomain)
		if ok != tc.ok || n != tc.number {
			t.Errorf("ParseClaimEmail(%q) = (%q,%v), want (%q,%v)", tc.in, n, ok, tc.number, tc.ok)
		}
	}
}

func FuzzParseClaimEmail(f *testing.F) {
	for _, s := range []string{"oc.1@rafac.mod.gov.uk", "SNCO.99@RAFAC.MOD.GOV.UK", "", "@", "a@b@c", "oc.1\x00@rafac.mod.gov.uk"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		n, ok := ParseClaimEmail(in, testDomain)
		if !ok {
			if n != "" {
				t.Fatalf("number %q returned with ok=false", n)
			}
			return
		}
		// Invariants for anything accepted: digits only, correct domain,
		// no control characters smuggled through.
		for _, r := range n {
			if r < '0' || r > '9' {
				t.Fatalf("non-digit in number %q from %q", n, in)
			}
		}
		low := strings.ToLower(strings.TrimSpace(in))
		if !strings.HasSuffix(low, "@"+testDomain) || strings.ContainsAny(low, "\r\n\x00 \t") {
			t.Fatalf("accepted unsafe address %q", in)
		}
	})
}

func TestClaimSquadron(t *testing.T) {
	s, d := newService(t, false)

	t.Run("password length boundary", func(t *testing.T) {
		if _, _, err := s.ClaimSquadron("oc.1@"+testDomain, strings.Repeat("a", 11), ""); err == nil {
			t.Error("11-char password accepted")
		}
		if _, _, err := s.ClaimSquadron("oc.1@"+testDomain, strings.Repeat("a", 12), ""); err != nil {
			t.Errorf("12-char password rejected: %v", err)
		}
	})

	t.Run("default squadron name", func(t *testing.T) {
		var name string
		d.QueryRow(`SELECT name FROM squadrons WHERE number = '1'`).Scan(&name)
		if name != "1 Squadron" {
			t.Errorf("name = %q", name)
		}
	})

	t.Run("password is stored hashed", func(t *testing.T) {
		var hash string
		d.QueryRow(`SELECT password_hash FROM users WHERE email = 'oc.1@` + testDomain + `'`).Scan(&hash)
		if hash == strings.Repeat("a", 12) || !strings.HasPrefix(hash, "$2") {
			t.Errorf("password not bcrypt-hashed: %q", hash)
		}
	})

	t.Run("duplicate squadron", func(t *testing.T) {
		if _, _, err := s.ClaimSquadron("snco.1@"+testDomain, goodPW, ""); err != ErrSquadronTaken {
			t.Errorf("err = %v, want ErrSquadronTaken", err)
		}
	})

	t.Run("failed claim leaves nothing behind", func(t *testing.T) {
		// An invite can register any address, including one that looks like
		// a claim address for a squadron nobody has claimed. Claiming that
		// squadron then inserts the squadron row, fails on the user row
		// (email taken) and must roll the squadron back.
		var sqID, ownerID int64
		d.QueryRow(`SELECT squadron_id, id FROM users WHERE email = 'oc.1@' || ?`, testDomain).Scan(&sqID, &ownerID)
		tok, _ := s.CreateInvite(sqID, ownerID, "oc.61@"+testDomain, "viewer")
		if _, _, err := s.AcceptInvite(tok, goodPW); err != nil {
			t.Fatal(err)
		}
		before := rowCount(t, d, "squadrons")
		if _, _, err := s.ClaimSquadron("oc.61@"+testDomain, goodPW, ""); err != ErrEmailTaken {
			t.Fatalf("err = %v, want ErrEmailTaken", err)
		}
		if after := rowCount(t, d, "squadrons"); after != before {
			t.Errorf("squadrons %d -> %d: orphan row left by failed claim", before, after)
		}
	})
}

func rowCount(t *testing.T, d *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Many people racing to claim the same squadron: exactly one must win.
func TestClaimSquadronIsRaceSafe(t *testing.T) {
	s, d := newService(t, true)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			prefix := []string{"oc", "snco"}[i%2]
			_, _, err := s.ClaimSquadron(prefix+".77@"+testDomain, goodPW, "")
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if err != ErrSquadronTaken && err != ErrEmailTaken {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 || rowCount(t, d, "squadrons") != 1 {
		t.Fatalf("wins=%d squadrons=%d, want 1/1", wins, rowCount(t, d, "squadrons"))
	}
}

func TestEmailVerification(t *testing.T) {
	s, d := newService(t, false)
	email := "oc.10@" + testDomain
	_, tok, err := s.ClaimSquadron(email, goodPW, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 64 {
		t.Errorf("token length %d, want 64 hex chars (256 bits)", len(tok))
	}

	if _, err := s.Authenticate(email, goodPW); err != ErrEmailNotVerified {
		t.Fatalf("unverified login: %v, want ErrEmailNotVerified", err)
	}
	if err := s.VerifyEmail("nope"); err != ErrTokenInvalid {
		t.Errorf("bogus token: %v", err)
	}
	if err := s.VerifyEmail(tok); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyEmail(tok); err != ErrTokenInvalid {
		t.Errorf("token reused: %v", err)
	}
	if _, err := s.Authenticate(email, goodPW); err != nil {
		t.Errorf("verified login: %v", err)
	}

	t.Run("expired token", func(t *testing.T) {
		email := "oc.11@" + testDomain
		_, tok, _ := s.ClaimSquadron(email, goodPW, "")
		d.Exec(`UPDATE email_verifications SET expires_at = ? WHERE token = ?`, time.Now().Add(-time.Second), tok)
		if err := s.VerifyEmail(tok); err != ErrTokenInvalid {
			t.Errorf("expired token: %v", err)
		}
		if _, err := s.Authenticate(email, goodPW); err != ErrEmailNotVerified {
			t.Errorf("user verified by an expired token: %v", err)
		}
	})
}

func TestAuthenticate(t *testing.T) {
	s, _ := newService(t, false)
	email, _ := claimVerified(t, s, "20")

	u, err := s.Authenticate("  "+strings.ToUpper(email), goodPW)
	if err != nil || u.Role != "owner" || u.Email != email {
		t.Fatalf("Authenticate = %+v, %v", u, err)
	}
	for name, pw := range map[string]string{"wrong": "wrong-password!", "empty": "", "case-changed": strings.ToUpper(goodPW), "prefix": goodPW[:6]} {
		if _, err := s.Authenticate(email, pw); err != ErrInvalidCredentials {
			t.Errorf("%s password: %v", name, err)
		}
	}
	if _, err := s.Authenticate("ghost@example.org", goodPW); err != ErrInvalidCredentials {
		t.Errorf("unknown user: %v", err)
	}
	// SQL metacharacters are just data.
	if _, err := s.Authenticate("' OR '1'='1", "x"); err != ErrInvalidCredentials {
		t.Errorf("sql-ish email: %v", err)
	}
}

// Unknown-email logins must still pay for a bcrypt comparison, otherwise
// response time reveals which addresses have accounts.
func TestAuthenticateUnknownUserCostsABcrypt(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	s, _ := newService(t, false)
	email, _ := claimVerified(t, s, "21")

	best := func(f func()) time.Duration {
		min := time.Hour
		for i := 0; i < 3; i++ {
			st := time.Now()
			f()
			if d := time.Since(st); d < min {
				min = d
			}
		}
		return min
	}
	known := best(func() { s.Authenticate(email, "wrong-password!") })
	unknown := best(func() { s.Authenticate("ghost@example.org", "wrong-password!") })
	if unknown < known/4 {
		t.Errorf("unknown-user login took %v vs %v for a real user: timing oracle", unknown, known)
	}
}

func TestSessions(t *testing.T) {
	s, d := newService(t, false)
	_, uid := claimVerified(t, s, "30")

	tok, exp, err := s.CreateSession(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 64 || time.Until(exp) < 29*24*time.Hour || time.Until(exp) > 31*24*time.Hour {
		t.Errorf("token len %d, expiry in %v", len(tok), time.Until(exp))
	}
	tok2, _, _ := s.CreateSession(uid)
	if tok == tok2 {
		t.Fatal("session tokens repeat")
	}

	if u, err := s.UserBySession(tok); err != nil || u == nil || u.ID != uid {
		t.Fatalf("UserBySession = %+v, %v", u, err)
	}
	if u, _ := s.UserBySession(""); u != nil {
		t.Error("empty token resolved to a user")
	}
	if u, _ := s.UserBySession("0000"); u != nil {
		t.Error("unknown token resolved to a user")
	}

	if err := s.DeleteSession(tok); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.UserBySession(tok); u != nil {
		t.Error("deleted session still valid")
	}
	if u, _ := s.UserBySession(tok2); u == nil {
		t.Error("deleting one session killed another")
	}

	d.Exec(`UPDATE sessions SET expires_at = ?`, time.Now().Add(-time.Second))
	if u, _ := s.UserBySession(tok2); u != nil {
		t.Error("expired session accepted")
	}
	if rowCount(t, d, "sessions") != 0 {
		t.Error("expired session not purged on use")
	}

	t.Run("deleting the user's squadron cascades to sessions", func(t *testing.T) {
		tk, _, _ := s.CreateSession(uid)
		d.Exec(`DELETE FROM squadrons`)
		if u, _ := s.UserBySession(tk); u != nil {
			t.Error("session survived user deletion")
		}
	})
}

func TestCSRFTokens(t *testing.T) {
	s, _ := newService(t, false)
	a, b := s.CSRFToken("session-a"), s.CSRFToken("session-b")
	if a == b || a != s.CSRFToken("session-a") {
		t.Fatal("CSRF token must be deterministic per session and differ between sessions")
	}
	if len(a) != 64 {
		t.Errorf("token length %d", len(a))
	}
	if !s.ValidCSRF("session-a", a) {
		t.Error("own token rejected")
	}
	for name, bad := range map[string]string{"other session": b, "empty": "", "truncated": a[:63], "upper-cased": strings.ToUpper(a), "padded": a + "0"} {
		if s.ValidCSRF("session-a", bad) {
			t.Errorf("%s token accepted", name)
		}
	}
	// Different server key => different token (rotating SESSION_KEY invalidates forms).
	other := NewService(nil, "a-completely-different-key-0123456789", false, testDomain, false)
	if other.CSRFToken("session-a") == a {
		t.Error("token independent of server key")
	}
}

func TestInvites(t *testing.T) {
	s, d := newService(t, false)
	ownerEmail, ownerID := claimVerified(t, s, "40")
	var sqID int64
	d.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, ownerID).Scan(&sqID)
	_ = ownerEmail

	tok, err := s.CreateInvite(sqID, ownerID, "  Friend@Example.ORG ", "editor")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := s.GetInvite(tok)
	if err != nil || inv.Email != "friend@example.org" || inv.Role != "editor" || inv.SquadronID != sqID {
		t.Fatalf("GetInvite = %+v, %v", inv, err)
	}

	pend, _ := s.PendingInvites(sqID)
	if len(pend) != 1 {
		t.Fatalf("pending = %d", len(pend))
	}
	if other, _ := s.PendingInvites(sqID + 1); len(other) != 0 {
		t.Error("pending invites leak across squadrons")
	}

	uid, gotSq, err := s.AcceptInvite(tok, goodPW)
	if err != nil || gotSq != sqID {
		t.Fatalf("AcceptInvite = %d,%d,%v", uid, gotSq, err)
	}
	u, err := s.Authenticate("friend@example.org", goodPW)
	if err != nil || u.Role != "editor" || u.SquadronID != sqID || !u.Verified() {
		t.Fatalf("invited user = %+v, %v", u, err)
	}
	if _, _, err := s.AcceptInvite(tok, goodPW); err != ErrInviteInvalid {
		t.Errorf("second accept: %v", err)
	}

	members, _ := s.ListMembers(sqID)
	if len(members) != 2 {
		t.Errorf("members = %d", len(members))
	}

	t.Run("weak password does not consume the invite", func(t *testing.T) {
		tk, _ := s.CreateInvite(sqID, ownerID, "weak@example.org", "viewer")
		if _, _, err := s.AcceptInvite(tk, "short"); err == nil {
			t.Fatal("short password accepted")
		}
		if _, err := s.GetInvite(tk); err != nil {
			t.Errorf("invite consumed by a failed accept: %v", err)
		}
	})

	t.Run("existing email cannot be re-registered via invite", func(t *testing.T) {
		tk, _ := s.CreateInvite(sqID, ownerID, "friend@example.org", "owner")
		if _, _, err := s.AcceptInvite(tk, goodPW); err != ErrEmailTaken {
			t.Errorf("err = %v, want ErrEmailTaken (no account takeover / role escalation)", err)
		}
		u, _ := s.Authenticate("friend@example.org", goodPW)
		if u.Role != "editor" {
			t.Errorf("role became %q", u.Role)
		}
	})

	t.Run("expiry", func(t *testing.T) {
		tk, _ := s.CreateInvite(sqID, ownerID, "late@example.org", "viewer")
		d.Exec(`UPDATE invites SET expires_at = ? WHERE token = ?`, time.Now().Add(-time.Second), tk)
		if _, err := s.GetInvite(tk); err != ErrInviteInvalid {
			t.Errorf("expired invite: %v", err)
		}
	})

	t.Run("revoke is scoped to the squadron", func(t *testing.T) {
		tk, _ := s.CreateInvite(sqID, ownerID, "rev@example.org", "viewer")
		s.RevokeInvite(sqID+99, tk)
		if _, err := s.GetInvite(tk); err != nil {
			t.Error("revoked by the wrong squadron")
		}
		s.RevokeInvite(sqID, tk)
		if _, err := s.GetInvite(tk); err != ErrInviteInvalid {
			t.Error("not revoked")
		}
	})

	t.Run("validation", func(t *testing.T) {
		for _, bad := range []struct{ email, role string }{
			{"", "viewer"}, {"nope", "viewer"}, {"a@b.c\r\nBcc: x@y.z", "viewer"},
			{"Name <a@b.c>", "viewer"}, {"a@b.c", "root"}, {"a@b.c", ""}, {"a@b.c", "OWNER"},
		} {
			if _, err := s.CreateInvite(sqID, ownerID, bad.email, bad.role); err == nil {
				t.Errorf("CreateInvite(%q,%q) accepted", bad.email, bad.role)
			}
		}
	})
}

func TestRandomTokensAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		tk, err := randomToken()
		if err != nil || len(tk) != 64 || seen[tk] {
			t.Fatalf("token %q err=%v dup=%v", tk, err, seen[tk])
		}
		seen[tk] = true
	}
}

func TestUserRoleHelpers(t *testing.T) {
	for role, want := range map[string][3]bool{ // owner, canEdit, verified-irrelevant
		"owner": {true, true, false}, "editor": {false, true, false}, "viewer": {false, false, false}, "": {false, false, false},
	} {
		u := User{Role: role}
		if u.IsOwner() != want[0] || u.CanEdit() != want[1] {
			t.Errorf("role %q: IsOwner=%v CanEdit=%v", role, u.IsOwner(), u.CanEdit())
		}
	}
}
