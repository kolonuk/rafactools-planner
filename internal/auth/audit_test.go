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
