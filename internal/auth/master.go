package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// MasterHomeNumber is the squadron number of the hidden "home" squadron the
// master admin account lives in. Claim numbers are digits only, so no real
// squadron can ever collide with it.
const MasterHomeNumber = "master"

var (
	ErrNotMasterAdmin = errors.New("only the master admin can do that")
	ErrNoSuchSquadron = errors.New("that squadron doesn't exist")
)

// EnsureMasterAdmin makes the database match the configured master admin:
// it creates the account if missing, updates the password if it changed
// (signing that account out when it does), and removes any previous master
// admin whose email is different. It is called at every startup, so a
// redeploy with new settings takes effect immediately.
//
// It refuses (returns an error and changes nothing) if the password does not
// meet the normal rules, or the email already belongs to an ordinary member:
// an ordinary account is never promoted silently.
//
// The returned string says what happened, for the log. It never contains
// the password.
func (s *Service) EnsureMasterAdmin(email, password string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return "", fmt.Errorf("master admin email %q is not a valid address", email)
	}
	if err := checkPassword(password); err != nil {
		return "", fmt.Errorf("master admin password rejected: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var existingMaster int
	err = tx.QueryRow(`SELECT is_master_admin FROM users WHERE email = ?`, email).Scan(&existingMaster)
	switch {
	case err == nil && existingMaster == 0: // a row exists and it is not the master admin
		return "", fmt.Errorf("master admin email %s already belongs to an ordinary account; refusing to promote it (remove that account first)", email)
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return "", err
	}

	// Home squadron.
	var homeID int64
	err = tx.QueryRow(`SELECT id FROM squadrons WHERE number = ?`, MasterHomeNumber).Scan(&homeID)
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.Exec(`INSERT INTO squadrons (number, name) VALUES (?, 'Master admin')`, MasterHomeNumber)
		if err != nil {
			return "", err
		}
		if homeID, err = res.LastInsertId(); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	result := "master admin unchanged"
	// A previous master admin with a different email is removed.
	res, err := tx.Exec(`DELETE FROM users WHERE is_master_admin = 1 AND email != ?`, email)
	if err != nil {
		return "", err
	}
	replaced := false
	if n, _ := res.RowsAffected(); n > 0 {
		replaced = true
		result = "previous master admin removed; "
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	var currentHash string
	err = tx.QueryRow(`SELECT password_hash FROM users WHERE email = ? AND is_master_admin = 1`, email).Scan(&currentHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(`INSERT INTO users (squadron_id, email, password_hash, display_name, role, email_verified_at, is_master_admin)
			VALUES (?, ?, ?, 'Master admin', 'owner', ?, 1)`, homeID, email, string(hash), time.Now()); err != nil {
			return "", err
		}
		result = "master admin created"
		if replaced {
			result = "previous master admin removed; master admin created"
		}
	case err != nil:
		return "", err
	case bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(password)) != nil:
		if _, err := tx.Exec(`UPDATE users SET password_hash = ? WHERE email = ? AND is_master_admin = 1`, string(hash), email); err != nil {
			return "", err
		}
		// A changed password signs the account out everywhere.
		if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE email = ?)`, email); err != nil {
			return "", err
		}
		result = "master admin password updated; sessions signed out"
	default:
		if replaced {
			result = "previous master admin removed; master admin unchanged"
		}
	}
	return result, tx.Commit()
}

// SwitchableSquadron is a squadron the master admin can look at.
type SwitchableSquadron struct {
	ID     int64
	Number string
	Name   string
}

// ListSwitchableSquadrons lists every real squadron (not the hidden home
// squadron of the master admin).
func (s *Service) ListSwitchableSquadrons() ([]SwitchableSquadron, error) {
	rows, err := s.db.Query(`SELECT id, number, name FROM squadrons WHERE number != ? ORDER BY CAST(number AS INTEGER), name`, MasterHomeNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SwitchableSquadron
	for rows.Next() {
		var q SwitchableSquadron
		if err := rows.Scan(&q.ID, &q.Number, &q.Name); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// SwitchSquadron makes the master admin's session look at another squadron
// (read-only; see UserBySession). squadronID 0 switches back to their own.
// Anyone who is not the master admin is refused, whatever they pass.
func (s *Service) SwitchSquadron(sessionToken string, userID, squadronID int64) error {
	var isMaster int
	if err := s.db.QueryRow(`SELECT is_master_admin FROM users WHERE id = ?`, userID).Scan(&isMaster); err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}
	if isMaster != 1 {
		return ErrNotMasterAdmin
	}
	if squadronID == 0 {
		_, err := s.db.Exec(`UPDATE sessions SET acting_squadron_id = NULL WHERE token = ? AND user_id = ?`, sessionToken, userID)
		return err
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM squadrons WHERE id = ? AND number != ?`, squadronID, MasterHomeNumber).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNoSuchSquadron
	}
	_, err := s.db.Exec(`UPDATE sessions SET acting_squadron_id = ? WHERE token = ? AND user_id = ?`, squadronID, sessionToken, userID)
	return err
}
