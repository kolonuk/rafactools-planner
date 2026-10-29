package auth

import (
	"strings"
	"testing"
)

const (
	masterEmail = "admin@example.org"
	masterPW    = "a long master password"
)

func masterCount(t *testing.T, s *Service) int {
	t.Helper()
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_master_admin = 1`).Scan(&n)
	return n
}

func TestEnsureMasterAdminCreatesAndKeepsInStep(t *testing.T) {
	s, d := newService(t, false)

	msg, err := s.EnsureMasterAdmin("  "+strings.ToUpper(masterEmail)+" ", masterPW)
	if err != nil || !strings.Contains(msg, "created") {
		t.Fatalf("create: %q, %v", msg, err)
	}
	u, err := s.Authenticate(masterEmail, masterPW)
	if err != nil || !u.Verified() {
		t.Fatalf("master cannot sign in: %v", err)
	}
	var home string
	d.QueryRow(`SELECT sq.number FROM squadrons sq JOIN users u ON u.squadron_id = sq.id WHERE u.email = ?`, masterEmail).Scan(&home)
	if home != MasterHomeNumber {
		t.Errorf("home squadron = %q", home)
	}

	t.Run("unchanged settings change nothing and keep sessions", func(t *testing.T) {
		tok, _, _ := s.CreateSession(u.ID)
		msg, err := s.EnsureMasterAdmin(masterEmail, masterPW)
		if err != nil || !strings.Contains(msg, "unchanged") {
			t.Fatalf("%q, %v", msg, err)
		}
		if got, _ := s.UserBySession(tok); got == nil {
			t.Error("a redeploy with the same settings signed the master admin out")
		}
	})
	t.Run("the setting only seeds: a stored password wins, and may be left empty", func(t *testing.T) {
		tok, _, _ := s.CreateSession(u.ID)
		for _, pw := range []string{"a different master password", "short", ""} {
			msg, err := s.EnsureMasterAdmin(masterEmail, pw)
			if err != nil || !strings.Contains(msg, "unchanged") {
				t.Fatalf("setting %q: %q, %v", pw, msg, err)
			}
		}
		if _, err := s.Authenticate(masterEmail, masterPW); err != nil {
			t.Errorf("stored password was overwritten: %v", err)
		}
		if got, _ := s.UserBySession(tok); got == nil {
			t.Error("a redeploy signed the master admin out")
		}
	})
	t.Run("a blank stored password is seeded from the setting", func(t *testing.T) {
		d.Exec(`UPDATE users SET password_hash = '' WHERE email = ?`, masterEmail)
		if _, err := s.EnsureMasterAdmin(masterEmail, ""); err == nil {
			t.Error("seeding with no password was accepted")
		}
		if _, err := s.EnsureMasterAdmin(masterEmail, "short"); err == nil {
			t.Error("seeding with a weak password was accepted")
		}
		msg, err := s.EnsureMasterAdmin(masterEmail, masterPW)
		if err != nil || !strings.Contains(msg, "set from") {
			t.Fatalf("%q, %v", msg, err)
		}
		if _, err := s.Authenticate(masterEmail, masterPW); err != nil {
			t.Errorf("seeded password: %v", err)
		}
	})
	t.Run("the explicit reset replaces the stored password and signs the account out", func(t *testing.T) {
		old, _, _ := s.CreateSession(u.ID)
		if err := s.ResetMasterPassword(masterEmail, "short"); err == nil {
			t.Fatal("weak reset accepted")
		}
		if err := s.ResetMasterPassword(masterEmail, "a different master password"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Authenticate(masterEmail, masterPW); err != ErrInvalidCredentials {
			t.Errorf("old password still works: %v", err)
		}
		if _, err := s.Authenticate(masterEmail, "a different master password"); err != nil {
			t.Errorf("new password: %v", err)
		}
		if got, _ := s.UserBySession(old); got != nil {
			t.Error("old session survived a password change")
		}
		if masterCount(t, s) != 1 {
			t.Errorf("%d master admins", masterCount(t, s))
		}
	})
	t.Run("a new email replaces the old master admin", func(t *testing.T) {
		msg, err := s.EnsureMasterAdmin("other@example.org", masterPW)
		if err != nil || !strings.Contains(msg, "previous master admin removed") {
			t.Fatalf("%q, %v", msg, err)
		}
		if _, err := s.Authenticate(masterEmail, "a different master password"); err != ErrInvalidCredentials {
			t.Error("the previous master admin can still sign in")
		}
		if _, err := s.Authenticate("other@example.org", masterPW); err != nil {
			t.Errorf("new master admin: %v", err)
		}
		if masterCount(t, s) != 1 {
			t.Errorf("%d master admins", masterCount(t, s))
		}
	})
}

func TestEnsureMasterAdminRefusals(t *testing.T) {
	s, _ := newService(t, false)
	ordinary, _ := claimVerified(t, s, "70")

	for name, tc := range map[string]struct{ email, pw string }{
		"short password":       {masterEmail, "only10char"},
		"password over 72":     {masterEmail, strings.Repeat("a", 73)},
		"empty password":       {masterEmail, ""},
		"not an email":         {"nope", masterPW},
		"display-name email":   {"Admin <admin@example.org>", masterPW},
		"an ordinary member":   {ordinary, masterPW},
		"ordinary, other case": {strings.ToUpper(ordinary), masterPW},
	} {
		if _, err := s.EnsureMasterAdmin(tc.email, tc.pw); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if masterCount(t, s) != 0 {
		t.Error("a master admin was created from refused settings")
	}
	// The ordinary account is untouched.
	u, err := s.Authenticate(ordinary, goodPW)
	if err != nil || u.IsMasterAdmin || u.Role != "owner" {
		t.Errorf("ordinary account changed: %+v %v", u, err)
	}
}

func TestOnlyOneMasterAdminCanExist(t *testing.T) {
	s, d := newService(t, false)
	s.EnsureMasterAdmin(masterEmail, masterPW)
	var home int64
	d.QueryRow(`SELECT id FROM squadrons WHERE number = ?`, MasterHomeNumber).Scan(&home)
	if _, err := d.Exec(`INSERT INTO users (squadron_id, email, password_hash, role, is_master_admin) VALUES (?, 'second@example.org', 'x', 'owner', 1)`, home); err == nil {
		t.Error("the database accepted a second master admin")
	}
}

func TestNobodyBecomesMasterAdminThroughNormalFlows(t *testing.T) {
	s, d := newService(t, false)
	_, ownerID := claimVerified(t, s, "71")
	var sq int64
	d.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, ownerID).Scan(&sq)
	tok, _ := s.CreateInvite(sq, ownerID, "new@example.org", "owner")
	s.AcceptInvite(tok, goodPW)
	if masterCount(t, s) != 0 {
		t.Error("claim or invite produced a master admin")
	}
	// the hidden home squadron cannot be claimed or collide with a real number
	if _, _, err := s.ClaimSquadron("oc.master@"+testDomain, goodPW, ""); err == nil {
		t.Error("a squadron called 'master' could be claimed")
	}
}

func TestMasterAdminSwitchingSquadrons(t *testing.T) {
	s, _ := newService(t, false)
	ownerEmail, ownerID := claimVerified(t, s, "72")
	_ = ownerEmail
	var targetSq int64
	s.db.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, ownerID).Scan(&targetSq)
	s.EnsureMasterAdmin(masterEmail, masterPW)
	master, _ := s.Authenticate(masterEmail, masterPW)
	mtok, _, _ := s.CreateSession(master.ID)
	otok, _, _ := s.CreateSession(ownerID)

	t.Run("lists real squadrons only", func(t *testing.T) {
		list, err := s.ListSwitchableSquadrons()
		if err != nil || len(list) != 1 || list[0].Number != "72" {
			t.Errorf("list = %+v, %v", list, err)
		}
	})
	t.Run("by default the master admin is in their own squadron", func(t *testing.T) {
		u, _ := s.UserBySession(mtok)
		if u.ViewingOther || u.Role != "owner" || !u.IsMasterAdmin || u.SquadronID != master.SquadronID {
			t.Errorf("%+v", u)
		}
	})
	t.Run("switching shows the other squadron as a viewer", func(t *testing.T) {
		if err := s.SwitchSquadron(mtok, master.ID, targetSq); err != nil {
			t.Fatal(err)
		}
		u, _ := s.UserBySession(mtok)
		if !u.ViewingOther || u.SquadronID != targetSq || u.Role != "viewer" || u.CanEdit() || u.IsOwner() || u.ViewedSquadronName == "" {
			t.Errorf("%+v", u)
		}
		// only this session switched
		other, _, _ := s.CreateSession(master.ID)
		if u2, _ := s.UserBySession(other); u2.ViewingOther {
			t.Error("a different session of the master admin was switched too")
		}
	})
	t.Run("switching back", func(t *testing.T) {
		if err := s.SwitchSquadron(mtok, master.ID, 0); err != nil {
			t.Fatal(err)
		}
		if u, _ := s.UserBySession(mtok); u.ViewingOther || u.Role != "owner" {
			t.Errorf("%+v", u)
		}
	})
	t.Run("nobody else can switch, whatever they pass", func(t *testing.T) {
		if err := s.SwitchSquadron(otok, ownerID, targetSq); err != ErrNotMasterAdmin {
			t.Errorf("ordinary owner: %v", err)
		}
		if err := s.SwitchSquadron(otok, ownerID, master.SquadronID); err != ErrNotMasterAdmin {
			t.Errorf("ordinary owner to the hidden squadron: %v", err)
		}
		if u, _ := s.UserBySession(otok); u.ViewingOther || u.IsMasterAdmin {
			t.Errorf("ordinary user changed: %+v", u)
		}
	})
	t.Run("cannot switch into the hidden home squadron or a missing one", func(t *testing.T) {
		if err := s.SwitchSquadron(mtok, master.ID, master.SquadronID); err != ErrNoSuchSquadron {
			t.Errorf("home squadron: %v", err)
		}
		if err := s.SwitchSquadron(mtok, master.ID, 99999); err != ErrNoSuchSquadron {
			t.Errorf("missing: %v", err)
		}
	})
	t.Run("a session cannot be switched by another user", func(t *testing.T) {
		// the UPDATE is keyed on both the token and the user id
		s.SwitchSquadron(otok, master.ID, targetSq) // master's id with the owner's session token
		if u, _ := s.UserBySession(otok); u.ViewingOther {
			t.Error("switched someone else's session")
		}
	})
	t.Run("a stray acting_squadron_id on an ordinary user's session is ignored", func(t *testing.T) {
		s.db.Exec(`UPDATE sessions SET acting_squadron_id = ? WHERE token = ?`, master.SquadronID, otok)
		u, _ := s.UserBySession(otok)
		if u.ViewingOther || u.SquadronID != targetSq || u.Role != "owner" {
			t.Errorf("ordinary user was moved: %+v", u)
		}
	})
	t.Run("deleting the viewed squadron sends the session home", func(t *testing.T) {
		s.SwitchSquadron(mtok, master.ID, targetSq)
		s.db.Exec(`DELETE FROM squadrons WHERE id = ?`, targetSq)
		u, _ := s.UserBySession(mtok)
		if u == nil || u.ViewingOther || u.SquadronID != master.SquadronID {
			t.Errorf("%+v", u)
		}
	})
}
