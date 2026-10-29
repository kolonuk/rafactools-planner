// Package programme is the domain model for a month's training programme:
// the squadron-wide settings (classifications, uniform colours, parade
// nights) and the programmes themselves (days, activity/instructor rows,
// closures, weekend notes, and the footer requirement tracker).
package programme

type UniformColour struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Hex   string `json:"hex"`
}

type Squadron struct {
	ID                int64
	Number            string // claimed unit number, e.g. "2491" — unique across the app
	Name              string
	Subtitle          string
	ProtectiveMarking string
	WebsiteURL        string // optional, shown on the dashboard and PDF title band
	ParadeWeekdays    []int  // ISO weekday: 1=Mon .. 7=Sun
	Classifications   []string
	UniformColours    []UniformColour
}

type Programme struct {
	ID    int64
	Year  int
	Month int
}

type Row struct {
	ID      int64
	DayID   int64
	Half    int
	RowType string // "activity" | "instructor"
	Texts   []string
}

type Day struct {
	ID         int64
	Date       string // yyyy-mm-dd
	IsParade   bool
	DayLabel   string
	UniformKey string
	Halves     int
	Notes      string
	Rows       []Row // ordered: half 1 activity, half 1 instructor, half 2 activity, half 2 instructor
}

type Closure struct {
	ID        int64
	StartDate string
	EndDate   string
	Message   string
}

type WeekendNote struct {
	ID        int64
	Date      string
	Text      string
	SortOrder int
}

type ReqEntry struct {
	ID        int64
	CadetName string
	Note      string
	SortOrder int
}

type ReqCategory struct {
	ID        int64
	Name      string
	DueLabel  string
	SortOrder int
	Entries   []ReqEntry
}

// FullProgramme is everything needed to render the edit page or export a PDF.
type FullProgramme struct {
	Programme
	Days         []Day
	Closures     []Closure
	WeekendNotes []WeekendNote
	Requirements []ReqCategory
}

// RowByHalfType is a small lookup helper for templates and the PDF renderer.
func (d Day) RowByHalfType(half int, rowType string) (Row, bool) {
	for _, r := range d.Rows {
		if r.Half == half && r.RowType == rowType {
			return r, true
		}
	}
	return Row{}, false
}
