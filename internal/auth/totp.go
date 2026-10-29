package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- RFC 6238 authenticator apps use HMAC-SHA1
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Two-step sign-in with an authenticator app (RFC 6238: HMAC-SHA1, 6 digits,
// 30 seconds), accepted a step either side of now to allow for clock drift.
// The code is the second factor after the password; it can only be used once.

const (
	totpStep    = 30
	totpDigits  = 6
	totpWindow  = 1
	totpMaxFail = 5
	totpLockFor = 15 * time.Minute
)

var (
	ErrBadCode     = errors.New("that code is wrong or has already been used")
	ErrTOTPLocked  = errors.New("too many wrong codes; try again in a few minutes")
	ErrNoTOTPSetup = errors.New("two-step sign-in has not been started")
)

var secretEnc = base32.StdEncoding.WithPadding(base32.NoPadding)

// totpFailures counts wrong codes per user in memory, so guessing a code is
// capped no matter how many addresses the guesses come from.
type totpGuard struct {
	mu    sync.Mutex
	count map[int64]int
	until map[int64]time.Time
}

func (g *totpGuard) locked(id int64, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if u, ok := g.until[id]; ok {
		if now.Before(u) {
			return true
		}
		delete(g.until, id)
		delete(g.count, id)
	}
	return false
}

func (g *totpGuard) fail(id int64, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.count == nil {
		g.count, g.until = map[int64]int{}, map[int64]time.Time{}
	}
	g.count[id]++
	if g.count[id] >= totpMaxFail {
		g.until[id] = now.Add(totpLockFor)
	}
}

func (g *totpGuard) reset(id int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.count, id)
	delete(g.until, id)
}

func totpCode(secret []byte, step int64) string {
	if step < 0 {
		step = 0
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) // #nosec G115 -- step is clamped to >= 0 above
	mac := hmac.New(sha1.New, secret)                // #nosec G401 -- RFC 6238
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, v%1000000)
}

// matchTOTP returns the step a code is valid for, or -1.
func matchTOTP(secretB32, code string, now time.Time) int64 {
	secret, err := secretEnc.DecodeString(strings.ToUpper(strings.ReplaceAll(secretB32, " ", "")))
	if err != nil {
		return -1
	}
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return -1
	}
	cur := now.Unix() / totpStep
	found := int64(-1)
	for d := int64(-totpWindow); d <= totpWindow; d++ {
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, cur+d)), []byte(code)) == 1 {
			found = cur + d
		}
	}
	return found
}

// TOTPEnabled reports whether the user has confirmed two-step sign-in.
func (s *Service) TOTPEnabled(userID int64) bool {
	var secret string
	if err := s.db.QueryRow(`SELECT totp_secret FROM users WHERE id = ?`, userID).Scan(&secret); err != nil {
		return false
	}
	return secret != ""
}

// BeginTOTP makes a new secret and keeps it pending until a code from the app
// confirms it. Starting again replaces an unconfirmed secret; a confirmed one
// has to be turned off first.
func (s *Service) BeginTOTP(userID int64) (secret, otpauthURI string, err error) {
	if s.TOTPEnabled(userID) {
		return "", "", errors.New("two-step sign-in is already on")
	}
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	secret = secretEnc.EncodeToString(raw)
	sealed, err := s.sealSecret(secret)
	if err != nil {
		return "", "", err
	}
	if _, err := s.db.Exec(`UPDATE users SET totp_pending = ? WHERE id = ?`, sealed, userID); err != nil {
		return "", "", err
	}
	return secret, s.otpauth(userID, secret), nil
}

// PendingTOTP returns the unconfirmed secret and its otpauth address, if any.
func (s *Service) PendingTOTP(userID int64) (secret, otpauthURI string) {
	if err := s.db.QueryRow(`SELECT totp_pending FROM users WHERE id = ?`, userID).Scan(&secret); err != nil || secret == "" {
		return "", ""
	}
	if secret = s.openSecret(secret); secret == "" {
		return "", ""
	}
	return secret, s.otpauth(userID, secret)
}

func (s *Service) otpauth(userID int64, secret string) string {
	var email string
	s.db.QueryRow(`SELECT email FROM users WHERE id = ?`, userID).Scan(&email)
	label := url.PathEscape("RAFACTools Planner:" + email)
	q := url.Values{"secret": {secret}, "issuer": {"RAFACTools Planner"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// ConfirmTOTP turns two-step sign-in on if the code matches the pending secret.
func (s *Service) ConfirmTOTP(userID int64, code string) error {
	if s.totp.locked(userID, time.Now()) {
		return ErrTOTPLocked
	}
	var pending string
	if err := s.db.QueryRow(`SELECT totp_pending FROM users WHERE id = ?`, userID).Scan(&pending); err != nil {
		return err
	}
	if pending == "" {
		return ErrNoTOTPSetup
	}
	step := matchTOTP(s.openSecret(pending), code, time.Now())
	if step < 0 {
		s.totp.fail(userID, time.Now())
		return ErrBadCode
	}
	s.totp.reset(userID)
	_, err := s.db.Exec(`UPDATE users SET totp_secret = ?, totp_pending = '', totp_last_step = ? WHERE id = ?`, pending, step, userID)
	return err
}

// VerifyTOTP checks the second factor at sign-in. A code works once: the step
// it belongs to is recorded, and the same or an older step is refused.
func (s *Service) VerifyTOTP(userID int64, code string) error {
	now := time.Now()
	if s.totp.locked(userID, now) {
		return ErrTOTPLocked
	}
	var secret string
	var last int64
	err := s.db.QueryRow(`SELECT totp_secret, totp_last_step FROM users WHERE id = ?`, userID).Scan(&secret, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBadCode
	} else if err != nil {
		return err
	}
	step := matchTOTP(s.openSecret(secret), code, now)
	if secret == "" || step < 0 || step <= last {
		s.totp.fail(userID, now)
		return ErrBadCode
	}
	// The conditional update is the replay guard: if two requests race with
	// the same code, only one changes a row.
	res, err := s.db.Exec(`UPDATE users SET totp_last_step = ? WHERE id = ? AND totp_last_step < ?`, step, userID, step)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.totp.fail(userID, now)
		return ErrBadCode
	}
	s.totp.reset(userID)
	return nil
}

// DisableTOTP turns two-step sign-in off. It needs the password and a current
// code, so a stolen session alone cannot remove the protection.
func (s *Service) DisableTOTP(userID int64, password, code string) error {
	var hash string
	if err := s.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return ErrWrongPassword
	}
	if err := s.VerifyTOTP(userID, code); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE users SET totp_secret = '', totp_pending = '', totp_last_step = 0 WHERE id = ?`, userID)
	return err
}

// ResetTOTPFor clears two-step sign-in for an address, for an operator who has
// lost the authenticator (see MASTER_ADMIN_RESET_2FA). It reports whether
// anything changed.
func (s *Service) ResetTOTPFor(email string) (bool, error) {
	res, err := s.db.Exec(`UPDATE users SET totp_secret = '', totp_pending = '', totp_last_step = 0
		WHERE email = ? AND totp_secret != ''`, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// TOTPCodeForTest returns the code an authenticator app would show for a
// base32 secret, offset 30-second steps from now. It exists so tests in other
// packages can act as the phone; nothing in the server calls it.
func TOTPCodeForTest(secretB32 string, now time.Time, offset int64) (string, error) {
	raw, err := secretEnc.DecodeString(secretB32)
	if err != nil {
		return "", err
	}
	return totpCode(raw, now.Unix()/totpStep+offset), nil
}
