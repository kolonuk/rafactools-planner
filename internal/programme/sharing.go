package programme

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// ErrExpired is returned for a public link that exists but has run out.
var ErrExpired = errors.New("link expired")

// ShareLifetime is how long a public link works once made or renewed.
const ShareLifetime = 1 // months

// Local is the time zone parade times are given in.
var Local = func() *time.Location {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// validClock reports whether s is a 24-hour HH:MM time.
func validClock(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	t, err := time.Parse("15:04", s)
	return err == nil && t.Format("15:04") == s
}

// parseSQLTime reads a timestamp as SQLite stores it ("2006-01-02 15:04:05",
// UTC), or RFC 3339 as some drivers return it. A bad value is the zero time.
func parseSQLTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05Z"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// Share is a squadron's public link: Token is empty until one is made.
type Share struct {
	Token     string
	ExpiresAt time.Time
}

// Expired reports whether the link has run out at the given time.
func (sh Share) Expired(at time.Time) bool { return !sh.ExpiresAt.IsZero() && !at.Before(sh.ExpiresAt) }

// GetShare returns the squadron's current public link, if it has one.
func (s *Store) GetShare(squadronID int64) (Share, error) {
	var token string
	var exp sql.NullString
	err := s.db.QueryRow(`SELECT share_token, share_expires_at FROM squadrons WHERE id = ?`, squadronID).Scan(&token, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return Share{}, ErrNotFound
	} else if err != nil {
		return Share{}, err
	}
	sh := Share{Token: token}
	if exp.Valid {
		sh.ExpiresAt = parseSQLTime(exp.String)
	}
	return sh, nil
}

// NewShare makes (or replaces) the squadron's public link: a fresh random
// token that lasts ShareLifetime months from now. The old link stops working
// at once.
func (s *Store) NewShare(squadronID int64) (Share, error) {
	raw := make([]byte, 24) // 192 bits
	if _, err := rand.Read(raw); err != nil {
		return Share{}, fmt.Errorf("make share token: %w", err)
	}
	sh := Share{
		Token:     base64.RawURLEncoding.EncodeToString(raw),
		ExpiresAt: Now().UTC().AddDate(0, ShareLifetime, 0).Truncate(time.Second),
	}
	res, err := s.db.Exec(`UPDATE squadrons SET share_token = ?, share_expires_at = ? WHERE id = ?`,
		sh.Token, sh.ExpiresAt.Format("2006-01-02 15:04:05"), squadronID)
	if err != nil {
		return Share{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Share{}, ErrNotFound
	}
	return sh, nil
}

// SquadronByShareToken finds the squadron a public link belongs to. An unknown
// token is ErrNotFound; a known one past its expiry is ErrExpired.
func (s *Store) SquadronByShareToken(token string) (*Squadron, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	var id int64
	var exp sql.NullString
	err := s.db.QueryRow(`SELECT id, share_expires_at FROM squadrons WHERE share_token = ?`, token).Scan(&id, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if exp.Valid {
		if (Share{Token: token, ExpiresAt: parseSQLTime(exp.String)}).Expired(Now().UTC()) {
			return nil, ErrExpired
		}
	}
	return s.GetSquadron(id)
}

// SetPublished turns a month's public visibility on or off.
func (s *Store) SetPublished(squadronID, programmeID int64, on bool) error {
	res, err := s.db.Exec(`UPDATE programmes SET published = ? WHERE id = ? AND squadron_id = ?`, boolToInt(on), programmeID, squadronID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PublishedProgrammes returns the squadron's published months in date order,
// in full.
func (s *Store) PublishedProgrammes(squadronID int64) ([]*FullProgramme, error) {
	rows, err := s.db.Query(`SELECT id FROM programmes WHERE squadron_id = ? AND published = 1 ORDER BY year, month`, squadronID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []*FullProgramme
	for _, id := range ids {
		fp, err := s.GetFullProgramme(squadronID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, fp)
	}
	return out, nil
}

// PublishedProgramme returns one published month of a squadron, or ErrNotFound
// when the month does not exist or is not published.
func (s *Store) PublishedProgramme(squadronID int64, year, month int) (*FullProgramme, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM programmes WHERE squadron_id = ? AND year = ? AND month = ? AND published = 1`, squadronID, year, month).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return s.GetFullProgramme(squadronID, id)
}
