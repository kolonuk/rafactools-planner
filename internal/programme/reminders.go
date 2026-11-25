package programme

import (
	"fmt"
	"time"
)

// ReminderNight is a parade night that has blank Activity or Instructor boxes and
// has not been reminded about yet.
type ReminderNight struct {
	Date          string // yyyy-mm-dd
	ProgrammeID   int64
	BlankActivity int // empty Activity boxes
	BlankIns      int // empty Instructor boxes
}

// ReminderDue is one squadron with nights to remind about.
type ReminderDue struct {
	SquadronID   int64
	SquadronName string
	Nights       []ReminderNight
}

// DueReminders finds, for every squadron that has reminders on, the parade nights
// from today to that many days ahead that still have an empty Activity or Instructor
// box and have not been reminded about before. A night under a closure is not a
// parade night. today is a date in the squadron's own time.
func (s *Store) DueReminders(today time.Time) ([]ReminderDue, error) {
	rows, err := s.db.Query(`SELECT id, name, reminder_days FROM squadrons WHERE reminder_days > 0 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	type sq struct {
		id   int64
		name string
		days int
	}
	var sqs []sq
	for rows.Next() {
		var q sq
		if err := rows.Scan(&q.id, &q.name, &q.days); err != nil {
			rows.Close()
			return nil, err
		}
		sqs = append(sqs, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	from := today.Format("2006-01-02")
	var out []ReminderDue
	for _, q := range sqs {
		to := today.AddDate(0, 0, q.days).Format("2006-01-02")
		progs, err := s.db.Query(`SELECT id FROM programmes WHERE squadron_id = ?
			AND (year * 12 + month) BETWEEN ? AND ?`, q.id, today.Year()*12+int(today.Month()), today.AddDate(0, 0, q.days).Year()*12+int(today.AddDate(0, 0, q.days).Month()))
		if err != nil {
			return nil, err
		}
		var ids []int64
		for progs.Next() {
			var id int64
			if err := progs.Scan(&id); err != nil {
				progs.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
		progs.Close()
		if err := progs.Err(); err != nil {
			return nil, err
		}
		due := ReminderDue{SquadronID: q.id, SquadronName: q.name}
		for _, id := range ids {
			fp, err := s.GetFullProgramme(q.id, id)
			if err != nil {
				return nil, err
			}
			for _, d := range openParadeNights(fp) {
				if d.Date < from || d.Date > to {
					continue
				}
				var sent int
				if err := s.db.QueryRow(`SELECT COUNT(*) FROM reminders_sent WHERE squadron_id = ? AND date = ?`, q.id, d.Date).Scan(&sent); err != nil {
					return nil, err
				}
				if sent > 0 {
					continue
				}
				n := ReminderNight{Date: d.Date, ProgrammeID: id}
				for _, r := range d.Rows {
					for _, t := range r.Texts {
						if rowTextBlank(t) {
							if r.RowType == "activity" {
								n.BlankActivity++
							} else {
								n.BlankIns++
							}
						}
					}
				}
				if n.BlankActivity+n.BlankIns > 0 {
					due.Nights = append(due.Nights, n)
				}
			}
		}
		if len(due.Nights) > 0 {
			sortNights(due.Nights)
			out = append(out, due)
		}
	}
	return out, nil
}

func rowTextBlank(t string) bool { return !rowHasText([]string{t}) }

func sortNights(n []ReminderNight) {
	for i := 1; i < len(n); i++ {
		for j := i; j > 0 && n[j].Date < n[j-1].Date; j-- {
			n[j], n[j-1] = n[j-1], n[j]
		}
	}
}

// MarkReminded records that the nights have been reminded about.
func (s *Store) MarkReminded(squadronID int64, dates []string) error {
	for _, d := range dates {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO reminders_sent (squadron_id, date) VALUES (?, ?)`, squadronID, d); err != nil {
			return fmt.Errorf("record reminder: %w", err)
		}
	}
	return nil
}

// ForgetOldReminders deletes the record of nights that are over.
func (s *Store) ForgetOldReminders(today time.Time) error {
	_, err := s.db.Exec(`DELETE FROM reminders_sent WHERE date < ?`, today.AddDate(0, 0, -7).Format("2006-01-02"))
	return err
}

// ShareDue is a squadron whose public link is about to stop working.
type ShareDue struct {
	SquadronID   int64
	SquadronName string
	ExpiresAt    time.Time
}

// ExpiringShares finds the squadrons whose public link ends within `within` of now,
// and whose owners have not yet been told about that expiry.
func (s *Store) ExpiringShares(now time.Time, within time.Duration) ([]ShareDue, error) {
	rows, err := s.db.Query(`SELECT id, name FROM squadrons WHERE share_token <> '' AND share_expires_at IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	type sq struct {
		id   int64
		name string
	}
	var all []sq
	for rows.Next() {
		var q sq
		if err := rows.Scan(&q.id, &q.name); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []ShareDue
	for _, q := range all {
		sh, err := s.GetShare(q.id)
		if err != nil || sh.ExpiresAt.IsZero() || !sh.ExpiresAt.After(now) || sh.ExpiresAt.After(now.Add(within)) {
			continue
		}
		var told string
		if err := s.db.QueryRow(`SELECT share_expiry_reminded FROM squadrons WHERE id = ?`, q.id).Scan(&told); err != nil {
			return nil, err
		}
		if told == sh.ExpiresAt.UTC().Format(time.RFC3339) {
			continue
		}
		out = append(out, ShareDue{q.id, q.name, sh.ExpiresAt})
	}
	return out, nil
}

// MarkShareReminded records that the owners were told about this expiry.
func (s *Store) MarkShareReminded(squadronID int64, expires time.Time) error {
	_, err := s.db.Exec(`UPDATE squadrons SET share_expiry_reminded = ? WHERE id = ?`, expires.UTC().Format(time.RFC3339), squadronID)
	return err
}
