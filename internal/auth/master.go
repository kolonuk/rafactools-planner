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
// it creates the account if missing and removes any previous master admin
// whose email is different. It is called at every startup.
//
// The password setting only SEEDS the account: it is used when the account is
// new or its stored password is blank. Once a password is stored (the master
// admin can change it on Your details) the stored one wins and the setting is
// ignored, so a redeploy never overwrites it. The setting may be empty when a
// stored password exists. ResetMasterPassword is the explicit recovery path.
//
// It refuses (returns an error and changes nothing) if a password that would
// be used does not meet the normal rules, or the email already belongs to an
// ordinary member: an ordinary account is never promoted silently.
//
// The returned string says what happened, for the log. It never contains
// the password.
func (s *Service) EnsureMasterAdmin(email, password string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return "", fmt.Errorf("master admin email %q is not a valid address", email)
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

	var currentHash string
	err = tx.QueryRow(`SELECT password_hash FROM users WHERE email = ? AND is_master_admin = 1`, email).Scan(&currentHash)
	missing := errors.Is(err, sql.ErrNoRows)
	var hash []byte
	if missing || (err == nil && currentHash == "") {
		if password == "" {
			return "", errors.New("the master admin has no stored password and MASTER_ADMIN_PASSWORD is empty")
		}
		if cerr := checkPassword(password); cerr != nil {
			return "", fmt.Errorf("master admin password rejected: %w", cerr)
		}
		if hash, err = bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost); err != nil {
			return "", fmt.Errorf("hash password: %w", err)
		}
		err = nil
		if !missing {
			if _, err := tx.Exec(`UPDATE users SET password_hash = ? WHERE email = ? AND is_master_admin = 1`, string(hash), email); err != nil {
				return "", err
			}
			result = "master admin password set from MASTER_ADMIN_PASSWORD"
		}
	}
	switch {
	case missing:
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
	default:
		if replaced {
			result = "previous master admin removed; master admin unchanged"
		}
	}
	return result, tx.Commit()
}

// ResetMasterPassword is the recovery path (MASTER_ADMIN_RESET_PASSWORD): it
// replaces the stored password of the master admin with password and signs
// that account out everywhere. Like seeding, it applies the normal rules.
func (s *Service) ResetMasterPassword(email, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := checkPassword(password); err != nil {
		return fmt.Errorf("master admin password rejected: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE users SET password_hash = ?, must_change_password = 0 WHERE email = ? AND is_master_admin = 1`, string(hash), email)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotMasterAdmin
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE email = ?)`, email); err != nil {
		return err
	}
	return tx.Commit()
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
