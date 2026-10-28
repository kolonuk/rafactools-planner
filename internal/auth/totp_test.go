package auth

import (
	"strings"
	"testing"
	"time"
)

func TestTOTPMatchesRFC6238Vector(t *testing.T) {
	// RFC 6238 appendix B, SHA-1, secret "12345678901234567890", T=59s: 94287082,
	// whose last six digits are what a 6-digit authenticator shows.
	secret := []byte("12345678901234567890")
	if got := totpCode(secret, 59/30); got != "287082" {
		t.Errorf("code at T=59 = %s, want 287082", got)
	}
	if got := totpCode(secret, 1111111109/30); got != "081804" {
		t.Errorf("code at T=1111111109 = %s, want 081804", got)
	}
}

func newTOTPUser(t *testing.T) (*Service, int64, string) {
	t.Helper()
	svc := newTestService(t)
	id, _, err := svc.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "A")
	if err != nil {
		t.Fatal(err)
	}
	secret, uri, err := svc.BeginTOTP(id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "otpauth://totp/") || !strings.Contains(uri, "secret="+secret) {
		t.Fatalf("otpauth uri = %q", uri)
	}
	return svc, id, secret
}

func codeNow(t *testing.T, secretB32 string, offset int64) string {
	t.Helper()
	raw, err := secretEnc.DecodeString(secretB32)
	if err != nil {
		t.Fatal(err)
	}
	return totpCode(raw, time.Now().Unix()/totpStep+offset)
}

func TestTOTPEnrolConfirmVerifyAndReplay(t *testing.T) {
	svc, id, secret := newTOTPUser(t)

	if svc.TOTPEnabled(id) {
		t.Fatal("enabled before it was confirmed")
	}
	if err := svc.VerifyTOTP(id, codeNow(t, secret, 0)); err == nil {
		t.Error("a pending (unconfirmed) secret was accepted at sign-in")
	}
	if err := svc.ConfirmTOTP(id, "000000"); err != ErrBadCode {
		t.Errorf("wrong code at confirm = %v", err)
	}
	if err := svc.ConfirmTOTP(id, codeNow(t, secret, 0)); err != nil {
		t.Fatal(err)
	}
	if !svc.TOTPEnabled(id) {
		t.Fatal("not enabled after confirming")
	}

	// The code used to confirm is spent; a later step still works once.
	if err := svc.VerifyTOTP(id, codeNow(t, secret, 0)); err != ErrBadCode {
		t.Errorf("replaying the confirming code = %v, want ErrBadCode", err)
	}
	next := codeNow(t, secret, 1)
	if err := svc.VerifyTOTP(id, next); err != nil {
		t.Fatalf("next step's code: %v", err)
	}
	if err := svc.VerifyTOTP(id, next); err != ErrBadCode {
		t.Errorf("same code twice = %v, want ErrBadCode", err)
	}
	if err := svc.VerifyTOTP(id, "12345"); err != ErrBadCode {
		t.Errorf("short code = %v", err)
	}
	if _, _, err := svc.BeginTOTP(id); err == nil {
		t.Error("a second secret was started while one is active")
	}
}

func TestTOTPLocksAfterRepeatedWrongCodes(t *testing.T) {
	svc, id, secret := newTOTPUser(t)
	if err := svc.ConfirmTOTP(id, codeNow(t, secret, 0)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < totpMaxFail; i++ {
		if err := svc.VerifyTOTP(id, "000000"); err != ErrBadCode {
			t.Fatalf("attempt %d = %v", i, err)
		}
	}
	if err := svc.VerifyTOTP(id, codeNow(t, secret, 1)); err != ErrTOTPLocked {
		t.Errorf("a right code while locked = %v, want ErrTOTPLocked", err)
	}
}

func TestDisableTOTPNeedsPasswordAndCode(t *testing.T) {
	svc, id, secret := newTOTPUser(t)
	if err := svc.ConfirmTOTP(id, codeNow(t, secret, 0)); err != nil {
		t.Fatal(err)
	}
	if err := svc.DisableTOTP(id, "wrong password here", codeNow(t, secret, 1)); err != ErrWrongPassword {
		t.Errorf("wrong password = %v", err)
	}
	if err := svc.DisableTOTP(id, "ownerpassword1", "000000"); err != ErrBadCode {
		t.Errorf("wrong code = %v", err)
	}
	if !svc.TOTPEnabled(id) {
		t.Fatal("a refused disable turned it off")
	}
	if err := svc.DisableTOTP(id, "ownerpassword1", codeNow(t, secret, 1)); err != nil {
		t.Fatal(err)
	}
	if svc.TOTPEnabled(id) {
		t.Error("still enabled")
	}
}

func TestResetTOTPForClearsIt(t *testing.T) {
	svc, id, secret := newTOTPUser(t)
	svc.ConfirmTOTP(id, codeNow(t, secret, 0))
	if ok, err := svc.ResetTOTPFor(" SNCO.1234@rafac.mod.gov.uk "); err != nil || !ok {
		t.Fatalf("reset = %v, %v", ok, err)
	}
	if svc.TOTPEnabled(id) {
		t.Error("still enabled after reset")
	}
	if ok, _ := svc.ResetTOTPFor("nobody@example.org"); ok {
		t.Error("reported a change for an unknown address")
	}
}
