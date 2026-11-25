package programme

import "sort"

// SyncFeedNights keeps the record of parade nights the calendar feed has shown. current
// holds the dates the feed shows now (yyyy-mm-dd, from today on). A night that was
// shown before and is not now (a closure, the night unticked, the month unpublished or
// deleted) is remembered as cancelled until its date has passed, so a calendar app
// that already has it can be told. A night that comes back is no longer cancelled.
// It returns the cancelled dates that are still ahead, in order.
func (s *Store) SyncFeedNights(squadronID int64, current []string, today string) ([]string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM feed_nights WHERE squadron_id = ? AND date < ?`, squadronID, today); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE feed_nights SET cancelled = 1 WHERE squadron_id = ?`, squadronID); err != nil {
		return nil, err
	}
	for _, d := range current {
		if _, err := tx.Exec(`INSERT INTO feed_nights (squadron_id, date, cancelled) VALUES (?, ?, 0)
			ON CONFLICT(squadron_id, date) DO UPDATE SET cancelled = 0`, squadronID, d); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(`SELECT date FROM feed_nights WHERE squadron_id = ? AND cancelled = 1`, squadronID)
	if err != nil {
		return nil, err
	}
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, tx.Commit()
}
