package programme

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Bank holidays. The government publishes the dates for England and Wales,
// Scotland and Northern Ireland; a copy is kept here (see internal/holidays for
// the fetching) and every month a squadron makes gets a one-day closure for each
// bank holiday in it, so nobody plans a parade night on one.

// Regions a squadron can follow. RegionNone turns the closures off.
const (
	RegionEnglandWales = "england-and-wales"
	RegionScotland     = "scotland"
	RegionNorthernIE   = "northern-ireland"
	RegionNone         = "none"
)

// ValidRegion reports whether r is one of the choices.
func ValidRegion(r string) bool {
	switch r {
	case RegionEnglandWales, RegionScotland, RegionNorthernIE, RegionNone:
		return true
	}
	return false
}

// Holiday is one bank holiday in one region.
type Holiday struct {
	Region string
	Date   string // yyyy-mm-dd
	Title  string
}

func (s *Store) listBankHolidays() ([]Holiday, error) {
	rows, err := s.db.Query(`SELECT region, date, title FROM bank_holidays ORDER BY region, date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Holiday
	for rows.Next() {
		var h Holiday
		if err := rows.Scan(&h.Region, &h.Date, &h.Title); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ReplaceBankHolidays makes the kept list exactly hols. It reports whether that
// changed anything (a date added, removed or renamed). A list with nothing valid
// in it is refused, so a bad download cannot wipe the closures.
func (s *Store) ReplaceBankHolidays(hols []Holiday) (bool, error) {
	clean := make([]Holiday, 0, len(hols))
	for _, h := range hols {
		if !ValidRegion(h.Region) || h.Region == RegionNone || h.Title == "" {
			continue
		}
		if _, err := parseISODate(h.Date); err != nil {
			continue
		}
		clean = append(clean, h)
	}
	if len(clean) == 0 {
		return false, fmt.Errorf("%w: the bank holiday list has no usable dates", ErrInvalidInput)
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].Region != clean[j].Region {
			return clean[i].Region < clean[j].Region
		}
		return clean[i].Date < clean[j].Date
	})
	old, err := s.listBankHolidays()
	if err != nil {
		return false, err
	}
	if len(old) == len(clean) {
		same := true
		for i := range old {
			if old[i] != clean[i] {
				same = false
				break
			}
		}
		if same {
			return false, nil
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM bank_holidays`); err != nil {
		return false, err
	}
	for _, h := range clean {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO bank_holidays (region, date, title) VALUES (?, ?, ?)`, h.Region, h.Date, h.Title); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// HolidaySync says when the list was last fetched and last tried (zero if never).
func (s *Store) HolidaySync() (fetched, attempted time.Time, err error) {
	var f, a sql.NullTime
	switch err = s.db.QueryRow(`SELECT fetched_at, attempted_at FROM holiday_sync WHERE id = 1`).Scan(&f, &a); {
	case errors.Is(err, sql.ErrNoRows):
		return time.Time{}, time.Time{}, nil
	case err != nil:
		return time.Time{}, time.Time{}, err
	}
	return f.Time, a.Time, nil
}

// MarkHolidaySync records a try, and a success if ok.
func (s *Store) MarkHolidaySync(now time.Time, ok bool) error {
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO holiday_sync (id) VALUES (1)`); err != nil {
		return err
	}
	if ok {
		_, err := s.db.Exec(`UPDATE holiday_sync SET fetched_at = ?, attempted_at = ? WHERE id = 1`, now.UTC(), now.UTC())
		return err
	}
	_, err := s.db.Exec(`UPDATE holiday_sync SET attempted_at = ? WHERE id = 1`, now.UTC())
	return err
}

// SquadronHolidayRegion is the list the squadron follows.
func (s *Store) SquadronHolidayRegion(squadronID int64) (string, error) {
	var r string
	err := s.db.QueryRow(`SELECT holiday_region FROM squadrons WHERE id = ?`, squadronID).Scan(&r)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return r, err
}

// SetHolidayRegion changes the list a squadron follows and brings its months in
// line: closures from the old list go, closures from the new one come.
func (s *Store) SetHolidayRegion(squadronID int64, region string) error {
	if !ValidRegion(region) {
		return fmt.Errorf("%w: choose one of the lists", ErrInvalidInput)
	}
	res, err := s.db.Exec(`UPDATE squadrons SET holiday_region = ? WHERE id = ?`, region, squadronID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return s.applyBankHolidays(squadronID)
}

// ApplyBankHolidays brings every squadron's current and future months in line
// with the kept list. It is run after the list changes.
func (s *Store) ApplyBankHolidays() error { return s.applyBankHolidays(0) }

func (s *Store) applyBankHolidays(squadronID int64) error {
	curY, curM := CurrentMonth()
	q := `SELECT id, squadron_id, year, month FROM programmes WHERE year * 12 + month >= ?`
	args := []any{curY*12 + curM}
	if squadronID != 0 {
		q += ` AND squadron_id = ?`
		args = append(args, squadronID)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return err
	}
	type prog struct {
		id, sq int64
		y, m   int
	}
	var progs []prog
	for rows.Next() {
		var p prog
		if err := rows.Scan(&p.id, &p.sq, &p.y, &p.m); err != nil {
			rows.Close()
			return err
		}
		progs = append(progs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range progs {
		if err := applyBankHolidaysTx(tx, p.id, p.sq, p.y, p.m); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// applyBankHolidaysTx makes a month's bank-holiday closures match the list for
// its squadron's region: one-day closures, none where a closure of any kind
// already covers the day, none where a person deleted it, and none left over
// from a holiday that has gone from the list.
func applyBankHolidaysTx(tx *sql.Tx, programmeID, squadronID int64, year, month int) error {
	var region string
	if err := tx.QueryRow(`SELECT holiday_region FROM squadrons WHERE id = ?`, squadronID).Scan(&region); err != nil {
		return err
	}
	want := map[string]string{} // date to title
	if region != RegionNone {
		prefix := fmt.Sprintf("%04d-%02d-", year, month)
		rows, err := tx.Query(`SELECT date, title FROM bank_holidays WHERE region = ? AND date LIKE ? ORDER BY date`, region, prefix+"%")
		if err != nil {
			return err
		}
		for rows.Next() {
			var d, t string
			if err := rows.Scan(&d, &t); err != nil {
				rows.Close()
				return err
			}
			want[d] = t
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	// Existing closures: bank ones are kept in step, the rest only block a day.
	rows, err := tx.Query(`SELECT id, start_date, end_date, message, source FROM programme_closures WHERE programme_id = ?`, programmeID)
	if err != nil {
		return err
	}
	var all []Closure
	for rows.Next() {
		var c Closure
		if err := rows.Scan(&c.ID, &c.StartDate, &c.EndDate, &c.Message, &c.Source); err != nil {
			rows.Close()
			return err
		}
		all = append(all, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var others []Closure
	have := map[string]bool{}
	for _, c := range all {
		if c.Source != "bank" {
			others = append(others, c)
			continue
		}
		title, ok := want[c.StartDate]
		switch {
		case !ok || c.StartDate != c.EndDate:
			if _, err := tx.Exec(`DELETE FROM programme_closures WHERE id = ?`, c.ID); err != nil {
				return err
			}
		case c.Message != title:
			if _, err := tx.Exec(`UPDATE programme_closures SET message = ? WHERE id = ?`, title, c.ID); err != nil {
				return err
			}
			have[c.StartDate] = true
		default:
			have[c.StartDate] = true
		}
	}
	for date, title := range want {
		if have[date] {
			continue
		}
		if _, covered := ClosureOn(date, others); covered {
			continue
		}
		var skipped int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM programme_holiday_skips WHERE programme_id = ? AND date = ?`, programmeID, date).Scan(&skipped); err != nil {
			return err
		}
		if skipped > 0 {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO programme_closures (programme_id, start_date, end_date, message, source) VALUES (?, ?, ?, ?, 'bank')`, programmeID, date, date, title); err != nil {
			return err
		}
	}
	return nil
}

// MaxReminderDays is the furthest ahead a reminder can be set.
const MaxReminderDays = 28

// ReminderDays is how many days before a parade night with blank boxes the
// squadron's staff are emailed. Zero is off.
func (s *Store) ReminderDays(squadronID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT reminder_days FROM squadrons WHERE id = ?`, squadronID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return n, err
}

// SetReminderDays saves it.
func (s *Store) SetReminderDays(squadronID int64, days int) error {
	if days < 0 || days > MaxReminderDays {
		return fmt.Errorf("%w: reminders can be 0 (off) to %d days before", ErrInvalidInput, MaxReminderDays)
	}
	res, err := s.db.Exec(`UPDATE squadrons SET reminder_days = ? WHERE id = ?`, days, squadronID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
