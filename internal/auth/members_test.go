package auth

import (
	"path/filepath"
	"testing"

	"rafactools-planner/internal/db"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewService(database, "test-session-key", false, "rafac.mod.gov.uk", true)
}

func TestLastOwnerCannotBeRemovedOrDemoted(t *testing.T) {
	svc := newTestService(t)

	ownerID, _, err := svc.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "Test Squadron")
	if err != nil {
		t.Fatalf("ClaimSquadron: %v", err)
	}
	members, err := svc.ListMembers(1)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	squadronID := members[0].SquadronID

	if err := svc.RemoveMember(squadronID, ownerID); err != ErrLastOwner {
		t.Errorf("RemoveMember(only owner) = %v, want ErrLastOwner", err)
	}
	if err := svc.SetMemberRole(squadronID, ownerID, "editor"); err != ErrLastOwner {
		t.Errorf("SetMemberRole(only owner -> editor) = %v, want ErrLastOwner", err)
	}

	// Add a second owner: now removing/demoting the first should work.
	token, err := svc.CreateInvite(squadronID, ownerID, "second.owner@example.com", "owner")
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	secondOwnerID, _, err := svc.AcceptInvite(token, "secondownerpassword1")
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	if err := svc.SetMemberRole(squadronID, ownerID, "editor"); err != nil {
		t.Errorf("SetMemberRole(first owner -> editor, second owner exists) = %v, want nil", err)
	}
	if err := svc.RemoveMember(squadronID, secondOwnerID); err != ErrLastOwner {
		t.Errorf("RemoveMember(second owner, now the only one since first was demoted) = %v, want ErrLastOwner", err)
	}
}

func TestSetMemberRoleRejectsInvalidRole(t *testing.T) {
	svc := newTestService(t)
	ownerID, _, err := svc.ClaimSquadron("snco.5555@rafac.mod.gov.uk", "ownerpassword1", "Test Squadron")
	if err != nil {
		t.Fatalf("ClaimSquadron: %v", err)
	}
	members, _ := svc.ListMembers(1)
	squadronID := members[0].SquadronID

	if err := svc.SetMemberRole(squadronID, ownerID, "superadmin"); err != ErrInvalidRole {
		t.Errorf("SetMemberRole(invalid role) = %v, want ErrInvalidRole", err)
	}
}
