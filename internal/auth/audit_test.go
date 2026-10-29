package auth

import (
	"strings"
	"testing"
	"time"
)

func TestAuditRecordsCapsAndPrunes(t *testing.T) {
	svc := newTestService(t)
	uid, _, _ := svc.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "A")
	var sq int64
	svc.db.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, uid).Scan(&sq)

	if err := svc.Audit(sq, uid, "x@example.org", AuditMemberRemove, strings.Repeat("é", 500)); err != nil {
		t.Fatal(err)
	}
	svc.db.Exec(`INSERT INTO audit_log (squadron_id, actor_email, action, at) VALUES (?, 'old@example.org', 'member.remove', ?)`, sq, time.Now().Add(-400*24*time.Hour))
	if err := svc.Audit(sq, uid, "y@example.org", AuditMemberRole, "z"); err != nil {
		t.Fatal(err)
	}

	got, err := svc.ListAudit(sq, 50)
	if err != nil || len(got) != 2 {
		t.Fatalf("entries = %v, %v (the year-old one must be pruned)", got, err)
	}
	if got[0].Action != AuditMemberRole {
		t.Errorf("newest first: %v", got[0])
	}
	if n := len([]rune(got[1].Detail)); n != maxAuditDetail {
		t.Errorf("detail kept %d runes, want %d", n, maxAuditDetail)
	}
	if AuditText("nonsense") != "Changed something" {
		t.Error("unknown action text")
	}
}

func TestAuditUserFindsSquadronAndCollapsesRepeatedFailures(t *testing.T) {
	svc := newTestService(t)
	uid, _, _ := svc.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "A")
	var sq int64
	svc.db.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, uid).Scan(&sq)

	for i := 0; i < 5; i++ {
		if err := svc.AuditUser(uid, AuditLoginFailed, ""); err != nil {
			t.Fatal(err)
		}
	}
	svc.AuditUser(uid, AuditLogin, "")
	svc.AuditUser(uid, AuditLogin, "")
	got, _ := svc.ListAudit(sq, 50)
	var fails, logins int
	for _, e := range got {
		switch e.Action {
		case AuditLoginFailed:
			fails++
		case AuditLogin:
			logins++
		}
		if e.ActorEmail != "snco.1234@rafac.mod.gov.uk" {
			t.Errorf("actor = %q", e.ActorEmail)
		}
	}
	if fails != 1 || logins != 2 {
		t.Errorf("failures=%d logins=%d, want 1 and 2", fails, logins)
	}
	if err := svc.AuditUser(99999, AuditLogin, ""); err == nil {
		t.Error("an unknown user was audited")
	}
}

func TestRecentAudit(t *testing.T) {
	svc := newTestService(t)
	uid, _, _ := svc.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "A")
	var sq int64
	svc.db.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, uid).Scan(&sq)
	if svc.RecentAudit(sq, AuditMasterView, time.Hour) {
		t.Fatal("recent before anything happened")
	}
	svc.Audit(sq, 0, "m@example.org", AuditMasterView, "")
	if !svc.RecentAudit(sq, AuditMasterView, time.Hour) {
		t.Error("not recent right after")
	}
	svc.db.Exec(`UPDATE audit_log SET at = ?`, time.Now().Add(-2*time.Hour))
	if svc.RecentAudit(sq, AuditMasterView, time.Hour) {
		t.Error("still recent after two hours")
	}
}
