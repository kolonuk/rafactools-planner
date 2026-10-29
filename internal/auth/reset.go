package auth

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ErrResetInvalid is returned for any reset token that is unknown, used or
// expired; callers must not say which.
var ErrResetInvalid = errors.New("this password reset link is invalid or has expired")

const (
	resetTTL = time.Hour
	// resetCooldown stops one address being used to flood someone's inbox:
	// within it, a repeat request sends nothing (and the page looks the same).
	resetCooldown = time.Minute
)

// hashResetToken is what is stored. The raw token only ever exists in the
// email and the link.
func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RequestPasswordReset prepares a reset for the account with this email.
// ok is true only when an email should be sent: the account exists, its
// address is verified, and no reset was sent in the last minute. Callers
// must respond identically whatever ok is, so the page cannot be used to
// discover which addresses have accounts.
func (s *Service) RequestPasswordReset(email string) (token string, ok bool, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var userID int64
	var verified sql.NullTime
	err = s.db.QueryRow(`SELECT id, email_verified_at FROM users WHERE email = ?`, email).Scan(&userID, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	} else if err != nil {
		return "", false, fmt.Errorf("lookup user: %w", err)
	}
	if !verified.Valid {
		return "", false, nil // they must confirm their address first
	}

	now := time.Now()
	var last time.Time
	// Select the column itself (not MAX(...)), which would lose its DATETIME
	// type and fail to scan into a time.Time.
	err = s.db.QueryRow(`SELECT created_at FROM password_resets WHERE user_id = ? ORDER BY created_at DESC LIMIT 1`, userID).Scan(&last)
	switch {
	case err == nil && now.Sub(last) < resetCooldown:
		return "", false, nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return "", false, fmt.Errorf("check cooldown: %w", err)
	}

	token, err = randomToken()
	if err != nil {
		return "", false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	// One live token per user; also sweep anything expired.
	if _, err := tx.Exec(`DELETE FROM password_resets WHERE user_id = ? OR expires_at < ?`, userID, now); err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(`INSERT INTO password_resets (token_hash, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		hashResetToken(token), userID, now.Add(resetTTL), now); err != nil {
		return "", false, fmt.Errorf("insert reset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return token, true, nil
}

// CheckPasswordReset reports whether a token is currently usable, without
// consuming it (opening the link must not use it up: mail scanners open links).
func (s *Service) CheckPasswordReset(token string) error {
	_, err := s.resetUser(s.db, token)
	return err
}

type queryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

func (s *Service) resetUser(q queryer, token string) (int64, error) {
	var userID int64
	var expires time.Time
	err := q.QueryRow(`SELECT user_id, expires_at FROM password_resets WHERE token_hash = ?`, hashResetToken(token)).Scan(&userID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrResetInvalid
	} else if err != nil {
		return 0, fmt.Errorf("lookup reset: %w", err)
	}
	if time.Now().After(expires) {
		return 0, ErrResetInvalid
	}
	return userID, nil
}

// ResetPassword sets a new password using a token, uses the token up, and
// signs the user out everywhere (anyone who had the old password, or a
// stolen session, is cut off). A rejected password does not use the token up.
func (s *Service) ResetPassword(token, newPassword string) error {
	if _, err := s.resetUser(s.db, token); err != nil {
		return err
	}
	if err := checkPassword(newPassword); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	userID, err := s.resetUser(tx, token) // re-check inside the transaction: single use
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM password_resets WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_ = s.AuditUser(userID, AuditPasswordReset, "")
	return nil
}

// ErrWrongPassword is returned when the "current password" given to
// ChangePassword is not the account's password.
var ErrWrongPassword = errors.New("your current password is not correct")

// ChangePassword sets a new password for a signed-in user who proves they
// know the current one. Every OTHER session of that user is signed out
// (someone else holding an old session loses access); the session making the
// change, keepSessionToken, stays signed in. Any pending reset links are
// cancelled.
func (s *Service) ChangePassword(userID int64, keepSessionToken, current, newPassword string) error {
	var hash string
	err := s.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash)
	if err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
		return ErrWrongPassword
	}
	if err := checkPassword(newPassword); err != nil {
		return err
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(newHash), userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ? AND token != ?`, userID, keepSessionToken); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM password_resets WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_ = s.AuditUser(userID, AuditPasswordChange, "")
	return nil
}
