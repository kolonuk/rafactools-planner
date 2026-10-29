package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Owner tools for members who cannot get in: mark a member inactive (no
// sign-in, no password reset until made active again) or send a temporary
// password by email.

// TempPasswordTTL is how long a temporary password works.
const TempPasswordTTL = 7 * 24 * time.Hour

var (
	ErrSelfAction     = errors.New("you can't do that to your own account; use Your details instead")
	ErrMemberInactive = errors.New("that member is inactive: make them active before sending a password")
)

// SetMemberActive marks a member of the squadron active or inactive. Going
// inactive signs them out everywhere and cancels any reset link or temporary
// password. The acting owner cannot change their own status, and the
// squadron's last active owner cannot be made inactive.
func (s *Service) SetMemberActive(squadronID, targetUserID, actingUserID int64, active bool) error {
	if targetUserID == actingUserID {
		return ErrSelfAction
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var isMaster bool
	err = tx.QueryRow(`SELECT is_master_admin FROM users WHERE id = ? AND squadron_id = ?`, targetUserID, squadronID).Scan(&isMaster)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotMember
	} else if err != nil {
		return fmt.Errorf("lookup member: %w", err)
	}
	if isMaster {
		return ErrNotMember // the site administrator is never managed from a squadron
	}
	if !active {
		last, err := isLastActiveOwner(tx, squadronID, targetUserID)
		if err != nil {
			return err
		}
		if last {
			return ErrLastOwner
		}
	}
	if _, err := tx.Exec(`UPDATE users SET active = ? WHERE id = ? AND squadron_id = ?`, boolInt(active), targetUserID, squadronID); err != nil {
		return err
	}
	if !active {
		for _, q := range []string{
			`DELETE FROM sessions WHERE user_id = ?`,
			`DELETE FROM password_resets WHERE user_id = ?`,
			`DELETE FROM email_changes WHERE user_id = ?`,
			`UPDATE users SET must_change_password = 0, temp_password_expires_at = NULL WHERE id = ?`,
		} {
			if _, err := tx.Exec(q, targetUserID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TempPassword is a prepared (not yet applied) temporary password. Plain is
// for the email only; it is never stored or shown to the owner.
type TempPassword struct {
	UserID    int64
	Email     string
	Plain     string
	ExpiresAt time.Time
	hash      string
	squadron  int64
}

// tempAlphabet leaves out look-alike characters (0/O, 1/l/I).
const tempAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

func newTempPlain() (string, error) {
	const groups, per = 4, 5
	raw := make([]byte, groups*per)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	var b strings.Builder
	for i, v := range raw {
		if i > 0 && i%per == 0 {
			b.WriteByte('-')
		}
		b.WriteByte(tempAlphabet[int(v)%len(tempAlphabet)])
	}
	return b.String(), nil
}

// PrepareTempPassword checks that an owner may send this member a temporary
// password and makes one, without changing anything yet: the caller emails it
// first and only then calls ApplyTempPassword, so a failed email never leaves a
// member locked out of an account whose new password nobody knows.
func (s *Service) PrepareTempPassword(squadronID, targetUserID, actingUserID int64) (*TempPassword, error) {
	if targetUserID == actingUserID {
		return nil, ErrSelfAction
	}
	var email string
	var active, isMaster bool
	err := s.db.QueryRow(`SELECT email, active, is_master_admin FROM users WHERE id = ? AND squadron_id = ?`, targetUserID, squadronID).Scan(&email, &active, &isMaster)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotMember
	} else if err != nil {
		return nil, fmt.Errorf("lookup member: %w", err)
	}
	if isMaster {
		return nil, ErrNotMember
	}
	if !active {
		return nil, ErrMemberInactive
	}
	plain, err := newTempPlain()
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	return &TempPassword{UserID: targetUserID, Email: email, Plain: plain, ExpiresAt: time.Now().Add(TempPasswordTTL), hash: string(hash), squadron: squadronID}, nil
}

// ApplyTempPassword makes the prepared password the member's password. They
// are signed out everywhere, other reset links are cancelled, and at their next
// sign-in they must choose their own password before doing anything else.
func (s *Service) ApplyTempPassword(t *TempPassword) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active bool
	if err := tx.QueryRow(`SELECT active FROM users WHERE id = ? AND squadron_id = ?`, t.UserID, t.squadron).Scan(&active); err != nil || !active {
		return ErrMemberInactive
	}
	if _, err := tx.Exec(`UPDATE users SET password_hash = ?, must_change_password = 1, temp_password_expires_at = ? WHERE id = ?`, t.hash, t.ExpiresAt, t.UserID); err != nil {
		return err
	}
	for _, q := range []string{`DELETE FROM sessions WHERE user_id = ?`, `DELETE FROM password_resets WHERE user_id = ?`} {
		if _, err := tx.Exec(q, t.UserID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- changing your email address -------------------------------------------

var (
	ErrEmailChangeNotAllowed = errors.New("addresses on the RAFAC domain can't be changed here")
	ErrEmailChangeInvalid    = errors.New("this email change link is invalid or has expired")
	ErrNewEmailInvalid       = errors.New("that doesn't look like a valid email address")
	ErrNewEmailOnClaimDomain = errors.New("choose an address that is not on the RAFAC domain")
)

const emailChangeTTL = 24 * time.Hour

func hashEmailToken(t string) string {
	sum := sha256.Sum256([]byte("email-change:" + t))
	return hex.EncodeToString(sum[:])
}

func domainOf(email string) string {
	_, d, _ := strings.Cut(strings.ToLower(email), "@")
	return d
}

// CanChangeEmail says whether this user may change their own address: only
// accounts whose address is NOT on the claim domain (the RAFAC addresses are
// what squadrons are claimed with, so they stay as they are), and never the
// master admin, whose address comes from the site's settings.
func (s *Service) CanChangeEmail(userID int64) bool {
	var email string
	var master bool
	if err := s.db.QueryRow(`SELECT email, is_master_admin FROM users WHERE id = ?`, userID).Scan(&email, &master); err != nil {
		return false
	}
	return !master && domainOf(email) != strings.ToLower(s.claimDomain)
}

// RequestEmailChange starts a change of address. It needs the account's
// password, refuses addresses on the claim domain and ones already in use, and
// returns a token for the link to email to the NEW address; nothing changes
// until that link is confirmed.
func (s *Service) RequestEmailChange(userID int64, newEmail, password string) (token string, normalized string, err error) {
	if !s.CanChangeEmail(userID) {
		return "", "", ErrEmailChangeNotAllowed
	}
	a, perr := mail.ParseAddress(strings.TrimSpace(newEmail))
	if perr != nil || a.Address != strings.TrimSpace(newEmail) || len(a.Address) > 200 {
		return "", "", ErrNewEmailInvalid
	}
	normalized = strings.ToLower(a.Address)
	if domainOf(normalized) == strings.ToLower(s.claimDomain) {
		return "", "", ErrNewEmailOnClaimDomain
	}
	var hash string
	if err := s.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash); err != nil {
		return "", "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", "", ErrWrongPassword
	}
	var taken int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, normalized).Scan(&taken); err != nil {
		return "", "", err
	}
	if taken > 0 {
		return "", "", ErrEmailTaken
	}
	token, err = randomToken()
	if err != nil {
		return "", "", err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.Exec(`DELETE FROM email_changes WHERE user_id = ? OR expires_at < ?`, userID, now); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(`INSERT INTO email_changes (token_hash, user_id, new_email, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		hashEmailToken(token), userID, normalized, now.Add(emailChangeTTL), now); err != nil {
		return "", "", err
	}
	return token, normalized, tx.Commit()
}

// EmailChangeTarget reports the address a link would switch to, without using
// the link up (opening it must not consume it: mail scanners open links).
func (s *Service) EmailChangeTarget(token string) (string, error) {
	var newEmail string
	var expires time.Time
	err := s.db.QueryRow(`SELECT new_email, expires_at FROM email_changes WHERE token_hash = ?`, hashEmailToken(token)).Scan(&newEmail, &expires)
	if err != nil || time.Now().After(expires) {
		return "", ErrEmailChangeInvalid
	}
	return newEmail, nil
}

// ConfirmEmailChange applies a confirmed change and returns the user, old and
// new addresses. The rules are checked again here, since things may have
// changed since the link was sent.
func (s *Service) ConfirmEmailChange(token string) (userID int64, oldEmail, newEmail string, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, "", "", err
	}
	defer tx.Rollback()
	var expires time.Time
	err = tx.QueryRow(`SELECT user_id, new_email, expires_at FROM email_changes WHERE token_hash = ?`, hashEmailToken(token)).Scan(&userID, &newEmail, &expires)
	if err != nil || time.Now().After(expires) {
		return 0, "", "", ErrEmailChangeInvalid
	}
	var active bool
	if err := tx.QueryRow(`SELECT email, active FROM users WHERE id = ?`, userID).Scan(&oldEmail, &active); err != nil || !active {
		return 0, "", "", ErrEmailChangeInvalid
	}
	if domainOf(oldEmail) == strings.ToLower(s.claimDomain) || domainOf(newEmail) == strings.ToLower(s.claimDomain) {
		return 0, "", "", ErrEmailChangeNotAllowed
	}
	if _, err := tx.Exec(`UPDATE users SET email = ? WHERE id = ?`, newEmail, userID); err != nil {
		return 0, "", "", ErrEmailTaken // the unique index: someone took it meanwhile
	}
	if _, err := tx.Exec(`DELETE FROM email_changes WHERE user_id = ?`, userID); err != nil {
		return 0, "", "", err
	}
	// Reset links were addressed to the old address; they are void.
	if _, err := tx.Exec(`DELETE FROM password_resets WHERE user_id = ?`, userID); err != nil {
		return 0, "", "", err
	}
	return userID, oldEmail, newEmail, tx.Commit()
}
