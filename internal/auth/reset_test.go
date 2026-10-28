package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordResetFlow(t *testing.T) {
	s, d := newService(t, false)
	email, uid := claimVerified(t, s, "90")
	oldSession, _, _ := s.CreateSession(uid)
	otherEmail, otherUID := claimVerified(t, s, "91")
	otherSession, _, _ := s.CreateSession(otherUID)

	tok, ok, err := s.RequestPasswordReset("  " + strings.ToUpper(email) + " ")
	if err != nil || !ok || len(tok) != 64 {
		t.Fatalf("request = %q, %v, %v", tok, ok, err)
	}

	t.Run("only a hash of the token is stored", func(t *testing.T) {
		var stored string
		d.QueryRow(`SELECT token_hash FROM password_resets`).Scan(&stored)
		if stored == tok || len(stored) != 64 {
			t.Errorf("stored value %q is the raw token or not a SHA-256 hash", stored)
		}
		var n int
		d.QueryRow(`SELECT COUNT(*) FROM password_resets WHERE token_hash = ?`, tok).Scan(&n)
		if n != 0 {
			t.Error("raw token is in the database")
		}
	})

	t.Run("checking a token does not use it up", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			if err := s.CheckPasswordReset(tok); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("a rejected password keeps the token usable", func(t *testing.T) {
		for _, bad := range []string{"short", strings.Repeat("a", 73)} {
			if err := s.ResetPassword(tok, bad); err == nil {
				t.Errorf("password of %d bytes accepted", len(bad))
			}
		}
		if err := s.CheckPasswordReset(tok); err != nil {
			t.Error("token consumed by a rejected password")
		}
	})

	if err := s.ResetPassword(tok, "a brand new password"); err != nil {
		t.Fatal(err)
	}

	t.Run("new password works, old does not", func(t *testing.T) {
		if _, err := s.Authenticate(email, "a brand new password"); err != nil {
			t.Errorf("new password: %v", err)
		}
		if _, err := s.Authenticate(email, goodPW); err != ErrInvalidCredentials {
			t.Errorf("old password still works: %v", err)
		}
	})
	t.Run("signs the user out everywhere, and only that user", func(t *testing.T) {
		if u, _ := s.UserBySession(oldSession); u != nil {
			t.Error("old session survived the reset")
		}
		if u, _ := s.UserBySession(otherSession); u == nil {
			t.Error("another user's session was killed")
		}
		if _, err := s.Authenticate(otherEmail, goodPW); err != nil {
			t.Errorf("another user's password changed: %v", err)
		}
	})
	t.Run("token is single use", func(t *testing.T) {
		if err := s.ResetPassword(tok, "yet another password"); err != ErrResetInvalid {
			t.Errorf("reuse: %v", err)
		}
		if err := s.CheckPasswordReset(tok); err != ErrResetInvalid {
			t.Errorf("check after use: %v", err)
		}
	})
}

func TestPasswordResetRequestRules(t *testing.T) {
	s, d := newService(t, false)
	email, _ := claimVerified(t, s, "92")

	t.Run("unknown address sends nothing", func(t *testing.T) {
		if tok, ok, err := s.RequestPasswordReset("ghost@example.org"); ok || tok != "" || err != nil {
			t.Errorf("= %q %v %v", tok, ok, err)
		}
	})
	t.Run("unverified address sends nothing", func(t *testing.T) {
		unverified := "oc.93@" + testDomain
		if _, _, err := s.ClaimSquadron(unverified, goodPW, ""); err != nil {
			t.Fatal(err)
		}
		if _, ok, _ := s.RequestPasswordReset(unverified); ok {
			t.Error("reset issued for an address that never confirmed it owns the inbox")
		}
	})

	first, ok, _ := s.RequestPasswordReset(email)
	if !ok {
		t.Fatal("first request refused")
	}
	t.Run("repeat within a minute sends nothing and keeps the first token", func(t *testing.T) {
		if _, ok, _ := s.RequestPasswordReset(email); ok {
			t.Error("second request inside the cooldown was honoured (inbox flooding)")
		}
		if err := s.CheckPasswordReset(first); err != nil {
			t.Errorf("first token invalidated by a throttled request: %v", err)
		}
	})
	t.Run("a later request replaces the earlier token", func(t *testing.T) {
		d.Exec(`UPDATE password_resets SET created_at = ?`, time.Now().Add(-2*time.Minute))
		second, ok, _ := s.RequestPasswordReset(email)
		if !ok || second == first {
			t.Fatalf("second = %q ok=%v", second, ok)
		}
		if err := s.CheckPasswordReset(first); err != ErrResetInvalid {
			t.Error("old token still valid after a new request")
		}
		if err := s.CheckPasswordReset(second); err != nil {
			t.Error("new token invalid")
		}
	})
	t.Run("expiry", func(t *testing.T) {
		d.Exec(`UPDATE password_resets SET expires_at = ?`, time.Now().Add(-time.Second))
		var tok string
		d.QueryRow(`SELECT token_hash FROM password_resets`).Scan(&tok)
		// can't recover the raw token; request a fresh one and age it instead
		d.Exec(`UPDATE password_resets SET created_at = ?`, time.Now().Add(-2*time.Minute))
		fresh, ok, _ := s.RequestPasswordReset(email)
		if !ok {
			t.Fatal("no fresh token")
		}
		d.Exec(`UPDATE password_resets SET expires_at = ?`, time.Now().Add(-time.Second))
		if err := s.ResetPassword(fresh, "an expired attempt!!"); err != ErrResetInvalid {
			t.Errorf("expired token: %v", err)
		}
		if _, err := s.Authenticate(email, goodPW); err != nil {
			t.Errorf("password changed by an expired token: %v", err)
		}
	})
	t.Run("garbage tokens", func(t *testing.T) {
		for _, bad := range []string{"", "nope", strings.Repeat("0", 64), "' OR '1'='1"} {
			if err := s.ResetPassword(bad, "a perfectly good password"); err != ErrResetInvalid {
				t.Errorf("token %q: %v", bad, err)
			}
		}
	})
}
