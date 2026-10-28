package auth

import (
	"fmt"
	"path/filepath"
	"sync"
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

// Two owners demoting or removing each other at the same moment must never
// leave a squadron with nobody who can manage it: the "is there another
// owner?" check and the change have to be one atomic step.
func TestLastOwnerGuardIsRaceSafe(t *testing.T) {
	svc := newTestService(t)
	aID, _, err := svc.ClaimSquadron("snco.1234@rafac.mod.gov.uk", "ownerpassword1", "Test")
	if err != nil {
		t.Fatal(err)
	}
	members, _ := svc.ListMembers(1)
	sq := members[0].SquadronID
	inviteOwner := func(email string) int64 {
		tok, err := svc.CreateInvite(sq, aID, email, "owner")
		if err != nil {
			t.Fatal(err)
		}
		id, _, err := svc.AcceptInvite(tok, "ownerpassword2")
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	bID := inviteOwner("b@example.org")

	owners := func() int {
		var n int
		svc.db.QueryRow(`SELECT COUNT(*) FROM users WHERE squadron_id = ? AND role = 'owner'`, sq).Scan(&n)
		return n
	}
	for i := 0; i < 60; i++ {
		// reset to two owners
		svc.db.Exec(`UPDATE users SET role = 'owner' WHERE id IN (?, ?)`, aID, bID)
		var wg sync.WaitGroup
		if i%2 == 0 {
			wg.Add(2)
			go func() { defer wg.Done(); svc.SetMemberRole(sq, aID, "viewer") }()
			go func() { defer wg.Done(); svc.SetMemberRole(sq, bID, "viewer") }()
		} else {
			wg.Add(2)
			go func() { defer wg.Done(); svc.RemoveMember(sq, aID) }()
			go func() { defer wg.Done(); svc.RemoveMember(sq, bID) }()
		}
		wg.Wait()
		if got := owners(); got != 1 {
			t.Fatalf("iteration %d: %d owners left, want exactly 1 (one change wins, the other is refused)", i, got)
		}
		if i%2 == 1 { // a removal succeeded: put the removed person back
			var n int
			svc.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id IN (?, ?)`, aID, bID).Scan(&n)
			if n < 2 {
				aID, bID = restoreOwners(t, svc, sq, aID, bID)
			}
		}
	}
}

var restoreCount int

// restoreOwners recreates whichever of the two owners a removal deleted.
func restoreOwners(t *testing.T, svc *Service, sq, aID, bID int64) (int64, int64) {
	t.Helper()
	var haveA, haveB int
	svc.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, aID).Scan(&haveA)
	svc.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, bID).Scan(&haveB)
	var survivor int64
	if haveA == 1 {
		survivor = aID
	} else {
		survivor = bID
	}
	restoreCount++
	email := fmt.Sprintf("restored%d@example.org", restoreCount)
	tok, err := svc.CreateInvite(sq, survivor, email, "owner")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := svc.AcceptInvite(tok, "ownerpassword2")
	if err != nil {
		t.Fatal(err)
	}
	return survivor, other
}
