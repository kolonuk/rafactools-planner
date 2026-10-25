// Package pdfgen renders a month's FullProgramme as a PDF matching the
// squadron's existing paper format: a landscape grid of Date/Day/Half/
// Activity-Instructor/classification columns, a Notes column, a split
// Saturday/Sunday Weekends column, a uniform-key legend, and a footer
// requirement tracker.
//
// This is a best-effort v1 layout built from ten real example months, not a
// pixel-perfect clone — see TODO.md "PDF layout fidelity" for known gaps
// (multi-week closure banners are drawn per-date rather than as one merged
// rowspan, and long-running weekend events aren't visually spanned across
// weeks).
package pdfgen

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/go-pdf/fpdf"

	"rafactools-planner/internal/programme"
)

const (
	lineH      = 5.0  // mm per text line within a row
	headerH    = 7.0  // mm for the grid header row
	titleBandH = 24.0 // mm for the logo/title band
)

var monthNames = []string{"", "January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}

type columns struct {
	date, day, half, label, notes, weekendSat, weekendSun float64
	classification                                        []float64
}

func newColumns(usableWidth float64, numClassifications int) columns {
	c := columns{date: 9, day: 9, half: 7, label: 24, notes: 42, weekendSat: 19, weekendSun: 19}
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

func (c columns) total() float64 {
	t := c.date + c.day + c.half + c.label + c.notes + c.weekendSat + c.weekendSun
	for _, w := range c.classification {
		t += w
	}
	return t
}

// Render builds the PDF and returns its bytes.
func Render(sq *programme.Squadron, fp *programme.FullProgramme) ([]byte, error) {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(8, 8, 8)
	pdf.SetAutoPageBreak(true, 8)
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
		for d := c.StartDate; d <= c.EndDate; d = nextDate(d) {
			closureByDate[d] = c
		}
	}

	for _, week := range weeks {
		drawWeek(pdf, cols, sq, week, weekendByDate, closureByDate, colourByKey)
	}

	drawUniformKey(pdf, sq, usableWidth)
	drawRequirementsFooter(pdf, fp.Requirements, usableWidth)
	drawProtectiveMarking(pdf, sq, usableWidth)

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
	pdf.SetFont("Helvetica", "", 7)
	pdf.CellFormat(usableWidth, 4, sq.ProtectiveMarking, "0", 1, "C", false, 0, "")
}

func drawTitleBand(pdf *fpdf.Fpdf, sq *programme.Squadron, fp *programme.FullProgramme, usableWidth float64) {
	pdf.SetFont("Helvetica", "B", 18)
	pdf.CellFormat(usableWidth, 10, sq.Name, "0", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(usableWidth, 6, sq.Subtitle, "0", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "BU", 13)
	pdf.CellFormat(usableWidth, 8, fmt.Sprintf("%s %d", monthNames[fp.Month], fp.Year), "0", 1, "C", false, 0, "")
}

func drawGridHeader(pdf *fpdf.Fpdf, cols columns, classifications []string) {
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(230, 230, 230)
	pdf.CellFormat(cols.date, headerH, "Date", "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols.day, headerH, "Day", "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols.half, headerH, "Half", "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols.label, headerH, "Activity/Instructor", "1", 0, "C", true, 0, "")
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

func rowHeightForDay(d programme.Day) float64 {
	if !d.IsParade {
		return lineH
	}
	return lineH * float64(2*d.Halves) // activity+instructor per half
}

func drawWeek(pdf *fpdf.Fpdf, cols columns, sq *programme.Squadron, w week,
	weekendByDate map[string][]string, closureByDate map[string]programme.Closure,
	colourByKey map[string]programme.UniformColour) {

	startY := pdf.GetY()
	pageBreakGuard(pdf, w)

	for _, d := range w.days {
		drawDayRow(pdf, cols, sq, d, closureByDate, colourByKey)
	}
	endY := pdf.GetY()

	drawWeekendCell(pdf, cols, startY, endY, w.days, weekendByDate)
}

// pageBreakGuard forces a new page (repeating the header) if this week won't
// fit in the remaining space, so a week is never split across pages.
func pageBreakGuard(pdf *fpdf.Fpdf, w week) {
	total := 0.0
	for _, d := range w.days {
		total += rowHeightForDay(d)
	}
	_, pageH := pdf.GetPageSize()
	_, _, _, bottom := pdf.GetMargins()
	if pdf.GetY()+total > pageH-bottom {
		pdf.AddPage()
	}
}

func drawDayRow(pdf *fpdf.Fpdf, cols columns, sq *programme.Squadron, d programme.Day,
	closureByDate map[string]programme.Closure, colourByKey map[string]programme.UniformColour) {

	h := rowHeightForDay(d)
	x, y := pdf.GetX(), pdf.GetY()

	if c, closed := closureByDate[d.Date]; closed {
		drawClosureRow(pdf, cols, d, c, h)
		return
	}

	dateNum := dateDay(d.Date)
	fillColour := colourByKey[d.UniformKey]
	hasFill := d.UniformKey != ""

	pdf.SetFont("Helvetica", "B", 8)
	if hasFill {
		r, g, b := hexToRGB(fillColour.Hex)
		pdf.SetFillColor(r, g, b)
	}
	pdf.CellFormat(cols.date, h, dateNum, "1", 0, "C", hasFill, 0, "")
	pdf.CellFormat(cols.day, h, d.DayLabel, "1", 0, "C", hasFill, 0, "")

	if !d.IsParade {
		pdf.SetFont("Helvetica", "", 8)
		blankW := cols.half + cols.label + sum(cols.classification)
		pdf.CellFormat(blankW, h, "", "1", 0, "C", false, 0, "")
		pdf.CellFormat(cols.notes, h, d.Notes, "1", 0, "L", false, 0, "")
		pdf.SetXY(x, y+h)
		return
	}

	// Parade night: one sub-row per half, each with an Activity and an
	// Instructor line, classification columns merged where adjacent text
	// matches (this is how the source PDFs show one shared activity without
	// a separate "merged" flag — see migrations/0001_init.sql).
	subH := lineH
	halfColX := x + cols.date + cols.day
	curY := y
	for half := 1; half <= d.Halves; half++ {
		for _, rt := range []string{"activity", "instructor"} {
			row, _ := d.RowByHalfType(half, rt)
			pdf.SetXY(halfColX, curY)
			if rt == "activity" {
				pdf.SetFont("Helvetica", "B", 7)
				pdf.CellFormat(cols.half, subH, fmt.Sprintf("%d", half), "1", 0, "C", false, 0, "")
			} else {
				pdf.SetFont("Helvetica", "", 7)
				pdf.CellFormat(cols.half, subH, "", "1", 0, "C", false, 0, "")
			}
			labelText := "Activity"
			if rt == "instructor" {
				labelText = "Instructor"
			}
			pdf.SetFont("Helvetica", "B", 7)
			pdf.CellFormat(cols.label, subH, labelText, "1", 0, "L", false, 0, "")

			drawMergedClassificationCells(pdf, cols.classification, row.Texts, subH, rt == "instructor")
			curY += subH // CellFormat's ln=0 only moves X, never Y — advance by hand
		}
	}

	// Notes spans the whole day (all halves).
	pdf.SetXY(halfColX+cols.half+cols.label+sum(cols.classification), y)
	pdf.SetFont("Helvetica", "", 7)
	pdf.CellFormat(cols.notes, h, d.Notes, "1", 0, "L", false, 0, "")

	pdf.SetXY(x, y+h)
}

func drawClosureRow(pdf *fpdf.Fpdf, cols columns, d programme.Day, c programme.Closure, h float64) {
	x, y := pdf.GetX(), pdf.GetY()
	pdf.SetFillColor(201, 201, 201)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.CellFormat(cols.date, h, dateDay(d.Date), "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols.day, h, d.DayLabel, "1", 0, "C", true, 0, "")

	bodyW := cols.half + cols.label + sum(cols.classification) + cols.notes
	if d.Date == c.StartDate {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetXY(x+cols.date+cols.day, y)
		pdf.MultiCell(bodyW, h, c.Message, "1", "C", true)
	} else {
		pdf.CellFormat(bodyW, h, "", "1", 0, "C", true, 0, "")
	}
	pdf.SetXY(x, y+h)
}

// drawMergedClassificationCells merges adjacent columns with identical text
// into a single wide cell, mirroring how the source PDFs collapse shared
// activities across classifications.
func drawMergedClassificationCells(pdf *fpdf.Fpdf, widths []float64, texts []string, h float64, bold bool) {
	n := len(widths)
	for len(texts) < n {
		texts = append(texts, "")
	}
	fontStyle := ""
	if bold {
		fontStyle = "B"
	}
	pdf.SetFont("Helvetica", fontStyle, 7)

	i := 0
	for i < n {
		j := i
		w := widths[i]
		for j+1 < n && texts[j+1] == texts[i] {
			j++
			w += widths[j]
		}
		pdf.CellFormat(w, h, texts[i], "1", 0, "C", false, 0, "")
		i = j + 1
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

	pdf.SetFont("Helvetica", "B", 6)
	pdf.SetXY(x, startY)
	pdf.CellFormat(cols.weekendSat, h, "SAT", "1", 0, "C", false, 0, "")
	pdf.SetXY(x+cols.weekendSat, startY)
	pdf.CellFormat(cols.weekendSun, h, "SUN", "1", 0, "C", false, 0, "")

	pdf.SetFont("Helvetica", "", 6)
	if sat != nil {
		pdf.SetXY(x, startY+4)
		pdf.MultiCell(cols.weekendSat, 3.2, joinLines(weekendByDate[sat.Date]), "0", "C", false)
	}
	if sun != nil {
		pdf.SetXY(x+cols.weekendSat, startY+4)
		pdf.MultiCell(cols.weekendSun, 3.2, joinLines(weekendByDate[sun.Date]), "0", "C", false)
	}

	pdf.SetXY(pdf.GetX(), endY)
}

func drawUniformKey(pdf *fpdf.Fpdf, sq *programme.Squadron, usableWidth float64) {
	pdf.Ln(3)
	pdf.SetFont("Helvetica", "B", 8)
	labelW := usableWidth * 0.14
	pdf.CellFormat(labelW, 7, "Uniform Key", "1", 0, "C", false, 0, "")

	swatchLabelW := (usableWidth - labelW) / float64(len(sq.UniformColours))
	pdf.SetFont("Helvetica", "", 7)
	for _, uc := range sq.UniformColours {
		r, g, b := hexToRGB(uc.Hex)
		pdf.SetFillColor(r, g, b)
		pdf.CellFormat(swatchLabelW, 7, uc.Label, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
}

func drawRequirementsFooter(pdf *fpdf.Fpdf, cats []programme.ReqCategory, usableWidth float64) {
	if len(cats) == 0 {
		return
	}
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(usableWidth, 6, "NOTES", "1", 1, "C", false, 0, "")

	w := usableWidth / float64(len(cats))
	startY := pdf.GetY()
	maxY := startY
	x0 := pdf.GetX()

	for i, cat := range cats {
		x := x0 + float64(i)*w
		pdf.SetXY(x, startY)
		pdf.SetFont("Helvetica", "B", 7)
		title := cat.Name
		if cat.DueLabel != "" {
			title += "\n" + cat.DueLabel
		}
		pdf.MultiCell(w, 3.5, title, "1", "C", false)
		y := pdf.GetY()

		pdf.SetFont("Helvetica", "", 7)
		for _, e := range cat.Entries {
			line := e.CadetName
			if e.Note != "" {
				line += " " + e.Note
			}
			pdf.SetXY(x, y)
			pdf.MultiCell(w, 3.5, line, "0", "C", false)
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
