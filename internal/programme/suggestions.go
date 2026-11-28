package programme

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Activity suggestions. A squadron's owner makes a public link; anyone with it (cadets,
// by the QR code on the printed programme) can type the name of an activity they would
// like. Editors see the suggestions and can make one a real activity of the squadron's.
// Only the name is collected, and nothing about the person.

const (
	maxSuggestionName = maxActivityTitle
	// maxPendingSuggestions stops a public form from filling the database.
	maxPendingSuggestions = 200
)

// Suggestion is one suggested activity.
type Suggestion struct {
	ID            int64
	Name          string
	CreatedAt     time.Time
	Added         bool   // an editor has made it an activity
	ActivityID    int64  // the squadron's own activity it became (0 if none: it matched a built-in one, or was removed)
	ActivityTitle string // that activity's title now, or when it was added if it has gone
}

// SuggestSettings is a squadron's suggestion link and whether its QR code is on the PDF.
type SuggestSettings struct {
	Token    string // empty: no link
	QROnPDF  bool
	Pending  int // suggestions not yet made activities
	HasToken bool
}

// GetSuggestSettings returns them.
func (s *Store) GetSuggestSettings(squadronID int64) (SuggestSettings, error) {
	var out SuggestSettings
	var on int
	err := s.db.QueryRow(`SELECT suggest_token, suggest_qr_on_pdf FROM squadrons WHERE id = ?`, squadronID).Scan(&out.Token, &on)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	} else if err != nil {
		return out, err
	}
	out.QROnPDF = on != 0 && out.Token != ""
	out.HasToken = out.Token != ""
	err = s.db.QueryRow(`SELECT COUNT(*) FROM activity_suggestions WHERE squadron_id = ? AND activity_title = ''`, squadronID).Scan(&out.Pending)
	return out, err
}

// NewSuggestToken makes the squadron's suggestion link, or a new one: the old link stops
// working at once.
func (s *Store) NewSuggestToken(squadronID int64) (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("make suggestion token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	res, err := s.db.Exec(`UPDATE squadrons SET suggest_token = ? WHERE id = ?`, token, squadronID)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrNotFound
	}
	return token, nil
}

// DisableSuggestions turns the link off (and so the QR code on the PDF).
func (s *Store) DisableSuggestions(squadronID int64) error {
	return mustChange(s.db.Exec(`UPDATE squadrons SET suggest_token = '', suggest_qr_on_pdf = 0 WHERE id = ?`, squadronID))
}

// SetSuggestQROnPDF chooses whether the QR code is printed on the PDF. It needs a link.
func (s *Store) SetSuggestQROnPDF(squadronID int64, on bool) error {
	set, err := s.GetSuggestSettings(squadronID)
	if err != nil {
		return err
	}
	if on && !set.HasToken {
		return fmt.Errorf("%w: make the suggestion link first", ErrInvalidInput)
	}
	_, err = s.db.Exec(`UPDATE squadrons SET suggest_qr_on_pdf = ? WHERE id = ?`, boolToInt(on), squadronID)
	return err
}

// SquadronBySuggestToken finds the squadron a suggestion link belongs to.
func (s *Store) SquadronBySuggestToken(token string) (*Squadron, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM squadrons WHERE suggest_token = ?`, token).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return s.GetSquadron(id)
}

// AddSuggestion records a suggestion from the public form. A name that is already waiting,
// or is already an activity, is accepted without adding anything (the person is thanked all
// the same). It reports whether it was new.
func (s *Store) AddSuggestion(squadronID int64, name string) (bool, error) {
	name, err := cleanField("the activity", name, maxSuggestionName)
	if err != nil {
		return false, err
	}
	if len([]rune(name)) < 2 {
		return false, fmt.Errorf("%w: type the name of the activity", ErrInvalidInput)
	}
	if isDefaultActivity(name) {
		return false, nil
	}
	custom, err := s.ListCustomActivities(squadronID)
	if err != nil {
		return false, err
	}
	for _, c := range custom {
		if sameActivity(c.Title, name) {
			return false, nil
		}
	}
	rows, err := s.db.Query(`SELECT name FROM activity_suggestions WHERE squadron_id = ? AND activity_title = ''`, squadronID)
	if err != nil {
		return false, err
	}
	pending := 0
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return false, err
		}
		pending++
		if sameActivity(n, name) {
			rows.Close()
			return false, nil
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}
	if pending >= maxPendingSuggestions {
		return false, fmt.Errorf("%w: there are a lot of suggestions waiting; try again later", ErrInvalidInput)
	}
	if _, err := s.db.Exec(`INSERT INTO activity_suggestions (squadron_id, name) VALUES (?, ?)`, squadronID, name); err != nil {
		return false, err
	}
	return true, nil
}

// ListSuggestions returns the squadron's suggestions, those still waiting first and then
// the ones already made activities, each newest first. An activity's title is its current one.
func (s *Store) ListSuggestions(squadronID int64) ([]Suggestion, error) {
	rows, err := s.db.Query(`SELECT g.id, g.name, g.created_at, COALESCE(a.id, 0), COALESCE(a.title, g.activity_title), g.activity_title
		FROM activity_suggestions g LEFT JOIN squadron_activities a ON a.id = g.activity_id
		WHERE g.squadron_id = ? ORDER BY (g.activity_title <> ''), g.created_at DESC, g.id DESC`, squadronID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Suggestion
	for rows.Next() {
		var g Suggestion
		var stored string
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt, &g.ActivityID, &g.ActivityTitle, &stored); err != nil {
			return nil, err
		}
		g.Added = stored != ""
		out = append(out, g)
	}
	return out, rows.Err()
}

// MakeSuggestionActivity makes a waiting suggestion one of the squadron's activities, by
// its name. If the name is already a built-in activity or one of the squadron's own, the
// suggestion is linked to that instead of adding a copy. It returns the suggestion as it is
// afterwards.
func (s *Store) MakeSuggestionActivity(squadronID, id int64) (Suggestion, error) {
	var name, done string
	err := s.db.QueryRow(`SELECT name, activity_title FROM activity_suggestions WHERE id = ? AND squadron_id = ?`, id, squadronID).Scan(&name, &done)
	if errors.Is(err, sql.ErrNoRows) {
		return Suggestion{}, ErrNotFound
	} else if err != nil {
		return Suggestion{}, err
	}
	if done != "" {
		return Suggestion{}, fmt.Errorf("%w: that suggestion is already an activity", ErrInvalidInput)
	}
	var activityID int64
	title := name
	switch addErr := s.AddActivity(squadronID, name); {
	case addErr == nil:
	case errors.Is(addErr, ErrInvalidInput) && isDefaultActivity(name):
		for _, d := range defaultActivities {
			if sameActivity(d, name) {
				title = d
			}
		}
	case errors.Is(addErr, ErrInvalidInput) && strings.Contains(addErr.Error(), "already in your list"):
	default:
		return Suggestion{}, addErr
	}
	if !isDefaultActivity(name) {
		custom, err := s.ListCustomActivities(squadronID)
		if err != nil {
			return Suggestion{}, err
		}
		for _, c := range custom {
			if sameActivity(c.Title, name) {
				activityID, title = c.ID, c.Title
			}
		}
	}
	if _, err := s.db.Exec(`UPDATE activity_suggestions SET activity_id = NULLIF(?, 0), activity_title = ? WHERE id = ? AND squadron_id = ?`, activityID, title, id, squadronID); err != nil {
		return Suggestion{}, err
	}
	return Suggestion{ID: id, Name: name, Added: true, ActivityID: activityID, ActivityTitle: title}, nil
}

// DeleteSuggestion removes a suggestion (without touching any activity made from it).
func (s *Store) DeleteSuggestion(squadronID, id int64) error {
	return mustChange(s.db.Exec(`DELETE FROM activity_suggestions WHERE id = ? AND squadron_id = ?`, id, squadronID))
}
