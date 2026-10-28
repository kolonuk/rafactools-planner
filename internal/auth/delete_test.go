package auth

import "testing"

func TestDeleteSquadronGuards(t *testing.T) {
	svc := newTestService(t)
	ownerID, _, err := svc.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "A")
	if err != nil {
		t.Fatal(err)
	}
	otherID, _, _ := svc.ClaimSquadron("snco.4321@rafac.mod.gov.uk", "ownerpassword1", "B")
	var sqA, sqB int64
	svc.db.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, ownerID).Scan(&sqA)
	svc.db.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, otherID).Scan(&sqB)

	tok, _ := svc.CreateInvite(sqA, ownerID, "ed@example.org", "editor")
	editorID, _, _ := svc.AcceptInvite(tok, "editorpassword1")

	if err := svc.DeleteSquadron(sqA, ownerID, "wrong password!!", "1234"); err != ErrWrongPassword {
		t.Errorf("wrong password = %v", err)
	}
	if err := svc.DeleteSquadron(sqA, ownerID, "ownerpassword1", "9999"); err != ErrConfirmMismatch {
		t.Errorf("wrong number = %v", err)
	}
	if err := svc.DeleteSquadron(sqA, editorID, "editorpassword1", "1234"); err != ErrNoSuchSquadron {
		t.Errorf("an editor = %v, want refused", err)
	}
	if err := svc.DeleteSquadron(sqA, otherID, "ownerpassword1", "1234"); err != ErrNoSuchSquadron {
		t.Errorf("another squadron's owner = %v, want refused", err)
	}
	if err := svc.DeleteSquadron(99999, ownerID, "ownerpassword1", "1234"); err != ErrNoSuchSquadron {
		t.Errorf("unknown squadron = %v", err)
	}
	var n int
	svc.db.QueryRow(`SELECT COUNT(*) FROM squadrons WHERE id = ?`, sqA).Scan(&n)
	if n != 1 {
		t.Fatal("a refused delete removed the squadron")
	}

	tokSess, _, _ := svc.CreateSession(editorID)
	svc.Audit(sqA, ownerID, "snco.1234@rafac.mod.gov.uk", AuditInviteSent, "x")
	if err := svc.DeleteSquadron(sqA, ownerID, "ownerpassword1", "1234"); err != nil {
		t.Fatal(err)
	}
	for table, where := range map[string]string{"squadrons": "id", "users": "squadron_id", "invites": "squadron_id", "audit_log": "squadron_id"} {
		svc.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+where+` = ?`, sqA).Scan(&n)
		if n != 0 {
			t.Errorf("%s: %d rows left for the deleted squadron", table, n)
		}
	}
	if u, _ := svc.UserBySession(tokSess); u != nil {
		t.Error("a member's session survived the squadron's deletion")
	}
	svc.db.QueryRow(`SELECT COUNT(*) FROM squadrons WHERE id = ?`, sqB).Scan(&n)
	if n != 1 {
		t.Error("the other squadron was deleted too")
	}
}

func TestMasterHomeSquadronCannotBeDeleted(t *testing.T) {
	s, _ := newService(t, false)
	if _, err := s.EnsureMasterAdmin(masterEmail, masterPW); err != nil {
		t.Fatal(err)
	}
	m, err := s.Authenticate(masterEmail, masterPW)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSquadron(m.SquadronID, m.ID, masterPW, MasterHomeNumber); err != ErrCannotDeleteHome {
		t.Errorf("deleting the master home squadron = %v, want ErrCannotDeleteHome", err)
	}
	if masterCount(t, s) != 1 {
		t.Error("the master admin is gone")
	}
}
