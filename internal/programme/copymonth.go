package programme

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Copying an earlier month into another. The nights are lined up by parade night:
// the first parade night of the source goes on the first of the destination, the
// second on the second, and so on, so every date and weekday comes out right for
// the destination. A night under a closure is not a parade night here. Only the
// uniform, the activities and the instructors are copied: notes, weekend notes,
// closures and the requirement tracker are left alone.

// openParadeNights are a month's parade nights that no closure covers, in date order.
func openParadeNights(fp *FullProgramme) []Day {
	var out []Day
	for _, d := range fp.Days {
		if !d.IsParade {
			continue
		}
		if _, closed := ClosureOn(d.Date, fp.Closures); closed {
			continue
		}
		out = append(out, d)
	}
	return out
}

func rowHasText(texts []string) bool {
	for _, t := range texts {
		if strings.TrimSpace(t) != "" {
			return true
		}
	}
	return false
}

// CopyableNights counts the parade nights of fp that have a uniform or any
// activity or instructor, which a copy into it would replace.
func CopyableNights(fp *FullProgramme) int {
	n := 0
	for _, d := range openParadeNights(fp) {
		used := d.UniformKey != ""
		for _, r := range d.Rows {
			used = used || rowHasText(r.Texts)
		}
		if used {
			n++
		}
	}
	return n
}

// CopyPlan says what copying src into dst would do.
type CopyPlan struct {
	Copied    int // nights that get the source's entries
	Dropped   int // source nights with no night to go to (dst has fewer)
	Untouched int // destination nights left as they are (src has fewer)
	Replaced  int // destination nights that already had entries and lose them
}

// PlanCopy works out the effect of CopyMonth without changing anything.
func PlanCopy(dst, src *FullProgramme) CopyPlan {
	from, to := openParadeNights(src), openParadeNights(dst)
	p := CopyPlan{Copied: min(len(from), len(to))}
	if len(from) > len(to) {
		p.Dropped = len(from) - len(to)
	}
	if len(to) > len(from) {
		p.Untouched = len(to) - len(from)
	}
	for _, d := range to[:p.Copied] {
		used := d.UniformKey != ""
		for _, r := range d.Rows {
			used = used || rowHasText(r.Texts)
		}
		if used {
			p.Replaced++
		}
	}
	return p
}

// CopyMonth copies the entries of the squadron's month srcID into its month dstID.
func (s *Store) CopyMonth(squadronID, dstID, srcID int64) (CopyPlan, error) {
	if dstID == srcID {
		return CopyPlan{}, fmt.Errorf("%w: choose a different month to copy from", ErrInvalidInput)
	}
	dst, err := s.GetFullProgramme(squadronID, dstID)
	if err != nil {
		return CopyPlan{}, err
	}
	src, err := s.GetFullProgramme(squadronID, srcID)
	if err != nil {
		return CopyPlan{}, err
	}
	plan := PlanCopy(dst, src)
	from, to := openParadeNights(src), openParadeNights(dst)

	var classJSON string
	if err := s.db.QueryRow(`SELECT classifications FROM squadrons WHERE id = ?`, squadronID).Scan(&classJSON); err != nil {
		return CopyPlan{}, err
	}
	var classes []string
	if err := json.Unmarshal([]byte(classJSON), &classes); err != nil {
		return CopyPlan{}, fmt.Errorf("parse classifications: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return CopyPlan{}, err
	}
	defer tx.Rollback()
	for i := 0; i < plan.Copied; i++ {
		f, t := from[i], to[i]
		halves := f.Halves
		if halves < 1 {
			halves = 1
		}
		if _, err := tx.Exec(`UPDATE programme_days SET uniform_key = ?, halves = ? WHERE id = ? AND programme_id = ?`, f.UniformKey, halves, t.ID, dstID); err != nil {
			return CopyPlan{}, err
		}
		if _, err := tx.Exec(`DELETE FROM programme_rows WHERE day_id = ?`, t.ID); err != nil {
			return CopyPlan{}, err
		}
		for half := 1; half <= halves; half++ {
			for _, rt := range []string{"activity", "instructor"} {
				texts := make([]string, len(classes))
				if r, ok := f.RowByHalfType(half, rt); ok {
					copy(texts, r.Texts)
				}
				b, _ := json.Marshal(texts)
				if _, err := tx.Exec(`INSERT INTO programme_rows (day_id, half, row_type, texts) VALUES (?, ?, ?, ?)`, t.ID, half, rt, string(b)); err != nil {
					return CopyPlan{}, err
				}
			}
		}
	}
	if _, err := tx.Exec(`UPDATE programmes SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, dstID); err != nil {
		return CopyPlan{}, err
	}
	return plan, tx.Commit()
}
