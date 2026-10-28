package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"rafactools-planner/internal/config"
)

// twoSquadrons seeds squadron A (with a programme, closure, requirement
// category and entry) and squadron B, each with a logged-in owner.
type twoSquadrons struct {
	a, b           *testApp
	ownerA, ownerB *client
	progA, progB   string
	catA, entryA   int64
	ownerAEmail    string
}

func seedTwoSquadrons(t *testing.T) twoSquadrons {
	t.Helper()
	app := newApp(t)
	ts := twoSquadrons{a: app, b: app}
	ts.ownerAEmail = app.owner("501")
	ownerBEmail := app.owner("502")
	ts.ownerA, ts.ownerB = app.login(ts.ownerAEmail), app.login(ownerBEmail)

	pa, err := app.store.CreateProgramme(app.squadronID(ts.ownerAEmail), 2030, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := app.store.CreateProgramme(app.squadronID(ownerBEmail), 2030, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	ts.progA, ts.progB = strconv.FormatInt(pa.ID, 10), strconv.FormatInt(pb.ID, 10)

	ts.catA, err = app.store.AddRequirementCategory(pa.ID, "A-only category", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.AddRequirementEntry(pa.ID, ts.catA, "Cdt A", "secret note"); err != nil {
		t.Fatal(err)
	}
	app.db.QueryRow(`SELECT id FROM programme_requirement_entries`).Scan(&ts.entryA)
	return ts
}

// Squadron B must not be able to read or change squadron A's data by
// guessing IDs. Every route with an ID in the path is tried.
func TestTenantIsolation(t *testing.T) {
	ts := seedTwoSquadrons(t)
	app := ts.a
	cat, entry := strconv.FormatInt(ts.catA, 10), strconv.FormatInt(ts.entryA, 10)

	t.Run("reads", func(t *testing.T) {
		wantStatus(t, ts.ownerB.get("/programmes/"+ts.progA), 404)
		wantStatus(t, ts.ownerB.get("/programmes/"+ts.progA+"/pdf"), 404)
		if b := body(ts.ownerB.get("/")); strings.Contains(b, "/programmes/"+ts.progA+`"`) {
			t.Error("A's programme listed on B's dashboard")
		}
	})

	t.Run("writes against A's programme ID are refused", func(t *testing.T) {
		for _, p := range []struct {
			path string
			form url.Values
		}{
			{"/programmes/" + ts.progA, nil},
			{"/programmes/" + ts.progA + "/closures", url.Values{"start_date": {"2030-05-01"}, "end_date": {"2030-05-02"}, "message": {"pwned"}}},
			{"/programmes/" + ts.progA + "/weekend-notes", url.Values{"date": {"2030-05-04"}, "text": {"pwned"}}},
			{"/programmes/" + ts.progA + "/requirements", url.Values{"name": {"pwned"}}},
			{"/programmes/" + ts.progA + "/requirements/" + cat + "/entries", url.Values{"cadet_name": {"pwned"}}},
			{"/programmes/" + ts.progA + "/requirements/" + cat + "/delete", nil},
			{"/programmes/" + ts.progA + "/requirements/" + cat + "/entries/" + entry + "/delete", nil},
		} {
			wantStatus(t, ts.ownerB.post(p.path, p.form), 404)
		}
	})

	// The regression for the cross-tenant bug: B uses its OWN programme ID
	// (so the programme-ownership check passes) but A's category/entry IDs.
	t.Run("forged category and entry IDs under B's own programme", func(t *testing.T) {
		ts.ownerB.post("/programmes/"+ts.progB+"/requirements/"+cat+"/entries", url.Values{"cadet_name": {"pwned"}, "note": {"pwned"}}).Body.Close()
		ts.ownerB.post("/programmes/"+ts.progB+"/requirements/"+cat+"/entries/"+entry+"/delete", nil).Body.Close()
		ts.ownerB.post("/programmes/"+ts.progB+"/requirements/"+cat+"/delete", nil).Body.Close()

		if n := count(t, app.db, `SELECT COUNT(*) FROM programme_requirement_entries WHERE cadet_name = 'pwned'`); n != 0 {
			t.Error("B injected an entry into A's category")
		}
		if n := count(t, app.db, `SELECT COUNT(*) FROM programme_requirement_entries WHERE id = ?`, ts.entryA); n != 1 {
			t.Error("B deleted A's entry")
		}
		if n := count(t, app.db, `SELECT COUNT(*) FROM programme_requirement_categories WHERE id = ?`, ts.catA); n != 1 {
			t.Error("B deleted A's category")
		}
	})

	t.Run("A's data is intact and still served to A", func(t *testing.T) {
		resp := ts.ownerA.get("/programmes/" + ts.progA)
		if b := body(resp); !strings.Contains(b, "A-only category") || !strings.Contains(b, "Cdt A") {
			t.Error("A's own page lost its data")
		}
		for _, bad := range []string{"pwned"} {
			if n := count(t, app.db, `SELECT COUNT(*) FROM programme_closures WHERE message = ?`, bad); n != 0 {
				t.Error("closure written to A")
			}
		}
	})

	t.Run("settings and invites are per-squadron", func(t *testing.T) {
		ts.ownerB.post("/squadron", url.Values{"name": {"Renamed By B"}}).Body.Close()
		var name string
		app.db.QueryRow(`SELECT name FROM squadrons WHERE number = '501'`).Scan(&name)
		if name == "Renamed By B" {
			t.Error("B renamed A")
		}
	})
}

func TestCSRFProtection(t *testing.T) {
	app := newApp(t)
	email := app.owner("601")
	c := app.login(email)
	other := app.login(app.owner("602"))

	form := func(tok string) url.Values {
		return url.Values{"csrf": {tok}, "year": {"2030"}, "month": {"6"}}
	}
	// Bad tokens: empty, garbage, and a *valid token for a different session*.
	for name, tok := range map[string]string{"empty": "", "garbage": "zz", "other session's": other.csrf} {
		resp := c.postRaw("/programmes", form(tok))
		if resp.StatusCode == http.StatusSeeOther {
			t.Errorf("%s token accepted", name)
		}
		resp.Body.Close()
	}
	if n := count(t, app.db, `SELECT COUNT(*) FROM programmes`); n != 0 {
		t.Fatalf("%d programmes created with bad CSRF", n)
	}
	// The correct token works, and the token a real form embeds is that token.
	page := body(c.get("/"))
	m := csrfInput.FindStringSubmatch(page)
	if m == nil || m[1] != c.csrf {
		t.Fatalf("dashboard form token %v does not match the session token", m)
	}
	wantStatus(t, c.postRaw("/programmes", form(c.csrf)), http.StatusSeeOther)
	if n := count(t, app.db, `SELECT COUNT(*) FROM programmes`); n != 1 {
		t.Fatalf("expected 1 programme, got %d", n)
	}
}

// GET requests must never change state (CSRF via <img src> etc.).
func TestGETRoutesDoNotMutate(t *testing.T) {
	ts := seedTwoSquadrons(t)
	before := count(t, ts.a.db, `SELECT (SELECT COUNT(*) FROM programmes)+(SELECT COUNT(*) FROM programme_closures)+(SELECT COUNT(*) FROM invites)+(SELECT COUNT(*) FROM programme_requirement_entries)`)
	for _, p := range []string{
		"/programmes/" + ts.progA + "/closures", "/programmes/" + ts.progA + "/requirements/1/delete",
		"/squadron/invites", "/logout", "/programmes?year=2031&month=1",
	} {
		resp := ts.ownerA.get(p)
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("GET %s -> %d, want 404/405", p, resp.StatusCode)
		}
		resp.Body.Close()
	}
	after := count(t, ts.a.db, `SELECT (SELECT COUNT(*) FROM programmes)+(SELECT COUNT(*) FROM programme_closures)+(SELECT COUNT(*) FROM invites)+(SELECT COUNT(*) FROM programme_requirement_entries)`)
	if before != after {
		t.Errorf("GETs changed state: %d -> %d", before, after)
	}
}

// "GET /" in a ServeMux matches every path; the dashboard route must be
// exact so typos and probes get a real 404 instead of the logged-in homepage.
func TestUnknownPathsReturn404(t *testing.T) {
	app := newApp(t)
	for _, c := range []*client{app.anon(), app.login(app.owner("901"))} {
		for _, p := range []string{"/nope", "/admin", "/.env", "/programmes", "/static/../etc/passwd"} {
			resp := c.get(p)
			resp.Body.Close()
			if resp.StatusCode != 404 && resp.StatusCode != 405 && resp.StatusCode != 307 {
				t.Errorf("GET %s -> %d, want 404/405", p, resp.StatusCode)
			}
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	for _, secure := range []bool{false, true} {
		app := newApp(t, func(c *config.Config) { c.CookieSecure = secure })
		for _, path := range []string{"/login", "/claim", "/health", "/nope", "/static/css/app.css"} {
			resp := app.anon().get(path)
			resp.Body.Close()
			h := resp.Header
			for k, want := range map[string]string{
				"X-Content-Type-Options": "nosniff",
				"X-Frame-Options":        "DENY",
				"Referrer-Policy":        "no-referrer",
			} {
				if h.Get(k) != want {
					t.Errorf("secure=%v %s: %s = %q, want %q", secure, path, k, h.Get(k), want)
				}
			}
			csp := h.Get("Content-Security-Policy")
			for _, must := range []string{"default-src 'self'", "script-src 'self'", "frame-ancestors 'none'", "object-src 'none'", "form-action 'self'"} {
				if !strings.Contains(csp, must) {
					t.Errorf("%s: CSP missing %q: %s", path, must, csp)
				}
			}
			// Own-origin scripts are allowed; anything that would let
			// injected markup run script is not.
			for _, never := range []string{"unsafe-inline", "unsafe-eval", "unsafe-hashes", "http:", "https:", "data:", "blob:", "*"} {
				for _, directive := range strings.Split(csp, ";") {
					d := strings.TrimSpace(directive)
					if strings.HasPrefix(d, "script-src") && strings.Contains(d, never) {
						t.Errorf("%s: script-src contains %q: %s", path, never, d)
					}
				}
			}
			if hsts := h.Get("Strict-Transport-Security"); secure != (hsts != "") {
				t.Errorf("secure=%v %s: HSTS = %q", secure, path, hsts)
			}
			if static := strings.HasPrefix(path, "/static/"); static == (h.Get("Cache-Control") == "no-store") {
				t.Errorf("%s: Cache-Control = %q", path, h.Get("Cache-Control"))
			}
		}
	}
}

func TestSecureCookieFlagFollowsConfig(t *testing.T) {
	for _, secure := range []bool{false, true} {
		app := newApp(t, func(c *config.Config) { c.CookieSecure = secure })
		resp := app.anon().get("/login")
		resp.Body.Close()
		var found bool
		for _, ck := range resp.Cookies() {
			if ck.Name == anonCSRFCookieName {
				found = true
				if ck.Secure != secure || !ck.HttpOnly {
					t.Errorf("secure=%v: anon csrf cookie %+v", secure, ck)
				}
			}
		}
		if !found {
			t.Error("no anon csrf cookie issued")
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	app := newApp(t)
	attempt := func(c *client) int {
		resp := c.postRaw("/login", url.Values{"csrf": {"x"}, "email": {"a@b.c"}, "password": {"x"}})
		resp.Body.Close()
		return resp.StatusCode
	}

	attacker := app.anon()
	attacker.xff = "203.0.113.50"
	for i := 0; i < 20; i++ {
		if got := attempt(attacker); got == http.StatusTooManyRequests {
			t.Fatalf("limited after only %d attempts", i)
		}
	}
	if got := attempt(attacker); got != http.StatusTooManyRequests {
		t.Fatalf("attempt 21 -> %d, want 429", got)
	}

	t.Run("spoofing a different leading XFF entry does not reset the limit", func(t *testing.T) {
		attacker.xff = "1.2.3.4, 203.0.113.50"
		if got := attempt(attacker); got != http.StatusTooManyRequests {
			t.Fatalf("spoofed XFF bypassed the limiter (%d)", got)
		}
	})

	t.Run("a different client is unaffected", func(t *testing.T) {
		bystander := app.anon()
		bystander.xff = "198.51.100.7"
		if got := attempt(bystander); got == http.StatusTooManyRequests {
			t.Fatal("limit leaked to another IP")
		}
	})

	t.Run("other auth endpoints share the limit", func(t *testing.T) {
		for _, p := range []string{"/claim", "/invite/abc"} {
			resp := attacker.postRaw(p, url.Values{})
			resp.Body.Close()
			if resp.StatusCode != http.StatusTooManyRequests {
				t.Errorf("POST %s -> %d, want 429", p, resp.StatusCode)
			}
		}
	})
}

func TestOversizedBodyRejected(t *testing.T) {
	app := newApp(t)
	c := app.login(app.owner("701"))
	big := strings.Repeat("A", 2<<20)
	resp := c.post("/squadron", url.Values{"name": {big}})
	resp.Body.Close()
	if resp.StatusCode == http.StatusSeeOther {
		t.Fatal("2MB form body accepted")
	}
	var name string
	app.db.QueryRow(`SELECT name FROM squadrons WHERE number = '701'`).Scan(&name)
	if len(name) > 1000 {
		t.Fatal("oversized value stored")
	}
}

// User-controlled text must be HTML-escaped wherever it is rendered.
func TestStoredXSSIsEscaped(t *testing.T) {
	app := newApp(t)
	email := app.owner("801")
	c := app.login(email)
	payload := `<script>alert(1)</script>"><img src=x onerror=alert(2)>`

	wantRedirect(t, c.post("/squadron", url.Values{"name": {payload}, "subtitle": {payload}, "protective_marking": {payload}, "classifications": {payload}}), "/squadron")
	pid, _ := app.store.CreateProgramme(app.squadronID(email), 2030, 8, 1)
	id := strconv.FormatInt(pid.ID, 10)
	c.post("/programmes/"+id+"/closures", url.Values{"start_date": {"2030-08-01"}, "end_date": {"2030-08-02"}, "message": {payload}}).Body.Close()
	c.post("/programmes/"+id+"/weekend-notes", url.Values{"date": {"2030-08-03"}, "text": {payload}}).Body.Close()
	cat, _ := app.store.AddRequirementCategory(pid.ID, payload, payload)
	app.store.AddRequirementEntry(pid.ID, cat, payload, payload)
	c.post("/squadron/invites", url.Values{"email": {"x@example.org"}, "role": {"viewer"}}).Body.Close()

	for _, path := range []string{"/", "/squadron", "/programmes/" + id} {
		page := body(c.get(path))
		if strings.Contains(page, "<script>alert(1)") || strings.Contains(page, "<img src=x") {
			t.Errorf("%s renders unescaped user input", path)
		}
	}
	// And the PDF export must cope with the same text.
	resp := c.get("/programmes/" + id + "/pdf")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/pdf" {
		t.Errorf("pdf: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	resp.Body.Close()
}

func TestBasePathPrefixesRedirectsAndLinks(t *testing.T) {
	app := newApp(t, func(c *config.Config) { c.BasePath = "/rafactools" })
	c := app.anon()
	wantRedirect(t, c.get("/"), "/rafactools/login")
	page := body(c.get("/login"))
	if !strings.Contains(page, `action="/rafactools/login"`) || !strings.Contains(page, `href="/rafactools/static/css/app.css"`) {
		t.Errorf("login page links are not prefixed:\n%.600s", page)
	}
	// Paths must not leak unprefixed absolute links.
	for _, m := range regexpHref.FindAllStringSubmatch(page, -1) {
		if strings.HasPrefix(m[1], "/") && !strings.HasPrefix(m[1], "/rafactools/") {
			t.Errorf("unprefixed internal link %q", m[1])
		}
	}
}
