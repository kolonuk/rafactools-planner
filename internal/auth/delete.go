package auth

import (
	"database/sql"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ErrCannotDeleteHome is returned for the master admin's hidden home squadron.
var ErrCannotDeleteHome = errors.New("that squadron cannot be deleted")

// DeleteSquadron permanently removes a squadron and everything under it:
// members, sign-in sessions, invites, programmes (with closures, weekend notes
// and the requirements tracker with its cadets' names), instructors and its
// security log. The foreign keys' cascades do the work, in one statement, so
// nothing is left behind. It needs the acting owner's password and the
// squadron's number typed back, so a stolen session or a mis-click cannot do it.
func (s *Service) DeleteSquadron(squadronID, actingUserID int64, password, typedNumber string) error {
	var number string
	err := s.db.QueryRow(`SELECT number FROM squadrons WHERE id = ?`, squadronID).Scan(&number)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoSuchSquadron
	}
	if err != nil {
		return err
	}
	if number == MasterHomeNumber {
		return ErrCannotDeleteHome
	}

	var hash, role string
	var sq int64
	if err := s.db.QueryRow(`SELECT password_hash, role, squadron_id FROM users WHERE id = ?`, actingUserID).Scan(&hash, &role, &sq); err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}
	if sq != squadronID || role != "owner" {
		return ErrNoSuchSquadron
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return ErrWrongPassword
	}
	if typedNumber != number {
		return ErrConfirmMismatch
	}
	// One transaction: first detach anything outside the squadron that still
	// names one of its members as author, then delete. (Within a squadron the
	// cascade handles it; this is belt and braces for stray cross-references.)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE programmes SET created_by = NULL WHERE created_by IN (SELECT id FROM users WHERE squadron_id = ?)`, squadronID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM squadrons WHERE id = ?`, squadronID); err != nil {
		return err
	}
	return tx.Commit()
}

// ErrConfirmMismatch means the typed squadron number did not match.
var ErrConfirmMismatch = errors.New("the squadron number you typed does not match")
