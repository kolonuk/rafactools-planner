package programme

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Version history. After every save, reset or restore a copy of the whole
// programme is kept with who did it, when, and what they did, and any copy can
// be put back. Months are identified by date, not by row id, so a copy can be
// put back onto the same month however its rows have changed since.

// maxVersionsKept is how many copies a programme keeps; the oldest go first.
const maxVersionsKept = 200

// ErrVersionNotFound is returned for a version number the programme does not have.
var ErrVersionNotFound = errors.New("that version doesn't exist")

type snapRow struct {
	Half    int      `json:"half"`
	RowType string   `json:"row_type"`
	Texts   []string `json:"texts"`
}

type snapDay struct {
	Date       string    `json:"date"`
	IsParade   bool      `json:"is_parade"`
	UniformKey string    `json:"uniform_key"`
	Halves     int       `json:"halves"`
	Notes      string    `json:"notes"`
	Rows       []snapRow `json:"rows"`
}

type snapClosure struct {
	Start   string `json:"start"`
	End     string `json:"end"`
	Message string `json:"message"`
}

type snapWeekend struct {
	Date  string `json:"date"`
	Text  string `json:"text"`
	Order int    `json:"order"`
}

type snapEntry struct {
	Cadet string `json:"cadet"`
	Note  string `json:"note"`
	Order int    `json:"order"`
}

type snapCategory struct {
	Name    string      `json:"name"`
	Due     string      `json:"due"`
	Order   int         `json:"order"`
	Entries []snapEntry `json:"entries"`
}

type snapshot struct {
	Days         []snapDay      `json:"days"`
	Closures     []snapClosure  `json:"closures"`
	WeekendNotes []snapWeekend  `json:"weekend_notes"`
	Requirements []snapCategory `json:"requirements"`
}

func takeSnapshot(fp *FullProgramme) snapshot {
	var s snapshot
	for _, d := range fp.Days {
		sd := snapDay{Date: d.Date, IsParade: d.IsParade, UniformKey: d.UniformKey, Halves: d.Halves, Notes: d.Notes}
		for _, r := range d.Rows {
			sd.Rows = append(sd.Rows, snapRow{Half: r.Half, RowType: r.RowType, Texts: r.Texts})
		}
		s.Days = append(s.Days, sd)
	}
	for _, c := range fp.Closures {
		s.Closures = append(s.Closures, snapClosure{Start: c.StartDate, End: c.EndDate, Message: c.Message})
	}
	for _, w := range fp.WeekendNotes {
		s.WeekendNotes = append(s.WeekendNotes, snapWeekend{Date: w.Date, Text: w.Text, Order: w.SortOrder})
	}
	for _, c := range fp.Requirements {
		sc := snapCategory{Name: c.Name, Due: c.DueLabel, Order: c.SortOrder}
		for _, e := range c.Entries {
			sc.Entries = append(sc.Entries, snapEntry{Cadet: e.CadetName, Note: e.Note, Order: e.SortOrder})
		}
		s.Requirements = append(s.Requirements, sc)
	}
	return s
}

// VersionInfo is one line of a programme's history.
type VersionInfo struct {
	Version   int
	At        time.Time
	UserEmail string
	Action    string
}

func hashSnapshot(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// RecordVersion keeps a copy of the programme as it is now. userEmail is who
// made the change ("" for the system); action says what they did. A copy that
// is identical to the newest one is not kept again (saving without changing
// anything leaves no entry). It reports the version number and whether a new
// one was made.
func (s *Store) RecordVersion(squadronID, programmeID, userID int64, userEmail, action string) (version int, made bool, err error) {
	fp, err := s.GetFullProgramme(squadronID, programmeID)
	if err != nil {
		return 0, false, err
	}
	data, err := json.Marshal(takeSnapshot(fp))
	if err != nil {
		return 0, false, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	var latest int
	var latestSnap string
	err = tx.QueryRow(`SELECT version, snapshot FROM programme_versions WHERE programme_id = ? ORDER BY version DESC LIMIT 1`, programmeID).Scan(&latest, &latestSnap)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	if err == nil && hashSnapshot([]byte(latestSnap)) == hashSnapshot(data) {
		return latest, false, nil
	}
	version = latest + 1
	var uid any
	if userID != 0 {
		uid = userID
	}
	if _, err := tx.Exec(`INSERT INTO programme_versions (programme_id, version, created_at, user_id, user_email, action, snapshot) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		programmeID, version, time.Now().UTC(), uid, userEmail, action, string(data)); err != nil {
		return 0, false, err
	}
	if _, err := tx.Exec(`DELETE FROM programme_versions WHERE programme_id = ? AND version <= ?`, programmeID, version-maxVersionsKept); err != nil {
		return 0, false, err
	}
	return version, true, tx.Commit()
}

// HasVersions reports whether any copy exists yet, so the caller can keep the
// starting point before the first change is made.
func (s *Store) HasVersions(programmeID int64) bool {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM programme_versions WHERE programme_id = ?`, programmeID).Scan(&n)
	return n > 0
}

// ListVersions returns a programme's history, newest first.
func (s *Store) ListVersions(squadronID, programmeID int64) ([]VersionInfo, error) {
	if _, err := s.GetFullProgramme(squadronID, programmeID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT version, created_at, user_email, action FROM programme_versions WHERE programme_id = ? ORDER BY version DESC`, programmeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VersionInfo
	for rows.Next() {
		var v VersionInfo
		if err := rows.Scan(&v.Version, &v.At, &v.UserEmail, &v.Action); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetVersionInfo returns one history line.
func (s *Store) GetVersionInfo(squadronID, programmeID int64, version int) (VersionInfo, error) {
	if _, err := s.GetFullProgramme(squadronID, programmeID); err != nil {
		return VersionInfo{}, err
	}
	var v VersionInfo
	err := s.db.QueryRow(`SELECT version, created_at, user_email, action FROM programme_versions WHERE programme_id = ? AND version = ?`, programmeID, version).
		Scan(&v.Version, &v.At, &v.UserEmail, &v.Action)
	if errors.Is(err, sql.ErrNoRows) {
		return VersionInfo{}, ErrVersionNotFound
	}
	return v, err
}

// RestoreVersion puts the programme back to a kept copy: the nights (which are
// parade nights, uniform, halves, notes and every box), the closures, the
// weekend notes and the requirement tracker. The month and its nights stay;
// boxes are made to fit the squadron's current classifications. The caller
// records the restore as a new version, so it can itself be undone.
func (s *Store) RestoreVersion(squadronID, programmeID int64, version int) error {
	if _, err := s.GetFullProgramme(squadronID, programmeID); err != nil {
		return err
	}
	var data string
	err := s.db.QueryRow(`SELECT snapshot FROM programme_versions WHERE programme_id = ? AND version = ?`, programmeID, version).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrVersionNotFound
	}
	if err != nil {
		return err
	}
	var snap snapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return fmt.Errorf("read version %d: %w", version, err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var classJSON string
	if err := tx.QueryRow(`SELECT classifications FROM squadrons WHERE id = ?`, squadronID).Scan(&classJSON); err != nil {
		return err
	}
	var classes []string
	if err := json.Unmarshal([]byte(classJSON), &classes); err != nil {
		return fmt.Errorf("parse classifications: %w", err)
	}
	fit := func(texts []string) []string {
		out := make([]string, len(classes))
		copy(out, texts)
		return out
	}

	for _, d := range snap.Days {
		var dayID int64
		err := tx.QueryRow(`SELECT id FROM programme_days WHERE programme_id = ? AND date = ?`, programmeID, d.Date).Scan(&dayID)
		if errors.Is(err, sql.ErrNoRows) {
			continue // a date the month no longer has
		}
		if err != nil {
			return err
		}
		halves := d.Halves
		if halves != 2 {
			halves = 1
		}
		if _, err := tx.Exec(`UPDATE programme_days SET is_parade = ?, uniform_key = ?, notes = ?, halves = ? WHERE id = ?`,
			boolToInt(d.IsParade), d.UniformKey, d.Notes, halves, dayID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM programme_rows WHERE day_id = ?`, dayID); err != nil {
			return err
		}
		for _, r := range d.Rows {
			if (r.Half != 1 && r.Half != 2) || (r.RowType != "activity" && r.RowType != "instructor") {
				continue
			}
			j, err := json.Marshal(fit(r.Texts))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO programme_rows (day_id, half, row_type, texts) VALUES (?, ?, ?, ?)`, dayID, r.Half, r.RowType, string(j)); err != nil {
				return err
			}
		}
	}

	for _, q := range []string{
		`DELETE FROM programme_closures WHERE programme_id = ?`,
		`DELETE FROM programme_weekend_notes WHERE programme_id = ?`,
		`DELETE FROM programme_requirement_categories WHERE programme_id = ?`,
	} {
		if _, err := tx.Exec(q, programmeID); err != nil {
			return err
		}
	}
	for _, c := range snap.Closures {
		if _, err := tx.Exec(`INSERT INTO programme_closures (programme_id, start_date, end_date, message) VALUES (?, ?, ?, ?)`, programmeID, c.Start, c.End, c.Message); err != nil {
			return err
		}
	}
	for _, w := range snap.WeekendNotes {
		if _, err := tx.Exec(`INSERT INTO programme_weekend_notes (programme_id, date, text, sort_order) VALUES (?, ?, ?, ?)`, programmeID, w.Date, w.Text, w.Order); err != nil {
			return err
		}
	}
	for _, c := range snap.Requirements {
		res, err := tx.Exec(`INSERT INTO programme_requirement_categories (programme_id, name, due_label, sort_order) VALUES (?, ?, ?, ?)`, programmeID, c.Name, c.Due, c.Order)
		if err != nil {
			return err
		}
		catID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, e := range c.Entries {
			if _, err := tx.Exec(`INSERT INTO programme_requirement_entries (category_id, cadet_name, note, sort_order) VALUES (?, ?, ?, ?)`, catID, e.Cadet, e.Note, e.Order); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(`UPDATE programmes SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, programmeID); err != nil {
		return err
	}
	return tx.Commit()
}

// FindProgramme returns the id of a squadron's programme for a month.
func (s *Store) FindProgramme(squadronID int64, year, month int) (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM programmes WHERE squadron_id = ? AND year = ? AND month = ?`, squadronID, year, month).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}
