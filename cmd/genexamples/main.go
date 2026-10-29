// Command genexamples regenerates the synthetic sample PDFs under
// examples/pdfs/. These are built entirely from fabricated data — no real
// squadron, cadet, or staff names — so they're safe to commit and share
// while still showing pdfgen's layout patterns (merged classification
// cells, closures, weekend notes, the footer requirement tracker, and the
// 4- vs 5-classification template variants). See
// .claude/skills/pdf-fidelity for why these exist and what they don't yet
// cover faithfully.
//
// Run with: go run ./cmd/genexamples
package main

import (
	"log"
	"os"
	"path/filepath"

	"rafactools-planner/internal/pdfgen"
	"rafactools-planner/internal/programme"
)

func main() {
	outDir := "examples/pdfs"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	examples := []struct {
		filename string
		squadron *programme.Squadron
		fp       *programme.FullProgramme
	}{
		{"standard-month.pdf", standardSquadron(), standardMonth()},
		{"closure-month.pdf", standardSquadron(), closureMonth()},
		{"five-classification-month.pdf", fiveClassificationSquadron(), fiveClassificationMonth()},
	}

	for _, ex := range examples {
		data, err := pdfgen.Render(ex.squadron, ex.fp)
		if err != nil {
			log.Fatalf("render %s: %v", ex.filename, err)
		}
		path := filepath.Join(outDir, ex.filename)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			log.Fatalf("write %s: %v", path, err)
		}
		log.Printf("wrote %s", path)
	}
}

func standardSquadron() *programme.Squadron {
	return &programme.Squadron{
		Name:            "9999 (Sampleford) Squadron",
		Subtitle:        "Example Air Cadet Unit",
		ParadeWeekdays:  []int{1, 4}, // Mon, Thu
		Classifications: []string{"Junior Cadets", "1st Class Cadets", "Leading Cadets", "Senior Cadets"},
		UniformColours: []programme.UniformColour{
			{Key: "green", Label: "No 3 (MTP) - Greens", Hex: "#8faa6e"},
			{Key: "dark_blue", Label: "No 2 (Working Blues)", Hex: "#3b5e82"},
			{Key: "light_blue", Label: "No 2A - Wedgewoods", Hex: "#a9c2db"},
			{Key: "white", Label: "Sports / Civvies", Hex: "#ffffff"},
			{Key: "grey", Label: "Closure", Hex: "#c9c9c9"},
		},
	}
}

func fiveClassificationSquadron() *programme.Squadron {
	sq := standardSquadron()
	sq.Classifications = []string{"Junior Cadets", "1st Class", "Leading", "Senior", "Master"}
	return sq
}

// day is a small constructor to keep the example tables below readable.
func day(id int64, date string, parade bool, label, uniform, notes string, halves int, rows ...programme.Row) programme.Day {
	return programme.Day{ID: id, Date: date, IsParade: parade, DayLabel: label, UniformKey: uniform, Notes: notes, Halves: halves, Rows: rows}
}

func row(dayID int64, half int, rowType string, texts ...string) programme.Row {
	return programme.Row{DayID: dayID, Half: half, RowType: rowType, Texts: texts}
}

func standardMonth() *programme.FullProgramme {
	var days []programme.Day
	id := int64(1)
	next := func() int64 { id++; return id }

	for d := 1; d <= 30; d++ {
		date := exampleDate(d)
		switch d {
		case 2: // Thursday — shared activity, with a day-level note
			dID := next()
			days = append(days, day(dID, date, true, "T", "green", "Kit check ahead of Ex Sample Challenger", 1,
				row(dID, 1, "activity", "Drill", "Drill", "Drill", "Drill"),
				row(dID, 1, "instructor", "Staff & Cadet NCOs", "Staff & Cadet NCOs", "Staff & Cadet NCOs", "Staff & Cadet NCOs"),
			))
		case 9: // Thursday — two halves, shared across all classifications
			dID := next()
			days = append(days, day(dID, date, true, "T", "green", "", 2,
				row(dID, 1, "activity", "Radio Exercises", "Radio Exercises", "Radio Exercises", "Radio Exercises"),
				row(dID, 1, "instructor", "Example Instructor A", "Example Instructor A", "Example Instructor A", "Example Instructor A"),
				row(dID, 2, "activity", "Drill", "Drill", "Drill", "Drill"),
				row(dID, 2, "instructor", "Staff & Cadet NCOs", "Staff & Cadet NCOs", "Staff & Cadet NCOs", "Staff & Cadet NCOs"),
			))
		case 16: // Thursday — map reading
			dID := next()
			days = append(days, day(dID, date, true, "T", "green", "", 1,
				row(dID, 1, "activity", "Map Reading / Navigation Activities", "Map Reading / Navigation Activities", "Map Reading / Navigation Activities", "Map Reading / Navigation Activities"),
				row(dID, 1, "instructor", "Example Instructor B", "Example Instructor B", "Example Instructor B", "Example Instructor B"),
			))
		case 23: // Thursday — quiz night
			dID := next()
			days = append(days, day(dID, date, true, "T", "green", "", 1,
				row(dID, 1, "activity", "Quiz Night - General Knowledge", "Quiz Night - General Knowledge", "Quiz Night - General Knowledge", "Quiz Night - General Knowledge"),
				row(dID, 1, "instructor", "All Available Staff / Cadet NCOs", "All Available Staff / Cadet NCOs", "All Available Staff / Cadet NCOs", "All Available Staff / Cadet NCOs"),
			))
		case 30: // Thursday — drill
			dID := next()
			days = append(days, day(dID, date, true, "T", "green", "", 1,
				row(dID, 1, "activity", "Drill", "Drill", "Drill", "Drill"),
				row(dID, 1, "instructor", "Staff & Cadet NCOs", "Staff & Cadet NCOs", "Staff & Cadet NCOs", "Staff & Cadet NCOs"),
			))
		case 6: // Monday — mandatory briefings night
			dID := next()
			days = append(days, day(dID, date, true, "M", "dark_blue", "Mandatory Briefings (see Notes table below)", 1,
				row(dID, 1, "activity", "First Aid Refresher", "First Aid Refresher", "First Aid Refresher", "First Aid Refresher"),
				row(dID, 1, "instructor", "Example Instructor C", "Example Instructor C", "Example Instructor C", "Example Instructor C"),
			))
		case 13: // Monday — OC's inspection, two halves
			dID := next()
			days = append(days, day(dID, date, true, "M", "dark_blue", "OC's Inspection", 2,
				row(dID, 1, "activity", "Uniform Inspection", "Uniform Inspection", "Uniform Inspection", "Uniform Inspection"),
				row(dID, 1, "instructor", "Example OC", "Example OC", "Example OC", "Example OC"),
				row(dID, 2, "activity", "Drill", "Drill", "Drill", "Drill"),
				row(dID, 2, "instructor", "Staff & Cadet NCOs", "Staff & Cadet NCOs", "Staff & Cadet NCOs", "Staff & Cadet NCOs"),
			))
		case 20: // Monday — split-by-classification example, two halves
			dID := next()
			days = append(days, day(dID, date, true, "M", "light_blue", "Blue Leadership Practical Assessments (see Notes table below)", 2,
				row(dID, 1, "activity", "1st Class Training", "Principles of Flight", "Jet Engines", "Air Power"),
				row(dID, 1, "instructor", "Training Team A", "Training Team B", "Training Team C", "Training Team D"),
				row(dID, 2, "activity", "Leadership Theory", "Leadership Theory", "Leadership Practical", "Leadership Practical"),
				row(dID, 2, "instructor", "Example Instructor D", "Example Instructor D", "Example Instructor D", "Example Instructor D"),
			))
		case 27: // Monday — presentation night
			dID := next()
			days = append(days, day(dID, date, true, "M", "dark_blue", "", 1,
				row(dID, 1, "activity", "Presentation Skills Theory", "Presentation Skills Theory", "Presentation Skills Theory", "Presentation Skills Theory"),
				row(dID, 1, "instructor", "Example Instructor C", "Example Instructor C", "Example Instructor C", "Example Instructor C"),
			))
		default:
			days = append(days, day(next(), date, false, "", "", "", 1))
		}
	}

	return &programme.FullProgramme{
		Programme: programme.Programme{ID: 1, Year: exampleYear, Month: exampleMonth},
		Days:      days,
		Closures:  nil,
		WeekendNotes: []programme.WeekendNote{
			{Date: exampleDate(4), Text: "4-5 Example - Ten Tors Training Weekend"},
			{Date: exampleDate(5), Text: "5 Example - DTC(I) Assessment Day"},
			{Date: exampleDate(11), Text: "11-13 Example - DofE Gold Expedition"},
			{Date: exampleDate(18), Text: "18 Example - Air Experience Flying"},
			{Date: exampleDate(25), Text: "25 Example - Road Marching 20km"},
			{Date: exampleDate(26), Text: "26-27 Example - CRTC Summer Camp"},
		},
		Requirements: []programme.ReqCategory{
			{Name: "Mandatory Briefings", DueLabel: "(TBC - example date)", Entries: []programme.ReqEntry{
				{CadetName: "Example Cadet 1"}, {CadetName: "Example Cadet 2"}, {CadetName: "Example Cadet 3"},
			}},
			{Name: "Blue Leadership Practical Assessments", DueLabel: "(TBC - example date)", Entries: []programme.ReqEntry{
				{CadetName: "Example Cadet 4", Note: "1 x Exercise"},
				{CadetName: "Example Cadet 5", Note: "2 x Exercises"},
			}},
			{Name: "Blue DofE Sports", DueLabel: "(TBC - example date)", Entries: []programme.ReqEntry{
				{CadetName: "Example Cadet 2"}, {CadetName: "Example Cadet 6"},
			}},
			{Name: "Blue Leadership Theory", DueLabel: "(TBC - example date)", Entries: []programme.ReqEntry{
				{CadetName: "Example Cadet 1"},
			}},
		},
	}
}

func closureMonth() *programme.FullProgramme {
	fp := standardMonth()
	fp.Closures = []programme.Closure{
		{StartDate: exampleDate(15), EndDate: exampleDate(21),
			Message: "EXAMPLE STAND DOWN - HALF TERM\n\nSEE YOU BACK NEXT WEEK"},
	}
	return fp
}

func fiveClassificationMonth() *programme.FullProgramme {
	var days []programme.Day
	id := int64(100)
	next := func() int64 { id++; return id }

	for d := 1; d <= 30; d++ {
		date := exampleDate(d)
		if d == 2 || d == 9 || d == 16 || d == 23 || d == 30 {
			dID := next()
			days = append(days, day(dID, date, true, "T", "green", "", 1,
				row(dID, 1, "activity", "Sports Evening", "Sports Evening", "Sports Evening", "Sports Evening", "Sports Evening"),
				row(dID, 1, "instructor", "Available Staff", "Available Staff", "Available Staff", "Available Staff", "Available Staff"),
			))
		} else {
			days = append(days, day(next(), date, false, "", "", "", 1))
		}
	}

	return &programme.FullProgramme{
		Programme: programme.Programme{ID: 2, Year: exampleYear, Month: exampleMonth},
		Days:      days,
		WeekendNotes: []programme.WeekendNote{
			{Date: exampleDate(4), Text: "Example Course - Sample Base"},
		},
	}
}

const (
	// 2026 isn't arbitrary: the hardcoded day-of-month numbers below (2, 6, 9,
	// 13, 16, 20, 23, 27, 30) are picked to actually fall on a
	// Thursday/Monday in April 2026, which groupIntoWeeks and the weekend
	// Sat/Sun matching in pdfgen both key off the *real* calendar weekday —
	// changing this without re-checking (`date -d 2026-04-06` etc.) will
	// silently misalign the week grouping and weekend notes.
	exampleYear  = 2026
	exampleMonth = 4
)

func exampleDate(day int) string {
	// Fixed example year/month — any non-leap April works; day is 1-indexed.
	return (func() string {
		const days = "01020304050607080910111213141516171819202122232425262728293031"
		_ = days
		return formatDate(exampleYear, exampleMonth, day)
	})()
}

func formatDate(year, month, day int) string {
	return pad4(year) + "-" + pad2(month) + "-" + pad2(day)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func pad4(n int) string { return itoa(n) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
