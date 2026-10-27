// Package pdfgen renders a month's FullProgramme as a single portrait A4
// page matching the squadron's existing paper format: a grid of Date/Day/
// Half/Activity-Instructor/classification columns, a Notes column, a split
// Saturday/Sunday Weekends column, a uniform-key legend, and a footer
// requirement tracker.
//
// This is a best-effort v1 layout modelled on a real squadron's paper
// programme, not a pixel-perfect clone — see TODO.md "PDF layout fidelity"
// for known gaps (multi-week closure banners are drawn per-date rather than
// as one merged rowspan, and long-running weekend events aren't visually
// spanned across weeks).
//
// Row heights are computed from wrapped line counts (via Fpdf.SplitText)
// before anything is drawn, not fixed — a cell's content decides how tall
// its row is, and every other cell in that row is drawn to match. Each row
// is drawn in two passes: first every cell's border+fill for the row's full
// height (so the grid lines stay a clean rectangle regardless of which cell
// actually needed the extra height), then the (possibly wrapped) text is
// overlaid without its own border, top-aligned within that cell.
package pdfgen

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"rafactools-planner/internal/programme"
	"rafactools-planner/internal/version"
)

const (
	lineH   = 3.6 // mm per wrapped text line within a grid row
	headerH = 6.0 // mm for the grid header row
	cellPad = 1.0 // mm horizontal inset used when estimating wrap width
)

var monthNames = []string{"", "January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}

// columns are tuned for portrait A4's ~194mm usable width — notably
// narrower than a landscape layout's fixed columns would be, which is why
// text wrapping (not single-line truncation/overflow) matters here.
type columns struct {
	date, day, half, label, notes, weekendSat, weekendSun float64
	classification                                        []float64
}

func newColumns(usableWidth float64, numClassifications int) columns {
	c := columns{date: 7, day: 6, half: 5, label: 12, notes: 24, weekendSat: 13, weekendSun: 13}
	fixed := c.date + c.day + c.half + c.label + c.notes + c.weekendSat + c.weekendSun
	remaining := usableWidth - fixed
	if numClassifications < 1 {
		numClassifications = 1
	}
	each := remaining / float64(numClassifications)
	c.classification = make([]float64, numClassifications)
	for i := range c.classification {
		c.classification[i] = each
	}
	return c
}

// Render builds the PDF and returns its bytes. The whole month is forced
// onto one portrait page: rows are packed as tight as wrapped content
// allows rather than left at a fixed height, and there is deliberately no
// page-break fallback — a month that still doesn't fit is a layout bug to
// fix here (tighter columns/fonts), not a second page to add.
func Render(sq *programme.Squadron, fp *programme.FullProgramme) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(8, 8, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	pageW, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	usableWidth := pageW - left - right
	cols := newColumns(usableWidth, len(sq.Classifications))

	colourByKey := map[string]programme.UniformColour{}
	for _, uc := range sq.UniformColours {
		colourByKey[uc.Key] = uc
	}

	drawProtectiveMarking(pdf, sq, usableWidth)
	drawTitleBand(pdf, sq, fp, usableWidth)
	drawGridHeader(pdf, cols, sq.Classifications)

	weeks := groupIntoWeeks(fp.Days)
	weekendByDate := map[string][]string{}
	for _, w := range fp.WeekendNotes {
		weekendByDate[w.Date] = append(weekendByDate[w.Date], w.Text)
	}
	closureByDate := map[string]programme.Closure{}
	for _, c := range fp.Closures {
		// nextDate returns its input unchanged for an unparseable date;
		// stop rather than spin forever on bad data.
		for d := c.StartDate; d <= c.EndDate; {
			closureByDate[d] = c
			n := nextDate(d)
			if n == d {
				break
			}
			d = n
		}
	}

	for _, week := range weeks {
		drawWeek(pdf, cols, sq, week, weekendByDate, closureByDate, colourByKey)
	}

	drawUniformKey(pdf, sq, usableWidth)
	drawRequirementsFooter(pdf, fp.Requirements, usableWidth)
	drawProtectiveMarking(pdf, sq, usableWidth)
	drawVersionFooter(pdf, usableWidth)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render pdf: %w", err)
	}
	return buf.Bytes(), nil
}

func drawProtectiveMarking(pdf *fpdf.Fpdf, sq *programme.Squadron, usableWidth float64) {
	if sq.ProtectiveMarking == "" {
		return
	}
	pdf.SetFont("Helvetica", "", 6)
	pdf.CellFormat(usableWidth, 3, sq.ProtectiveMarking, "0", 1, "C", false, 0, "")
}

const changelogURL = "https://github.com/kolonuk/rafactools-planner/blob/main/CHANGELOG.md"

// drawVersionFooter prints the build version with a clickable link to the
// changelog — same information the web footer shows (see layout.html),
// so a printed/saved PDF is traceable back to the code that produced it.
func drawVersionFooter(pdf *fpdf.Fpdf, usableWidth float64) {
	pdf.SetFont("Helvetica", "", 6)
	pdf.SetTextColor(120, 120, 120)
	// Plain hyphen, not an em-dash: fpdf's core Helvetica font is
	// WinAnsi/Latin-1, not UTF-8 — a Go string literal's em-dash (3 UTF-8
	// bytes) gets drawn as three mojibake glyphs instead of transcoded. Same
	// trap exists for any other non-Latin1 character reaching CellFormat/
	// MultiCell anywhere in this file (a squadron name with a smart quote,
	// say) — seen directly: rendered as "â€"" first time this line used "—".
	// Not fixed project-wide here (would need an embedded Unicode TTF via
	// AddUTF8Font); tracked in TODO.md.
	pdf.CellFormat(usableWidth, 3, "v"+version.Version+" - version history", "0", 1, "C", false, 0, changelogURL)
	pdf.SetTextColor(0, 0, 0)
}

func drawTitleBand(pdf *fpdf.Fpdf, sq *programme.Squadron, fp *programme.FullProgramme, usableWidth float64) {
	pdf.SetFont("Helvetica", "B", 14)
	pdf.CellFormat(usableWidth, 7, sq.Name, "0", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(usableWidth, 4.5, sq.Subtitle, "0", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "BU", 11)
	pdf.CellFormat(usableWidth, 6, fmt.Sprintf("%s %d", monthNames[fp.Month], fp.Year), "0", 1, "C", false, 0, "")
}

func drawGridHeader(pdf *fpdf.Fpdf, cols columns, classifications []string) {
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetFillColor(230, 230, 230)
	pdf.CellFormat(cols.date, headerH, "Date", "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols.day, headerH, "Day", "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols.half, headerH, "Half", "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols.label, headerH, "Activity", "1", 0, "C", true, 0, "")
	for i, name := range classifications {
		pdf.CellFormat(cols.classification[i], headerH, name, "1", 0, "C", true, 0, "")
	}
	pdf.CellFormat(cols.notes, headerH, "Notes", "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols.weekendSat+cols.weekendSun, headerH, "Weekends", "1", 1, "C", true, 0, "")
}

type week struct {
	days []programme.Day // only days within the current month, Monday first
}

// groupIntoWeeks splits a month's days (already ordered by date) into
// Monday-to-Sunday blocks for the Weekends column, which pairs each week's
// Saturday and Sunday regardless of how many parade nights fall between them.
func groupIntoWeeks(days []programme.Day) []week {
	var weeks []week
	var current week
	for _, d := range days {
		t, _ := time.Parse("2006-01-02", d.Date)
		if t.Weekday() == time.Monday && len(current.days) > 0 {
			weeks = append(weeks, current)
			current = week{}
		}
		current.days = append(current.days, d)
	}
	if len(current.days) > 0 {
		weeks = append(weeks, current)
	}
	return weeks
}

func nextDate(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

// segment is one merged run of adjacent classification columns sharing the
// same text (see mergeSegments), carrying its on-page x/width alongside so
// the two-pass border-then-text drawing can reuse the same geometry.
type segment struct {
	x, w float64
	text string
}

// mergeSegments collapses adjacent columns with identical text into one
// wide segment, mirroring how the source PDFs show a single shared activity
// across some or all classifications without a separate "merged" flag (see
// migrations/0001_init.sql).
func mergeSegments(startX float64, widths []float64, texts []string) []segment {
	n := len(widths)
	for len(texts) < n {
		texts = append(texts, "")
	}
	var segs []segment
	x := startX
	i := 0
	for i < n {
		j := i
		w := widths[i]
		for j+1 < n && texts[j+1] == texts[i] {
			j++
			w += widths[j]
		}
		segs = append(segs, segment{x: x, w: w, text: texts[i]})
		x += w
		i = j + 1
	}
	return segs
}

// wrapLineCount returns how many lines text needs at the current font
// within width w (minimum 1, so an empty cell still gets one line's worth
// of height).
func wrapLineCount(pdf *fpdf.Fpdf, text string, w float64) int {
	if strings.TrimSpace(text) == "" {
		return 1
	}
	lines := pdf.SplitText(text, w-2*cellPad)
	if len(lines) < 1 {
		return 1
	}
	return len(lines)
}

// subRow is one Activity or Instructor line within a parade night's half,
// pre-measured so its drawn height matches whatever it actually needs.
type subRow struct {
	half     int
	rowType  string // "activity" | "instructor"
	segments []segment
	height   float64
}

// dayLayout is a day's fully pre-measured geometry: every subRow's height
// and the day's total, computed once and reused for both the page-fit check
// (pageBreakGuard) and the actual drawing (drawDayRow) so they can never
// disagree.
type dayLayout struct {
	total      float64
	subRows    []subRow // parade days only
	closure    *programme.Closure
	closureLns int
}

func computeDayLayout(pdf *fpdf.Fpdf, cols columns, d programme.Day, closureByDate map[string]programme.Closure, startX float64) dayLayout {
	if c, closed := closureByDate[d.Date]; closed {
		pdf.SetFont("Helvetica", "B", 9)
		lines := strings.Split(c.Message, "\n")
		maxLines := 0
		bodyW := cols.half + cols.label + sum(cols.classification) + cols.notes
		for _, l := range lines {
			maxLines += wrapLineCount(pdf, l, bodyW)
		}
		if maxLines < 1 {
			maxLines = 1
		}
		h := float64(maxLines) * 5.0
		return dayLayout{total: h, closure: &c, closureLns: maxLines}
	}

	pdf.SetFont("Helvetica", "", 6.5)
	notesLines := wrapLineCount(pdf, d.Notes, cols.notes)

	if !d.IsParade {
		h := float64(notesLines) * lineH
		if h < lineH {
			h = lineH
		}
		return dayLayout{total: h}
	}

	classX := startX + cols.half + cols.label
	var subRows []subRow
	total := 0.0
	for half := 1; half <= d.Halves; half++ {
		for _, rt := range []string{"activity", "instructor"} {
			row, _ := d.RowByHalfType(half, rt)
			bold := rt == "activity"
			style := ""
			if bold {
				style = "B"
			}
			pdf.SetFont("Helvetica", style, 6.5)
			segs := mergeSegments(classX, cols.classification, row.Texts)
			maxLines := 1
			for _, s := range segs {
				if n := wrapLineCount(pdf, s.text, s.w); n > maxLines {
					maxLines = n
				}
			}
			h := float64(maxLines) * lineH
			subRows = append(subRows, subRow{half: half, rowType: rt, segments: segs, height: h})
			total += h
		}
	}

	notesHeight := float64(notesLines) * lineH
	if notesHeight > total {
		scale := notesHeight / total
		for i := range subRows {
			subRows[i].height *= scale
		}
		total = notesHeight
	}
	return dayLayout{total: total, subRows: subRows}
}

func drawWeek(pdf *fpdf.Fpdf, cols columns, _ *programme.Squadron, w week,
	weekendByDate map[string][]string, closureByDate map[string]programme.Closure,
	colourByKey map[string]programme.UniformColour) {

	left, _, _, _ := pdf.GetMargins()
	startY := pdf.GetY()

	// Pre-measure every day in the week before drawing any of it: the
	// Weekends column spans the whole week in one merged cell, so if its
	// Sat/Sun note text needs more vertical room than the days' own content
	// would otherwise get, every day in the week must grow to match —
	// otherwise the note text overflows past the week block's bottom edge
	// and collides with whatever comes next (this is exactly the bug a
	// narrower portrait Weekends column made newly visible: the same text
	// needs more wrapped lines in a narrower cell).
	layouts := make([]dayLayout, len(w.days))
	daysTotal := 0.0
	for i, d := range w.days {
		layouts[i] = computeDayLayout(pdf, cols, d, closureByDate, left+cols.date+cols.day)
		daysTotal += layouts[i].total
	}
	if need := weekendRequiredHeight(pdf, cols, w.days, weekendByDate); need > daysTotal && daysTotal > 0 {
		scale := need / daysTotal
		for i := range layouts {
			layouts[i].total *= scale
			for j := range layouts[i].subRows {
				layouts[i].subRows[j].height *= scale
			}
		}
	}

	for i, d := range w.days {
		drawDayRow(pdf, cols, left, d, layouts[i], colourByKey)
	}
	endY := pdf.GetY()

	drawWeekendCell(pdf, cols, startY, endY, w.days, weekendByDate)
}

// weekendRequiredHeight is how tall the Weekends column's merged cell needs
// to be to fit its Saturday/Sunday note text, mirroring the font/line-height
// drawWeekendCell actually draws with.
func weekendRequiredHeight(pdf *fpdf.Fpdf, cols columns, days []programme.Day, weekendByDate map[string][]string) float64 {
	const weekendLineH = 2.6
	const headerAllowance = 3.0
	pdf.SetFont("Helvetica", "", 5.5)

	need := 0.0
	for _, d := range days {
		t, err := time.Parse("2006-01-02", d.Date)
		if err != nil || (t.Weekday() != time.Saturday && t.Weekday() != time.Sunday) {
			continue
		}
		w := cols.weekendSat
		if t.Weekday() == time.Sunday {
			w = cols.weekendSun
		}
		text := joinLines(weekendByDate[d.Date])
		lines := wrapLineCount(pdf, text, w)
		if h := headerAllowance + float64(lines)*weekendLineH; h > need {
			need = h
		}
	}
	return need
}

func drawDayRow(pdf *fpdf.Fpdf, cols columns, left float64, d programme.Day,
	layout dayLayout, colourByKey map[string]programme.UniformColour) {

	y := pdf.GetY()
	h := layout.total

	if layout.closure != nil {
		drawClosureRow(pdf, cols, left, y, d, *layout.closure, h)
		pdf.SetXY(left, y+h)
		return
	}

	dateNum := dateDay(d.Date)
	fillColour := colourByKey[d.UniformKey]
	hasFill := d.UniformKey != ""

	// Pass 1: borders/fill for the whole row, using the pre-measured height
	// so the grid stays a clean rectangle regardless of which cell actually
	// needed the extra lines.
	pdf.SetFont("Helvetica", "B", 7)
	if hasFill {
		r, g, b := hexToRGB(fillColour.Hex)
		pdf.SetFillColor(r, g, b)
	}
	pdf.SetXY(left, y)
	pdf.CellFormat(cols.date, h, "", "1", 0, "C", hasFill, 0, "")
	pdf.CellFormat(cols.day, h, "", "1", 0, "C", hasFill, 0, "")

	notesX := left + cols.date + cols.day + cols.half + cols.label + sum(cols.classification)

	if !d.IsParade {
		blankW := cols.half + cols.label + sum(cols.classification)
		pdf.CellFormat(blankW, h, "", "1", 0, "C", false, 0, "")
		pdf.CellFormat(cols.notes, h, "", "1", 0, "L", false, 0, "")
	} else {
		halfColX := left + cols.date + cols.day
		curY := y
		for _, sr := range layout.subRows {
			pdf.SetXY(halfColX, curY)
			pdf.CellFormat(cols.half, sr.height, "", "1", 0, "C", false, 0, "")
			pdf.CellFormat(cols.label, sr.height, "", "1", 0, "L", false, 0, "")
			for _, s := range sr.segments {
				pdf.SetXY(s.x, curY)
				pdf.CellFormat(s.w, sr.height, "", "1", 0, "C", false, 0, "")
			}
			curY += sr.height
		}
		pdf.SetXY(notesX, y)
		pdf.CellFormat(cols.notes, h, "", "1", 0, "L", false, 0, "")
	}

	// Pass 2: overlay text without a border, top-aligned within whatever
	// height pass 1 gave the cell. Date/Day are centered vertically since
	// they're always exactly one line regardless of the row's full height.
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(left, y+(h-lineH)/2)
	pdf.CellFormat(cols.date, lineH, dateNum, "0", 0, "C", false, 0, "")
	pdf.CellFormat(cols.day, lineH, d.DayLabel, "0", 0, "C", false, 0, "")

	if d.IsParade {
		halfColX := left + cols.date + cols.day
		curY := y
		for _, sr := range layout.subRows {
			labelText := "Activity"
			if sr.rowType == "instructor" {
				labelText = "Instructor"
			}
			style := ""
			if sr.rowType == "activity" {
				style = "B"
			}
			pdf.SetFont("Helvetica", "B", 6)
			pdf.SetXY(halfColX, curY+(sr.height-lineH)/2)
			halfText := ""
			if sr.rowType == "activity" {
				halfText = fmt.Sprintf("%d", sr.half)
			}
			pdf.CellFormat(cols.half, lineH, halfText, "0", 0, "C", false, 0, "")
			pdf.CellFormat(cols.label, lineH, labelText, "0", 0, "L", false, 0, "")

			pdf.SetFont("Helvetica", style, 6.5)
			for _, s := range sr.segments {
				pdf.SetXY(s.x, curY)
				pdf.MultiCell(s.w, lineH, s.text, "0", "C", false)
			}
			curY += sr.height
		}
	}

	pdf.SetFont("Helvetica", "", 6.5)
	pdf.SetXY(notesX, y)
	pdf.MultiCell(cols.notes, lineH, d.Notes, "0", "L", false)

	pdf.SetXY(left, y+h)
}

func drawClosureRow(pdf *fpdf.Fpdf, cols columns, left, y float64, d programme.Day, c programme.Closure, h float64) {
	pdf.SetFillColor(201, 201, 201)
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetXY(left, y)
	pdf.CellFormat(cols.date, h, dateDay(d.Date), "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols.day, h, d.DayLabel, "1", 0, "C", true, 0, "")

	bodyW := cols.half + cols.label + sum(cols.classification) + cols.notes
	if d.Date == c.StartDate {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetXY(left+cols.date+cols.day, y)
		pdf.MultiCell(bodyW, h, c.Message, "1", "C", true)
	} else {
		pdf.CellFormat(bodyW, h, "", "1", 0, "C", true, 0, "")
	}
}

func drawWeekendCell(pdf *fpdf.Fpdf, cols columns, startY, endY float64, days []programme.Day, weekendByDate map[string][]string) {
	h := endY - startY
	leftOfWeekend := cols.date + cols.day + cols.half + cols.label + sum(cols.classification) + cols.notes
	left, _, _, _ := pdf.GetMargins()
	x := left + leftOfWeekend

	var sat, sun *programme.Day
	for i := range days {
		d := days[i]
		t, _ := time.Parse("2006-01-02", d.Date)
		switch t.Weekday() {
		case time.Saturday:
			sat = &days[i]
		case time.Sunday:
			sun = &days[i]
		}
	}

	// Border for the full merged cell first (CellFormat vertically centers
	// single-line text within whatever height it's given — drawing "SAT" at
	// the full week height h would center it in the MIDDLE of the cell,
	// where a long wrapped note starting from the top would run straight
	// into it on a short week). So: draw the outline at full height with no
	// text, then the "SAT"/"SUN" label in its own small fixed-height slice
	// at the top, then the note text below that — never spanning the full h.
	const headerAllowance = 3.0
	pdf.SetXY(x, startY)
	pdf.CellFormat(cols.weekendSat, h, "", "1", 0, "C", false, 0, "")
	pdf.SetXY(x+cols.weekendSat, startY)
	pdf.CellFormat(cols.weekendSun, h, "", "1", 0, "C", false, 0, "")

	pdf.SetFont("Helvetica", "B", 5.5)
	pdf.SetXY(x, startY)
	pdf.CellFormat(cols.weekendSat, headerAllowance, "SAT", "0", 0, "C", false, 0, "")
	pdf.SetXY(x+cols.weekendSat, startY)
	pdf.CellFormat(cols.weekendSun, headerAllowance, "SUN", "0", 0, "C", false, 0, "")

	pdf.SetFont("Helvetica", "", 5.5)
	if sat != nil {
		pdf.SetXY(x, startY+headerAllowance)
		pdf.MultiCell(cols.weekendSat, 2.6, joinLines(weekendByDate[sat.Date]), "0", "C", false)
	}
	if sun != nil {
		pdf.SetXY(x+cols.weekendSat, startY+headerAllowance)
		pdf.MultiCell(cols.weekendSun, 2.6, joinLines(weekendByDate[sun.Date]), "0", "C", false)
	}

	pdf.SetXY(pdf.GetX(), endY)
}

func drawUniformKey(pdf *fpdf.Fpdf, sq *programme.Squadron, usableWidth float64) {
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "B", 7)
	labelW := usableWidth * 0.14
	pdf.CellFormat(labelW, 6, "Uniform Key", "1", 0, "C", false, 0, "")

	swatchLabelW := (usableWidth - labelW) / float64(len(sq.UniformColours))
	pdf.SetFont("Helvetica", "", 6)
	for _, uc := range sq.UniformColours {
		r, g, b := hexToRGB(uc.Hex)
		pdf.SetFillColor(r, g, b)
		pdf.CellFormat(swatchLabelW, 6, uc.Label, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
}

func drawRequirementsFooter(pdf *fpdf.Fpdf, cats []programme.ReqCategory, usableWidth float64) {
	if len(cats) == 0 {
		return
	}
	pdf.Ln(3)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.CellFormat(usableWidth, 5, "NOTES", "1", 1, "C", false, 0, "")

	w := usableWidth / float64(len(cats))
	startY := pdf.GetY()
	maxY := startY
	x0 := pdf.GetX()

	for i, cat := range cats {
		x := x0 + float64(i)*w
		pdf.SetXY(x, startY)
		pdf.SetFont("Helvetica", "B", 6)
		title := cat.Name
		if cat.DueLabel != "" {
			title += "\n" + cat.DueLabel
		}
		pdf.MultiCell(w, 2.8, title, "1", "C", false)
		y := pdf.GetY()

		pdf.SetFont("Helvetica", "", 6)
		for _, e := range cat.Entries {
			line := e.CadetName
			if e.Note != "" {
				line += " " + e.Note
			}
			pdf.SetXY(x, y)
			pdf.MultiCell(w, 2.8, line, "0", "C", false)
			y = pdf.GetY()
		}
		if y > maxY {
			maxY = y
		}
	}
	pdf.SetXY(x0, maxY)
}

func sum(ws []float64) float64 {
	t := 0.0
	for _, w := range ws {
		t += w
	}
	return t
}

func dateDay(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%d", t.Day())
}

func joinLines(lines []string) string {
	sort.Strings(lines) // stable, deterministic order; entries aren't otherwise ordered
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

func hexToRGB(hex string) (int, int, int) {
	if len(hex) != 7 || hex[0] != '#' {
		return 255, 255, 255
	}
	var r, g, b int
	fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}
