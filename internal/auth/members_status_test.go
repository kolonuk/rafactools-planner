package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// A squadron with an owner and an invited editor (not on the claim domain).
func squadronWithEditor(t *testing.T) (svc *Service, sq, ownerID, editorID int64) {
	t.Helper()
	svc = newTestService(t)
	ownerID, _, err := svc.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "A")
	if err != nil {
		t.Fatal(err)
	}
	svc.db.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, ownerID).Scan(&sq)
	tok, err := svc.CreateInvite(sq, ownerID, "ed@example.org", "editor")
	if err != nil {
		t.Fatal(err)
	}
	editorID, _, err = svc.AcceptInvite(tok, "editorpassword1")
	if err != nil {
		t.Fatal(err)
	}
	return svc, sq, ownerID, editorID
}

func TestInactiveMemberCannotSignInOrReset(t *testing.T) {
	svc, sq, owner, editor := squadronWithEditor(t)
	sess, _, _ := svc.CreateSession(editor)
	svc.RequestPasswordReset("ed@example.org") // a pending link

	if err := svc.SetMemberActive(sq, editor, owner, false); err != nil {
		t.Fatal(err)
	}
	if u, _ := svc.UserBySession(sess); u != nil {
		t.Error("an inactive member's existing session still works")
	}
	if _, err := svc.Authenticate("ed@example.org", "editorpassword1"); !errors.Is(err, ErrAccountInactive) {
		t.Errorf("right password, inactive = %v, want ErrAccountInactive", err)
	}
	if _, err := svc.Authenticate("ed@example.org", "wrong password here"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password, inactive = %v: it must not reveal the status", err)
	}
	if _, ok, _ := svc.RequestPasswordReset("ed@example.org"); ok {
		t.Error("a reset was sent to an inactive member")
	}
	var n int
	svc.db.QueryRow(`SELECT COUNT(*) FROM password_resets WHERE user_id = ?`, editor).Scan(&n)
	if n != 0 {
		t.Error("a pending reset link survived deactivation")
	}

	if err := svc.SetMemberActive(sq, editor, owner, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate("ed@example.org", "editorpassword1"); err != nil {
		t.Errorf("after reactivation: %v", err)
	}
}

func TestResetLinkIssuedBeforeDeactivationIsVoid(t *testing.T) {
	svc, sq, owner, editor := squadronWithEditor(t)
	token, ok, err := svc.RequestPasswordReset("ed@example.org")
	if err != nil || !ok {
		t.Fatalf("reset: %v %v", ok, err)
	}
	// Re-insert after deactivation to prove the link itself is refused, not only deleted.
	svc.SetMemberActive(sq, editor, owner, false)
	svc.db.Exec(`INSERT INTO password_resets (token_hash, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		hashResetToken(token), editor, time.Now().Add(time.Hour), time.Now())
	if err := svc.CheckPasswordReset(token); !errors.Is(err, ErrResetInvalid) {
		t.Errorf("link for an inactive member = %v, want ErrResetInvalid", err)
	}
	if err := svc.ResetPassword(token, "a brand new password!"); !errors.Is(err, ErrResetInvalid) {
		t.Errorf("reset for an inactive member = %v", err)
	}
}

func TestDeactivationGuards(t *testing.T) {
	svc, sq, owner, editor := squadronWithEditor(t)
	if err := svc.SetMemberActive(sq, owner, owner, false); !errors.Is(err, ErrSelfAction) {
		t.Errorf("self = %v", err)
	}
	// The editor is the acting user here, but the target is the only owner.
	if err := svc.SetMemberActive(sq, owner, editor, false); !errors.Is(err, ErrLastOwner) {
		t.Errorf("last owner = %v, want ErrLastOwner", err)
	}
	if err := svc.SetMemberActive(sq, 99999, owner, false); !errors.Is(err, ErrNotMember) {
		t.Errorf("unknown = %v", err)
	}
	// Another squadron's member is not reachable.
	other, _, _ := svc.ClaimSquadron("snco.4321@rafac.mod.gov.uk", "ownerpassword1", "B")
	if err := svc.SetMemberActive(sq, other, owner, false); !errors.Is(err, ErrNotMember) {
		t.Errorf("another squadron's member = %v", err)
	}

	// With a second owner, either can be deactivated, and an inactive owner
	// does not count as one: the remaining owner is then the last.
	tok, _ := svc.CreateInvite(sq, owner, "second@example.org", "owner")
	second, _, _ := svc.AcceptInvite(tok, "secondpassword12")
	if err := svc.SetMemberActive(sq, second, owner, false); err != nil {
		t.Fatalf("second owner: %v", err)
	}
	if err := svc.SetMemberActive(sq, owner, second, false); !errors.Is(err, ErrLastOwner) {
		t.Errorf("deactivating the only ACTIVE owner = %v, want ErrLastOwner", err)
	}
	if err := svc.SetMemberRole(sq, owner, "viewer"); !errors.Is(err, ErrLastOwner) {
		t.Errorf("demoting the only active owner = %v", err)
	}
	// An inactive owner can still be removed.
	if err := svc.RemoveMember(sq, second); err != nil {
		t.Errorf("removing an inactive owner: %v", err)
	}
}

func TestMasterAdminIsNeverManagedFromASquadron(t *testing.T) {
	s, _ := newService(t, false)
	s.EnsureMasterAdmin(masterEmail, masterPW)
	m, _ := s.Authenticate(masterEmail, masterPW)
	if err := s.SetMemberActive(m.SquadronID, m.ID, m.ID+999, false); !errors.Is(err, ErrNotMember) {
		t.Errorf("deactivating the master admin = %v", err)
	}
	if _, err := s.PrepareTempPassword(m.SquadronID, m.ID, m.ID+999); !errors.Is(err, ErrNotMember) {
		t.Errorf("temp password for the master admin = %v", err)
	}
}

func TestTemporaryPasswordLifecycle(t *testing.T) {
	svc, sq, owner, editor := squadronWithEditor(t)
	old, _, _ := svc.CreateSession(editor)

	if _, err := svc.PrepareTempPassword(sq, owner, owner); !errors.Is(err, ErrSelfAction) {
		t.Errorf("self = %v", err)
	}
	tp, err := svc.PrepareTempPassword(sq, editor, owner)
	if err != nil {
		t.Fatal(err)
	}
	if tp.Email != "ed@example.org" || len(tp.Plain) < 20 || strings.ContainsAny(tp.Plain, "0O1lI") {
		t.Errorf("prepared = %+v", tp)
	}
	// Preparing changes nothing: the old password still works.
	if _, err := svc.Authenticate("ed@example.org", "editorpassword1"); err != nil {
		t.Fatalf("prepare changed the password: %v", err)
	}
	if err := svc.ApplyTempPassword(tp); err != nil {
		t.Fatal(err)
	}
	if u, _ := svc.UserBySession(old); u != nil {
		t.Error("old session survived")
	}
	if _, err := svc.Authenticate("ed@example.org", "editorpassword1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("old password after reset = %v", err)
	}
	u, err := svc.Authenticate("ed@example.org", tp.Plain)
	if err != nil || !u.MustChangePassword {
		t.Fatalf("temp login: %v must=%v", err, u != nil && u.MustChangePassword)
	}
	var stored string
	svc.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, editor).Scan(&stored)
	if strings.Contains(stored, tp.Plain) {
		t.Error("the temporary password is stored in the clear")
	}

	// The member chooses their own: the flag clears and the temporary one is dead.
	tok, _, _ := svc.CreateSession(editor)
	if err := svc.ChangePassword(editor, tok, tp.Plain, "my own new password"); err != nil {
		t.Fatal(err)
	}
	u, err = svc.Authenticate("ed@example.org", "my own new password")
	if err != nil || u.MustChangePassword {
		t.Errorf("after choosing their own: %v must=%v", err, u != nil && u.MustChangePassword)
	}
	if _, err := svc.Authenticate("ed@example.org", tp.Plain); err == nil {
		t.Error("the temporary password still works after being replaced")
	}
}

func TestTemporaryPasswordExpiresAfterSevenDays(t *testing.T) {
	svc, sq, owner, editor := squadronWithEditor(t)
	tp, _ := svc.PrepareTempPassword(sq, editor, owner)
	if d := time.Until(tp.ExpiresAt); d < 6*24*time.Hour+23*time.Hour || d > 7*24*time.Hour+time.Minute {
		t.Errorf("expires in %v, want 7 days", d)
	}
	svc.ApplyTempPassword(tp)
	svc.db.Exec(`UPDATE users SET temp_password_expires_at = ? WHERE id = ?`, time.Now().Add(-time.Minute), editor)
	if _, err := svc.Authenticate("ed@example.org", tp.Plain); !errors.Is(err, ErrTempPasswordExpired) {
		t.Errorf("expired temp password = %v, want ErrTempPasswordExpired", err)
	}
	// They are not stuck: the ordinary reset link still works.
	if _, ok, _ := svc.RequestPasswordReset("ed@example.org"); !ok {
		t.Error("no reset link for a member whose temporary password expired")
	}
}

func TestNoTemporaryPasswordForInactiveOrOtherSquadronMembers(t *testing.T) {
	svc, sq, owner, editor := squadronWithEditor(t)
	svc.SetMemberActive(sq, editor, owner, false)
	if _, err := svc.PrepareTempPassword(sq, editor, owner); !errors.Is(err, ErrMemberInactive) {
		t.Errorf("inactive = %v, want ErrMemberInactive", err)
	}
	other, _, _ := svc.ClaimSquadron("snco.4321@rafac.mod.gov.uk", "ownerpassword1", "B")
	if _, err := svc.PrepareTempPassword(sq, other, owner); !errors.Is(err, ErrNotMember) {
		t.Errorf("other squadron = %v", err)
	}
	// Deactivated between prepare and apply: refused.
	svc.SetMemberActive(sq, editor, owner, true)
	tp, _ := svc.PrepareTempPassword(sq, editor, owner)
	svc.SetMemberActive(sq, editor, owner, false)
	if err := svc.ApplyTempPassword(tp); !errors.Is(err, ErrMemberInactive) {
		t.Errorf("apply after deactivation = %v", err)
	}
}

func TestChangeEmailRules(t *testing.T) {
	svc, _, owner, editor := squadronWithEditor(t)

	if svc.CanChangeEmail(owner) {
		t.Error("a RAFAC-domain address can be changed")
	}
	if _, _, err := svc.RequestEmailChange(owner, "new@example.org", "ownerpassword1"); !errors.Is(err, ErrEmailChangeNotAllowed) {
		t.Errorf("RAFAC address = %v", err)
	}
	if !svc.CanChangeEmail(editor) {
		t.Fatal("an ordinary address cannot be changed")
	}
	for name, tc := range map[string]struct {
		email, pw string
		want      error
	}{
		"wrong password":      {"new@example.org", "wrong password!!", ErrWrongPassword},
		"not an address":      {"nope", "editorpassword1", ErrNewEmailInvalid},
		"display name form":   {"Bob <bob@example.org>", "editorpassword1", ErrNewEmailInvalid},
		"on the claim domain": {"oc.9@rafac.mod.gov.uk", "editorpassword1", ErrNewEmailOnClaimDomain},
		"already in use":      {"SNCO.1234@rafac.mod.gov.uk", "editorpassword1", ErrNewEmailOnClaimDomain},
	} {
		if _, _, err := svc.RequestEmailChange(editor, tc.email, tc.pw); !errors.Is(err, tc.want) {
			t.Errorf("%s = %v, want %v", name, err, tc.want)
		}
	}
	tok2, _ := svc.CreateInvite(1, owner, "taken@example.org", "viewer")
	svc.AcceptInvite(tok2, "takenpassword123")
	if _, _, err := svc.RequestEmailChange(editor, "Taken@Example.org", "editorpassword1"); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("in use = %v", err)
	}
}

func TestChangeEmailFlowConfirmsOnceAndKeepsSessions(t *testing.T) {
	svc, _, _, editor := squadronWithEditor(t)
	sess, _, _ := svc.CreateSession(editor)
	svc.RequestPasswordReset("ed@example.org")

	_, normalized, err := svc.RequestEmailChange(editor, " Fresh.Address@Example.org ", "editorpassword1")
	if err == nil || normalized != "" {
		// leading/trailing spaces are not an address: the caller trims first
		t.Logf("untrimmed input -> %v (callers trim)", err)
	}
	token, normalized, err := svc.RequestEmailChange(editor, "Fresh.Address@Example.org", "editorpassword1")
	if err != nil || normalized != "fresh.address@example.org" {
		t.Fatalf("request: %q %v", normalized, err)
	}
	// Nothing changes before the link is confirmed, and previewing does not use it.
	if u, _ := svc.Authenticate("ed@example.org", "editorpassword1"); u == nil {
		t.Fatal("address changed before confirmation")
	}
	if to, err := svc.EmailChangeTarget(token); err != nil || to != normalized {
		t.Errorf("target = %q %v", to, err)
	}
	uid, oldE, newE, err := svc.ConfirmEmailChange(token)
	if err != nil || uid != editor || oldE != "ed@example.org" || newE != normalized {
		t.Fatalf("confirm: %d %q %q %v", uid, oldE, newE, err)
	}
	if _, err := svc.Authenticate("ed@example.org", "editorpassword1"); err == nil {
		t.Error("the old address still signs in")
	}
	if _, err := svc.Authenticate(normalized, "editorpassword1"); err != nil {
		t.Errorf("the new address cannot sign in: %v", err)
	}
	if u, _ := svc.UserBySession(sess); u == nil || u.Email != normalized {
		t.Errorf("the session was lost or shows the old address: %+v", u)
	}
	if _, _, _, err := svc.ConfirmEmailChange(token); !errors.Is(err, ErrEmailChangeInvalid) {
		t.Errorf("link reused = %v", err)
	}
	var n int
	svc.db.QueryRow(`SELECT COUNT(*) FROM password_resets WHERE user_id = ?`, editor).Scan(&n)
	if n != 0 {
		t.Error("a reset link for the old address survived")
	}
}

func TestChangeEmailLinkExpiresAndNewRequestReplacesOld(t *testing.T) {
	svc, _, _, editor := squadronWithEditor(t)
	t1, _, _ := svc.RequestEmailChange(editor, "one@example.org", "editorpassword1")
	t2, _, _ := svc.RequestEmailChange(editor, "two@example.org", "editorpassword1")
	if _, err := svc.EmailChangeTarget(t1); !errors.Is(err, ErrEmailChangeInvalid) {
		t.Errorf("the older link still works: %v", err)
	}
	svc.db.Exec(`UPDATE email_changes SET expires_at = ?`, time.Now().Add(-time.Minute))
	if _, _, _, err := svc.ConfirmEmailChange(t2); !errors.Is(err, ErrEmailChangeInvalid) {
		t.Errorf("expired link = %v", err)
	}
}
