package programme

import "strings"

// Shape is the size and fullness of a programme with none of its text: enough
// for a problem report to show how big and how complete the month was (and so
// whether a layout or limit is involved) without any activity, instructor or
// cadet names leaving the site.
type Shape struct {
	Year, Month          int
	Days                 int `json:"days"`
	ParadeNights         int `json:"parade_nights"`
	SecondHalves         int `json:"second_halves"`
	Cells                int `json:"cells"`
	FilledCells          int `json:"filled_cells"`
	LongestCellRunes     int `json:"longest_cell_chars"`
	NightsWithNotes      int `json:"nights_with_notes"`
	Closures             int `json:"closures"`
	WeekendNotes         int `json:"weekend_notes"`
	RequirementCategory  int `json:"requirement_categories"`
	RequirementEntries   int `json:"requirement_entries"`
	DistinctUniformKeys  int `json:"distinct_uniform_keys"`
	ActivityRowsPerNight int `json:"max_rows_per_night"`
}

// Shape counts what a programme contains. It reads no text except to measure
// it, and returns numbers only.
func (fp *FullProgramme) Shape() Shape {
	s := Shape{Year: fp.Year, Month: fp.Month, Days: len(fp.Days), Closures: len(fp.Closures),
		WeekendNotes: len(fp.WeekendNotes), RequirementCategory: len(fp.Requirements)}
	keys := map[string]bool{}
	for _, d := range fp.Days {
		if d.IsParade {
			s.ParadeNights++
			if d.UniformKey != "" {
				keys[d.UniformKey] = true
			}
		}
		if d.Halves == 2 {
			s.SecondHalves++
		}
		if strings.TrimSpace(d.Notes) != "" {
			s.NightsWithNotes++
		}
		if len(d.Rows) > s.ActivityRowsPerNight {
			s.ActivityRowsPerNight = len(d.Rows)
		}
		for _, r := range d.Rows {
			for _, c := range r.Texts {
				s.Cells++
				if strings.TrimSpace(c) != "" {
					s.FilledCells++
				}
				if n := len([]rune(c)); n > s.LongestCellRunes {
					s.LongestCellRunes = n
				}
			}
		}
	}
	for _, c := range fp.Requirements {
		s.RequirementEntries += len(c.Entries)
	}
	s.DistinctUniformKeys = len(keys)
	return s
}

// SchemaInfo reports how many database migrations have been applied and the
// name of the newest, for problem reports.
func (s *Store) SchemaInfo() (applied int, newest string) {
	_ = s.db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(name), '') FROM schema_migrations`).Scan(&applied, &newest)
	return applied, newest
}

// Ping checks the database answers, for the health check.
func (s *Store) Ping() error {
	var one int
	return s.db.QueryRow(`SELECT 1`).Scan(&one)
}
