package auth

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Moving a login to another squadron ("account transfer").
//
//   - Only an account with a personal email address can move: one on the claim
//     domain (the RAFAC addresses a squadron is claimed with) stays where it is.
//   - The person moves themself, or an owner of their squadron asks for them (and
//     then the person must agree by the link emailed to them).
//   - An owner of the squadron they go to must accept, and sets their role there.
//   - From the moment a transfer is waiting for that, the account is switched off:
//     it cannot sign in. It is switched back on when the transfer ends: in the new
//     squadron if accepted, in the old one if declined, cancelled or expired.
//   - Only the login moves. The old squadron's programmes, history, requirement
//     tracker and instructor list stay with it.

// TransferLifetime is how long a transfer may wait.
const TransferLifetime = 14 * 24 * time.Hour

// Transfer statuses.
const (
	TransferAwaitingPerson = "awaiting_person"
	TransferPending        = "pending"
	TransferAccepted       = "accepted"
	TransferDeclined       = "declined"
	TransferCancelled      = "cancelled"
	TransferExpired        = "expired"
)

var (
	ErrTransferNotAllowed   = errors.New("this account cannot be moved to another squadron")
	ErrTransferOfficial     = errors.New("accounts with a claim-domain (official RAFAC) address stay with their squadron; only personal email addresses can be moved")
	ErrTransferOnlyOwner    = errors.New("the only owner of a squadron cannot be moved: make someone else an owner first, or delete the squadron")
	ErrTransferNoTarget     = errors.New("there is no claimed squadron with that number")
	ErrTransferSameSquadron = errors.New("that is the squadron the account is already in")
	ErrTransferOpen         = errors.New("a move for this person is already waiting")
	ErrTransferInvite       = errors.New("that squadron has an invitation waiting for this email address: ask them to cancel it first")
	ErrTransferNoOwner      = errors.New("that squadron has no active owner who can accept")
	ErrTransferGone         = errors.New("that move is no longer waiting")
	ErrTransferRole         = errors.New("choose a role: owner, editor or viewer")
	ErrTransferTemp         = errors.New("this account is using a temporary password: they must choose their own first")
)

// Transfer is one request to move a login.
type Transfer struct {
	ID            int64
	UserID        int64
	Email         string
	DisplayName   string
	FromID, ToID  int64
	FromNumber    string
	FromName      string
	ToNumber      string
	ToName        string
	RequestedBy   int64
	ByOwner       bool
	Role          string
	Status        string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	PersonalToken string // only set by StartTransfer: the link to email to the person
	// WasPending is set when a transfer ends: it had been waiting for the target squadron's owners.
	WasPending bool
}

const transferSelect = `SELECT t.id, t.user_id, u.email, u.display_name, t.from_squadron_id, fs.number, fs.name, t.to_squadron_id, ts.number, ts.name,
	COALESCE(t.requested_by, 0), t.by_owner, t.role, t.status, t.created_at, t.expires_at
	FROM account_transfers t JOIN users u ON u.id = t.user_id
	JOIN squadrons fs ON fs.id = t.from_squadron_id JOIN squadrons ts ON ts.id = t.to_squadron_id `

func scanTransfer(row interface{ Scan(...any) error }) (*Transfer, error) {
	var t Transfer
	var byOwner int
	if err := row.Scan(&t.ID, &t.UserID, &t.Email, &t.DisplayName, &t.FromID, &t.FromNumber, &t.FromName, &t.ToID, &t.ToNumber, &t.ToName,
		&t.RequestedBy, &byOwner, &t.Role, &t.Status, &t.CreatedAt, &t.ExpiresAt); err != nil {
		return nil, err
	}
	t.ByOwner = byOwner != 0
	return &t, nil
}

func hashTransferToken(t string) string {
	sum := sha256.Sum256([]byte("account-transfer:" + t))
	return hex.EncodeToString(sum[:])
}

// Name is how the person is shown: their display name, else their address.
func (t *Transfer) Name() string {
	if t.DisplayName != "" {
		return t.DisplayName
	}
	return t.Email
}

// checkMovable returns why a user cannot be moved (nil if they can).
func (s *Service) checkMovable(q interface {
	QueryRow(string, ...any) *sql.Row
}, userID int64) (email string, fromSquadron int64, err error) {
	var master, active, mustChange int
	var role string
	var verified sql.NullTime
	err = q.QueryRow(`SELECT email, squadron_id, role, is_master_admin, active, must_change_password, email_verified_at FROM users WHERE id = ?`, userID).
		Scan(&email, &fromSquadron, &role, &master, &active, &mustChange, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNotMember
	} else if err != nil {
		return "", 0, err
	}
	switch {
	case master != 0:
		return "", 0, ErrTransferNotAllowed
	case domainOf(email) == strings.ToLower(s.claimDomain):
		return "", 0, ErrTransferOfficial
	case active == 0 || !verified.Valid:
		return "", 0, ErrTransferNotAllowed
	case mustChange != 0:
		return "", 0, ErrTransferTemp
	}
	if role == "owner" {
		var others int
		if err := q.QueryRow(`SELECT COUNT(*) FROM users WHERE squadron_id = ? AND role = 'owner' AND active = 1 AND id != ?`, fromSquadron, userID).Scan(&others); err != nil {
			return "", 0, err
		}
		if others == 0 {
			return "", 0, ErrTransferOnlyOwner
		}
	}
	return email, fromSquadron, nil
}

// MovableReason returns why the user cannot be moved, or nil.
func (s *Service) MovableReason(userID int64) error {
	_, _, err := s.checkMovable(s.db, userID)
	return err
}

// StartTransfer asks to move userID to the squadron with number toNumber. requestedBy
// is the person themself or an owner of their squadron. If the person asked, the
// transfer is pending at once and their account is switched off. If an owner asked,
// it waits for the person to agree first (PersonalToken is the link to email them).
func (s *Service) StartTransfer(userID, requestedBy int64, toNumber string) (*Transfer, error) {
	toNumber = strings.TrimSpace(toNumber)
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	email, from, err := s.checkMovable(tx, userID)
	if err != nil {
		return nil, err
	}
	byOwner := requestedBy != userID
	if byOwner {
		var role string
		var active int
		var sq int64
		if err := tx.QueryRow(`SELECT role, active, squadron_id FROM users WHERE id = ?`, requestedBy).Scan(&role, &active, &sq); err != nil || role != "owner" || active == 0 || sq != from {
			return nil, ErrTransferNotAllowed // only an owner of the person's own squadron may ask for them
		}
	}
	var to int64
	switch err := tx.QueryRow(`SELECT id FROM squadrons WHERE number = ?`, toNumber).Scan(&to); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, ErrTransferNoTarget
	case err != nil:
		return nil, err
	}
	if to == from {
		return nil, ErrTransferSameSquadron
	}
	var owners int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE squadron_id = ? AND role = 'owner' AND active = 1 AND email_verified_at IS NOT NULL`, to).Scan(&owners); err != nil {
		return nil, err
	}
	if owners == 0 {
		return nil, ErrTransferNoOwner
	}
	var invites int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM invites WHERE squadron_id = ? AND lower(email) = lower(?) AND accepted_at IS NULL AND expires_at > ?`, to, email, time.Now().UTC()).Scan(&invites); err != nil {
		return nil, err
	}
	if invites > 0 {
		return nil, ErrTransferInvite
	}

	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	status := TransferPending
	if byOwner {
		status = TransferAwaitingPerson
	}
	now := time.Now().UTC()
	res, err := tx.Exec(`INSERT INTO account_transfers (user_id, from_squadron_id, to_squadron_id, requested_by, by_owner, status, person_token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, userID, from, to, requestedBy, boolInt(byOwner), status, hashTransferToken(token), now, now.Add(TransferLifetime))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrTransferOpen
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	if status == TransferPending {
		if err := switchOff(tx, userID); err != nil {
			return nil, err
		}
	}
	t, err := scanTransfer(tx.QueryRow(transferSelect+`WHERE t.id = ?`, id))
	if err != nil {
		return nil, err
	}
	t.PersonalToken = token
	return t, tx.Commit()
}

// switchOff makes the account unable to sign in and ends its sessions and links.
func switchOff(tx *sql.Tx, userID int64) error {
	for _, q := range []string{
		`UPDATE users SET active = 0 WHERE id = ?`,
		`DELETE FROM sessions WHERE user_id = ?`,
		`DELETE FROM password_resets WHERE user_id = ?`,
		`DELETE FROM email_changes WHERE user_id = ?`,
	} {
		if _, err := tx.Exec(q, userID); err != nil {
			return err
		}
	}
	return nil
}

// TransferByToken finds the transfer a person's emailed link is for.
func (s *Service) TransferByToken(token string) (*Transfer, error) {
	if token == "" {
		return nil, ErrTransferGone
	}
	t, err := scanTransfer(s.db.QueryRow(transferSelect+`WHERE t.person_token_hash = ?`, hashTransferToken(token)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTransferGone
	}
	return t, err
}

// OpenTransferFor returns the transfer that is waiting for the user, if any.
func (s *Service) OpenTransferFor(userID int64) (*Transfer, error) {
	t, err := scanTransfer(s.db.QueryRow(transferSelect+`WHERE t.user_id = ? AND t.status IN ('awaiting_person','pending')`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// TransferGet returns one transfer by id.
func (s *Service) TransferGet(id int64) (*Transfer, error) {
	t, err := scanTransfer(s.db.QueryRow(transferSelect+`WHERE t.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTransferGone
	}
	return t, err
}

// ConfirmTransfer is the person agreeing, by their emailed link, to a move an owner
// asked for. The transfer becomes pending and the account is switched off.
func (s *Service) ConfirmTransfer(token string) (*Transfer, error) {
	return s.confirm(`t.person_token_hash = ?`, hashTransferToken(token))
}

// ConfirmTransferFor is the same for a person who is signed in: they agree to the
// move waiting for them.
func (s *Service) ConfirmTransferFor(userID int64) (*Transfer, error) {
	return s.confirm(`t.user_id = ?`, userID)
}

func (s *Service) confirm(where string, arg any) (*Transfer, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t, err := scanTransfer(tx.QueryRow(transferSelect+`WHERE `+where+` AND t.status = 'awaiting_person'`, arg))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (t.Status != TransferAwaitingPerson || !t.ExpiresAt.After(time.Now()))) {
		return nil, ErrTransferGone
	} else if err != nil {
		return nil, err
	}
	// Everything may have changed since the owner asked.
	if _, from, err := s.checkMovable(tx, t.UserID); err != nil {
		return nil, err
	} else if from != t.FromID {
		return nil, ErrTransferGone
	}
	if _, err := tx.Exec(`UPDATE account_transfers SET status = 'pending', expires_at = ? WHERE id = ?`, time.Now().UTC().Add(TransferLifetime), t.ID); err != nil {
		return nil, err
	}
	if err := switchOff(tx, t.UserID); err != nil {
		return nil, err
	}
	t.Status = TransferPending
	return t, tx.Commit()
}

// finish ends an open transfer without a move: the account is switched back on
// (only a pending transfer had switched it off).
func finish(tx *sql.Tx, t *Transfer, status string, by int64) error {
	t.WasPending = t.Status == TransferPending
	if _, err := tx.Exec(`UPDATE account_transfers SET status = ?, resolved_at = ?, resolved_by = NULLIF(?, 0) WHERE id = ?`, status, time.Now().UTC(), by, t.ID); err != nil {
		return err
	}
	if t.Status == TransferPending {
		if _, err := tx.Exec(`UPDATE users SET active = 1 WHERE id = ?`, t.UserID); err != nil {
			return err
		}
	}
	t.Status = status
	return nil
}

func openTransfer(tx *sql.Tx, where string, args ...any) (*Transfer, error) {
	t, err := scanTransfer(tx.QueryRow(transferSelect+`WHERE `+where+` AND t.status IN ('awaiting_person','pending')`, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTransferGone
	}
	return t, err
}

// CancelTransfer ends a waiting transfer. actingUserID is whoever cancels: the person
// (if they can sign in), whoever asked, or an owner of the squadron it leaves. byToken
// is the person's emailed link, which works while their account is switched off.
func (s *Service) CancelTransfer(id, actingUserID int64, personToken string) (*Transfer, error) {
	return s.end(id, TransferCancelled, func(t *Transfer, tx *sql.Tx) bool {
		if personToken != "" {
			return scanOK(tx, `SELECT 1 FROM account_transfers WHERE id = ? AND person_token_hash = ?`, t.ID, hashTransferToken(personToken))
		}
		return actingUserID == t.UserID || actingUserID == t.RequestedBy || isOwnerOf(tx, actingUserID, t.FromID)
	}, actingUserID)
}

// DeclineTransfer is an owner of the squadron the person is going to saying no, or
// the person themself declining a move an owner asked for.
func (s *Service) DeclineTransfer(id, actingUserID int64, personToken string) (*Transfer, error) {
	return s.end(id, TransferDeclined, func(t *Transfer, tx *sql.Tx) bool {
		if personToken != "" {
			return scanOK(tx, `SELECT 1 FROM account_transfers WHERE id = ? AND person_token_hash = ?`, t.ID, hashTransferToken(personToken))
		}
		return t.Status == TransferPending && isOwnerOf(tx, actingUserID, t.ToID)
	}, actingUserID)
}

func scanOK(tx *sql.Tx, q string, args ...any) bool {
	var one int
	return tx.QueryRow(q, args...).Scan(&one) == nil
}

func isOwnerOf(tx *sql.Tx, userID, squadronID int64) bool {
	return scanOK(tx, `SELECT 1 FROM users WHERE id = ? AND squadron_id = ? AND role = 'owner' AND active = 1`, userID, squadronID)
}

func (s *Service) end(id int64, status string, allowed func(*Transfer, *sql.Tx) bool, by int64) (*Transfer, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t, err := openTransfer(tx, `t.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if !allowed(t, tx) {
		return nil, ErrTransferNotAllowed
	}
	if err := finish(tx, t, status, by); err != nil {
		return nil, err
	}
	return t, tx.Commit()
}

// IncomingTransfers are the moves waiting for an owner of the squadron to accept.
func (s *Service) IncomingTransfers(squadronID int64) ([]Transfer, error) {
	return s.listTransfers(`WHERE t.to_squadron_id = ? AND t.status = 'pending' ORDER BY t.id`, squadronID)
}

// OutgoingTransfers are the open moves of the squadron's members to other squadrons.
func (s *Service) OutgoingTransfers(squadronID int64) ([]Transfer, error) {
	return s.listTransfers(`WHERE t.from_squadron_id = ? AND t.status IN ('awaiting_person','pending') ORDER BY t.id`, squadronID)
}

func (s *Service) listTransfers(where string, args ...any) ([]Transfer, error) {
	rows, err := s.db.Query(transferSelect+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Transfer
	for rows.Next() {
		t, err := scanTransfer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// TransferOwners are the addresses of a squadron's active owners with a confirmed address.
func (s *Service) TransferOwners(squadronID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT email FROM users WHERE squadron_id = ? AND role = 'owner' AND active = 1 AND email_verified_at IS NOT NULL ORDER BY email`, squadronID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AcceptTransfer is an owner of the receiving squadron accepting a pending move and
// setting the role. In one step the person's squadron and role change, every session of
// theirs ends so they sign in afresh, and the account is switched back on. The password,
// two-step sign-in and display name are kept.
func (s *Service) AcceptTransfer(id, ownerID int64, role string) (*Transfer, error) {
	if role != "owner" && role != "editor" && role != "viewer" {
		return nil, ErrTransferRole
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t, err := openTransfer(tx, `t.id = ?`, id)
	if err != nil || t.Status != TransferPending || !t.ExpiresAt.After(time.Now()) {
		return nil, ErrTransferGone
	}
	if !isOwnerOf(tx, ownerID, t.ToID) {
		return nil, ErrTransferNotAllowed
	}
	// The person has been switched off since the request, so the checks that need an
	// active account are made against what is stored, apart from that.
	var email, srole string
	var sq int64
	var master, mustChange int
	var verified sql.NullTime
	if err := tx.QueryRow(`SELECT email, squadron_id, role, is_master_admin, must_change_password, email_verified_at FROM users WHERE id = ?`, t.UserID).
		Scan(&email, &sq, &srole, &master, &mustChange, &verified); err != nil {
		return nil, err
	}
	if sq != t.FromID || master != 0 || !verified.Valid || mustChange != 0 || domainOf(email) == strings.ToLower(s.claimDomain) {
		return nil, ErrTransferGone
	}
	if srole == "owner" {
		var others int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE squadron_id = ? AND role = 'owner' AND active = 1 AND id != ?`, t.FromID, t.UserID).Scan(&others); err != nil {
			return nil, err
		}
		if others == 0 {
			return nil, ErrTransferOnlyOwner
		}
	}
	var invites int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM invites WHERE squadron_id = ? AND lower(email) = lower(?) AND accepted_at IS NULL AND expires_at > ?`, t.ToID, email, time.Now().UTC()).Scan(&invites); err != nil {
		return nil, err
	}
	if invites > 0 {
		return nil, ErrTransferInvite
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE users SET squadron_id = ?, role = ?, active = 1 WHERE id = ?`, []any{t.ToID, role, t.UserID}},
		{`DELETE FROM sessions WHERE user_id = ?`, []any{t.UserID}},
		{`UPDATE account_transfers SET status = 'accepted', role = ?, resolved_at = ?, resolved_by = ? WHERE id = ?`, []any{role, time.Now().UTC(), ownerID, t.ID}},
	} {
		if _, err := tx.Exec(q.sql, q.args...); err != nil {
			return nil, fmt.Errorf("accept transfer: %w", err)
		}
	}
	t.Status, t.Role = TransferAccepted, role
	return t, tx.Commit()
}

// ExpireTransfers ends every transfer whose time has run out, switching accounts back
// on, and returns them so the people involved can be told.
func (s *Service) ExpireTransfers(now time.Time) ([]Transfer, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(transferSelect+`WHERE t.status IN ('awaiting_person','pending') AND t.expires_at <= ?`, now.UTC())
	if err != nil {
		return nil, err
	}
	var due []*Transfer
	for rows.Next() {
		t, err := scanTransfer(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		due = append(due, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []Transfer
	for _, t := range due {
		if err := finish(tx, t, TransferExpired, 0); err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, tx.Commit()
}
