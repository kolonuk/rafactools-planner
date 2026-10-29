package web

import (
	"bytes"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestProgrammeLifecycle(t *testing.T) {
	app := newApp(t)
	email := app.owner("1001")
	c := app.login(email)
	sq := app.squadronID(email)

	t.Run("dashboard empty state then create", func(t *testing.T) {
		if b := body(c.get("/")); !strings.Contains(b, "csrf") {
			t.Fatal("dashboard has no create form")
		}
		resp := c.post("/programmes", url.Values{"year": {"2030"}, "month": {"4"}})
		wantStatus(t, resp, 303)
		loc := resp.Header.Get("Location")
		if !strings.HasPrefix(loc, "/programmes/") {
			t.Fatalf("redirect %q", loc)
		}
		if b := body(c.get(loc)); !strings.Contains(b, "April") {
			t.Errorf("edit page does not mention the month")
		}
		if b := body(c.get("/")); !strings.Contains(b, "April") || !strings.Contains(b, "2030") {
			t.Error("dashboard does not list the new programme")
		}
	})

	t.Run("invalid create requests re-render with an error and create nothing", func(t *testing.T) {
		before := count(t, app.db, `SELECT COUNT(*) FROM programmes`)
		for name, form := range map[string]url.Values{
			"month 0":     {"year": {"2030"}, "month": {"0"}},
			"month 13":    {"year": {"2030"}, "month": {"13"}},
			"non-numeric": {"year": {"abc"}, "month": {"1"}},
			"missing":     {},
			"duplicate":   {"year": {"2030"}, "month": {"4"}},
			"negative":    {"year": {"-1"}, "month": {"-1"}},
		} {
			resp := c.post("/programmes", form)
			if resp.StatusCode == 303 {
				t.Errorf("%s: accepted", name)
			}
			resp.Body.Close()
		}
		if after := count(t, app.db, `SELECT COUNT(*) FROM programmes`); after != before {
			t.Errorf("programmes %d -> %d", before, after)
		}
	})

	var pid int64
	app.db.QueryRow(`SELECT id FROM programmes WHERE squadron_id = ?`, sq).Scan(&pid)
	id := strconv.FormatInt(pid, 10)

	t.Run("save the grid via the same field names the template uses", func(t *testing.T) {
		page := body(c.get("/programmes/" + id))
		fp, _ := app.store.GetFullProgramme(sq, pid)
		var day = fp.Days[0]
		for _, d := range fp.Days {
			if d.IsParade {
				day = d
				break
			}
		}
		notesField := "day_" + strconv.FormatInt(day.ID, 10) + "_notes"
		rowField := "row_" + strconv.FormatInt(day.Rows[0].ID, 10) + "_text_0"
		for _, f := range []string{notesField, rowField} {
			if !strings.Contains(page, `name="`+f+`"`) {
				t.Fatalf("edit page has no field %q: the handler and template disagree", f)
			}
		}
		wantRedirect(t, c.post("/programmes/"+id, url.Values{
			notesField: {"Bring PT kit"}, rowField: {"Fieldcraft"},
			"day_" + strconv.FormatInt(day.ID, 10) + "_parade": {"1"},
		}), "/programmes/"+id)
		page = body(c.get("/programmes/" + id))
		if !strings.Contains(page, "Bring PT kit") || !strings.Contains(page, "Fieldcraft") {
			t.Error("saved values not shown on reload")
		}
	})

	t.Run("add second half", func(t *testing.T) {
		fp, _ := app.store.GetFullProgramme(sq, pid)
		var day int64
		for _, d := range fp.Days {
			if d.IsParade {
				day = d.ID
				break
			}
		}
		wantRedirect(t, c.post("/programmes/"+id+"/days/"+strconv.FormatInt(day, 10)+"/add-half", nil), "/programmes/"+id)
		fp, _ = app.store.GetFullProgramme(sq, pid)
		for _, d := range fp.Days {
			if d.ID == day && d.Halves != 2 {
				t.Error("halves not updated")
			}
		}
	})

	t.Run("closures: invalid dates are a 400, valid ones are saved and shown", func(t *testing.T) {
		for _, bad := range []url.Values{
			{"start_date": {"a"}, "end_date": {"b"}},
			{"start_date": {"2030-04-10"}, "end_date": {"2030-04-01"}},
			{"start_date": {"2030-04-01"}, "end_date": {"2040-04-01"}},
		} {
			bad.Set("message", "x")
			wantStatus(t, c.post("/programmes/"+id+"/closures", bad), 400)
		}
		wantRedirect(t, c.post("/programmes/"+id+"/closures", url.Values{"start_date": {"2030-04-14"}, "end_date": {"2030-04-21"}, "message": {"Easter break"}}), "/programmes/"+id)
		if b := body(c.get("/programmes/" + id)); !strings.Contains(b, "Easter break") {
			t.Error("closure not displayed")
		}
		var cid int64
		app.db.QueryRow(`SELECT id FROM programme_closures`).Scan(&cid)
		wantRedirect(t, c.post("/programmes/"+id+"/closures/"+strconv.FormatInt(cid, 10)+"/delete", nil), "/programmes/"+id)
		if count(t, app.db, `SELECT COUNT(*) FROM programme_closures`) != 0 {
			t.Error("closure not deleted")
		}
	})

	t.Run("weekend notes", func(t *testing.T) {
		wantStatus(t, c.post("/programmes/"+id+"/weekend-notes", url.Values{"date": {"nope"}, "text": {"x"}}), 400)
		wantRedirect(t, c.post("/programmes/"+id+"/weekend-notes", url.Values{"date": {"2030-04-06"}, "text": {"Camp"}}), "/programmes/"+id)
		if count(t, app.db, `SELECT COUNT(*) FROM programme_weekend_notes`) != 1 {
			t.Error("note not stored")
		}
	})

	t.Run("requirements", func(t *testing.T) {
		wantRedirect(t, c.post("/programmes/"+id+"/requirements", url.Values{"name": {"Subs"}, "due_label": {"due"}}), "/programmes/"+id)
		var cat int64
		app.db.QueryRow(`SELECT id FROM programme_requirement_categories`).Scan(&cat)
		catS := strconv.FormatInt(cat, 10)
		wantRedirect(t, c.post("/programmes/"+id+"/requirements/"+catS+"/entries", url.Values{"cadet_name": {"Smith"}, "note": {"paid"}}), "/programmes/"+id)
		if b := body(c.get("/programmes/" + id)); !strings.Contains(b, "Smith") {
			t.Error("entry not displayed")
		}
	})

	t.Run("pdf export", func(t *testing.T) {
		resp := c.get("/programmes/" + id + "/pdf")
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/pdf" {
			t.Fatalf("%d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		if cd := resp.Header.Get("Content-Disposition"); cd != `inline; filename="April_2030_Programme.pdf"` {
			t.Errorf("Content-Disposition = %q", cd)
		}
		b := []byte(body(resp))
		if !bytes.HasPrefix(b, []byte("%PDF-")) || len(b) < 1000 {
			t.Errorf("not a plausible PDF (%d bytes)", len(b))
		}
	})

	t.Run("bad IDs", func(t *testing.T) {
		for _, p := range []string{"/programmes/abc", "/programmes/0", "/programmes/-1", "/programmes/99999999999999999999", "/programmes/abc/pdf"} {
			resp := c.get(p)
			if resp.StatusCode != 404 {
				t.Errorf("GET %s -> %d, want 404", p, resp.StatusCode)
			}
			resp.Body.Close()
		}
	})
}

func TestSquadronSettingsPage(t *testing.T) {
	app := newApp(t)
	email := app.owner("1002")
	c := app.login(email)

	wantRedirect(t, c.post("/squadron", url.Values{
		"name": {"Renamed Sqn"}, "subtitle": {"Sub"}, "protective_marking": {"OFFICIAL"},
		"classifications": {"Alpha\n\n  Bravo  \r\nCharlie"}, "parade_weekdays": {"2", "5", "x", "9"},
	}), "/squadron")
	sq, _ := app.store.GetSquadron(app.squadronID(email))
	if sq.Name != "Renamed Sqn" || sq.ProtectiveMarking != "OFFICIAL" {
		t.Errorf("settings not saved: %+v", sq)
	}
	if got := strings.Join(sq.Classifications, "|"); got != "Alpha|Bravo|Charlie" {
		t.Errorf("classifications = %q (blank lines/whitespace should be trimmed)", got)
	}

	if got := sq.ParadeWeekdays; len(got) != 2 || got[0] != 2 || got[1] != 5 {
		t.Errorf("parade weekdays = %v, want [2 5] (non-numeric and out-of-range values dropped)", got)
	}
	wantRedirect(t, c.post("/squadron", url.Values{"name": {"   "}}), "/squadron")
	if sq2, _ := app.store.GetSquadron(app.squadronID(email)); sq2.Name != "Renamed Sqn" {
		t.Errorf("blank name overwrote the squadron name: %q", sq2.Name)
	}

	// Blank classification list must not wipe the existing ones (the PDF
	// grid needs at least one column).
	wantRedirect(t, c.post("/squadron", url.Values{"name": {"Renamed Sqn"}, "classifications": {"  \n "}}), "/squadron")
	sq, _ = app.store.GetSquadron(app.squadronID(email))
	if len(sq.Classifications) != 3 {
		t.Errorf("classifications wiped: %v", sq.Classifications)
	}
	if b := body(c.get("/squadron")); !strings.Contains(b, "Renamed Sqn") {
		t.Error("settings page does not show the saved name")
	}
}

// Changing the classification list used to leave existing months with headings
// and text boxes that no longer lined up.
func TestChangingClassificationsKeepsExistingMonthsConsistent(t *testing.T) {
	app := newApp(t)
	email := app.owner("1601")
	c := app.login(email)
	sq := app.squadronID(email)
	p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	pid := strconv.FormatInt(p.ID, 10)
	app.db.Exec(`UPDATE programme_rows SET texts = '["Drill","","",""]' WHERE id = (SELECT MIN(id) FROM programme_rows)`)

	inputsPerRow := func() int {
		page := body(c.get("/programmes/" + pid))
		first := strings.Index(page, `name="row_`)
		end := strings.Index(page[first:], "</tr>")
		return strings.Count(page[first:first+end], `name="row_`)
	}
	headings := func() int {
		page := body(c.get("/programmes/" + pid))
		return len(regexp.MustCompile(`<th[ >]`).FindAllString(page[strings.Index(page, "<thead>"):strings.Index(page, "</thead>")], -1)) - 5
	}

	wantRedirect(t, c.post("/squadron", url.Values{"name": {"N"}, "classifications": {"A\nB\nC\nD\nE\nF"}}), "/squadron")
	if inputsPerRow() != 6 || headings() != 6 {
		t.Errorf("after growing to 6: %d boxes, %d headings", inputsPerRow(), headings())
	}
	wantStatus(t, c.get("/programmes/"+pid+"/pdf"), 200)

	// Shrinking over empty boxes works; over typed text it is refused with the reason.
	wantRedirect(t, c.post("/squadron", url.Values{"name": {"N"}, "classifications": {"A\nB\nC\nD"}}), "/squadron")
	if inputsPerRow() != 4 {
		t.Errorf("after shrinking to 4: %d boxes", inputsPerRow())
	}
	app.db.Exec(`UPDATE programme_rows SET texts = '["x","y","z","Senior text"]' WHERE id = (SELECT MIN(id) FROM programme_rows)`)
	resp := c.post("/squadron", url.Values{"name": {"N"}, "classifications": {"A\nB"}})
	page := body(resp)
	if resp.StatusCode != 400 || !strings.Contains(page, "still have text") || !strings.Contains(page, "D") {
		t.Errorf("refusal: %d\n%.400s", resp.StatusCode, page)
	}
	if strings.Contains(page, "invalid input:") {
		t.Error("internal error prefix leaked")
	}
	if got, _ := app.store.GetSquadron(sq); len(got.Classifications) != 4 {
		t.Errorf("classification list changed despite the refusal: %v", got.Classifications)
	}
}

func TestNewMonthCanCopyLastMonthsCategoryNames(t *testing.T) {
	app := newApp(t)
	email := app.owner("1601")
	sq := app.squadronID(email)
	c := app.login(email)
	prev, _ := app.store.CreateProgramme(sq, 2031, 5, 1)
	cat, _ := app.store.AddRequirementCategory(prev.ID, "Fieldcraft", "by June")
	app.store.AddRequirementEntry(prev.ID, cat, "Cdt Private Name", "note")

	if page := body(c.get("/")); !strings.Contains(page, `name="copy_categories"`) {
		t.Error("the New month form has no copy option")
	}

	resp := c.post("/programmes", url.Values{"year": {"2031"}, "month": {"6"}, "copy_categories": {"1"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	page := body(c.get(strings.TrimPrefix(resp.Header.Get("Location"), "")))
	if !strings.Contains(page, "Fieldcraft") || strings.Contains(page, "Cdt Private Name") {
		t.Errorf("copied page should list the category but not the cadet:\n%s", page)
	}

	// Left unticked, nothing is copied.
	resp = c.post("/programmes", url.Values{"year": {"2031"}, "month": {"7"}})
	if page := body(c.get(resp.Header.Get("Location"))); strings.Contains(page, "Fieldcraft") {
		t.Error("categories were copied although the box was not ticked")
	}
}
