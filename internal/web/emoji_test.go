package web

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"rafactools-planner/internal/programme"
)

// Smileys and other emoji (four bytes in UTF-8, some made of several joined
// characters) must work in every text box: saved whole, shown back whole, and
// the PDF still produced.
const fun = "Fun 😀 night 🎉 Zoë 👨‍👩‍👧 👍🏽 ❤️"

func TestEmojiWorkInEveryTextField(t *testing.T) {
	app := newApp(t)
	email := app.owner("2401")
	c := app.login(email)
	sq := app.squadronID(email)

	// Squadron settings: name, subtitle, classification names.
	wantRedirect(t, c.post("/squadron", url.Values{"name": {"Sqn " + fun}, "subtitle": {"Sub " + fun}, "protective_marking": {"OFFICIAL"},
		"parade_weekdays": {"1", "2", "3", "4", "5"}, "classifications": {"Junior " + "😀" + "\nSecond 🎉\nThird\nFourth"}}), "/squadron")
	got, _ := app.store.GetSquadron(sq)
	if got.Name != "Sqn "+fun || got.Subtitle != "Sub "+fun || got.Classifications[0] != "Junior 😀" || got.Classifications[1] != "Second 🎉" {
		t.Errorf("squadron settings stored as %+v", got)
	}

	// Your name, and an instructor's name and rank.
	wantRedirect(t, c.post("/account", url.Values{"display_name": {"Me " + fun}}), "/account?saved=1")
	wantRedirect(t, c.post("/instructors", url.Values{"rank": {"Sgt 😀"}, "name": {"Smith 🎉"}}), "/instructors")

	// Programme: boxes in every classification, a night's notes, closure, weekend note, tracker.
	p, _ := app.store.CreateProgramme(sq, 2038, 3, 1)
	pid := strconv.FormatInt(p.ID, 10)
	fp, _ := app.store.GetFullProgramme(sq, p.ID)
	night := firstDay(fp, true)
	form := gridForm(t, app, sq, p.ID, nil)
	pre := "day_" + strconv.FormatInt(night.ID, 10) + "_"
	form.Set(pre+"notes", "Notes "+fun)
	for _, r := range night.Rows {
		for i := range r.Texts {
			form.Set("row_"+strconv.FormatInt(r.ID, 10)+"_text_"+strconv.Itoa(i), "Box "+strconv.Itoa(i)+" "+fun)
		}
	}
	wantRedirect(t, c.post("/programmes/"+pid, form), "/programmes/"+pid)
	wantRedirect(t, c.post("/programmes/"+pid+"/closures", url.Values{"start_date": {"2038-03-20"}, "end_date": {"2038-03-21"}, "message": {"Closed " + fun}}), "/programmes/"+pid)
	wantRedirect(t, c.post("/programmes/"+pid+"/weekend-notes", url.Values{"date": {"2038-03-06"}, "text": {"Camp " + fun}}), "/programmes/"+pid)
	wantRedirect(t, c.post("/programmes/"+pid+"/requirements", url.Values{"name": {"Cat " + fun}, "due_label": {"Due " + fun}}), "/programmes/"+pid)
	cat := int64(0)
	app.db.QueryRow(`SELECT id FROM programme_requirement_categories`).Scan(&cat)
	wantRedirect(t, c.post("/programmes/"+pid+"/requirements/"+strconv.FormatInt(cat, 10)+"/entries", url.Values{"cadet_name": {"Cdt " + fun}, "note": {"Note " + fun}}), "/programmes/"+pid)

	back, _ := app.store.GetFullProgramme(sq, p.ID)
	var nb programme.Day
	for _, d := range back.Days {
		if d.ID == night.ID {
			nb = d
		}
	}
	act, _ := nb.RowByHalfType(1, "activity")
	if nb.Notes != "Notes "+fun || act.Texts[0] != "Box 0 "+fun || act.Texts[3] != "Box 3 "+fun {
		t.Errorf("night: notes=%q boxes=%q", nb.Notes, act.Texts)
	}
	if back.Closures[0].Message != "Closed "+fun || back.WeekendNotes[0].Text != "Camp "+fun ||
		back.Requirements[0].Name != "Cat "+fun || back.Requirements[0].DueLabel != "Due "+fun ||
		back.Requirements[0].Entries[0].CadetName != "Cdt "+fun || back.Requirements[0].Entries[0].Note != "Note "+fun {
		t.Errorf("closure/weekend/tracker not stored whole: %+v", back)
	}

	// The pages show them back whole.
	page := body(c.get("/programmes/" + pid))
	for _, want := range []string{"Box 0 " + fun, "Notes " + fun, "Cat " + fun, "Cdt " + fun} {
		if !strings.Contains(page, want) {
			t.Errorf("the programme page lacks %q", want)
		}
	}
	for path, want := range map[string]string{"/instructors": "Sgt 😀 Smith 🎉", "/account": "Me " + fun, "/": "Sqn " + fun} {
		if !strings.Contains(body(c.get(path)), want) {
			t.Errorf("%s does not show %q", path, want)
		}
	}

	// The PDF is produced, and so is the history and the CSV.
	wantStatus(t, c.get("/programmes/"+pid+"/pdf"), 200)
	wantStatus(t, c.get("/programmes/"+pid+"/history"), 200)
	wantStatus(t, c.get("/squadron/security-log.csv"), 200)
}

func TestEmojiInInvitesReportsAndReferences(t *testing.T) {
	app := newApp(t, withReports(true))
	email := app.owner("2402")
	c := app.login(email)

	postReport(t, c, map[string]string{"description": "It broke 😀 " + fun, "expected": "Should work 🎉", "category": "bug"}, nil)
	rep, _ := app.mail.lastReport(t)
	if !strings.Contains(rep.Body, "It broke 😀 "+fun) || !strings.Contains(rep.Body, "Should work 🎉") {
		t.Errorf("the report lost its emoji:\n%s", rep.Body)
	}
	if !strings.Contains(rep.Subject, "😀") {
		t.Errorf("subject = %q", rep.Subject)
	}
}
