package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSigningInMakesThisMonthsProgrammeWhenItIsMissing(t *testing.T) {
	app := newApp(t)
	email := app.owner("2301")
	sq := app.squadronID(email)
	setClock(t, time.Date(2036, 6, 15, 12, 0, 0, 0, time.UTC))

	if _, err := app.store.FindProgramme(sq, 2036, 6); err == nil {
		t.Fatal("the month already exists")
	}
	c := app.login(email) // any user signing in
	id, err := app.store.FindProgramme(sq, 2036, 6)
	if err != nil {
		t.Fatalf("signing in did not make June 2036: %v", err)
	}
	list, _ := app.store.ListVersions(sq, id)
	if len(list) != 1 || !strings.Contains(list[0].Action, "automatically") || list[0].UserEmail != email {
		t.Errorf("history = %+v", list)
	}
	if !strings.Contains(body(c.get("/")), "June 2036") {
		t.Error("the dashboard does not list it")
	}
	// Signing in again, and opening the dashboard, make no second one.
	app.login(email)
	c.get("/")
	if n := count(t, app.db, `SELECT COUNT(*) FROM programmes WHERE squadron_id = `+itoaID(sq)); n != 1 {
		t.Errorf("%d programmes, want 1", n)
	}
}

func TestAnyRoleTriggersItAndEarlierMonthsAreNeverMade(t *testing.T) {
	app := newApp(t)
	owner := app.owner("2302")
	sq := app.squadronID(owner)
	app.member(owner, "vi-month@example.org", "viewer")
	setClock(t, time.Date(2036, 9, 3, 12, 0, 0, 0, time.UTC))

	app.login("vi-month@example.org") // a viewer
	if _, err := app.store.FindProgramme(sq, 2036, 9); err != nil {
		t.Errorf("a viewer signing in did not make the month: %v", err)
	}
	if n := count(t, app.db, `SELECT COUNT(*) FROM programmes WHERE year*12+month < 2036*12+9`); n != 0 {
		t.Errorf("%d historic programmes were made", n)
	}
	// A session that lasts across a month end gets the new month when the dashboard opens.
	c := app.login(owner)
	setClock(t, time.Date(2036, 10, 1, 0, 30, 0, 0, time.UTC))
	c.get("/")
	if _, err := app.store.FindProgramme(sq, 2036, 10); err != nil {
		t.Errorf("the dashboard did not make the new month: %v", err)
	}
}

func TestMasterAdminSigningInMakesNoProgramme(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	setClock(t, time.Date(2037, 1, 5, 12, 0, 0, 0, time.UTC))
	before := count(t, ts.a.db, `SELECT COUNT(*) FROM programmes`)
	ts.a.login("master@example.org")
	master.get("/")
	if after := count(t, ts.a.db, `SELECT COUNT(*) FROM programmes`); after != before {
		t.Errorf("the master admin's sign-in made %d programme(s)", after-before)
	}
}

func TestThePastCannotBeCreatedFromTheForm(t *testing.T) {
	app := newApp(t)
	email := app.owner("2303")
	c := app.login(email)
	setClock(t, time.Date(2036, 6, 15, 12, 0, 0, 0, time.UTC))
	sq := app.squadronID(email)

	for name, f := range map[string]url.Values{
		"last month": {"year": {"2036"}, "month": {"5"}},
		"last year":  {"year": {"2035"}, "month": {"12"}},
	} {
		resp := c.post("/programmes", f)
		page := body(resp)
		if resp.StatusCode == http.StatusSeeOther || !strings.Contains(page, "already passed") {
			t.Errorf("%s: %d %.200s", name, resp.StatusCode, page)
		}
		if strings.Contains(page, "invalid input") {
			t.Errorf("%s: the message has its internal prefix: %s", name, page)
		}
	}
	if n := count(t, app.db, `SELECT COUNT(*) FROM programmes WHERE squadron_id = `+itoaID(sq)+` AND year*12+month < 2036*12+6`); n != 0 {
		t.Errorf("%d historic programmes exist", n)
	}
	if resp := c.post("/programmes", url.Values{"year": {"2036"}, "month": {"7"}}); resp.StatusCode != http.StatusSeeOther {
		t.Errorf("a future month = %d, want 303", resp.StatusCode)
	}
}
