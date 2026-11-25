package programme

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

// The activity list offered in the grid's Activity boxes: a built-in list that
// ships with the app, plus each squadron's own. It is only a list of
// suggestions: the boxes stay free text.

//go:embed activities_default.json
var defaultActivitiesJSON []byte

var defaultActivities = func() []string {
	var out []string
	if err := json.Unmarshal(defaultActivitiesJSON, &out); err != nil {
		panic("activities_default.json: " + err.Error())
	}
	return out
}()

const (
	maxActivityTitle = 120
	maxActivities    = 300 // per squadron
)

// DefaultActivities is the built-in list, in its own order.
func DefaultActivities() []string { return append([]string(nil), defaultActivities...) }

// sameActivity is how two titles are compared: ignoring case and surrounding or repeated spaces.
func sameActivity(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

func isDefaultActivity(title string) bool {
	for _, d := range defaultActivities {
		if sameActivity(d, title) {
			return true
		}
	}
	return false
}

// CustomActivity is an activity a squadron added.
type CustomActivity struct {
	ID    int64
	Title string
}

// ListCustomActivities returns the squadron's own activities, alphabetically.
func (s *Store) ListCustomActivities(squadronID int64) ([]CustomActivity, error) {
	rows, err := s.db.Query(`SELECT id, title FROM squadron_activities WHERE squadron_id = ? ORDER BY lower(title), id`, squadronID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomActivity
	for rows.Next() {
		var a CustomActivity
		if err := rows.Scan(&a.ID, &a.Title); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ActivityOptions is the built-in list followed by the squadron's own, for the
// grid's suggestions.
func (s *Store) ActivityOptions(squadronID int64) ([]string, error) {
	custom, err := s.ListCustomActivities(squadronID)
	if err != nil {
		return nil, err
	}
	out := DefaultActivities()
	for _, c := range custom {
		out = append(out, c.Title)
	}
	return out, nil
}

func cleanActivity(title string) (string, error) {
	title, err := cleanField("the activity", title, maxActivityTitle)
	if err != nil {
		return "", err
	}
	if title == "" {
		return "", fmt.Errorf("%w: the activity must not be blank", ErrInvalidInput)
	}
	if isDefaultActivity(title) {
		return "", fmt.Errorf("%w: %q is already in the built-in list", ErrInvalidInput, title)
	}
	return title, nil
}

// AddActivity adds an activity to the squadron's own list.
func (s *Store) AddActivity(squadronID int64, title string) error {
	title, err := cleanActivity(title)
	if err != nil {
		return err
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM squadron_activities WHERE squadron_id = ?`, squadronID).Scan(&n); err != nil {
		return err
	}
	if n >= maxActivities {
		return fmt.Errorf("%w: the list already has %d activities", ErrInvalidInput, maxActivities)
	}
	if _, err := s.db.Exec(`INSERT INTO squadron_activities (squadron_id, title) VALUES (?, ?)`, squadronID, title); err != nil {
		if isUnique(err) {
			return fmt.Errorf("%w: %q is already in your list", ErrInvalidInput, title)
		}
		return err
	}
	return nil
}

// UpdateActivity renames one of the squadron's own activities.
func (s *Store) UpdateActivity(squadronID, id int64, title string) error {
	title, err := cleanActivity(title)
	if err != nil {
		return err
	}
	err = mustChange(s.db.Exec(`UPDATE squadron_activities SET title = ? WHERE id = ? AND squadron_id = ?`, title, id, squadronID))
	if err != nil && isUnique(err) {
		return fmt.Errorf("%w: %q is already in your list", ErrInvalidInput, title)
	}
	return err
}

// DeleteActivity removes one of the squadron's own activities.
func (s *Store) DeleteActivity(squadronID, id int64) error {
	return mustChange(s.db.Exec(`DELETE FROM squadron_activities WHERE id = ? AND squadron_id = ?`, id, squadronID))
}

// RemoveDuplicatesOfDefaults deletes, for every squadron, own activities that
// match a built-in one (ignoring case and surrounding spaces), since the built-in
// entry already covers them. It logs what it removed and returns how many.
func (s *Store) RemoveDuplicatesOfDefaults() (int, error) {
	rows, err := s.db.Query(`SELECT id, squadron_id, title FROM squadron_activities`)
	if err != nil {
		return 0, err
	}
	type dup struct {
		id, sq int64
		title  string
	}
	var dups []dup
	for rows.Next() {
		var d dup
		if err := rows.Scan(&d.id, &d.sq, &d.title); err != nil {
			rows.Close()
			return 0, err
		}
		if isDefaultActivity(d.title) {
			dups = append(dups, d)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	sort.Slice(dups, func(i, j int) bool { return dups[i].id < dups[j].id })
	for _, d := range dups {
		if _, err := s.db.Exec(`DELETE FROM squadron_activities WHERE id = ?`, d.id); err != nil && err != sql.ErrNoRows {
			return 0, err
		}
		log.Printf("activities: removed squadron %d's own %q: it is in the built-in list", d.sq, d.title)
	}
	return len(dups), nil
}
