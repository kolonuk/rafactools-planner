package programme

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// What changed between one kept version of a programme and the one before it, in
// words: "Changed uniform for 17 Nov from Blue to Working dress". The tracker's
// entries are counted, never named: the history is read by the whole squadron.

// maxChangeLines is how many lines of change one version shows before "and N more".
const maxChangeLines = 8

func shortDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("2 Jan")
}

func shortRange(a, b string) string {
	if a == b {
		return shortDate(a)
	}
	return shortDate(a) + " to " + shortDate(b)
}

func quote(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "nothing"
	}
	if len([]rune(s)) > 40 {
		s = string([]rune(s)[:40]) + "…"
	}
	return "“" + s + "”"
}

// describeChanges lists what differs between two snapshots. uniform maps colour keys to
// labels and classes are the squadron's classification names.
func describeChanges(prev, cur snapshot, uniform map[string]string, classes []string) []string {
	var out []string
	label := func(k string) string {
		if k == "" {
			return "none"
		}
		if l := uniform[k]; l != "" {
			return l
		}
		return k
	}
	prevDays := map[string]snapDay{}
	for _, d := range prev.Days {
		prevDays[d.Date] = d
	}
	for _, d := range cur.Days {
		p, had := prevDays[d.Date]
		if !had {
			continue
		}
		date := shortDate(d.Date)
		switch {
		case d.IsParade && !p.IsParade:
			out = append(out, "Made "+date+" a parade night")
		case !d.IsParade && p.IsParade:
			out = append(out, "Made "+date+" not a parade night")
		}
		if d.UniformKey != p.UniformKey {
			out = append(out, fmt.Sprintf("Changed uniform for %s from %s to %s", date, label(p.UniformKey), label(d.UniformKey)))
		}
		if d.Halves != p.Halves && d.IsParade {
			if d.Halves > p.Halves {
				out = append(out, "Added a second part on "+date)
			} else {
				out = append(out, "Removed the second part on "+date)
			}
		}
		if strings.TrimSpace(d.Notes) != strings.TrimSpace(p.Notes) {
			out = append(out, "Changed the notes on "+date)
		}
		old := map[string][]string{}
		for _, r := range p.Rows {
			old[fmt.Sprintf("%d/%s", r.Half, r.RowType)] = r.Texts
		}
		for _, r := range d.Rows {
			before := old[fmt.Sprintf("%d/%s", r.Half, r.RowType)]
			for i, text := range r.Texts {
				was := ""
				if i < len(before) {
					was = before[i]
				}
				if strings.TrimSpace(was) == strings.TrimSpace(text) {
					continue
				}
				class := fmt.Sprintf("class %d", i+1)
				if i < len(classes) {
					class = classes[i]
				}
				what := "activity"
				if r.RowType == "instructor" {
					what = "instructor"
				}
				part := ""
				if r.Half > 1 {
					part = fmt.Sprintf(", part %d", r.Half)
				}
				switch {
				case strings.TrimSpace(was) == "":
					out = append(out, fmt.Sprintf("Set %s on %s (%s%s) to %s", what, date, class, part, quote(text)))
				case strings.TrimSpace(text) == "":
					out = append(out, fmt.Sprintf("Cleared %s on %s (%s%s), was %s", what, date, class, part, quote(was)))
				default:
					out = append(out, fmt.Sprintf("Changed %s on %s (%s%s) from %s to %s", what, date, class, part, quote(was), quote(text)))
				}
			}
		}
	}

	out = append(out, diffSet(prev.Closures, cur.Closures,
		func(c snapClosure) string { return c.Start + "|" + c.End },
		func(c snapClosure) string { return c.Message },
		func(c snapClosure) string { return "closure " + shortRange(c.Start, c.End) })...)
	out = append(out, diffSet(prev.WeekendNotes, cur.WeekendNotes,
		func(w snapWeekend) string { return w.Date + "|" + w.Text },
		func(w snapWeekend) string { return w.Text },
		func(w snapWeekend) string { return "weekend note for " + shortDate(w.Date) + " " + quote(w.Text) })...)

	prevCats := map[string]snapCategory{}
	for _, c := range prev.Requirements {
		prevCats[c.Name] = c
	}
	seen := map[string]bool{}
	for _, c := range cur.Requirements {
		seen[c.Name] = true
		p, had := prevCats[c.Name]
		switch {
		case !had:
			out = append(out, "Added the tracker category "+quote(c.Name))
			continue
		case p.Due != c.Due:
			out = append(out, "Changed the due label of "+quote(c.Name))
		}
		pe, ce := entrySet(p), entrySet(c)
		added, removed := 0, 0
		for k := range ce {
			if !pe[k] {
				added++
			}
		}
		for k := range pe {
			if !ce[k] {
				removed++
			}
		}
		if added > 0 || removed > 0 {
			out = append(out, fmt.Sprintf("Changed the entries in %s (%d added, %d removed)", quote(c.Name), added, removed))
		}
	}
	for _, c := range prev.Requirements {
		if !seen[c.Name] {
			out = append(out, "Removed the tracker category "+quote(c.Name))
		}
	}
	return out
}

func entrySet(c snapCategory) map[string]bool {
	m := map[string]bool{}
	for _, e := range c.Entries {
		m[e.Cadet+"\x00"+e.Note] = true
	}
	return m
}

// diffSet reports items added to and removed from a list. An item is the same if key
// matches; if the key matches but the text differs it is a change.
func diffSet[T any](prev, cur []T, key, text func(T) string, name func(T) string) []string {
	var out []string
	pm := map[string]T{}
	for _, p := range prev {
		pm[key(p)] = p
	}
	cm := map[string]bool{}
	for _, c := range cur {
		cm[key(c)] = true
		if _, ok := pm[key(c)]; !ok {
			out = append(out, "Added "+name(c))
		}
	}
	var gone []string
	for k, p := range pm {
		if !cm[k] {
			gone = append(gone, "Removed "+name(p))
		}
	}
	sort.Strings(gone)
	return append(out, gone...)
}

func capLines(lines []string) []string {
	if len(lines) <= maxChangeLines {
		return lines
	}
	rest := len(lines) - maxChangeLines
	more := fmt.Sprintf("and %d more change", rest)
	if rest != 1 {
		more += "s"
	}
	return append(lines[:maxChangeLines:maxChangeLines], more)
}

// HistoryChanges describes each kept version of a programme by what it changed from
// the version before it, keyed by version number. The first version (the starting
// point) has nothing to compare with and so no entry.
func (s *Store) HistoryChanges(squadronID, programmeID int64) (map[int][]string, error) {
	sq, err := s.GetSquadron(squadronID)
	if err != nil {
		return nil, err
	}
	if _, err := s.GetFullProgramme(squadronID, programmeID); err != nil {
		return nil, err
	}
	uniform := map[string]string{}
	for _, c := range sq.UniformColours {
		uniform[c.Key] = c.Label
	}
	rows, err := s.db.Query(`SELECT version, snapshot FROM programme_versions WHERE programme_id = ? ORDER BY version`, programmeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int][]string{}
	var prev *snapshot
	for rows.Next() {
		var v int
		var raw string
		if err := rows.Scan(&v, &raw); err != nil {
			return nil, err
		}
		var cur snapshot
		if err := json.Unmarshal([]byte(raw), &cur); err != nil {
			prev = nil
			continue
		}
		if prev != nil {
			if lines := describeChanges(*prev, cur, uniform, sq.Classifications); len(lines) > 0 {
				out[v] = capLines(lines)
			}
		}
		c := cur
		prev = &c
	}
	return out, rows.Err()
}
