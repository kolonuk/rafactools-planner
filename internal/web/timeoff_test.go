package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"rafactools-planner/internal/programme"
)

func TestTimeOffIsSharedWarnsInTheGridAndStaysOutOfFiles(t *testing.T) {
	app := newApp(t)
	owner := app.owner("1501")
	app.member(owner, "editor1501@example.org", "editor")
	app.member(owner, "viewer1501@example.org", "viewer")
	sq := app.squadronID(owner)
	setClock(t, time.Date(2030, 4, 1, 9, 0, 0, 0, time.UTC))
	o, v := app.login(owner), app.login("viewer1501@example.org")

	t.Run("a viewer adds their own time off and everyone sees it", func(t *testing.T) {
		wantRedirect(t, v.post("/time-off", url.Values{"name": {"Someone Else"}, "start_date": {"2030-04-08"}, "end_date": {"2030-04-12"}, "note": {"exams"}}), "/time-off")
		page := body(o.get("/time-off"))
		if !strings.Contains(page, "viewer1501@example.org") || !strings.Contains(page, "exams") {
			t.Errorf("the owner does not see the viewer's time off:\n%.600s", page)
		}
		if strings.Contains(page, "Someone Else") {
			t.Error("a viewer added time off for another person")
		}
	})

	t.Run("an owner adds for anyone and a viewer cannot change it", func(t *testing.T) {
		wantRedirect(t, o.post("/time-off", url.Values{"name": {"Fg Off Smith"}, "start_date": {"2030-04-08"}, "end_date": {"2030-04-08"}, "note": {"course"}}), "/time-off")
		list, _ := app.store.ListAbsences(sq, "")
		var smith programme.Absence
		for _, a := range list {
			if a.Name == "Fg Off Smith" {
				smith = a
			}
		}
		if smith.ID == 0 {
			t.Fatal("not saved")
		}
		id := strconv.FormatInt(smith.ID, 10)
		wantStatus(t, v.post("/time-off/"+id+"/delete", url.Values{}), http.StatusForbidden)
		wantStatus(t, v.post("/time-off/"+id+"/edit", url.Values{"start_date": {"2030-04-08"}, "end_date": {"2030-04-09"}}), http.StatusForbidden)
		stranger := app.login(app.owner("1502"))
		wantStatus(t, stranger.post("/time-off/"+id+"/delete", url.Values{}), http.StatusNotFound)
		wantRedirect(t, o.post("/time-off/"+id+"/edit", url.Values{"start_date": {"2030-04-08"}, "end_date": {"2030-04-09"}, "note": {"course, 2 days"}}), "/time-off")
		if got, _ := app.store.GetAbsence(sq, smith.ID); got.End != "2030-04-09" || got.Note != "course, 2 days" {
			t.Errorf("edit = %+v", got)
		}
		wantStatus(t, o.post("/time-off", url.Values{"name": {"X"}, "start_date": {"2030-04-09"}, "end_date": {"2030-04-08"}}), http.StatusBadRequest)
		if page := body(app.login(app.owner("1503")).get("/time-off")); strings.Contains(page, "Smith") {
			t.Error("another squadron sees this squadron's time off")
		}
	})

	t.Run("the grid warns about an instructor who is away, and only on those nights", func(t *testing.T) {
		p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
		pid := strconv.FormatInt(p.ID, 10)
		parade, parade2, _ := firstParadeAndPlain(t, app, sq, p.ID)
		ins := func(d programme.Day, text string) programme.DayInput {
			return programme.DayInput{ID: d.ID, IsParade: true, Rows: []programme.RowInput{{ID: d.Rows[0].ID, Texts: []string{"Drill", "", "", ""}}, {ID: d.Rows[1].ID, Texts: []string{text, "", "", ""}}}}
		}
		if err := app.store.SaveDaysAndRows(sq, p.ID, []programme.DayInput{ins(parade, "Fg Off Smith"), ins(parade2, "Fg Off Smith")}); err != nil {
			t.Fatal(err)
		}
		// Smith is away on the first night only.
		if err := app.store.AddAbsence(sq, "Fg Off Smith", 0, 1, parade.Date, parade.Date, "away that night"); err != nil {
			t.Fatal(err)
		}
		page := body(o.get("/programmes/" + pid))
		if got := strings.Count(page, `class="ins away"`); got != 1 {
			t.Fatalf("%d boxes are red, want 1 (nights %s and %s):\n%.500s", got, parade.Date, parade2.Date, page)
		}
		if !strings.Contains(page, "Fg Off Smith is away") {
			t.Error("the warning text is missing")
		}
		// A different name, or the same name with other letters, is not flagged.
		if err := app.store.SaveDaysAndRows(sq, p.ID, []programme.DayInput{ins(parade, "Smithson"), ins(parade2, "Fg Off Smith")}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(body(o.get("/programmes/"+pid)), `class="ins away"`); got != 0 {
			t.Errorf("%d boxes are red with nobody away", got)
		}
	})

	t.Run("it is not in the PDF, the PNG or the calendar", func(t *testing.T) {
		list, _ := app.store.ListProgrammes(sq)
		pid := strconv.FormatInt(list[0].ID, 10)
		wantRedirect(t, o.post("/programmes/"+pid+"/publish", url.Values{"published": {"1"}}), "/programmes/"+pid)
		share, _ := app.store.GetShare(sq)
		anon := app.anon()
		for _, path := range []string{"/share/" + share.Token + "/calendar.ics", "/share/" + share.Token + "/2030-04.pdf", "/share/" + share.Token + "/2030-04.png", "/programmes/" + pid + "/pdf", "/programmes/" + pid + "/png"} {
			resp := anon.get(path)
			if strings.HasPrefix(path, "/programmes") {
				resp = o.get(path)
			}
			b := body(resp)
			if strings.Contains(b, "course") || strings.Contains(b, "away that night") || strings.Contains(b, "exams") || strings.Contains(b, "is away") {
				t.Errorf("%s shows time off", path)
			}
		}
	})
}
