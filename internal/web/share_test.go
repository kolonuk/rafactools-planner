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
