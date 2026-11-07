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

func TestPublicLinkServesOnlyPublishedMonthsAndExpires(t *testing.T) {
	app := newApp(t)
	email := app.owner("1301")
	sq := app.squadronID(email)
	c := app.login(email)
	setClock(t, time.Date(2030, 4, 1, 9, 0, 0, 0, time.UTC))

	p, err := app.store.CreateProgramme(sq, 2030, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	pid := strconv.FormatInt(p.ID, 10)
	parade, _, _ := firstParadeAndPlain(t, app, sq, p.ID)
	if err := app.store.SaveDaysAndRows(sq, p.ID, []programme.DayInput{{
		ID: parade.ID, IsParade: true, UniformKey: "green", Notes: "bring a pen",
		Rows: []programme.RowInput{
			{ID: parade.Rows[0].ID, Texts: []string{"Drill", "Drill", "", "First Aid"}},
			{ID: parade.Rows[1].ID, Texts: []string{"Flt Lt Secret", "", "", ""}},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	app.store.AddRequirementCategory(p.ID, "Fieldcraft", "")

	anon := app.anon()
	t.Run("no link yet", func(t *testing.T) {
		wantStatus(t, anon.get("/share/nope/calendar.ics"), http.StatusNotFound)
	})

	wantRedirect(t, c.post("/squadron/share", url.Values{}), "/squadron#sharing")
	share, _ := app.store.GetShare(sq)
	if len(share.Token) < 30 {
		t.Fatalf("token %q is too short", share.Token)
	}
	cal := "/share/" + share.Token + "/calendar.ics"
	pdf := "/share/" + share.Token + "/2030-04.pdf"

	t.Run("an unpublished month is not shared", func(t *testing.T) {
		out := body(anon.get(cal))
		if strings.Contains(out, "BEGIN:VEVENT") {
			t.Errorf("unpublished month leaked into the calendar:\n%s", out)
		}
		wantStatus(t, anon.get(pdf), http.StatusNotFound)
	})

	wantRedirect(t, c.post("/programmes/"+pid+"/publish", url.Values{"published": {"1"}}), "/programmes/"+pid)

	t.Run("a published month appears in the calendar", func(t *testing.T) {
		resp := anon.get(cal)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
			t.Errorf("content type %q", ct)
		}
		out := body(resp)
		date := strings.ReplaceAll(parade.Date, "-", "")
		for _, want := range []string{
			"BEGIN:VEVENT", "SUMMARY:1301 Sqn parade night",
			"DTSTART:" + date + "T180000Z", // 19:00 BST is 18:00 UTC in April
			"DTEND:" + date + "T203000Z",
			"URL:http://example.test/share/" + share.Token + "/2030-04.pdf",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("calendar lacks %q:\n%s", want, out)
			}
		}
		desc := unfold(out)
		for _, want := range []string{`Drill: Junior Cadets\, 1st Class Cadets`, `First Aid: Senior Cadets`, "Uniform: No 3 (MTP) - Greens"} {
			if !strings.Contains(desc, want) {
				t.Errorf("description lacks %q:\n%s", want, desc)
			}
		}
		for _, secret := range []string{"Flt Lt Secret", "bring a pen", "Fieldcraft"} {
			if strings.Contains(desc, secret) {
				t.Errorf("%q must not be in the public calendar", secret)
			}
		}
	})

	t.Run("one event per parade day, none before today", func(t *testing.T) {
		setClock(t, time.Date(2030, 4, 30, 9, 0, 0, 0, time.UTC))
		share2, _ := app.store.GetShare(sq)
		out := body(anon.get("/share/" + share2.Token + "/calendar.ics"))
		fp, _ := app.store.GetFullProgramme(sq, p.ID)
		want := 0
		for _, d := range fp.Days {
			if d.IsParade && d.Date >= "2030-04-30" {
				want++
			}
		}
		if got := strings.Count(out, "BEGIN:VEVENT"); got != want {
			t.Errorf("%d events, want %d", got, want)
		}
		setClock(t, time.Date(2030, 4, 1, 9, 0, 0, 0, time.UTC))
	})

	t.Run("the PDF is public and has no tracker", func(t *testing.T) {
		resp := anon.get(pdf)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
			t.Errorf("content type %q", ct)
		}
		if !strings.HasPrefix(body(resp), "%PDF") {
			t.Error("not a PDF")
		}
		wantStatus(t, anon.get("/share/"+share.Token+"/2030-05.pdf"), http.StatusNotFound)
		wantStatus(t, anon.get("/share/"+share.Token+"/junk.pdf"), http.StatusNotFound)
		wantStatus(t, anon.get("/share/"+share.Token+"/other.txt"), http.StatusNotFound)
	})

	t.Run("a wrong token is not found", func(t *testing.T) {
		wantStatus(t, anon.get("/share/"+share.Token+"x/calendar.ics"), http.StatusNotFound)
	})

	t.Run("the link stops after a month and a new one replaces it", func(t *testing.T) {
		setClock(t, time.Date(2030, 5, 2, 9, 0, 0, 0, time.UTC))
		resp := anon.get(cal)
		if resp.StatusCode != http.StatusGone {
			t.Fatalf("status %d, want 410", resp.StatusCode)
		}
		if !strings.Contains(body(resp), "expired") {
			t.Error("the answer should say the link expired")
		}
		wantStatus(t, anon.get(pdf), http.StatusGone)
		wantRedirect(t, c.post("/squadron/share", url.Values{}), "/squadron#sharing")
		renewed, _ := app.store.GetShare(sq)
		if renewed.Token == share.Token {
			t.Error("renewing must make a new token")
		}
		wantStatus(t, anon.get(cal), http.StatusNotFound) // the old link is dead for good
		wantStatus(t, anon.get("/share/"+renewed.Token+"/calendar.ics"), http.StatusOK)
	})

	t.Run("unpublishing removes the month", func(t *testing.T) {
		setClock(t, time.Date(2030, 4, 1, 9, 0, 0, 0, time.UTC))
		wantRedirect(t, c.post("/squadron/share", url.Values{}), "/squadron#sharing")
		now, _ := app.store.GetShare(sq)
		wantRedirect(t, c.post("/programmes/"+pid+"/publish", url.Values{"published": {"0"}}), "/programmes/"+pid)
		if strings.Contains(body(anon.get("/share/"+now.Token+"/calendar.ics")), "BEGIN:VEVENT") {
			t.Error("unpublished month is still in the calendar")
		}
	})
}

func TestOnlyEditorsPublishAndOnlyOwnersMakeTheLink(t *testing.T) {
	app := newApp(t)
	owner := app.owner("1302")
	app.member(owner, "viewer@example.org", "viewer")
	sq := app.squadronID(owner)
	p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	pid := strconv.FormatInt(p.ID, 10)

	v := app.login("viewer@example.org")
	wantStatus(t, v.post("/programmes/"+pid+"/publish", url.Values{"published": {"1"}}), http.StatusForbidden)
	wantStatus(t, v.post("/squadron/share", url.Values{}), http.StatusForbidden)

	// Someone from another squadron cannot publish this squadron's month.
	other := app.login(app.owner("1303"))
	wantStatus(t, other.post("/programmes/"+pid+"/publish", url.Values{"published": {"1"}}), http.StatusNotFound)
	if fp, _ := app.store.GetFullProgramme(sq, p.ID); fp.Published {
		t.Error("another squadron published this month")
	}
}

func TestParadeTimesAreValidated(t *testing.T) {
	app := newApp(t)
	email := app.owner("1304")
	sq := app.squadronID(email)
	c := app.login(email)
	save := func(start, end string) *http.Response {
		return c.post("/squadron", url.Values{"name": {"1304 Sqn"}, "classifications": {"A\nB"}, "parade_weekdays": {"1"}, "parade_start": {start}, "parade_end": {end}})
	}
	for _, bad := range [][2]string{{"7pm", "21:30"}, {"19:00", "25:00"}, {"21:30", "19:00"}, {"19:00", "19:00"}} {
		wantStatus(t, save(bad[0], bad[1]), http.StatusBadRequest)
	}
	wantRedirect(t, save("18:30", "21:00"), "/squadron")
	got, _ := app.store.GetSquadron(sq)
	if got.ParadeStart != "18:30" || got.ParadeEnd != "21:00" {
		t.Errorf("times = %s to %s", got.ParadeStart, got.ParadeEnd)
	}
	fresh, _ := app.store.GetSquadron(app.squadronID(app.owner("1305")))
	if fresh.ParadeStart != "19:00" || fresh.ParadeEnd != "21:30" {
		t.Errorf("default times = %s to %s, want 19:00 to 21:30", fresh.ParadeStart, fresh.ParadeEnd)
	}
}

// unfold undoes calendar line folding.
func unfold(s string) string { return strings.ReplaceAll(s, "\r\n ", "") }

func TestFilesAreCachedUntilTheMonthChanges(t *testing.T) {
	app := newApp(t)
	email := app.owner("1306")
	sq := app.squadronID(email)
	c := app.login(email)
	setClock(t, time.Date(2030, 4, 1, 9, 0, 0, 0, time.UTC))
	p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	pid := strconv.FormatInt(p.ID, 10)
	parade, _, _ := firstParadeAndPlain(t, app, sq, p.ID)
	save := func(text string) {
		t.Helper()
		if err := app.store.SaveDaysAndRows(sq, p.ID, []programme.DayInput{{ID: parade.ID, IsParade: true, Rows: []programme.RowInput{{ID: parade.Rows[0].ID, Texts: []string{text, "", "", ""}}}}}); err != nil {
			t.Fatal(err)
		}
	}
	save("Drill")
	wantRedirect(t, c.post("/squadron/share", url.Values{}), "/squadron#sharing")
	wantRedirect(t, c.post("/programmes/"+pid+"/publish", url.Values{"published": {"1"}}), "/programmes/"+pid)
	share, _ := app.store.GetShare(sq)
	anon := app.anon()

	fetch := func(cl *client, path string) (etag string, status int, ctype string) {
		resp := cl.get(path)
		defer resp.Body.Close()
		return resp.Header.Get("ETag"), resp.StatusCode, resp.Header.Get("Content-Type")
	}

	pdfPath, pngPath := "/share/"+share.Token+"/2030-04.pdf", "/share/"+share.Token+"/2030-04.png"
	etag1, status, _ := fetch(anon, pdfPath)
	if status != 200 || etag1 == "" {
		t.Fatalf("public pdf: status %d etag %q", status, etag1)
	}
	if _, status, ctype := fetch(anon, pngPath); status != 200 || ctype != "image/png" {
		t.Errorf("public png: status %d type %q", status, ctype)
	}
	if _, status, ctype := fetch(c, "/programmes/"+pid+"/png"); status != 200 || ctype != "image/png" {
		t.Errorf("signed-in png: status %d type %q", status, ctype)
	}

	t.Run("an unchanged month answers 304 from the cache", func(t *testing.T) {
		req, _ := http.NewRequest("GET", app.srv.URL+pdfPath, nil)
		req.Header.Set("If-None-Match", etag1)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotModified {
			t.Errorf("status %d, want 304", resp.StatusCode)
		}
		if etag2, _, _ := fetch(anon, pdfPath); etag2 != etag1 {
			t.Error("the file changed although the month did not")
		}
	})

	t.Run("a saved change gives a new file", func(t *testing.T) {
		save("Shooting")
		if etag3, _, _ := fetch(anon, pdfPath); etag3 == etag1 {
			t.Error("the month changed but the public PDF is the old one")
		}
	})

	t.Run("a closure changes it too, though it does not touch the save time", func(t *testing.T) {
		before, _, _ := fetch(anon, pdfPath)
		wantRedirect(t, c.post("/programmes/"+pid+"/closures", url.Values{"start_date": {parade.Date}, "end_date": {parade.Date}, "message": {"Stand-down"}}), "/programmes/"+pid)
		if after, _, _ := fetch(anon, pdfPath); after == before {
			t.Error("a closure was added but the public PDF did not change")
		}
	})

	t.Run("a new public link changes the QR code and so the file", func(t *testing.T) {
		before, _, _ := fetch(anon, pdfPath)
		wantRedirect(t, c.post("/squadron/share", url.Values{}), "/squadron#sharing")
		renewed, _ := app.store.GetShare(sq)
		after, status, _ := fetch(anon, "/share/"+renewed.Token+"/2030-04.pdf")
		if status != 200 || after == before {
			t.Errorf("status %d, same file after a new link: %v", status, after == before)
		}
	})
}

// Every script a page loads must be inside the page's content block, or it is
// never written out (a script tag after the final {{end}} is silently dropped).
func TestMonthPageLoadsItsScripts(t *testing.T) {
	app := newApp(t)
	email := app.owner("1307")
	c := app.login(email)
	p, _ := app.store.CreateProgramme(app.squadronID(email), 2030, 4, 1)
	page := body(c.get("/programmes/" + strconv.FormatInt(p.ID, 10)))
	for _, js := range []string{"programme-grid.js", "programme-parade.js", "programme-uniform.js", "menu.js", "edit.js", "theme.js"} {
		if !strings.Contains(page, "/static/js/gen/"+js) {
			t.Errorf("the month page does not load %s", js)
		}
	}
	if !strings.Contains(body(c.get("/instructors")), "/static/js/gen/edit.js") {
		t.Error("the instructors page does not load edit.js")
	}
}

func TestTheQRPictureIsOnlyForSignedInMembersOfTheSquadron(t *testing.T) {
	app := newApp(t)
	email := app.owner("1308")
	sq := app.squadronID(email)
	c := app.login(email)
	setClock(t, time.Date(2030, 4, 1, 9, 0, 0, 0, time.UTC))
	p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	path := "/programmes/" + strconv.FormatInt(p.ID, 10) + "/qr.png"
	if resp := c.get(path); resp.StatusCode != http.StatusNotFound { // no link yet
		t.Errorf("without a link: status %d, want 404", resp.StatusCode)
	}
	wantRedirect(t, c.post("/programmes/"+strconv.FormatInt(p.ID, 10)+"/publish", url.Values{"published": {"1"}}), "/programmes/"+strconv.FormatInt(p.ID, 10))
	resp := c.get(path)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" {
		t.Errorf("with a link: status %d type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	resp.Body.Close()
	wantRedirect(t, app.anon().get(path), "/login")
	other := app.login(app.owner("1309"))
	if resp := other.get(path); resp.StatusCode != http.StatusNotFound {
		t.Errorf("another squadron: status %d, want 404", resp.StatusCode)
	}
}

// publishMonth makes and publishes a month with one parade night's activity filled in.
func publishMonth(t *testing.T, app *testApp, c *client, sq int64, year, month int, activity string) *programme.Programme {
	t.Helper()
	p, err := app.store.CreateProgramme(sq, year, month, 1)
	if err != nil {
		t.Fatal(err)
	}
	parade, _, _ := firstParadeAndPlain(t, app, sq, p.ID)
	if err := app.store.SaveDaysAndRows(sq, p.ID, []programme.DayInput{{ID: parade.ID, IsParade: true, UniformKey: "green",
		Rows: []programme.RowInput{{ID: parade.Rows[0].ID, Texts: []string{activity, "", "", ""}}}}}); err != nil {
		t.Fatal(err)
	}
	pid := strconv.FormatInt(p.ID, 10)
	wantRedirect(t, c.post("/programmes/"+pid+"/publish", url.Values{"published": {"1"}}), "/programmes/"+pid)
	return p
}

func TestOneCalendarLinkCoversEveryPublishedMonth(t *testing.T) {
	app := newApp(t)
	email := app.owner("1310")
	sq := app.squadronID(email)
	c := app.login(email)
	setClock(t, time.Date(2030, 4, 1, 9, 0, 0, 0, time.UTC))
	apr := publishMonth(t, app, c, sq, 2030, 4, "April drill")
	publishMonth(t, app, c, sq, 2030, 5, "May shooting")
	if _, err := app.store.CreateProgramme(sq, 2030, 6, 1); err != nil { // never published
		t.Fatal(err)
	}
	share, _ := app.store.GetShare(sq)
	anon := app.anon()
	feed := func() string {
		resp := anon.get("/share/" + share.Token + "/calendar.ics")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("calendar status %d", resp.StatusCode)
		}
		return unfold(body(resp))
	}

	out := feed()
	for _, want := range []string{"April drill", "May shooting", "/2030-04.pdf", "/2030-05.pdf"} {
		if !strings.Contains(out, want) {
			t.Errorf("the one calendar lacks %q", want)
		}
	}
	if strings.Contains(out, "2030-06.pdf") {
		t.Error("an unpublished month is in the calendar")
	}
	if n := strings.Count(out, "BEGIN:VCALENDAR"); n != 1 {
		t.Errorf("%d calendars in the feed, want 1", n)
	}
	// The same single link is what the squadron is given for every month.
	if s := app.store; s != nil {
		if again, _ := s.GetShare(sq); again.Token != share.Token {
			t.Error("the link changed")
		}
	}

	wantRedirect(t, c.post("/programmes/"+strconv.FormatInt(apr.ID, 10)+"/publish", url.Values{"published": {"0"}}), "/programmes/"+strconv.FormatInt(apr.ID, 10))
	out = feed()
	if strings.Contains(out, "April drill") || !strings.Contains(out, "May shooting") {
		t.Errorf("after unpublishing April the calendar should have only May:\n%s", out)
	}
}

func TestTheCalendarIsKeptUntilWhatItShowsChanges(t *testing.T) {
	app := newApp(t)
	email := app.owner("1311")
	sq := app.squadronID(email)
	c := app.login(email)
	setClock(t, time.Date(2030, 4, 1, 9, 0, 0, 0, time.UTC))
	apr := publishMonth(t, app, c, sq, 2030, 4, "April drill")
	share, _ := app.store.GetShare(sq)
	path := "/share/" + share.Token + "/calendar.ics"
	anon := app.anon()
	etag := func() string {
		t.Helper()
		resp := anon.get(path)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
		return resp.Header.Get("ETag")
	}
	parade, _, _ := firstParadeAndPlain(t, app, sq, apr.ID)
	e0 := etag()
	if e0 == "" || etag() != e0 || etag() != e0 {
		t.Fatalf("the calendar changed between requests with no edit (%q)", e0)
	}

	t.Run("a repeat request with the ETag is answered 304", func(t *testing.T) {
		req, _ := http.NewRequest("GET", app.srv.URL+path, nil)
		req.Header.Set("If-None-Match", e0)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotModified {
			t.Errorf("status %d, want 304", resp.StatusCode)
		}
	})

	t.Run("things the calendar does not show leave it alone", func(t *testing.T) {
		app.store.AddRequirementCategory(apr.ID, "Fieldcraft", "")
		app.store.AddWeekendNote(apr.ID, "2030-04-06", "x") // this one it does show: see below
		before := etag()
		cat, _ := app.store.AddRequirementCategory(apr.ID, "Briefings", "")
		app.store.AddRequirementEntry(apr.ID, cat, "Cdt Example", "note")
		if err := app.store.SaveDaysAndRows(sq, apr.ID, []programme.DayInput{{ID: parade.ID, IsParade: true, UniformKey: "green", Notes: "bring a pen",
			Rows: []programme.RowInput{{ID: parade.Rows[0].ID, Texts: []string{"April drill", "", "", ""}}}}}); err != nil {
			t.Fatal(err)
		}
		if etag() != before {
			t.Error("a note or the requirement tracker changed the calendar, which shows neither")
		}
		// An unpublished month's edits do not either.
		other, _ := app.store.CreateProgramme(sq, 2030, 7, 1)
		app.store.AddClosure(other.ID, "2030-07-01", "2030-07-02", "Closed")
		if etag() != before {
			t.Error("an unpublished month changed the calendar")
		}
	})

	t.Run("things it shows change it", func(t *testing.T) {
		last := etag()
		step := func(what string, change func()) {
			t.Helper()
			change()
			if now := etag(); now == last {
				t.Errorf("%s did not change the calendar", what)
			} else {
				last = now
			}
		}
		step("a new activity", func() {
			app.store.SaveDaysAndRows(sq, apr.ID, []programme.DayInput{{ID: parade.ID, IsParade: true, UniformKey: "green",
				Rows: []programme.RowInput{{ID: parade.Rows[0].ID, Texts: []string{"Shooting", "", "", ""}}}}})
		})
		step("a closure over the night", func() { app.store.AddClosure(apr.ID, parade.Date, parade.Date, "Stand-down") })
		step("another month being published", func() { publishMonth(t, app, c, sq, 2030, 5, "May drill") })
		step("new parade times", func() {
			wantRedirect(t, c.post("/squadron", url.Values{"name": {"1311 Sqn"}, "classifications": {"A\nB\nC\nD"}, "parade_weekdays": {"1", "4"}, "parade_start": {"18:00"}, "parade_end": {"21:00"}}), "/squadron")
		})
		step("a night passing", func() { setClock(t, time.Date(2030, 4, 5, 9, 0, 0, 0, time.UTC)) })
		step("a month being unpublished", func() {
			wantRedirect(t, c.post("/programmes/"+strconv.FormatInt(apr.ID, 10)+"/publish", url.Values{"published": {"0"}}), "/programmes/"+strconv.FormatInt(apr.ID, 10))
		})
	})
}

func TestTheCalendarIsRateLimited(t *testing.T) {
	app := newApp(t)
	email := app.owner("1312")
	sq := app.squadronID(email)
	c := app.login(email)
	setClock(t, time.Date(2030, 4, 1, 9, 0, 0, 0, time.UTC))
	publishMonth(t, app, c, sq, 2030, 4, "April drill")
	share, _ := app.store.GetShare(sq)
	path := "/share/" + share.Token + "/calendar.ics"

	greedy := app.anon()
	greedy.xff = "203.0.113.7"
	ok, limited := 0, 0
	var retry string
	for i := 0; i < 70; i++ {
		resp := greedy.get(path)
		switch resp.StatusCode {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			limited++
			retry = resp.Header.Get("Retry-After")
		}
		resp.Body.Close()
	}
	if ok != 60 || limited != 10 || retry == "" {
		t.Errorf("%d answered, %d limited, Retry-After %q: want 60 answered, 10 limited, a Retry-After", ok, limited, retry)
	}
	// Someone else is not held up by that caller's use.
	polite := app.anon()
	polite.xff = "203.0.113.8"
	if resp := polite.get(path); resp.StatusCode != http.StatusOK {
		t.Errorf("another caller got %d", resp.StatusCode)
	}
}
