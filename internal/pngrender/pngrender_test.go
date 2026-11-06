package pngrender

import (
	"bytes"
	"image/png"
	"testing"
	"time"

	"rafactools-planner/internal/programme"
)

func sample() (*programme.Squadron, *programme.FullProgramme) {
	sq := &programme.Squadron{
		Number: "1", Name: "1 Sqn", Subtitle: "Air Cadets", Classifications: []string{"Junior", "Senior"},
		UniformColours: []programme.UniformColour{{Key: "g", Label: "Greens", Hex: "#8faa6e"}, {Key: "b", Label: "Blues", Hex: "#3b5e82"}},
	}
	fp := &programme.FullProgramme{Programme: programme.Programme{Year: 2030, Month: 2}}
	for d := 4; d <= 25; d += 7 {
		date := time.Date(2030, 2, d, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		fp.Days = append(fp.Days, programme.Day{
			ID: int64(d), Date: date, IsParade: true, UniformKey: "g", Halves: 2,
			Rows: []programme.Row{
				{Half: 1, RowType: "activity", Texts: []string{"First aid with a rather long title that must wrap", "Drill"}},
				{Half: 1, RowType: "instructor", Texts: []string{"Sgt A", "Cpl B"}},
				{Half: 2, RowType: "activity", Texts: []string{"Games", ""}},
			},
		})
	}
	fp.Closures = []programme.Closure{{StartDate: "2030-02-18", EndDate: "2030-02-18", Message: "Half term"}}
	fp.WeekendNotes = []programme.WeekendNote{{Date: "2030-02-09", Text: "Air show"}}
	fp.Requirements = []programme.ReqCategory{{Name: "Fieldcraft", Entries: []programme.ReqEntry{{CadetName: "Secret Cadet"}}}}
	return sq, fp
}

func TestRenderMakesAPNG(t *testing.T) {
	sq, fp := sample()
	data, err := Render(sq, fp, Options{CalendarURL: "webcal://example.test/share/t/calendar.ics", CalendarExpires: time.Date(2030, 3, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != int(width) || b.Dy() < 400 || b.Dy() > 6000 {
		t.Errorf("odd size %dx%d", b.Dx(), b.Dy())
	}
}

func TestRenderCopesWithAnEmptyMonth(t *testing.T) {
	sq := &programme.Squadron{Name: "Empty"}
	if _, err := Render(sq, &programme.FullProgramme{Programme: programme.Programme{Year: 2030, Month: 1}}, Options{}); err != nil {
		t.Fatal(err)
	}
}

func TestTextColourFollowsTheBackground(t *testing.T) {
	if textOn("#ffffff") != "#000000" || textOn("#000000") != "#ffffff" || textOn("nonsense") != "#000000" {
		t.Error("unexpected text colours")
	}
}
