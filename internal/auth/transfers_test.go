package auth

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

type transferWorld struct {
	s              *Service
	db             *sql.DB
	ownerA, ownerB int64 // owners of squadrons 101 and 202 (claim-domain addresses)
	sqA, sqB       int64
	mover          int64
}

func newTransferWorld(t *testing.T) *transferWorld {
	t.Helper()
	s, d := newService(t, false)
	w := &transferWorld{s: s, db: d}
	_, w.ownerA = claimVerified(t, s, "101")
	_, w.ownerB = claimVerified(t, s, "202")
	d.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, w.ownerA).Scan(&w.sqA)
	d.QueryRow(`SELECT squadron_id FROM users WHERE id = ?`, w.ownerB).Scan(&w.sqB)
	tok, err := s.CreateInvite(w.sqA, w.ownerA, "mover@example.org", "editor")
	if err != nil {
		t.Fatal(err)
	}
	if w.mover, _, err = s.AcceptInvite(tok, goodPW); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w *transferWorld) userRow(t *testing.T, id int64) (sq int64, role string, active bool) {
	t.Helper()
	var a int
	if err := w.db.QueryRow(`SELECT squadron_id, role, active FROM users WHERE id = ?`, id).Scan(&sq, &role, &a); err != nil {
		t.Fatal(err)
	}
	return sq, role, a == 1
}

func (w *transferWorld) session(t *testing.T, id int64) {
	t.Helper()
	if _, err := w.db.Exec(`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`, "tok-"+time.Now().Format("150405.000000000"), id, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func (w *transferWorld) sessions(id int64) int {
	var n int
	w.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, id).Scan(&n)
	return n
}

func TestTransferSwitchesOffThenMovesOnAcceptance(t *testing.T) {
	w := newTransferWorld(t)
	w.session(t, w.mover)

	tr, err := w.s.StartTransfer(w.mover, w.mover, " 202 ")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != TransferPending || tr.ToNumber != "202" || tr.FromNumber != "101" || tr.PersonalToken == "" {
		t.Fatalf("transfer = %+v", tr)
	}
	if _, _, active := w.userRow(t, w.mover); active {
		t.Error("the account is still switched on while the move waits")
	}
	if w.sessions(w.mover) != 0 {
		t.Error("sessions were not ended")
	}
	if _, err := w.s.Authenticate("mover@example.org", goodPW); !errors.Is(err, ErrTransferWaiting) {
		t.Errorf("signing in = %v, want ErrTransferWaiting", err)
	}
	if err := w.s.SetMemberActive(w.sqA, w.mover, w.ownerA, true); !errors.Is(err, ErrTransferWaiting) {
		t.Errorf("an owner switching the account on = %v, want ErrTransferWaiting", err)
	}
	if _, err := w.s.StartTransfer(w.mover, w.mover, "202"); err == nil {
		t.Error("a second move was started while one waits") // inactive now, so it is refused one way or another
	}
	in, _ := w.s.IncomingTransfers(w.sqB)
	out, _ := w.s.OutgoingTransfers(w.sqA)
	if len(in) != 1 || len(out) != 1 || in[0].ID != tr.ID {
		t.Fatalf("incoming %v outgoing %v", in, out)
	}
	if owners, _ := w.s.TransferOwners(w.sqB); len(owners) != 1 || owners[0] != "oc.202@rafac.mod.gov.uk" {
		t.Errorf("owners to tell = %v", owners)
	}

	if _, err := w.s.AcceptTransfer(tr.ID, w.ownerA, "editor"); !errors.Is(err, ErrTransferNotAllowed) {
		t.Errorf("an owner of the other squadron accepted: %v", err)
	}
	if _, err := w.s.AcceptTransfer(tr.ID, w.ownerB, "admin"); !errors.Is(err, ErrTransferRole) {
		t.Errorf("a made-up role: %v", err)
	}
	done, err := w.s.AcceptTransfer(tr.ID, w.ownerB, "viewer")
	if err != nil || done.Status != TransferAccepted || done.Role != "viewer" {
		t.Fatalf("accept = %+v, %v", done, err)
	}
	sq, role, active := w.userRow(t, w.mover)
	if sq != w.sqB || role != "viewer" || !active {
		t.Errorf("after the move: squadron %d (want %d), role %q, active %v", sq, w.sqB, role, active)
	}
	u, err := w.s.Authenticate("mover@example.org", goodPW)
	if err != nil || u.SquadronID != w.sqB {
		t.Errorf("signing in after the move: %v %+v", err, u)
	}
	if _, err := w.s.AcceptTransfer(tr.ID, w.ownerB, "viewer"); !errors.Is(err, ErrTransferGone) {
		t.Errorf("accepted twice: %v", err)
	}
	if open, _ := w.s.OpenTransferFor(w.mover); open != nil {
		t.Error("a finished move is still open")
	}
}

func TestTransferRules(t *testing.T) {
	w := newTransferWorld(t)
	for _, tc := range []struct {
		name string
		do   func() error
		want error
	}{
		{"a claim-domain address stays", func() error { _, e := w.s.StartTransfer(w.ownerA, w.ownerA, "202"); return e }, ErrTransferOfficial},
		{"no such squadron", func() error { _, e := w.s.StartTransfer(w.mover, w.mover, "999"); return e }, ErrTransferNoTarget},
		{"the same squadron", func() error { _, e := w.s.StartTransfer(w.mover, w.mover, "101"); return e }, ErrTransferSameSquadron},
		{"someone else's request needs an owner", func() error { _, e := w.s.StartTransfer(w.mover, w.ownerB, "202"); return e }, ErrTransferNotAllowed},
		{"an owner of the target cannot move another squadron's member", func() error { _, e := w.s.StartTransfer(w.mover, w.ownerB, "101"); return e }, ErrTransferNotAllowed},
	} {
		if err := tc.do(); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}

	t.Run("the master admin and unconfirmed or temporary accounts", func(t *testing.T) {
		w.db.Exec(`UPDATE users SET is_master_admin = 1 WHERE id = ?`, w.mover)
		if _, err := w.s.StartTransfer(w.mover, w.mover, "202"); !errors.Is(err, ErrTransferNotAllowed) {
			t.Errorf("master admin: %v", err)
		}
		w.db.Exec(`UPDATE users SET is_master_admin = 0, must_change_password = 1 WHERE id = ?`, w.mover)
		if _, err := w.s.StartTransfer(w.mover, w.mover, "202"); !errors.Is(err, ErrTransferTemp) {
			t.Errorf("temporary password: %v", err)
		}
		w.db.Exec(`UPDATE users SET must_change_password = 0, email_verified_at = NULL WHERE id = ?`, w.mover)
		if _, err := w.s.StartTransfer(w.mover, w.mover, "202"); !errors.Is(err, ErrTransferNotAllowed) {
			t.Errorf("unconfirmed email: %v", err)
		}
		w.db.Exec(`UPDATE users SET email_verified_at = CURRENT_TIMESTAMP WHERE id = ?`, w.mover)
	})

	t.Run("the only owner cannot go", func(t *testing.T) {
		tok, _ := w.s.CreateInvite(w.sqA, w.ownerA, "second@example.org", "owner")
		second, _, err := w.s.AcceptInvite(tok, goodPW)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.s.StartTransfer(second, second, "202"); err != nil { // oc.101 is still an owner
			t.Fatalf("one of two owners: %v", err)
		}
		tr, _ := w.s.OpenTransferFor(second)
		w.s.CancelTransfer(tr.ID, second, "")
		w.db.Exec(`UPDATE users SET active = 0 WHERE id = ?`, w.ownerA)
		if _, err := w.s.StartTransfer(second, second, "202"); !errors.Is(err, ErrTransferOnlyOwner) {
			t.Errorf("only active owner: %v", err)
		}
		w.db.Exec(`UPDATE users SET active = 1 WHERE id = ?`, w.ownerA)
	})

	t.Run("an invitation waiting in the target blocks it", func(t *testing.T) {
		if _, err := w.s.CreateInvite(w.sqB, w.ownerB, "Mover@Example.org", "editor"); err != nil {
			t.Fatal(err)
		}
		if _, err := w.s.StartTransfer(w.mover, w.mover, "202"); !errors.Is(err, ErrTransferInvite) {
			t.Errorf("got %v", err)
		}
	})
}

func TestTransferDeclinedCancelledAndExpiredSwitchTheAccountBackOn(t *testing.T) {
	w := newTransferWorld(t)
	start := func() *Transfer {
		t.Helper()
		tr, err := w.s.StartTransfer(w.mover, w.mover, "202")
		if err != nil {
			t.Fatal(err)
		}
		return tr
	}
	stillThere := func(what string) {
		t.Helper()
		sq, role, active := w.userRow(t, w.mover)
		if sq != w.sqA || role != "editor" || !active {
			t.Errorf("%s: squadron %d, role %q, active %v", what, sq, role, active)
		}
		if open, _ := w.s.OpenTransferFor(w.mover); open != nil {
			t.Errorf("%s: the move is still open", what)
		}
	}

	tr := start()
	if _, err := w.s.DeclineTransfer(tr.ID, w.ownerA, ""); !errors.Is(err, ErrTransferNotAllowed) {
		t.Errorf("an owner of the squadron it leaves declined: %v", err)
	}
	if _, err := w.s.DeclineTransfer(tr.ID, w.ownerB, ""); err != nil {
		t.Fatal(err)
	}
	stillThere("declined")

	tr = start()
	if _, err := w.s.CancelTransfer(tr.ID, w.ownerB, ""); !errors.Is(err, ErrTransferNotAllowed) {
		t.Errorf("the target's owner cancelled: %v", err)
	}
	if _, err := w.s.CancelTransfer(tr.ID, 0, "not-the-token"); !errors.Is(err, ErrTransferNotAllowed) {
		t.Errorf("a wrong link cancelled: %v", err)
	}
	if _, err := w.s.CancelTransfer(tr.ID, 0, tr.PersonalToken); err != nil {
		t.Fatalf("cancelling by the emailed link: %v", err)
	}
	stillThere("cancelled by the person's link")

	tr = start()
	if _, err := w.s.CancelTransfer(tr.ID, w.ownerA, ""); err != nil {
		t.Fatalf("an owner of the old squadron cancelling: %v", err)
	}
	stillThere("cancelled by an owner")

	tr = start()
	w.db.Exec(`UPDATE account_transfers SET expires_at = ? WHERE id = ?`, time.Now().Add(-time.Minute).UTC(), tr.ID)
	if _, err := w.s.AcceptTransfer(tr.ID, w.ownerB, "editor"); !errors.Is(err, ErrTransferGone) {
		t.Errorf("an expired move was accepted: %v", err)
	}
	expired, err := w.s.ExpireTransfers(time.Now())
	if err != nil || len(expired) != 1 || expired[0].Status != TransferExpired {
		t.Fatalf("expired = %+v, %v", expired, err)
	}
	stillThere("expired")
	if again, _ := w.s.ExpireTransfers(time.Now()); len(again) != 0 {
		t.Error("expired twice")
	}
}

func TestTransferAnOwnerAsksAndThePersonMustAgree(t *testing.T) {
	w := newTransferWorld(t)
	tr, err := w.s.StartTransfer(w.mover, w.ownerA, "202")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != TransferAwaitingPerson || !tr.ByOwner {
		t.Fatalf("transfer = %+v", tr)
	}
	if _, _, active := w.userRow(t, w.mover); !active {
		t.Error("the account was switched off before the person agreed")
	}
	if in, _ := w.s.IncomingTransfers(w.sqB); len(in) != 0 {
		t.Error("the target's owners can see a move the person has not agreed to")
	}
	if _, err := w.s.AcceptTransfer(tr.ID, w.ownerB, "editor"); !errors.Is(err, ErrTransferGone) {
		t.Errorf("accepted before the person agreed: %v", err)
	}
	if _, err := w.s.ConfirmTransfer("wrong"); !errors.Is(err, ErrTransferGone) {
		t.Errorf("a wrong link confirmed: %v", err)
	}
	if t2, err := w.s.TransferByToken(tr.PersonalToken); err != nil || t2.ID != tr.ID {
		t.Fatalf("by token: %v", err)
	}
	pending, err := w.s.ConfirmTransfer(tr.PersonalToken)
	if err != nil || pending.Status != TransferPending {
		t.Fatalf("confirm = %+v, %v", pending, err)
	}
	if _, _, active := w.userRow(t, w.mover); active {
		t.Error("the account stayed on after the person agreed")
	}
	if _, err := w.s.ConfirmTransfer(tr.PersonalToken); !errors.Is(err, ErrTransferGone) {
		t.Errorf("confirmed twice: %v", err)
	}
	if _, err := w.s.AcceptTransfer(tr.ID, w.ownerB, "editor"); err != nil {
		t.Fatal(err)
	}

	t.Run("the person can decline instead", func(t *testing.T) {
		w2 := newTransferWorld(t)
		tr, _ := w2.s.StartTransfer(w2.mover, w2.ownerA, "202")
		if _, err := w2.s.DeclineTransfer(tr.ID, 0, tr.PersonalToken); err != nil {
			t.Fatal(err)
		}
		if sq, _, active := w2.userRow(t, w2.mover); sq != w2.sqA || !active {
			t.Error("declining changed the account")
		}
	})
}
