package web

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestInstructorsRequireEditorOrOwner(t *testing.T) {
	app := newApp(t)
	ownerEmail := app.owner("1501")
	app.member(ownerEmail, "ed@example.org", "editor")
	app.member(ownerEmail, "vi@example.org", "viewer")

	wantRedirect(t, app.anon().get("/instructors"), "/login")
	wantRedirect(t, app.anon().postRaw("/instructors", url.Values{}), "/login")

	viewer := app.login("vi@example.org")
	wantStatus(t, viewer.get("/instructors"), 403)
	wantStatus(t, viewer.post("/instructors", url.Values{"name": {"Smith"}}), 403)
	wantStatus(t, viewer.post("/instructors/1/delete", nil), 403)
	if n := count(t, app.db, `SELECT COUNT(*) FROM instructors`); n != 0 {
		t.Fatalf("a viewer created %d instructors", n)
	}
	// the nav link is only shown to people who can use it
	if strings.Contains(body(viewer.get("/")), `href="/instructors"`) {
		t.Error("viewer sees an Instructors link they cannot use")
	}
	if !strings.Contains(body(app.login("ed@example.org").get("/")), `href="/instructors"`) {
		t.Error("editor has no Instructors link")
	}
	wantStatus(t, app.login("ed@example.org").get("/instructors"), 200)
}

func TestInstructorListFlowAndGridSuggestions(t *testing.T) {
	app := newApp(t)
	email := app.owner("1502")
	c := app.login(email)
	sq := app.squadronID(email)
	prog, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	editURL := "/programmes/" + strconv.FormatInt(prog.ID, 10)

	wantRedirect(t, c.post("/instructors", url.Values{"rank": {"Fg Off"}, "name": {"Smith"}}), "/instructors")
	wantRedirect(t, c.post("/instructors", url.Values{"name": {"Jones"}}), "/instructors")
	page := body(c.get("/instructors"))
	if !strings.Contains(page, "Fg Off Smith") || !strings.Contains(page, "Jones") {
		t.Errorf("list page does not show the people:\n%.800s", page)
	}

	grid := body(c.get(editURL))
	for _, want := range []string{`<option value="All Staff">`, `<option value="Fg Off Smith">`, `<option value="Jones">`, `<option value="` + email + `">`} {
		if !strings.Contains(grid, want) {
			t.Errorf("grid suggestions lack %s", want)
		}
	}
	if strings.Count(grid, `<option value="All Staff">`) != 1 {
		t.Error("All Staff listed more than once")
	}

	// A name that matches a member's display name is only suggested once.
	wantRedirect(t, c.post("/account", url.Values{"display_name": {"Jones"}}), "/account?saved=1")
	grid = body(c.get(editURL))
	if strings.Count(grid, `<option value="Jones">`) != 1 {
		t.Errorf("duplicate suggestion for Jones: %d", strings.Count(grid, `<option value="Jones">`))
	}

	var id int64
	app.db.QueryRow(`SELECT id FROM instructors WHERE name = 'Smith'`).Scan(&id)
	wantRedirect(t, c.post("/instructors/"+strconv.FormatInt(id, 10)+"/delete", nil), "/instructors")
	if strings.Contains(body(c.get(editURL)), "Fg Off Smith") {
		t.Error("removed instructor is still suggested")
	}
}

func TestInstructorValidationAndEscaping(t *testing.T) {
	app := newApp(t)
	c := app.login(app.owner("1503"))

	for name, form := range map[string]url.Values{
		"no name":       {"name": {""}},
		"name too long": {"name": {strings.Repeat("a", 81)}},
		"rank too long": {"name": {"x"}, "rank": {strings.Repeat("r", 41)}},
		"control char":  {"name": {"bad\x00name"}},
	} {
		resp := c.post("/instructors", form)
		page := body(resp)
		if resp.StatusCode != 400 || !strings.Contains(page, `class="error"`) {
			t.Errorf("%s: status %d", name, resp.StatusCode)
		}
		if strings.Contains(page, "invalid input:") {
			t.Errorf("%s: internal error prefix leaked into the message", name)
		}
	}
	wantRedirect(t, c.post("/instructors", url.Values{"name": {"Smith"}}), "/instructors")
	resp := c.post("/instructors", url.Values{"name": {"Smith"}})
	if page := body(resp); resp.StatusCode != 400 || !strings.Contains(page, "already in the list") {
		t.Errorf("duplicate: %d", resp.StatusCode)
	}

	// Markup in a name is shown as text, in the page and in the datalist.
	payload := `<script>alert(1)</script>"><img src=x onerror=alert(2)>`
	if resp := c.post("/instructors", url.Values{"name": {payload}}); resp.StatusCode != 303 {
		t.Fatalf("payload rejected: %d", resp.StatusCode)
	}
	prog, _ := app.store.CreateProgramme(app.squadronID("oc.1503@rafac.mod.gov.uk"), 2030, 4, 1)
	for _, path := range []string{"/instructors", "/programmes/" + strconv.FormatInt(prog.ID, 10)} {
		page := body(c.get(path))
		if strings.Contains(page, "<script>alert(1)") || strings.Contains(page, "<img src=x") {
			t.Errorf("%s renders an instructor name unescaped", path)
		}
	}
}

func TestInstructorsAreTenantScoped(t *testing.T) {
	ts := seedTwoSquadrons(t)
	app := ts.a
	wantRedirect(t, ts.ownerA.post("/instructors", url.Values{"name": {"Alpha Only"}}), "/instructors")
	var id int64
	app.db.QueryRow(`SELECT id FROM instructors`).Scan(&id)

	if strings.Contains(body(ts.ownerB.get("/instructors")), "Alpha Only") {
		t.Error("B can see A's instructors")
	}
	wantStatus(t, ts.ownerB.post("/instructors/"+strconv.FormatInt(id, 10)+"/delete", nil), 404)
	if count(t, app.db, `SELECT COUNT(*) FROM instructors`) != 1 {
		t.Error("B deleted A's instructor")
	}
	pidB := ts.progB
	if strings.Contains(body(ts.ownerB.get("/programmes/"+pidB)), "Alpha Only") {
		t.Error("A's instructor is suggested in B's grid")
	}
	// forged form token
	resp := ts.ownerA.postRaw("/instructors", url.Values{"csrf": {"forged"}, "name": {"Evil"}})
	wantStatus(t, resp, 400)
}

func TestAccountDisplayName(t *testing.T) {
	app := newApp(t)
	ownerEmail := app.owner("1504")
	app.member(ownerEmail, "vi@example.org", "viewer")
	owner, viewer := app.login(ownerEmail), app.login("vi@example.org")

	wantStatus(t, app.anon().get("/account"), 303)
	// everyone signed in can set their own name, viewers included
	wantRedirect(t, viewer.post("/account", url.Values{"display_name": {"  Cdt   Smith "}}), "/account?saved=1")
	var stored string
	app.db.QueryRow(`SELECT display_name FROM users WHERE email = 'vi@example.org'`).Scan(&stored)
	if stored != "Cdt Smith" {
		t.Errorf("stored %q, want whitespace tidied", stored)
	}
	if !strings.Contains(body(viewer.get("/account?saved=1")), "Saved.") || !strings.Contains(body(viewer.get("/")), "Cdt Smith (viewer)") {
		t.Error("name not shown back to the person")
	}
	// the owner sees it in the members table with the email alongside
	if members := body(owner.get("/squadron")); !strings.Contains(members, "Cdt Smith") || !strings.Contains(members, "vi@example.org") {
		t.Error("members table does not show name and email")
	}
	// it only changes the person's own record
	var other string
	app.db.QueryRow(`SELECT display_name FROM users WHERE email = ?`, ownerEmail).Scan(&other)
	if other != "" {
		t.Errorf("another user's name changed to %q", other)
	}

	for name, v := range map[string]string{"too long": strings.Repeat("n", 81), "control char": "bad\x01name"} {
		resp := viewer.post("/account", url.Values{"display_name": {v}})
		if resp.StatusCode != 400 {
			t.Errorf("%s: status %d", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
	// blank clears it, falling back to the email
	wantRedirect(t, viewer.post("/account", url.Values{"display_name": {""}}), "/account?saved=1")
	if !strings.Contains(body(viewer.get("/")), "vi@example.org (viewer)") {
		t.Error("blank name did not fall back to the email")
	}
	wantStatus(t, viewer.postRaw("/account", url.Values{"csrf": {"forged"}, "display_name": {"X"}}), 400)
}

func TestChangePasswordFromYourDetails(t *testing.T) {
	app := newApp(t)
	email := app.owner("1505")
	c := app.login(email)
	other := app.login(email) // a second session of the same user

	if !strings.Contains(body(c.get("/account")), `action="/account/password"`) {
		t.Fatal("no change-password form on Your details")
	}
	t.Run("wrong current password", func(t *testing.T) {
		resp := c.post("/account/password", url.Values{"current_password": {"nope nope nope"}, "new_password": {"a brand new password"}})
		page := body(resp)
		if resp.StatusCode != 400 || !strings.Contains(page, "current password is not correct") {
			t.Errorf("%d", resp.StatusCode)
		}
	})
	t.Run("weak new password", func(t *testing.T) {
		for _, pw := range []string{"short", strings.Repeat("a", 73)} {
			resp := c.post("/account/password", url.Values{"current_password": {testPassword}, "new_password": {pw}})
			if page := body(resp); resp.StatusCode != 400 || !strings.Contains(page, "password must be") {
				t.Errorf("%d-byte: %d", len(pw), resp.StatusCode)
			}
		}
	})
	t.Run("form token required", func(t *testing.T) {
		wantStatus(t, c.postRaw("/account/password", url.Values{"csrf": {"forged"}, "current_password": {testPassword}, "new_password": {"a brand new password"}}), 400)
	})
	wantStatus(t, app.anon().postRaw("/account/password", url.Values{}), 303)

	wantRedirect(t, c.post("/account/password", url.Values{"current_password": {testPassword}, "new_password": {"a brand new password"}}), "/account?changed=1")
	wantStatus(t, c.get("/account?changed=1"), 200) // still signed in here
	wantRedirect(t, other.get("/"), "/login")       // signed out elsewhere

	cl := app.anon()
	lt := scrapeCSRF(t, cl.get("/login"))
	wantStatus(t, cl.postRaw("/login", url.Values{"csrf": {lt}, "email": {email}, "password": {testPassword}}), 401)
	cl2 := app.anon()
	lt2 := scrapeCSRF(t, cl2.get("/login"))
	wantRedirect(t, cl2.postRaw("/login", url.Values{"csrf": {lt2}, "email": {email}, "password": {"a brand new password"}}), "/")
}

func TestChangePasswordSharesTheAuthRateLimit(t *testing.T) {
	app := newApp(t)
	c := app.login(app.owner("1506"))
	c.xff = "203.0.113.200"
	last := 0
	for i := 0; i < 25; i++ {
		resp := c.post("/account/password", url.Values{"current_password": {"wrong wrong wrong"}, "new_password": {"a brand new password"}})
		last = resp.StatusCode
		resp.Body.Close()
	}
	if last != 429 {
		t.Errorf("25th wrong-password attempt -> %d, want 429 (guessing the current password must be throttled)", last)
	}
}

func TestRenameInstructorFlowAndPermissions(t *testing.T) {
	app := newApp(t)
	email := app.owner("1503")
	c := app.login(email)
	app.member(email, "vi-ren@example.org", "viewer")
	wantRedirect(t, c.post("/instructors", url.Values{"rank": {"Fg Off"}, "name": {"Smith"}}), "/instructors")
	wantRedirect(t, c.post("/instructors", url.Values{"name": {"Jones"}}), "/instructors")
	var id int64
	app.db.QueryRow(`SELECT id FROM instructors WHERE name = 'Smith'`).Scan(&id)
	edit := "/instructors/" + strconv.FormatInt(id, 10) + "/edit"

	if !strings.Contains(body(c.get("/instructors")), edit) {
		t.Error("the list has no Edit link")
	}
	form := body(c.get(edit))
	if !strings.Contains(form, `value="Smith"`) || !strings.Contains(form, `value="Fg Off"`) {
		t.Errorf("edit form is not filled in:\n%s", form)
	}

	wantRedirect(t, c.post(edit, url.Values{"rank": {"Flt Lt"}, "name": {"Smythe"}}), "/instructors")
	if page := body(c.get("/instructors")); !strings.Contains(page, "Flt Lt Smythe") || strings.Contains(page, "Fg Off Smith") {
		t.Errorf("rename not shown:\n%s", page)
	}

	// A clash re-shows the form with the reason and what was typed.
	resp := c.post(edit, url.Values{"name": {"Jones"}})
	if page := body(resp); resp.StatusCode != 400 || !strings.Contains(page, "already in the list") || !strings.Contains(page, `value="Jones"`) {
		t.Errorf("duplicate rename = %d:\n%s", resp.StatusCode, page)
	}

	// Viewers and other squadrons cannot reach it.
	wantStatus(t, app.login("vi-ren@example.org").get(edit), 403)
	wantStatus(t, app.login("vi-ren@example.org").post(edit, url.Values{"name": {"X"}}), 403)
	other := app.login(app.owner("1504"))
	wantStatus(t, other.get(edit), 404)
	wantStatus(t, other.post(edit, url.Values{"name": {"Hijacked"}}), 404)
	if n := count(t, app.db, `SELECT COUNT(*) FROM instructors WHERE name = 'Hijacked'`); n != 0 {
		t.Error("another squadron renamed this squadron's instructor")
	}
	wantStatus(t, c.postRaw(edit, url.Values{"name": {"NoToken"}}), 400)
}
