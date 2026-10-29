package web

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"rafactools-planner/internal/programme"
)

// gridForm builds what the edit page would post for programme p: every night
// as stored (parade or not, with the text of its rows), with overrides applied.
func gridForm(t *testing.T, app *testApp, sq, pid int64, override func(d programme.Day, v url.Values)) url.Values {
	t.Helper()
	fp, err := app.store.GetFullProgramme(sq, pid)
	if err != nil {
		t.Fatal(err)
	}
	v := url.Values{}
	for _, d := range fp.Days {
		p := "day_" + strconv.FormatInt(d.ID, 10) + "_"
		if d.IsParade {
			v.Set(p+"parade", "1")
		}
		v.Set(p+"uniform", d.UniformKey)
		v.Set(p+"notes", d.Notes)
		if d.IsParade {
			for _, r := range d.Rows {
				for i, txt := range r.Texts {
					v.Set("row_"+strconv.FormatInt(r.ID, 10)+"_text_"+strconv.Itoa(i), txt)
				}
			}
		}
		if override != nil {
			override(d, v)
		}
	}
	return v
}

func firstDay(fp *programme.FullProgramme, parade bool) programme.Day {
	for _, d := range fp.Days {
		if d.IsParade == parade {
			return d
		}
	}
	return programme.Day{}
}

func TestNonParadeNightsOfferTheBoxesTheyWillHaveWhenTicked(t *testing.T) {
	app := newApp(t)
	email := app.owner("2101")
	c := app.login(email)
	sq := app.squadronID(email)
	p, _ := app.store.CreateProgramme(sq, 2034, 3, 1)
	fp, _ := app.store.GetFullProgramme(sq, p.ID)
	off := firstDay(fp, false)
	page := body(c.get("/programmes/" + strconv.FormatInt(p.ID, 10)))

	// New boxes for a night with no rows, hidden and disabled until ticked.
	id := strconv.FormatInt(off.ID, 10)
	for _, want := range []string{`name="day_` + id + `_new_activity_0"`, `name="day_` + id + `_new_instructor_0"`, `class="pending-row"`, `class="pending-label" hidden`, "programme-parade.js"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	input := regexp.MustCompile(`<input[^>]*name="day_` + id + `_new_activity_0"[^>]*>`).FindString(page)
	if !strings.Contains(input, " disabled") || !strings.Contains(input, " hidden") {
		t.Errorf("a pending box is submitted or visible by default: %s", input)
	}
	// A night that is a parade night has no pending boxes.
	on := firstDay(fp, true)
	if strings.Contains(page, `name="day_`+strconv.FormatInt(on.ID, 10)+`_new_activity_0"`) {
		t.Error("a parade night offers pending boxes")
	}
}

func TestTickingANightAndSavingCreatesItsRowsWithTheTypedText(t *testing.T) {
	app := newApp(t)
	email := app.owner("2102")
	c := app.login(email)
	sq := app.squadronID(email)
	p, _ := app.store.CreateProgramme(sq, 2034, 4, 1)
	fp, _ := app.store.GetFullProgramme(sq, p.ID)
	off := firstDay(fp, false)
	id := strconv.FormatInt(off.ID, 10)

	form := gridForm(t, app, sq, p.ID, func(d programme.Day, v url.Values) {
		if d.ID == off.ID {
			v.Set("day_"+id+"_parade", "1")
			v.Set("day_"+id+"_new_activity_0", "Drill")
			v.Set("day_"+id+"_new_instructor_1", "Sgt Jones")
		}
	})
	wantRedirect(t, c.post("/programmes/"+strconv.FormatInt(p.ID, 10), form), "/programmes/"+strconv.FormatInt(p.ID, 10))
	got, _ := app.store.GetFullProgramme(sq, p.ID)
	var night programme.Day
	for _, d := range got.Days {
		if d.ID == off.ID {
			night = d
		}
	}
	act, _ := night.RowByHalfType(1, "activity")
	ins, _ := night.RowByHalfType(1, "instructor")
	if !night.IsParade || act.Texts[0] != "Drill" || ins.Texts[1] != "Sgt Jones" {
		t.Errorf("night after saving: parade=%v activity=%v instructor=%v", night.IsParade, act.Texts, ins.Texts)
	}
}

func TestPendingTextIsIgnoredWhenTheNightIsNotTicked(t *testing.T) {
	app := newApp(t)
	email := app.owner("2103")
	c := app.login(email)
	sq := app.squadronID(email)
	p, _ := app.store.CreateProgramme(sq, 2034, 5, 1)
	fp, _ := app.store.GetFullProgramme(sq, p.ID)
	off := firstDay(fp, false)
	id := strconv.FormatInt(off.ID, 10)

	form := gridForm(t, app, sq, p.ID, func(d programme.Day, v url.Values) {
		if d.ID == off.ID {
			v.Set("day_"+id+"_new_activity_0", "should not be kept") // typed, then unticked
		}
	})
	c.post("/programmes/"+strconv.FormatInt(p.ID, 10), form)
	got, _ := app.store.GetFullProgramme(sq, p.ID)
	for _, d := range got.Days {
		if d.ID == off.ID && (d.IsParade || len(d.Rows) != 0) {
			t.Errorf("an unticked night got rows: %+v", d)
		}
	}
}

// Regression: a night that was a parade night, was unticked and saved, keeps
// its text through later saves (its boxes are not on the page, so they send
// nothing, and nothing may be blanked for that), and ticking it again brings
// the text back.
func TestUntickedNightKeepsItsTextThroughLaterSaves(t *testing.T) {
	app := newApp(t)
	email := app.owner("2104")
	c := app.login(email)
	sq := app.squadronID(email)
	p, _ := app.store.CreateProgramme(sq, 2034, 6, 1)
	fp, _ := app.store.GetFullProgramme(sq, p.ID)
	night := firstDay(fp, true)
	id := strconv.FormatInt(night.ID, 10)
	pid := "/programmes/" + strconv.FormatInt(p.ID, 10)
	row0 := strconv.FormatInt(night.Rows[0].ID, 10)

	// Type text and save; untick and save; save again twice.
	c.post(pid, gridForm(t, app, sq, p.ID, func(d programme.Day, v url.Values) {
		if d.ID == night.ID {
			v.Set("row_"+row0+"_text_0", "Precious activity")
		}
	}))
	c.post(pid, gridForm(t, app, sq, p.ID, func(d programme.Day, v url.Values) {
		if d.ID == night.ID {
			v.Del("day_" + id + "_parade") // unticked; its boxes are still in this form
		}
	}))
	for i := 0; i < 2; i++ {
		c.post(pid, gridForm(t, app, sq, p.ID, nil)) // now the page has no boxes for that night
	}
	got, _ := app.store.GetFullProgramme(sq, p.ID)
	for _, d := range got.Days {
		if d.ID == night.ID {
			r, _ := d.RowByHalfType(1, "activity")
			if d.IsParade || r.Texts[0] != "Precious activity" {
				t.Fatalf("after later saves: parade=%v text=%q (the text was blanked)", d.IsParade, r.Texts[0])
			}
		}
	}

	// The page offers the kept text in the hidden boxes, under the real names.
	page := body(c.get(pid))
	if !strings.Contains(page, `name="row_`+row0+`_text_0" value="Precious activity" disabled hidden`) {
		t.Errorf("the kept text is not offered when the night is ticked again")
	}
	// Ticking it again and saving (the enabled boxes now post under their real names).
	c.post(pid, gridForm(t, app, sq, p.ID, func(d programme.Day, v url.Values) {
		if d.ID == night.ID {
			v.Set("day_"+id+"_parade", "1")
			v.Set("row_"+row0+"_text_0", "Precious activity")
		}
	}))
	got, _ = app.store.GetFullProgramme(sq, p.ID)
	for _, d := range got.Days {
		if d.ID == night.ID {
			r, _ := d.RowByHalfType(1, "activity")
			if !d.IsParade || r.Texts[0] != "Precious activity" {
				t.Errorf("re-ticked: parade=%v text=%q", d.IsParade, r.Texts[0])
			}
		}
	}
}

// The Activity/Instructor labels have a column of their own, between the
// Uniform column and the first classification, so the first classification's
// heading sits over its own boxes.
func TestGridHasUniformAndLabelColumnsBeforeTheClassifications(t *testing.T) {
	app := newApp(t)
	email := app.owner("2105")
	c := app.login(email)
	sq := app.squadronID(email)
	p, _ := app.store.CreateProgramme(sq, 2034, 7, 1)
	page := body(c.get("/programmes/" + strconv.FormatInt(p.ID, 10)))
	head := page[strings.Index(page, "<thead>"):strings.Index(page, "</thead>")]
	order := []string{">Date<", ">Day<", ">Parade<", ">Uniform<", ">Row<", ">Junior Cadets<", ">1st Class Cadets<", ">Leading Cadets<", ">Senior Cadets<", ">Notes<"}
	at := -1
	for _, h := range order {
		i := strings.Index(head, h)
		if i < 0 || i < at {
			t.Fatalf("heading %q missing or out of order in:\n%s", h, head)
		}
		at = i
	}
	// A parade night's first line: date, day, tickbox, uniform, THEN the Activity label, then the boxes.
	row := page[strings.Index(page, `class="day-start"`):]
	row = row[:strings.Index(row, "</tr>")]
	if iu, il, ib := strings.Index(row, "_uniform"), strings.Index(row, `class="row-label">Activity`), strings.Index(row, `name="row_`); !(iu < il && il < ib) {
		t.Errorf("cells out of order (uniform %d, label %d, first box %d)", iu, il, ib)
	}
}
