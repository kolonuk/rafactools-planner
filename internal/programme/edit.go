package programme

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Editing in place: change an entry that was added earlier. Each method checks
// that the entry belongs to programmeID, which callers have already checked
// against the signed-in person's squadron, so a forged id cannot reach another
// squadron's data. Text is trimmed and must not be blank (a blank box would
// leave an invisible entry that can only be deleted).

func requireText(label, v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("%w: %s must not be blank", ErrInvalidInput, label)
	}
	return v, nil
}

// UpdateClosure changes a closure's dates and message.
func (s *Store) UpdateClosure(programmeID, closureID int64, start, end, message string) error {
	st, err := parseISODate(start)
	if err != nil {
		return err
	}
	en, err := parseISODate(end)
	if err != nil {
		return err
	}
	if en.Before(st) {
		return fmt.Errorf("%w: closure ends before it starts", ErrInvalidInput)
	}
	if en.Sub(st) > maxClosureDays*24*time.Hour {
		return fmt.Errorf("%w: closure is longer than %d days", ErrInvalidInput, maxClosureDays)
	}
	if message, err = requireText("the message", message); err != nil {
		return err
	}
	return mustChange(s.db.Exec(`UPDATE programme_closures SET start_date = ?, end_date = ?, message = ? WHERE id = ? AND programme_id = ?`,
		start, end, message, closureID, programmeID))
}

// UpdateWeekendNote changes a weekend note's date and text.
func (s *Store) UpdateWeekendNote(programmeID, noteID int64, date, text string) error {
	if _, err := parseISODate(date); err != nil {
		return err
	}
	text, err := requireText("the note", text)
	if err != nil {
		return err
	}
	return mustChange(s.db.Exec(`UPDATE programme_weekend_notes SET date = ?, text = ? WHERE id = ? AND programme_id = ?`,
		date, text, noteID, programmeID))
}

// UpdateRequirementCategory renames a requirement category and changes its due label.
func (s *Store) UpdateRequirementCategory(programmeID, categoryID int64, name, dueLabel string) error {
	name, err := requireText("the category name", name)
	if err != nil {
		return err
	}
	return mustChange(s.db.Exec(`UPDATE programme_requirement_categories SET name = ?, due_label = ? WHERE id = ? AND programme_id = ?`,
		name, strings.TrimSpace(dueLabel), categoryID, programmeID))
}

// UpdateRequirementEntry changes a cadet's name and note in a category.
func (s *Store) UpdateRequirementEntry(programmeID, categoryID, entryID int64, cadetName, note string) error {
	cadetName, err := requireText("the cadet's name", cadetName)
	if err != nil {
		return err
	}
	return mustChange(s.db.Exec(`UPDATE programme_requirement_entries SET cadet_name = ?, note = ? WHERE id = ? AND category_id = ?
		AND category_id IN (SELECT id FROM programme_requirement_categories WHERE programme_id = ?)`,
		cadetName, strings.TrimSpace(note), entryID, categoryID, programmeID))
}

// mustChange turns "no row matched" into ErrNotFound.
func mustChange(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
