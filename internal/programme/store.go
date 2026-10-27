package programme

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNotFound = errors.New("not found")

// ErrInvalidInput marks user-supplied data that failed validation (as opposed
// to a database failure); handlers map it to a 400.
var ErrInvalidInput = errors.New("invalid input")

// maxClosureDays bounds a closure's span so one row can't make the PDF
// renderer expand millions of dates.
const maxClosureDays = 366

func parseISODate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q is not a yyyy-mm-dd date", ErrInvalidInput, s)
	}
	return t, nil
}

// defaultDayLabels gives each ISO weekday a short label matching the source
// PDFs (Monday "M", Thursday "T"); editable per-day afterwards since
// squadrons are free to parade any day(s) of the week.
var defaultDayLabels = map[time.Weekday]string{
	time.Monday:    "M",
	time.Tuesday:   "Tu",
	time.Wednesday: "W",
	time.Thursday:  "T",
	time.Friday:    "F",
	time.Saturday:  "Sa",
	time.Sunday:    "Su",
}

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// --- Squadron settings -----------------------------------------------------

func (s *Store) GetSquadron(squadronID int64) (*Squadron, error) {
	var sq Squadron
	var paradeJSON, classJSON, coloursJSON string
	err := s.db.QueryRow(`SELECT id, number, name, subtitle, protective_marking, parade_weekdays, classifications, uniform_colours
		FROM squadrons WHERE id = ?`, squadronID).
		Scan(&sq.ID, &sq.Number, &sq.Name, &sq.Subtitle, &sq.ProtectiveMarking, &paradeJSON, &classJSON, &coloursJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("lookup squadron: %w", err)
	}
	if err := json.Unmarshal([]byte(paradeJSON), &sq.ParadeWeekdays); err != nil {
		return nil, fmt.Errorf("parse parade_weekdays: %w", err)
	}
	if err := json.Unmarshal([]byte(classJSON), &sq.Classifications); err != nil {
		return nil, fmt.Errorf("parse classifications: %w", err)
	}
	if err := json.Unmarshal([]byte(coloursJSON), &sq.UniformColours); err != nil {
		return nil, fmt.Errorf("parse uniform_colours: %w", err)
	}
	return &sq, nil
}

func (s *Store) UpdateSquadronSettings(sq *Squadron) error {
	paradeJSON, err := json.Marshal(sq.ParadeWeekdays)
	if err != nil {
		return err
	}
	classJSON, err := json.Marshal(sq.Classifications)
	if err != nil {
		return err
	}
	coloursJSON, err := json.Marshal(sq.UniformColours)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE squadrons SET name=?, subtitle=?, protective_marking=?, parade_weekdays=?, classifications=?, uniform_colours=? WHERE id=?`,
		sq.Name, sq.Subtitle, sq.ProtectiveMarking, string(paradeJSON), string(classJSON), string(coloursJSON), sq.ID)
	return err
}

// --- Programmes --------------------------------------------------------

func (s *Store) ListProgrammes(squadronID int64) ([]Programme, error) {
	rows, err := s.db.Query(`SELECT id, year, month, status FROM programmes WHERE squadron_id = ? ORDER BY year DESC, month DESC`, squadronID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Programme
	for rows.Next() {
		var p Programme
		if err := rows.Scan(&p.ID, &p.Year, &p.Month, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreateProgramme generates one programme_days row per calendar date in the
// month, flags parade nights per the squadron's configured weekdays, and
// seeds each parade night with an empty half-1 activity/instructor row pair.
func (s *Store) CreateProgramme(squadronID int64, year, month int, createdBy int64) (*Programme, error) {
	sq, err := s.GetSquadron(squadronID)
	if err != nil {
		return nil, err
	}
	paradeSet := map[time.Weekday]bool{}
	for _, wd := range sq.ParadeWeekdays {
		paradeSet[isoWeekdayToTime(wd)] = true
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO programmes (squadron_id, year, month, created_by) VALUES (?, ?, ?, ?)`,
		squadronID, year, month, createdBy)
	if err != nil {
		if isUnique(err) {
			return nil, fmt.Errorf("a programme for %04d-%02d already exists", year, month)
		}
		return nil, fmt.Errorf("insert programme: %w", err)
	}
	programmeID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	for d := first; d.Month() == first.Month(); d = d.AddDate(0, 0, 1) {
		isParade := paradeSet[d.Weekday()]
		label := ""
		if isParade {
			label = defaultDayLabels[d.Weekday()]
		}
		res, err := tx.Exec(`INSERT INTO programme_days (programme_id, date, is_parade, day_label, halves) VALUES (?, ?, ?, ?, 1)`,
			programmeID, d.Format("2006-01-02"), boolToInt(isParade), label)
		if err != nil {
			return nil, fmt.Errorf("insert day %s: %w", d.Format("2006-01-02"), err)
		}
		if isParade {
			dayID, err := res.LastInsertId()
			if err != nil {
				return nil, err
			}
			emptyTexts, _ := json.Marshal(make([]string, len(sq.Classifications)))
			for _, rt := range []string{"activity", "instructor"} {
				if _, err := tx.Exec(`INSERT INTO programme_rows (day_id, half, row_type, texts) VALUES (?, 1, ?, ?)`,
					dayID, rt, string(emptyTexts)); err != nil {
					return nil, fmt.Errorf("insert %s row: %w", rt, err)
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Programme{ID: programmeID, Year: year, Month: month, Status: "draft"}, nil
}

func (s *Store) DeleteProgramme(squadronID, programmeID int64) error {
	res, err := s.db.Exec(`DELETE FROM programmes WHERE id = ? AND squadron_id = ?`, programmeID, squadronID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetProgrammeStatus(squadronID, programmeID int64, status string) error {
	res, err := s.db.Exec(`UPDATE programmes SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND squadron_id = ?`,
		status, programmeID, squadronID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetFullProgramme loads everything needed to render the edit page or export
// a PDF, scoped to squadronID so one squadron can never read another's data.
func (s *Store) GetFullProgramme(squadronID, programmeID int64) (*FullProgramme, error) {
	var fp FullProgramme
	err := s.db.QueryRow(`SELECT id, year, month, status FROM programmes WHERE id = ? AND squadron_id = ?`, programmeID, squadronID).
		Scan(&fp.ID, &fp.Year, &fp.Month, &fp.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("lookup programme: %w", err)
	}

	dayRows, err := s.db.Query(`SELECT id, date, is_parade, day_label, uniform_key, halves, notes FROM programme_days WHERE programme_id = ? ORDER BY date`, programmeID)
	if err != nil {
		return nil, err
	}
	defer dayRows.Close()

	dayIdx := map[int64]int{}
	for dayRows.Next() {
		var d Day
		var isParade int
		if err := dayRows.Scan(&d.ID, &d.Date, &isParade, &d.DayLabel, &d.UniformKey, &d.Halves, &d.Notes); err != nil {
			return nil, err
		}
		d.IsParade = isParade != 0
		dayIdx[d.ID] = len(fp.Days)
		fp.Days = append(fp.Days, d)
	}
	if err := dayRows.Err(); err != nil {
		return nil, err
	}

	rowRows, err := s.db.Query(`SELECT r.id, r.day_id, r.half, r.row_type, r.texts
		FROM programme_rows r JOIN programme_days d ON d.id = r.day_id
		WHERE d.programme_id = ? ORDER BY r.day_id, r.half, r.row_type`, programmeID)
	if err != nil {
		return nil, err
	}
	defer rowRows.Close()

	for rowRows.Next() {
		var r Row
		var textsJSON string
		if err := rowRows.Scan(&r.ID, &r.DayID, &r.Half, &r.RowType, &textsJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(textsJSON), &r.Texts); err != nil {
			return nil, fmt.Errorf("parse row %d texts: %w", r.ID, err)
		}
		if idx, ok := dayIdx[r.DayID]; ok {
			fp.Days[idx].Rows = append(fp.Days[idx].Rows, r)
		}
	}
	if err := rowRows.Err(); err != nil {
		return nil, err
	}

	fp.Closures, err = s.listClosures(programmeID)
	if err != nil {
		return nil, err
	}
	fp.WeekendNotes, err = s.listWeekendNotes(programmeID)
	if err != nil {
		return nil, err
	}
	fp.Requirements, err = s.listRequirements(programmeID)
	if err != nil {
		return nil, err
	}
	return &fp, nil
}

// --- Bulk grid save ------------------------------------------------------

type RowInput struct {
	ID    int64
	Texts []string
}

type DayInput struct {
	ID         int64
	IsParade   bool
	DayLabel   string
	UniformKey string
	Notes      string
	Rows       []RowInput
}

// SaveDaysAndRows writes back the whole edit-grid form in one transaction.
// Day and row IDs must already belong to programmeID/squadronID — the caller
// loads them from GetFullProgramme first, so stray IDs can't cross tenants.
func (s *Store) SaveDaysAndRows(squadronID, programmeID int64, days []DayInput) error {
	// Ownership check: programme must belong to this squadron.
	var owner int64
	if err := s.db.QueryRow(`SELECT squadron_id FROM programmes WHERE id = ?`, programmeID).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if owner != squadronID {
		return ErrNotFound
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, d := range days {
		res, err := tx.Exec(`UPDATE programme_days SET is_parade=?, day_label=?, uniform_key=?, notes=?
			WHERE id = ? AND programme_id = ?`,
			boolToInt(d.IsParade), d.DayLabel, d.UniformKey, d.Notes, d.ID, programmeID)
		if err != nil {
			return fmt.Errorf("update day %d: %w", d.ID, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("day %d does not belong to programme %d", d.ID, programmeID)
		}
		if d.IsParade {
			if err := seedParadeRows(tx, squadronID, d.ID); err != nil {
				return err
			}
		}

		for _, r := range d.Rows {
			textsJSON, err := json.Marshal(r.Texts)
			if err != nil {
				return err
			}
			res, err := tx.Exec(`UPDATE programme_rows SET texts = ? WHERE id = ? AND day_id = ?`, string(textsJSON), r.ID, d.ID)
			if err != nil {
				return fmt.Errorf("update row %d: %w", r.ID, err)
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return fmt.Errorf("row %d does not belong to day %d", r.ID, d.ID)
			}
		}
	}

	if _, err := tx.Exec(`UPDATE programmes SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, programmeID); err != nil {
		return err
	}
	return tx.Commit()
}

// seedParadeRows gives a parade night its half-1 activity and instructor
// rows if it has none — the case when a night is ticked as a parade night
// after the programme was generated. Existing rows are never touched, so
// unticking and re-ticking keeps what was typed.
func seedParadeRows(tx *sql.Tx, squadronID, dayID int64) error {
	var have int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM programme_rows WHERE day_id = ?`, dayID).Scan(&have); err != nil {
		return err
	}
	if have > 0 {
		return nil
	}
	var classJSON string
	if err := tx.QueryRow(`SELECT classifications FROM squadrons WHERE id = ?`, squadronID).Scan(&classJSON); err != nil {
		return err
	}
	var classes []string
	if err := json.Unmarshal([]byte(classJSON), &classes); err != nil {
		return fmt.Errorf("parse classifications: %w", err)
	}
	empty, _ := json.Marshal(make([]string, len(classes)))
	for _, rt := range []string{"activity", "instructor"} {
		if _, err := tx.Exec(`INSERT INTO programme_rows (day_id, half, row_type, texts) VALUES (?, 1, ?, ?)`, dayID, rt, string(empty)); err != nil {
			return fmt.Errorf("seed %s row: %w", rt, err)
		}
	}
	return nil
}

// AddHalf turns on the second half for a parade day, seeding empty
// activity/instructor rows for it if they don't already exist.
func (s *Store) AddHalf(squadronID, programmeID, dayID int64, classificationCount int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE programme_days SET halves = 2
		WHERE id = ? AND programme_id = ? AND programme_id IN (SELECT id FROM programmes WHERE squadron_id = ?)`,
		dayID, programmeID, squadronID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	emptyTexts, _ := json.Marshal(make([]string, classificationCount))
	for _, rt := range []string{"activity", "instructor"} {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO programme_rows (day_id, half, row_type, texts) VALUES (?, 2, ?, ?)`,
			dayID, rt, string(emptyTexts)); err != nil {
			return fmt.Errorf("insert half-2 %s row: %w", rt, err)
		}
	}
	return tx.Commit()
}

// --- Closures ------------------------------------------------------------

func (s *Store) listClosures(programmeID int64) ([]Closure, error) {
	rows, err := s.db.Query(`SELECT id, start_date, end_date, message FROM programme_closures WHERE programme_id = ? ORDER BY start_date`, programmeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Closure
	for rows.Next() {
		var c Closure
		if err := rows.Scan(&c.ID, &c.StartDate, &c.EndDate, &c.Message); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) AddClosure(programmeID int64, start, end, message string) error {
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
	_, err = s.db.Exec(`INSERT INTO programme_closures (programme_id, start_date, end_date, message) VALUES (?, ?, ?, ?)`,
		programmeID, start, end, message)
	return err
}

func (s *Store) DeleteClosure(programmeID, closureID int64) error {
	_, err := s.db.Exec(`DELETE FROM programme_closures WHERE id = ? AND programme_id = ?`, closureID, programmeID)
	return err
}

// --- Weekend notes ---------------------------------------------------------

func (s *Store) listWeekendNotes(programmeID int64) ([]WeekendNote, error) {
	rows, err := s.db.Query(`SELECT id, date, text, sort_order FROM programme_weekend_notes WHERE programme_id = ? ORDER BY date, sort_order`, programmeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WeekendNote
	for rows.Next() {
		var w WeekendNote
		if err := rows.Scan(&w.ID, &w.Date, &w.Text, &w.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) AddWeekendNote(programmeID int64, date, text string) error {
	if _, err := parseISODate(date); err != nil {
		return err
	}
	var maxOrder sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(sort_order) FROM programme_weekend_notes WHERE programme_id = ? AND date = ?`, programmeID, date).Scan(&maxOrder); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO programme_weekend_notes (programme_id, date, text, sort_order) VALUES (?, ?, ?, ?)`,
		programmeID, date, text, maxOrder.Int64+1)
	return err
}

func (s *Store) DeleteWeekendNote(programmeID, noteID int64) error {
	_, err := s.db.Exec(`DELETE FROM programme_weekend_notes WHERE id = ? AND programme_id = ?`, noteID, programmeID)
	return err
}

// --- Footer requirement tracker --------------------------------------------

func (s *Store) listRequirements(programmeID int64) ([]ReqCategory, error) {
	catRows, err := s.db.Query(`SELECT id, name, due_label, sort_order FROM programme_requirement_categories WHERE programme_id = ? ORDER BY sort_order, id`, programmeID)
	if err != nil {
		return nil, err
	}
	defer catRows.Close()

	var cats []ReqCategory
	catIdx := map[int64]int{}
	for catRows.Next() {
		var c ReqCategory
		if err := catRows.Scan(&c.ID, &c.Name, &c.DueLabel, &c.SortOrder); err != nil {
			return nil, err
		}
		catIdx[c.ID] = len(cats)
		cats = append(cats, c)
	}
	if err := catRows.Err(); err != nil {
		return nil, err
	}
	if len(cats) == 0 {
		return cats, nil
	}

	entryRows, err := s.db.Query(`SELECT e.id, e.category_id, e.cadet_name, e.note, e.sort_order
		FROM programme_requirement_entries e JOIN programme_requirement_categories c ON c.id = e.category_id
		WHERE c.programme_id = ? ORDER BY e.category_id, e.sort_order, e.id`, programmeID)
	if err != nil {
		return nil, err
	}
	defer entryRows.Close()

	for entryRows.Next() {
		var e ReqEntry
		var catID int64
		if err := entryRows.Scan(&e.ID, &catID, &e.CadetName, &e.Note, &e.SortOrder); err != nil {
			return nil, err
		}
		if idx, ok := catIdx[catID]; ok {
			cats[idx].Entries = append(cats[idx].Entries, e)
		}
	}
	return cats, entryRows.Err()
}

func (s *Store) AddRequirementCategory(programmeID int64, name, dueLabel string) (int64, error) {
	var maxOrder sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(sort_order) FROM programme_requirement_categories WHERE programme_id = ?`, programmeID).Scan(&maxOrder); err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO programme_requirement_categories (programme_id, name, due_label, sort_order) VALUES (?, ?, ?, ?)`,
		programmeID, name, dueLabel, maxOrder.Int64+1)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DeleteRequirementCategory(programmeID, categoryID int64) error {
	_, err := s.db.Exec(`DELETE FROM programme_requirement_categories WHERE id = ? AND programme_id = ?`, categoryID, programmeID)
	return err
}

// AddRequirementEntry adds an entry to a category, but only if that category
// belongs to programmeID — callers verify programmeID against the user's
// squadron, so this check is what stops a forged catID reaching another
// squadron's data.
func (s *Store) AddRequirementEntry(programmeID, categoryID int64, cadetName, note string) error {
	var owned int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM programme_requirement_categories WHERE id = ? AND programme_id = ?`, categoryID, programmeID).Scan(&owned); err != nil {
		return err
	}
	if owned == 0 {
		return sql.ErrNoRows
	}
	var maxOrder sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(sort_order) FROM programme_requirement_entries WHERE category_id = ?`, categoryID).Scan(&maxOrder); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO programme_requirement_entries (category_id, cadet_name, note, sort_order) VALUES (?, ?, ?, ?)`,
		categoryID, cadetName, note, maxOrder.Int64+1)
	return err
}

func (s *Store) DeleteRequirementEntry(programmeID, categoryID, entryID int64) error {
	_, err := s.db.Exec(`DELETE FROM programme_requirement_entries WHERE id = ? AND category_id = ?
		AND category_id IN (SELECT id FROM programme_requirement_categories WHERE programme_id = ?)`, entryID, categoryID, programmeID)
	return err
}

// --- helpers ---------------------------------------------------------------

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isoWeekdayToTime converts an ISO weekday int (1=Mon..7=Sun) to time.Weekday
// (0=Sun..6=Sat).
func isoWeekdayToTime(iso int) time.Weekday {
	if iso == 7 {
		return time.Sunday
	}
	return time.Weekday(iso)
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE")
}
