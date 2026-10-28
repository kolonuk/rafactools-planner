package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// masterSessionTTL is how long the master admin stays signed in. The account
// can read every squadron, so it asks for the password again far more often
// than the 30 days other people get.
const masterSessionTTL = 8 * time.Hour

const maxUserAgent = 200

// ErrNoSuchSession is returned when a session id does not belong to the user.
var ErrNoSuchSession = errors.New("no such session")

// SessionInfo describes one place a person is signed in. ID is a one-way hash
// of the cookie value, safe to put in a page: it identifies a session without
// being able to impersonate it.
type SessionInfo struct {
	ID        string
	Device    string
	CreatedAt time.Time
	ExpiresAt time.Time
	Current   bool
}

func sessionID(token string) string {
	sum := sha256.Sum256([]byte("session-id:" + token))
	return hex.EncodeToString(sum[:8])
}

// CreateSessionFor is CreateSession that also records the browser, and gives
// the master admin the shorter lifetime.
func (s *Service) CreateSessionFor(userID int64, userAgent string) (token string, expiresAt time.Time, err error) {
	token, err = randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	var master bool
	if err := s.db.QueryRow(`SELECT is_master_admin FROM users WHERE id = ?`, userID).Scan(&master); err != nil {
		return "", time.Time{}, fmt.Errorf("lookup user: %w", err)
	}
	ttl := sessionTTL
	if master {
		ttl = masterSessionTTL
	}
	expiresAt = time.Now().Add(ttl)
	if len(userAgent) > maxUserAgent {
		userAgent = userAgent[:maxUserAgent]
	}
	_, err = s.db.Exec(`INSERT INTO sessions (token, user_id, expires_at, user_agent) VALUES (?, ?, ?, ?)`,
		token, userID, expiresAt, strings.ToValidUTF8(userAgent, ""))
	return token, expiresAt, err
}

// ListSessions returns the user's live sessions, newest first. currentToken
// marks the one making the request.
func (s *Service) ListSessions(userID int64, currentToken string) ([]SessionInfo, error) {
	rows, err := s.db.Query(`SELECT token, user_agent, created_at, expires_at FROM sessions
		WHERE user_id = ? AND expires_at > ? ORDER BY created_at DESC, rowid DESC`, userID, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionInfo
	for rows.Next() {
		var tok, ua string
		var si SessionInfo
		if err := rows.Scan(&tok, &ua, &si.CreatedAt, &si.ExpiresAt); err != nil {
			return nil, err
		}
		si.ID = sessionID(tok)
		si.Device = DeviceLabel(ua)
		si.Current = subtle.ConstantTimeCompare([]byte(tok), []byte(currentToken)) == 1
		out = append(out, si)
	}
	return out, rows.Err()
}

// RevokeSession signs one of the user's own sessions out. The id must belong
// to userID, so one person can never end another's session. Ending the current
// session is refused here: that is what Log out is for.
func (s *Service) RevokeSession(userID int64, id, currentToken string) error {
	rows, err := s.db.Query(`SELECT token FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	var target string
	for rows.Next() {
		var tok string
		if err := rows.Scan(&tok); err != nil {
			rows.Close()
			return err
		}
		if subtle.ConstantTimeCompare([]byte(sessionID(tok)), []byte(id)) == 1 {
			target = tok
		}
	}
	rows.Close()
	if target == "" || target == currentToken {
		return ErrNoSuchSession
	}
	_, err = s.db.Exec(`DELETE FROM sessions WHERE token = ? AND user_id = ?`, target, userID)
	return err
}

// RevokeOtherSessions signs the user out everywhere except the current
// browser and reports how many sessions ended.
func (s *Service) RevokeOtherSessions(userID int64, currentToken string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ? AND token != ?`, userID, currentToken)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeviceLabel turns a User-Agent header into a short, coarse description such
// as "Firefox on Linux". Only a fixed set of words can come out, so nothing
// the client sent is ever shown back as text.
func DeviceLabel(ua string) string {
	browser := "Unknown browser"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"), strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/"), strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/"), strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	system := ""
	switch {
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		system = "iOS"
	case strings.Contains(ua, "Android"):
		system = "Android"
	case strings.Contains(ua, "Windows"):
		system = "Windows"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		system = "macOS"
	case strings.Contains(ua, "CrOS"):
		system = "ChromeOS"
	case strings.Contains(ua, "Linux"), strings.Contains(ua, "X11"):
		system = "Linux"
	}
	if system == "" {
		return browser
	}
	return browser + " on " + system
}
