package programme

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Time off for instructors and other staff. It is shown to every member of the
// squadron and warns in the planning grid, and it never goes into the PDF, the
// PNG or the calendar feed.

const (
	maxAbsenceNote  = 200
	maxAbsenceDays  = 366
	maxAbsencesEach = 1000 // per squadron
)

// Absence is one stretch of time off for one person.
type Absence struct {
	ID        int64
	Name      string // as the person appears in the Instructor boxes
	UserID    int64  // the person's login, or 0 if they have none
	Start     string // yyyy-mm-dd
	End       string
	Note      string
	CreatedBy int64
}

func cleanAbsence(name, start, end, note string) (string, string, string, string, error) {
	name, err := cleanField("the name", name, maxInstructorName+maxInstructorRank+1)
	if err != nil {
		return "", "", "", "", err
	}
	if name == "" {
		return "", "", "", "", fmt.Errorf("%w: choose who is away", ErrInvalidInput)
	}
	if note, err = cleanField("the note", note, maxAbsenceNote); err != nil {
		return "", "", "", "", err
	}
	st, err := parseISODate(start)
	if err != nil {
		return "", "", "", "", err
	}
	en, err := parseISODate(end)
	if err != nil {
		return "", "", "", "", err
	}
	if en.Before(st) {
		return "", "", "", "", fmt.Errorf("%w: the time off ends before it starts", ErrInvalidInput)
	}
	if en.Sub(st) > maxAbsenceDays*24*time.Hour {
		return "", "", "", "", fmt.Errorf("%w: time off is longer than %d days", ErrInvalidInput, maxAbsenceDays)
	}
	return name, start, end, note, nil
}

// ListAbsences returns the squadron's time off that has not finished before
// fromDate (use "" for all of it), soonest first.
func (s *Store) ListAbsences(squadronID int64, fromDate string) ([]Absence, error) {
	rows, err := s.db.Query(`SELECT id, name, COALESCE(user_id, 0), start_date, end_date, note, COALESCE(created_by, 0)
		FROM instructor_absences WHERE squadron_id = ? AND end_date >= ? ORDER BY start_date, end_date, name COLLATE NOCASE, id`, squadronID, fromDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Absence
	for rows.Next() {
		var a Absence
		if err := rows.Scan(&a.ID, &a.Name, &a.UserID, &a.Start, &a.End, &a.Note, &a.CreatedBy); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAbsence returns one entry of the squadron's.
func (s *Store) GetAbsence(squadronID, id int64) (Absence, error) {
	var a Absence
	err := s.db.QueryRow(`SELECT id, name, COALESCE(user_id, 0), start_date, end_date, note, COALESCE(created_by, 0)
		FROM instructor_absences WHERE id = ? AND squadron_id = ?`, id, squadronID).
		Scan(&a.ID, &a.Name, &a.UserID, &a.Start, &a.End, &a.Note, &a.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Absence{}, ErrNotFound
	}
	return a, err
}

// AddAbsence records time off. userID is the person's login (0 if none) and
// createdBy whoever typed it in.
func (s *Store) AddAbsence(squadronID int64, name string, userID, createdBy int64, start, end, note string) error {
	name, start, end, note, err := cleanAbsence(name, start, end, note)
	if err != nil {
		return err
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM instructor_absences WHERE squadron_id = ?`, squadronID).Scan(&n); err != nil {
		return err
	}
	if n >= maxAbsencesEach {
		return fmt.Errorf("%w: there are already %d entries; delete old ones first", ErrInvalidInput, maxAbsencesEach)
	}
	_, err = s.db.Exec(`INSERT INTO instructor_absences (squadron_id, name, user_id, start_date, end_date, note, created_by)
		VALUES (?, ?, NULLIF(?, 0), ?, ?, ?, NULLIF(?, 0))`, squadronID, name, userID, start, end, note, createdBy)
	return err
}

// UpdateAbsence changes the dates and note of an entry (and the name, if given).
func (s *Store) UpdateAbsence(squadronID, id int64, name, start, end, note string) error {
	if strings.TrimSpace(name) == "" {
		old, err := s.GetAbsence(squadronID, id)
		if err != nil {
			return err
		}
		name = old.Name
	}
	name, start, end, note, err := cleanAbsence(name, start, end, note)
	if err != nil {
		return err
	}
	return mustChange(s.db.Exec(`UPDATE instructor_absences SET name = ?, start_date = ?, end_date = ?, note = ? WHERE id = ? AND squadron_id = ?`,
		name, start, end, note, id, squadronID))
}

// DeleteAbsence removes an entry.
func (s *Store) DeleteAbsence(squadronID, id int64) error {
	return mustChange(s.db.Exec(`DELETE FROM instructor_absences WHERE id = ? AND squadron_id = ?`, id, squadronID))
}

// nameWords lower-cases text and reduces it to single-space separated words, so
// "Fg Off  Smith," and "fg off smith" compare equal.
func nameWords(s string) string {
	f := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	return strings.Join(f, " ")
}

// AwayMatcher finds, for the text in an Instructor box and a date, who in it is
// away. A name matches when it appears in the text as whole words, so "Smith"
// does not match "Smithson". aliases adds other ways a person is written
// (key: the name as stored; value: others, e.g. without the rank).
type AwayMatcher struct {
	absences []Absence
	words    [][]string // per absence: the forms of the name, as words
}

// NewAwayMatcher builds a matcher over the entries.
func NewAwayMatcher(absences []Absence, aliases map[string][]string) *AwayMatcher {
	m := &AwayMatcher{absences: absences}
	for _, a := range absences {
		forms := []string{nameWords(a.Name)}
		for _, alt := range aliases[a.Name] {
			forms = append(forms, nameWords(alt))
		}
		m.words = append(m.words, forms)
	}
	return m
}

// Away returns the entries covering date for people named in text.
func (m *AwayMatcher) Away(text, date string) []Absence {
	if m == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	padded := " " + nameWords(text) + " "
	var out []Absence
	for i, a := range m.absences {
		if date < a.Start || date > a.End {
			continue
		}
		for _, f := range m.words[i] {
			if f != "" && strings.Contains(padded, " "+f+" ") {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// Describe is a short phrase for a warning: "Smith: away 3 Nov to 10 Nov (leave)".
func (a Absence) Describe() string {
	st, _ := time.Parse("2006-01-02", a.Start)
	en, _ := time.Parse("2006-01-02", a.End)
	span := st.Format("2 Jan")
	if a.End != a.Start {
		span += " to " + en.Format("2 Jan")
	}
	out := a.Name + " is away " + span
	if a.Note != "" {
		out += " (" + a.Note + ")"
	}
	return out
}
