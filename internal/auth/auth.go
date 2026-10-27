// Package auth handles claiming a squadron, inviting members, email
// confirmation, login sessions and CSRF tokens.
//
// There are two ways to get an account:
//  1. Claim: the first person to sign up with an email matching
//     snco.<number>@<domain> or oc.<number>@<domain> creates that squadron
//     and becomes its owner. Each squadron number can only be claimed once.
//     This path self-restricts to the RAFAC domain and still requires
//     clicking an emailed confirmation link, since anyone can type any email
//     into the claim form.
//  2. Invite: an existing member of a squadron invites any email address.
//     Accepting the invite link creates the account immediately verified —
//     the invite link landing in that inbox is itself the proof, and the
//     inviting member is vouching for them.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailDomainNotAllowed = errors.New("email must be your squadron's snco.<number>@ or oc.<number>@ address")
	ErrSquadronTaken         = errors.New("that squadron number has already been claimed")
	ErrEmailTaken            = errors.New("an account with that email already exists")
	ErrInvalidCredentials    = errors.New("invalid email or password")
	ErrEmailNotVerified      = errors.New("email address has not been verified yet")
	ErrTokenInvalid          = errors.New("verification link is invalid or has expired")
	ErrInviteInvalid         = errors.New("invite link is invalid, expired, or already used")
)

// claimPattern matches the local-part of an email that's allowed to claim a
// squadron: snco.<digits> or oc.<digits>, case-insensitive.
var claimPattern = regexp.MustCompile(`(?i)^(?:snco|oc)\.(\d+)$`)

// ParseClaimEmail checks email against domain and the snco./oc. prefix
// convention, returning the squadron number it would claim.
func ParseClaimEmail(email, domain string) (number string, ok bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	domain = strings.ToLower(strings.TrimSpace(domain))
	at := strings.LastIndex(email, "@")
	if at < 0 || email[at+1:] != domain {
		return "", false
	}
	m := claimPattern.FindStringSubmatch(email[:at])
	if m == nil {
		return "", false
	}
	return m[1], true
}

type User struct {
	ID              int64
	SquadronID      int64
	Email           string
	DisplayName     string
	Role            string // owner | editor | viewer
	EmailVerifiedAt sql.NullTime
}

func (u User) Verified() bool { return u.EmailVerifiedAt.Valid }
func (u User) IsOwner() bool  { return u.Role == "owner" }
func (u User) CanEdit() bool  { return u.Role == "owner" || u.Role == "editor" }

type Invite struct {
	Token      string
	SquadronID int64
	Email      string
	Role       string
	ExpiresAt  time.Time
	Accepted   bool
}

type Service struct {
	db                    *sql.DB
	sessionKey            []byte
	secure                bool
	claimDomain           string
	skipEmailVerification bool
}

func NewService(db *sql.DB, sessionKey string, cookieSecure bool, claimDomain string, skipEmailVerification bool) *Service {
	return &Service{
		db: db, sessionKey: []byte(sessionKey), secure: cookieSecure,
		claimDomain: claimDomain, skipEmailVerification: skipEmailVerification,
	}
}

func (s *Service) CookieSecure() bool  { return s.secure }
func (s *Service) ClaimDomain() string { return s.claimDomain }

// ClaimSquadron creates a brand-new squadron and its first user (role
// "owner"). squadronName is the free-text display name the claimant chooses,
// e.g. "2491 (Lyneham) Squadron".
func (s *Service) ClaimSquadron(email, password, squadronName string) (userID int64, token string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	number, ok := ParseClaimEmail(email, s.claimDomain)
	if !ok {
		return 0, "", ErrEmailDomainNotAllowed
	}
	if err := checkPassword(password); err != nil {
		return 0, "", err
	}
	squadronName = strings.TrimSpace(squadronName)
	if squadronName == "" {
		squadronName = number + " Squadron"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, "", fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO squadrons (number, name) VALUES (?, ?)`, number, squadronName)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, "", ErrSquadronTaken
		}
		return 0, "", fmt.Errorf("insert squadron: %w", err)
	}
	squadronID, err := res.LastInsertId()
	if err != nil {
		return 0, "", err
	}

	// SkipEmailVerification (dev only — see config.Config) marks the owner
	// verified immediately instead of going through the token+email dance,
	// so claiming a squadron needs nothing but this form. Never set outside
	// dev: it removes the one check that the claiming email is real.
	userInsert := `INSERT INTO users (squadron_id, email, password_hash, role) VALUES (?, ?, ?, 'owner')`
	if s.skipEmailVerification {
		userInsert = `INSERT INTO users (squadron_id, email, password_hash, role, email_verified_at) VALUES (?, ?, ?, 'owner', CURRENT_TIMESTAMP)`
	}
	res, err = tx.Exec(userInsert, squadronID, email, string(hash))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, "", ErrEmailTaken
		}
		return 0, "", fmt.Errorf("insert user: %w", err)
	}
	userID, err = res.LastInsertId()
	if err != nil {
		return 0, "", err
	}

	if !s.skipEmailVerification {
		token, err = randomToken()
		if err != nil {
			return 0, "", err
		}
		if _, err := tx.Exec(`INSERT INTO email_verifications (token, user_id, expires_at) VALUES (?, ?, ?)`,
			token, userID, time.Now().Add(24*time.Hour)); err != nil {
			return 0, "", fmt.Errorf("insert verification token: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, "", err
	}
	return userID, token, nil
}

// VerifyEmail consumes a verification token and marks the owning user as
// verified. Tokens are single-use and expire after 24h.
func (s *Service) VerifyEmail(token string) error {
	var userID int64
	var expiresAt time.Time
	err := s.db.QueryRow(`SELECT user_id, expires_at FROM email_verifications WHERE token = ?`, token).
		Scan(&userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTokenInvalid
	} else if err != nil {
		return fmt.Errorf("lookup token: %w", err)
	}
	if time.Now().After(expiresAt) {
		return ErrTokenInvalid
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE users SET email_verified_at = CURRENT_TIMESTAMP WHERE id = ?`, userID); err != nil {
		return fmt.Errorf("mark verified: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM email_verifications WHERE token = ?`, token); err != nil {
		return fmt.Errorf("consume token: %w", err)
	}
	return tx.Commit()
}

const inviteTTL = 7 * 24 * time.Hour

// CreateInvite lets any existing member invite an arbitrary email address
// into their squadron. role must be "owner", "editor" or "viewer".
func (s *Service) CreateInvite(squadronID, invitedBy int64, email, role string) (token string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if addr, perr := mail.ParseAddress(email); perr != nil || addr.Address != email {
		return "", errors.New("invalid email address")
	}
	switch role {
	case "owner", "editor", "viewer":
	default:
		return "", fmt.Errorf("invalid role %q", role)
	}

	token, err = randomToken()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`INSERT INTO invites (token, squadron_id, email, role, invited_by, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		token, squadronID, email, role, invitedBy, time.Now().Add(inviteTTL))
	if err != nil {
		return "", fmt.Errorf("insert invite: %w", err)
	}
	return token, nil
}

func (s *Service) GetInvite(token string) (*Invite, error) {
	var inv Invite
	var accepted sql.NullTime
	err := s.db.QueryRow(`SELECT token, squadron_id, email, role, expires_at, accepted_at FROM invites WHERE token = ?`, token).
		Scan(&inv.Token, &inv.SquadronID, &inv.Email, &inv.Role, &inv.ExpiresAt, &accepted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteInvalid
	} else if err != nil {
		return nil, fmt.Errorf("lookup invite: %w", err)
	}
	inv.Accepted = accepted.Valid
	if inv.Accepted || time.Now().After(inv.ExpiresAt) {
		return nil, ErrInviteInvalid
	}
	return &inv, nil
}

// AcceptInvite creates the invited user, already verified, and consumes the
// invite. The account is tied to the invite's squadron and role.
func (s *Service) AcceptInvite(token, password string) (userID int64, squadronID int64, err error) {
	inv, err := s.GetInvite(token)
	if err != nil {
		return 0, 0, err
	}
	if err := checkPassword(password); err != nil {
		return 0, 0, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, 0, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO users (squadron_id, email, password_hash, role, email_verified_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		inv.SquadronID, inv.Email, string(hash), inv.Role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, 0, ErrEmailTaken
		}
		return 0, 0, fmt.Errorf("insert invited user: %w", err)
	}
	userID, err = res.LastInsertId()
	if err != nil {
		return 0, 0, err
	}

	if _, err := tx.Exec(`UPDATE invites SET accepted_at = CURRENT_TIMESTAMP WHERE token = ?`, token); err != nil {
		return 0, 0, fmt.Errorf("mark invite accepted: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return userID, inv.SquadronID, nil
}

// PendingInvites lists unaccepted, unexpired invites for a squadron's admin UI.
func (s *Service) PendingInvites(squadronID int64) ([]Invite, error) {
	rows, err := s.db.Query(`
		SELECT token, squadron_id, email, role, expires_at FROM invites
		WHERE squadron_id = ? AND accepted_at IS NULL AND expires_at > CURRENT_TIMESTAMP
		ORDER BY created_at DESC`, squadronID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Invite
	for rows.Next() {
		var inv Invite
		if err := rows.Scan(&inv.Token, &inv.SquadronID, &inv.Email, &inv.Role, &inv.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// RevokeInvite deletes a pending invite, scoped to the squadron so one
// squadron can't revoke another's.
func (s *Service) RevokeInvite(squadronID int64, token string) error {
	_, err := s.db.Exec(`DELETE FROM invites WHERE token = ? AND squadron_id = ?`, token, squadronID)
	return err
}

// ListMembers returns every user belonging to a squadron, for the squadron
// settings page.
func (s *Service) ListMembers(squadronID int64) ([]User, error) {
	rows, err := s.db.Query(`SELECT id, squadron_id, email, display_name, role, email_verified_at FROM users WHERE squadron_id = ? ORDER BY role, email`, squadronID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.SquadronID, &u.Email, &u.DisplayName, &u.Role, &u.EmailVerifiedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Authenticate checks email+password and requires a verified address.
func (s *Service) Authenticate(email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u User
	var hash string
	err := s.db.QueryRow(`SELECT id, squadron_id, email, display_name, role, password_hash, email_verified_at FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.SquadronID, &u.Email, &u.DisplayName, &u.Role, &hash, &u.EmailVerifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Burn the same bcrypt time as a real check so response latency
		// doesn't reveal which emails have accounts.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, ErrInvalidCredentials
	} else if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	if !u.Verified() {
		return nil, ErrEmailNotVerified
	}
	return &u, nil
}

// dummyHash is a valid bcrypt hash (of a random value) used to equalise
// Authenticate's timing for unknown emails.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("timing-equaliser"), bcrypt.DefaultCost)

const sessionTTL = 30 * 24 * time.Hour

func (s *Service) CreateSession(userID int64) (token string, expiresAt time.Time, err error) {
	token, err = randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = time.Now().Add(sessionTTL)
	_, err = s.db.Exec(`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`, token, userID, expiresAt)
	return token, expiresAt, err
}

func (s *Service) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// UserBySession resolves a session cookie value to its user, or nil if the
// session is missing/expired.
func (s *Service) UserBySession(token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	var u User
	var expiresAt time.Time
	err := s.db.QueryRow(`
		SELECT u.id, u.squadron_id, u.email, u.display_name, u.role, u.email_verified_at, se.expires_at
		FROM sessions se JOIN users u ON u.id = se.user_id
		WHERE se.token = ?`, token).
		Scan(&u.ID, &u.SquadronID, &u.Email, &u.DisplayName, &u.Role, &u.EmailVerifiedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("lookup session: %w", err)
	}
	if time.Now().After(expiresAt) {
		_ = s.DeleteSession(token)
		return nil, nil
	}
	return &u, nil
}

// CSRFToken derives a token from the session cookie value so a page refresh
// doesn't need a DB write — it's valid exactly as long as the session is.
func (s *Service) CSRFToken(sessionToken string) string {
	mac := hmac.New(sha256.New, s.sessionKey)
	mac.Write([]byte("csrf:" + sessionToken))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) ValidCSRF(sessionToken, submitted string) bool {
	expected := s.CSRFToken(sessionToken)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(submitted)) == 1
}

// checkPassword enforces 12-72 bytes. bcrypt silently ignores everything past
// 72 bytes (and newer x/crypto refuses outright), so a longer password would
// either be weaker than it looks or fail with an internal error message.
func checkPassword(password string) error {
	switch {
	case len(password) < 12:
		return errors.New("password must be at least 12 characters")
	case len(password) > 72:
		return errors.New("password must be at most 72 characters")
	}
	return nil
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
