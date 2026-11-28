package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestActivitySuggestionsEndToEnd(t *testing.T) {
	app := newApp(t)
	owner := app.owner("2001")
	app.member(owner, "editor2001@example.org", "editor")
	app.member(owner, "viewer2001@example.org", "viewer")
	sq := app.squadronID(owner)
	o, e, v := app.login(owner), app.login("editor2001@example.org"), app.login("viewer2001@example.org")

	t.Run("only owners make the link, and nothing works before it exists", func(t *testing.T) {
		wantStatus(t, app.anon().get("/suggest/nope"), http.StatusNotFound)
		wantStatus(t, e.post("/squadron/suggest", url.Values{"action": {"new"}}), http.StatusForbidden)
		wantStatus(t, o.get("/squadron/suggest-qr.png"), http.StatusNotFound)
		wantStatus(t, o.post("/squadron/suggest", url.Values{"action": {"pdf"}, "qr_on_pdf": {"1"}}), http.StatusBadRequest)
		wantRedirect(t, o.post("/squadron/suggest", url.Values{"action": {"new"}}), "/squadron#suggestions")
	})
	set, _ := app.store.GetSuggestSettings(sq)
	path := "/suggest/" + set.Token

	t.Run("a cadet suggests an activity with no sign-in", func(t *testing.T) {
		anon := app.anon()
		page := body(anon.get(path))
		if !strings.Contains(page, "Suggest an activity") || !strings.Contains(page, "2001 Sqn") || !strings.Contains(page, `name="name"`) {
			t.Fatalf("the form:\n%.500s", page)
		}
		if strings.Contains(page, `name="email"`) || strings.Contains(page, `name="phone"`) {
			t.Error("the form asks for more than a name")
		}
		submit := func(c *client, v url.Values) *http.Response {
			v.Set("csrf", scrapeCSRF(t, c.get(path)))
			return c.postRaw(path, v)
		}
		wantRedirect(t, submit(app.anon(), url.Values{"name": {"Kayaking"}}), path+"?thanks=1")
		wantRedirect(t, submit(app.anon(), url.Values{"name": {"kayaking"}}), path+"?thanks=1") // a repeat is thanked, not stored twice
		wantStatus(t, submit(app.anon(), url.Values{"name": {"x"}}), http.StatusBadRequest)
		// A filled-in honeypot is thanked and thrown away.
		wantRedirect(t, submit(app.anon(), url.Values{"name": {"Spam activity"}, "website": {"http://spam.example"}}), path+"?thanks=1")
		// No form token: refused.
		wantStatus(t, app.anon().postRaw(path, url.Values{"name": {"No token"}}), http.StatusBadRequest)
		if got, _ := app.store.ListSuggestions(sq); len(got) != 1 || got[0].Name != "Kayaking" {
			t.Errorf("suggestions = %+v", got)
		}
		if !strings.Contains(body(app.anon().get(path+"?thanks=1")), "Thank you") {
			t.Error("no thanks shown")
		}
	})

	t.Run("it is rate limited", func(t *testing.T) {
		c := app.anon()
		c.xff = "203.0.113.77"
		blocked := false
		for i := 0; i < 15 && !blocked; i++ {
			resp := c.postRaw(path, url.Values{"csrf": {scrapeCSRF(t, c.get(path))}, "name": {"Idea " + strconv.Itoa(i) + "x"}})
			blocked = resp.StatusCode == http.StatusTooManyRequests
			resp.Body.Close()
		}
		if !blocked {
			t.Error("fifteen quick submissions from one address were all accepted")
		}
	})

	t.Run("editors see the inbox, add one as an activity and the button goes", func(t *testing.T) {
		wantStatus(t, v.get("/activities"), http.StatusForbidden)
		page := body(e.get("/activities"))
		if !strings.Contains(page, "Kayaking") || !strings.Contains(page, "Add as activity") {
			t.Fatalf("the inbox:\n%.800s", page)
		}
		list, _ := app.store.ListSuggestions(sq)
		id := strconv.FormatInt(list[len(list)-1].ID, 10)
		for _, g := range list {
			if g.Name == "Kayaking" {
				id = strconv.FormatInt(g.ID, 10)
			}
		}
		wantStatus(t, v.post("/activities/suggestions/"+id+"/add", url.Values{}), http.StatusForbidden)
		stranger := app.login(app.owner("2002"))
		wantStatus(t, stranger.post("/activities/suggestions/"+id+"/add", url.Values{}), http.StatusNotFound)
		wantRedirect(t, e.post("/activities/suggestions/"+id+"/add", url.Values{}), "/activities#suggestions")
		page = body(e.get("/activities"))
		if !strings.Contains(page, "Added as the activity <strong>Kayaking</strong>") {
			t.Errorf("the added activity's name is not shown:\n%s", page)
		}
		if strings.Contains(page, "Add Kayaking as an activity") {
			t.Error("the Add button is still there after it became an activity")
		}
		if opts := app.activityOptionsFor(sq); !contains(opts, "Kayaking") {
			t.Errorf("the activity is not offered in the grid: %v", opts)
		}
		wantStatus(t, e.post("/activities/suggestions/"+id+"/add", url.Values{}), http.StatusBadRequest) // already done
		wantRedirect(t, e.post("/activities/suggestions/"+id+"/delete", url.Values{}), "/activities#suggestions")
	})

	t.Run("the QR code goes on the PDF only when the owner chooses", func(t *testing.T) {
		p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
		pid := strconv.FormatInt(p.ID, 10)
		before := etagOf(t, o, "/programmes/"+pid+"/pdf")
		wantRedirect(t, o.post("/squadron/suggest", url.Values{"action": {"pdf"}, "qr_on_pdf": {"1"}}), "/squadron#suggestions")
		withQR := etagOf(t, o, "/programmes/"+pid+"/pdf")
		if withQR == before {
			t.Error("choosing the QR code did not change the PDF")
		}
		if !strings.Contains(body(o.get("/squadron")), "/squadron/suggest-qr.png") {
			t.Error("the settings page has no QR picture")
		}
		resp := o.get("/squadron/suggest-qr.png")
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" {
			t.Errorf("QR picture: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		resp.Body.Close()
		wantRedirect(t, o.post("/squadron/suggest", url.Values{"action": {"pdf"}}), "/squadron#suggestions")
		if etagOf(t, o, "/programmes/"+pid+"/pdf") != before {
			t.Error("turning the option off did not take the QR code off")
		}
	})

	t.Run("a new link stops the old one; turning it off stops all", func(t *testing.T) {
		wantRedirect(t, o.post("/squadron/suggest", url.Values{"action": {"new"}}), "/squadron#suggestions")
		wantStatus(t, app.anon().get(path), http.StatusNotFound)
		now, _ := app.store.GetSuggestSettings(sq)
		wantStatus(t, app.anon().get("/suggest/"+now.Token), http.StatusOK)
		wantRedirect(t, o.post("/squadron/suggest", url.Values{"action": {"off"}}), "/squadron#suggestions")
		wantStatus(t, app.anon().get("/suggest/"+now.Token), http.StatusNotFound)
	})
}

func etagOf(t *testing.T, c *client, path string) string {
	t.Helper()
	resp := c.get(path)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: %d", path, resp.StatusCode)
	}
	return resp.Header.Get("ETag")
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func (a *testApp) activityOptionsFor(sq int64) []string {
	opts, _ := a.store.ActivityOptions(sq)
	return opts
}
