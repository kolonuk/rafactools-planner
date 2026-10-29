package programme

import (
	"fmt"
	"strings"
	"unicode"
)

// Instructor is one entry in a squadron's list of staff.
type Instructor struct {
	ID   int64
	Name string
	Rank string // optional, e.g. "Fg Off"
}

// Display is how the instructor appears in the grid: "Fg Off Smith".
func (i Instructor) Display() string {
	if i.Rank == "" {
		return i.Name
	}
	return i.Rank + " " + i.Name
}

const (
	maxInstructorName = 80
	maxInstructorRank = 40
	// maxInstructors keeps one squadron from growing the list without bound.
	maxInstructors = 200
)

// cleanField trims a user-typed value and rejects control characters and
// line breaks (the value ends up in an HTML attribute and in grid text).
func cleanField(label, v string, max int) (string, error) {
	v = strings.Join(strings.Fields(v), " ") // collapses runs of whitespace, trims
	if len([]rune(v)) > max {
		return "", fmt.Errorf("%w: %s is longer than %d characters", ErrInvalidInput, label, max)
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: %s contains a control character", ErrInvalidInput, label)
		}
	}
	return v, nil
}

// ListInstructors returns the squadron's instructors, alphabetically.
func (s *Store) ListInstructors(squadronID int64) ([]Instructor, error) {
	rows, err := s.db.Query(`SELECT id, name, rank FROM instructors WHERE squadron_id = ? ORDER BY name COLLATE NOCASE, rank COLLATE NOCASE`, squadronID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Instructor
	for rows.Next() {
		var i Instructor
		if err := rows.Scan(&i.ID, &i.Name, &i.Rank); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// AddInstructor adds a name (required) and rank (optional) to the squadron's
// list. The same name and rank cannot be added twice.
func (s *Store) AddInstructor(squadronID int64, name, rank string) error {
	name, err := cleanField("the name", name, maxInstructorName)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("%w: enter a name", ErrInvalidInput)
	}
	rank, err = cleanField("the rank", rank, maxInstructorRank)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM instructors WHERE squadron_id = ?`, squadronID).Scan(&n); err != nil {
		return err
	}
	if n >= maxInstructors {
		return fmt.Errorf("%w: the list is full (%d people)", ErrInvalidInput, maxInstructors)
	}
	if _, err := tx.Exec(`INSERT INTO instructors (squadron_id, name, rank) VALUES (?, ?, ?)`, squadronID, name, rank); err != nil {
		if isUnique(err) {
			return fmt.Errorf("%w: %s is already in the list", ErrInvalidInput, Instructor{Name: name, Rank: rank}.Display())
		}
		return err
	}
	return tx.Commit()
}

// DeleteInstructor removes an entry, only if it belongs to the squadron.
func (s *Store) DeleteInstructor(squadronID, id int64) error {
	res, err := s.db.Exec(`DELETE FROM instructors WHERE id = ? AND squadron_id = ?`, id, squadronID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetInstructor returns one entry, only if it belongs to the squadron.
func (s *Store) GetInstructor(squadronID, id int64) (Instructor, error) {
	var i Instructor
	err := s.db.QueryRow(`SELECT id, name, rank FROM instructors WHERE id = ? AND squadron_id = ?`, id, squadronID).Scan(&i.ID, &i.Name, &i.Rank)
	if err != nil {
		return Instructor{}, ErrNotFound
	}
	return i, nil
}

// UpdateInstructor changes an entry's name and rank, with the same rules as
// AddInstructor. Text already typed into programmes is not touched: it is free
// text and stays as it was written.
func (s *Store) UpdateInstructor(squadronID, id int64, name, rank string) error {
	name, err := cleanField("the name", name, maxInstructorName)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("%w: enter a name", ErrInvalidInput)
	}
	rank, err = cleanField("the rank", rank, maxInstructorRank)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE instructors SET name = ?, rank = ? WHERE id = ? AND squadron_id = ?`, name, rank, id, squadronID)
	if err != nil {
		if isUnique(err) {
			return fmt.Errorf("%w: %s is already in the list", ErrInvalidInput, Instructor{Name: name, Rank: rank}.Display())
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
