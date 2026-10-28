package web

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"rafactools-planner/internal/programme"
)

func firstParadeAndPlain(t *testing.T, app *testApp, sq, pid int64) (parade, parade2, plain programme.Day) {
	t.Helper()
	fp, err := app.store.GetFullProgramme(sq, pid)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range fp.Days {
		switch {
		case d.IsParade && n == 0:
			parade, n = d, 1
		case d.IsParade && n == 1:
			parade2, n = d, 2
		case !d.IsParade && plain.ID == 0:
			plain = d
		}
	}
	return
}

func rowText(t *testing.T, app *testApp, sq, pid, dayID int64) string {
	t.Helper()
	fp, _ := app.store.GetFullProgramme(sq, pid)
	for _, d := range fp.Days {
		if d.ID == dayID {
			if r, ok := d.RowByHalfType(1, "activity"); ok && len(r.Texts) > 0 {
				return r.Texts[0]
			}
		}
	}
	return ""
}

func TestMoveActivitiesEndpoint(t *testing.T) {
	app := newApp(t)
	email := app.owner("1201")
	sq := app.squadronID(email)
	c := app.login(email)
	p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	pid := strconv.FormatInt(p.ID, 10)
	parade, parade2, plain := firstParadeAndPlain(t, app, sq, p.ID)
	app.store.SaveDaysAndRows(sq, p.ID, []programme.DayInput{{ID: parade.ID, IsParade: true, Rows: []programme.RowInput{{ID: parade.Rows[0].ID, Texts: []string{"Drill", "", "", ""}}}}})
	path := func(day int64) string { return "/programmes/" + pid + "/days/" + strconv.FormatInt(day, 10) + "/move" }
	id := func(d int64) string { return strconv.FormatInt(d, 10) }

	t.Run("moves onto a non-parade date", func(t *testing.T) {
		wantRedirect(t, c.post(path(parade.ID), url.Values{"target_day": {id(plain.ID)}}), "/programmes/"+pid)
		if rowText(t, app, sq, p.ID, plain.ID) != "Drill" || rowText(t, app, sq, p.ID, parade.ID) != "" {
			t.Error("activities did not move")
		}
		if !strings.Contains(body(c.get("/programmes/"+pid)), `value="Drill"`) {
			t.Error("edit page does not show the moved activity")
		}
	})

	t.Run("swaps with a night that has activities", func(t *testing.T) {
		app.store.SaveDaysAndRows(sq, p.ID, []programme.DayInput{{ID: parade2.ID, IsParade: true, Rows: []programme.RowInput{{ID: parade2.Rows[0].ID, Texts: []string{"Shooting", "", "", ""}}}}})
		wantRedirect(t, c.post(path(parade2.ID), url.Values{"target_day": {id(plain.ID)}}), "/programmes/"+pid)
		if rowText(t, app, sq, p.ID, plain.ID) != "Shooting" || rowText(t, app, sq, p.ID, parade2.ID) != "Drill" {
			t.Errorf("not swapped: plain=%q parade2=%q", rowText(t, app, sq, p.ID, plain.ID), rowText(t, app, sq, p.ID, parade2.ID))
		}
	})

	t.Run("bad requests", func(t *testing.T) {
		for name, form := range map[string]url.Values{
			"no target":   {},
			"non-numeric": {"target_day": {"abc"}},
			"same day":    {"target_day": {id(parade2.ID)}},
			"nonexistent": {"target_day": {"999999"}},
		} {
			resp := c.post(path(parade2.ID), form)
			if resp.StatusCode != 400 && resp.StatusCode != 404 {
				t.Errorf("%s: status %d, want 400/404", name, resp.StatusCode)
			}
			resp.Body.Close()
		}
		// A night that isn't a parade night has nothing to move.
		wantStatus(t, c.post(path(parade.ID+1000), url.Values{"target_day": {id(parade2.ID)}}), 404)
	})

	t.Run("form token is required", func(t *testing.T) {
		resp := c.postRaw(path(parade2.ID), url.Values{"csrf": {"forged"}, "target_day": {id(parade.ID)}})
		if resp.StatusCode != 400 {
			t.Errorf("status %d, want 400", resp.StatusCode)
		}
		resp.Body.Close()
		if rowText(t, app, sq, p.ID, parade2.ID) != "Drill" {
			t.Error("a forged request moved data")
		}
	})
}

func TestMoveActivitiesIsEditorOnlyAndTenantScoped(t *testing.T) {
	ts := seedTwoSquadrons(t)
	app := ts.a
	sqA := app.squadronID(ts.ownerAEmail)
	pidA, _ := strconv.ParseInt(ts.progA, 10, 64)
	parade, parade2, _ := firstParadeAndPlain(t, app, sqA, pidA)
	app.store.SaveDaysAndRows(sqA, pidA, []programme.DayInput{{ID: parade.ID, IsParade: true, Rows: []programme.RowInput{{ID: parade.Rows[0].ID, Texts: []string{"Secret plan", "", "", ""}}}}})
	path := "/programmes/" + ts.progA + "/days/" + strconv.FormatInt(parade.ID, 10) + "/move"
	form := func() url.Values { return url.Values{"target_day": {strconv.FormatInt(parade2.ID, 10)}} }

	// Another squadron's owner cannot move A's activities, with A's ids...
	wantStatus(t, ts.ownerB.post(path, form()), 404)
	// ...nor by pointing their own programme id at A's days.
	own := "/programmes/" + ts.progB + "/days/" + strconv.FormatInt(parade.ID, 10) + "/move"
	wantStatus(t, ts.ownerB.post(own, form()), 404)
	if rowText(t, app, sqA, pidA, parade.ID) != "Secret plan" {
		t.Fatal("another squadron moved data")
	}

	// A viewer of squadron A is refused.
	app.member(ts.ownerAEmail, "viewer@example.org", "viewer")
	wantStatus(t, app.login("viewer@example.org").post(path, form()), 403)
	if rowText(t, app, sqA, pidA, parade.ID) != "Secret plan" {
		t.Fatal("a viewer moved data")
	}
}

var formOpen = regexp.MustCompile(`<form[ >]`)

// Browsers silently drop a <form> nested inside another <form>, which once
// made "Add a second half" submit the whole grid instead. Every control that
// belongs to a separate form must use the form= attribute, and the form it
// names must exist outside the grid form.
func TestEditPageHasNoNestedFormsAndWiredControls(t *testing.T) {
	app := newApp(t)
	email := app.owner("1202")
	sq := app.squadronID(email)
	c := app.login(email)
	p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	page := body(c.get("/programmes/" + strconv.FormatInt(p.ID, 10)))

	start := strings.Index(page, `class="grid-form"`)
	end := strings.Index(page[start:], "</form>")
	if start < 0 || end < 0 {
		t.Fatal("grid form not found")
	}
	gridForm := page[start : start+end]
	if n := len(formOpen.FindAllString(gridForm, -1)); n != 0 {
		t.Errorf("%d <form> element(s) nested inside the grid form", n)
	}

	// Every form="…" reference resolves to a form that exists after the grid.
	rest := page[start+end:]
	refs := regexp.MustCompile(`form="([a-z]+-[0-9]+)"`).FindAllStringSubmatch(gridForm, -1)
	if len(refs) == 0 {
		t.Fatal("no controls reference an external form")
	}
	for _, m := range refs {
		if !strings.Contains(rest, `<form id="`+m[1]+`"`) {
			t.Errorf("control references form %q, which does not exist", m[1])
		}
	}
	for _, want := range []string{`class="drag-handle"`, `<details class="move">`, `id="grid-status"`, `src="/static/js/gen/programme-grid.js"`} {
		if !strings.Contains(page, want) {
			t.Errorf("edit page lacks %s", want)
		}
	}
}

func TestProgrammeScriptIsServedAsJavaScript(t *testing.T) {
	app := newApp(t)
	resp := app.anon().get("/static/js/gen/programme-grid.js")
	b := body(resp)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("%d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	// nosniff is on, so a wrong type would stop it running. Also: it must be
	// a module with no eval / inline-handler tricks the CSP would block.
	if !strings.Contains(b, "export {}") || strings.Contains(b, "eval(") || strings.Contains(b, "new Function") {
		t.Error("compiled script is not a clean module")
	}
}

func TestViewersGetNeitherGridNorScript(t *testing.T) {
	app := newApp(t)
	ownerEmail := app.owner("1203")
	app.member(ownerEmail, "v@example.org", "viewer")
	p, _ := app.store.CreateProgramme(app.squadronID(ownerEmail), 2030, 4, 1)
	page := body(app.login("v@example.org").get("/programmes/" + strconv.FormatInt(p.ID, 10)))
	for _, bad := range []string{"drag-handle", "programme-grid.js", `name="target_day"`} {
		if strings.Contains(page, bad) {
			t.Errorf("viewer page contains %q", bad)
		}
	}
}

// Removing a member who created a programme or sent an invite used to fail
// with a raw foreign-key error, because those rows pointed at the member.
func TestRemoveMemberWhoCreatedAProgrammeAndSentInvites(t *testing.T) {
	app := newApp(t)
	ownerEmail := app.owner("1204")
	app.member(ownerEmail, "ed@example.org", "owner") // an owner, so they may invite
	ed := app.login("ed@example.org")
	ed.post("/programmes", url.Values{"year": {"2030"}, "month": {"4"}}).Body.Close()
	ed.post("/squadron/invites", url.Values{"email": {"pending@example.org"}, "role": {"viewer"}}).Body.Close()
	var edID int64
	app.db.QueryRow(`SELECT id FROM users WHERE email = 'ed@example.org'`).Scan(&edID)

	owner := app.login(ownerEmail)
	wantRedirect(t, owner.post("/squadron/members/"+strconv.FormatInt(edID, 10)+"/remove", nil), "/squadron")

	if count(t, app.db, `SELECT COUNT(*) FROM users WHERE email = 'ed@example.org'`) != 0 {
		t.Error("member not removed")
	}
	if count(t, app.db, `SELECT COUNT(*) FROM programmes`) != 1 {
		t.Error("their programme should stay")
	}
	if count(t, app.db, `SELECT COUNT(*) FROM programmes WHERE created_by IS NULL`) != 1 {
		t.Error("programme still points at a removed user")
	}
	if count(t, app.db, `SELECT COUNT(*) FROM invites WHERE email = 'pending@example.org'`) != 0 {
		t.Error("invites sent by a removed member should be withdrawn")
	}
	wantRedirect(t, ed.get("/"), "/login") // and they are signed out
}
