// Package pngrender draws a month's programme as a PNG picture: the same
// information as the PDF (dates, uniform colours, activities and instructors,
// closures, weekend events and the uniform key) laid out as one tall page that
// reads well on a phone and is easy to share. It is drawn directly in Go, with
// no outside programme, so it works in the minimal container image.
//
// The requirement tracker (cadet names) and the notes on a night are never
// drawn.
package pngrender

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/skip2/go-qrcode"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"rafactools-planner/internal/programme"
	"rafactools-planner/internal/version"
)

// Options are the extras that can be drawn at the foot of the picture.
type Options struct {
	// CalendarURL, when set, is drawn as a QR code, with the date the link stops.
	CalendarURL     string
	CalendarExpires time.Time
}

const (
	width    = 1500.0
	margin   = 30.0
	pad      = 8.0
	fontSize = 22.0
	smallFnt = 19.0
	dateW    = 120.0
)

var (
	regular, bold *truetype.Font
)

func init() {
	var err error
	if regular, err = truetype.Parse(goregular.TTF); err != nil {
		panic(err)
	}
	if bold, err = truetype.Parse(gobold.TTF); err != nil {
		panic(err)
	}
}

func face(f *truetype.Font, size float64) font.Face {
	return truetype.NewFace(f, &truetype.Options{Size: size})
}

// Render draws the month and returns the PNG bytes.
func Render(sq *programme.Squadron, fp *programme.FullProgramme, opts Options) ([]byte, error) {
	// Draw on a canvas that is certainly tall enough, then crop to the content.
	dc := gg.NewContext(int(width), 12000)
	dc.SetColor(color.White)
	dc.Clear()

	y := margin
	y = drawTitle(dc, sq, fp, y)
	y = drawTable(dc, sq, fp, y)
	y = drawWeekend(dc, fp, y)
	y = drawKey(dc, sq, y)
	y = drawFooter(dc, opts, y)

	img, ok := dc.Image().(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("render png: cannot crop image")
	}
	out := img.SubImage(image.Rect(0, 0, int(width), int(y+margin)))
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("render png: %w", err)
	}
	return buf.Bytes(), nil
}

func drawTitle(dc *gg.Context, sq *programme.Squadron, fp *programme.FullProgramme, y float64) float64 {
	dc.SetColor(color.Black)
	dc.SetFontFace(face(bold, 38))
	dc.DrawString(sq.Name, margin, y+38)
	y += 52
	dc.SetFontFace(face(regular, 24))
	if sq.Subtitle != "" {
		dc.DrawString(sq.Subtitle, margin, y+24)
		y += 34
	}
	dc.SetFontFace(face(bold, 30))
	month := time.Month(fp.Month).String() + " " + fmt.Sprint(fp.Year) + " programme"
	dc.DrawString(month, margin, y+30)
	return y + 50
}

// column layout: a date cell, then one cell per classification.
func columnWidths(n int) (date, class float64) {
	if n < 1 {
		n = 1
	}
	return dateW, (width - 2*margin - dateW) / float64(n)
}

func drawTable(dc *gg.Context, sq *programme.Squadron, fp *programme.FullProgramme, y float64) float64 {
	dateCol, classCol := columnWidths(len(sq.Classifications))
	colour := map[string]string{}
	for _, c := range sq.UniformColours {
		colour[c.Key] = c.Hex
	}

	// Header.
	rowH := 40.0
	fillRect(dc, margin, y, width-2*margin, rowH, "#dde3ee")
	dc.SetFontFace(face(bold, fontSize))
	setColour(dc, "#000000")
	drawCentred(dc, "Date", margin, y, dateCol, rowH)
	for i, name := range sq.Classifications {
		x := margin + dateCol + float64(i)*classCol
		drawCentred(dc, name, x, y, classCol, rowH)
	}
	strokeRect(dc, margin, y, width-2*margin, rowH)
	y += rowH

	for _, d := range fp.Days {
		if !d.IsParade {
			continue
		}
		t, err := time.Parse("2006-01-02", d.Date)
		if err != nil {
			continue
		}
		label := t.Format("Mon 2")
		if cl, closed := programme.ClosureOn(d.Date, fp.Closures); closed {
			h := 44.0
			fillRect(dc, margin, y, dateCol, h, "#c9c9c9")
			dc.SetFontFace(face(bold, fontSize))
			setColour(dc, "#000000")
			drawCentred(dc, label, margin, y, dateCol, h)
			fillRect(dc, margin+dateCol, y, width-2*margin-dateCol, h, "#c9c9c9")
			msg := "Closed"
			if cl.Message != "" {
				msg += ": " + cl.Message
			}
			dc.SetFontFace(face(regular, fontSize))
			setColour(dc, "#000000")
			drawCentred(dc, msg, margin+dateCol, y, width-2*margin-dateCol, h)
			strokeRect(dc, margin, y, width-2*margin, h)
			y += h
			continue
		}

		// Each half is an activity line over an instructor line per class.
		type cell struct{ act, ins []string }
		halves := max(d.Halves, 1)
		cells := make([][]cell, halves)
		halfH := make([]float64, halves) // every class shares each half's height, so the lines line up
		h := pad * 2
		for half := 1; half <= halves; half++ {
			cells[half-1] = make([]cell, len(sq.Classifications))
			maxLines := 1
			for i := range sq.Classifications {
				var c cell
				if r, ok := d.RowByHalfType(half, "activity"); ok && i < len(r.Texts) {
					dc.SetFontFace(face(bold, fontSize))
					c.act = wrap(dc, r.Texts[i], classCol-2*pad)
				}
				if r, ok := d.RowByHalfType(half, "instructor"); ok && i < len(r.Texts) {
					dc.SetFontFace(face(regular, smallFnt))
					c.ins = wrap(dc, r.Texts[i], classCol-2*pad)
				}
				cells[half-1][i] = c
				if n := len(c.act) + len(c.ins); n > maxLines {
					maxLines = n
				}
			}
			halfH[half-1] = float64(maxLines)*(fontSize+6) + 4
			h += halfH[half-1]
		}
		if h < 56 {
			h = 56
		}

		fill := colour[d.UniformKey]
		if fill == "" {
			fill = "#ffffff"
		}
		fillRect(dc, margin, y, dateCol, h, fill)
		dc.SetFontFace(face(bold, fontSize))
		setColour(dc, textOn(fill))
		drawCentred(dc, label, margin, y, dateCol, h)
		for i := range sq.Classifications {
			x := margin + dateCol + float64(i)*classCol
			fillRect(dc, x, y, classCol, h, "#ffffff")
			cy := y + pad
			for half := 0; half < halves; half++ {
				start := cy
				c := cells[half][i]
				dc.SetFontFace(face(bold, fontSize))
				setColour(dc, "#000000")
				for _, l := range c.act {
					cy += fontSize + 6
					dc.DrawString(l, x+pad, cy-4)
				}
				dc.SetFontFace(face(regular, smallFnt))
				setColour(dc, "#555555")
				for _, l := range c.ins {
					cy += fontSize + 6
					dc.DrawString(l, x+pad, cy-4)
				}
				cy = start + halfH[half]
				if half < halves-1 { // a thin line between the two halves of a night
					setColour(dc, "#cccccc")
					dc.DrawLine(x+pad, cy-2, x+classCol-pad, cy-2)
					dc.SetLineWidth(1)
					dc.Stroke()
				}
			}
			strokeRect(dc, x, y, classCol, h)
		}
		strokeRect(dc, margin, y, dateCol, h)
		y += h
	}
	return y + 24
}

func drawWeekend(dc *gg.Context, fp *programme.FullProgramme, y float64) float64 {
	if len(fp.WeekendNotes) == 0 {
		return y
	}
	setColour(dc, "#000000")
	dc.SetFontFace(face(bold, fontSize+2))
	dc.DrawString("Weekend events", margin, y+fontSize)
	y += fontSize + 14
	dc.SetFontFace(face(regular, fontSize))
	for _, n := range fp.WeekendNotes {
		label := n.Date
		if t, err := time.Parse("2006-01-02", n.Date); err == nil {
			label = t.Format("Mon 2 Jan")
		}
		for _, l := range wrap(dc, label+": "+n.Text, width-2*margin) {
			dc.DrawString(l, margin, y+fontSize)
			y += fontSize + 6
		}
	}
	return y + 18
}

func drawKey(dc *gg.Context, sq *programme.Squadron, y float64) float64 {
	if len(sq.UniformColours) == 0 {
		return y
	}
	setColour(dc, "#000000")
	dc.SetFontFace(face(bold, fontSize))
	dc.DrawString("Uniform key", margin, y+fontSize)
	y += fontSize + 12
	w := (width - 2*margin) / float64(len(sq.UniformColours))
	dc.SetFontFace(face(regular, smallFnt))
	for i, c := range sq.UniformColours {
		x := margin + float64(i)*w
		fillRect(dc, x, y, w, 40, c.Hex)
		setColour(dc, textOn(c.Hex))
		drawCentred(dc, c.Label, x, y, w, 40)
		strokeRect(dc, x, y, w, 40)
	}
	return y + 40 + 24
}

func drawFooter(dc *gg.Context, opts Options, y float64) float64 {
	if opts.CalendarURL != "" {
		if qr, err := qrcode.New(opts.CalendarURL, qrcode.Medium); err == nil {
			const size = 190
			dc.DrawImage(qr.Image(size), int(margin), int(y))
			setColour(dc, "#000000")
			dc.SetFontFace(face(bold, fontSize))
			dc.DrawString("Add this programme to your calendar", margin+size+20, y+40)
			dc.SetFontFace(face(regular, smallFnt))
			dc.DrawString("Scan the code with your phone.", margin+size+20, y+72)
			if !opts.CalendarExpires.IsZero() {
				dc.DrawString("This link works until "+opts.CalendarExpires.In(programme.Local).Format("2 January 2006")+".", margin+size+20, y+100)
			}
			y += size + 16
		}
	}
	setColour(dc, "#777777")
	dc.SetFontFace(face(regular, 16))
	dc.DrawString("v"+version.Version, margin, y+16)
	return y + 24
}

// wrap breaks text into lines no wider than w. A single word wider than w is
// kept whole on its own line.
func wrap(dc *gg.Context, text string, w float64) []string {
	var lines []string
	for _, para := range strings.Split(strings.TrimSpace(text), "\n") {
		cur := ""
		for _, word := range strings.Fields(para) {
			try := word
			if cur != "" {
				try = cur + " " + word
			}
			if tw, _ := dc.MeasureString(try); tw > w && cur != "" {
				lines = append(lines, cur)
				cur = word
			} else {
				cur = try
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
	}
	return lines
}

func drawCentred(dc *gg.Context, s string, x, y, w, h float64) {
	tw, _ := dc.MeasureString(s)
	if tw > w-2*pad {
		lines := wrap(dc, s, w-2*pad)
		total := float64(len(lines)) * (fontSize + 4)
		ty := y + (h-total)/2
		for _, l := range lines {
			lw, _ := dc.MeasureString(l)
			dc.DrawString(l, x+(w-lw)/2, ty+fontSize)
			ty += fontSize + 4
		}
		return
	}
	dc.DrawString(s, x+(w-tw)/2, y+h/2+fontSize/3)
}

func fillRect(dc *gg.Context, x, y, w, h float64, hex string) {
	setColour(dc, hex)
	dc.DrawRectangle(x, y, w, h)
	dc.Fill()
}

func strokeRect(dc *gg.Context, x, y, w, h float64) {
	setColour(dc, "#444444")
	dc.SetLineWidth(1.5)
	dc.DrawRectangle(x, y, w, h)
	dc.Stroke()
}

func setColour(dc *gg.Context, hex string) { dc.SetColor(parseHex(hex)) }

func parseHex(hex string) color.Color {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return color.White
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.White
	}
	return color.RGBA{r, g, b, 255}
}

// textOn picks black or white text for readability on a background colour.
func textOn(hex string) string {
	c, ok := parseHex(hex).(color.RGBA)
	if !ok {
		return "#000000"
	}
	if 0.299*float64(c.R)+0.587*float64(c.G)+0.114*float64(c.B) > 150 {
		return "#000000"
	}
	return "#ffffff"
}
