package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"rafactools-planner/internal/config"
	"rafactools-planner/internal/docs"
)

func TestDocsRequireLogin(t *testing.T) {
	app := newApp(t)
	c := app.anon()
	for _, p := range []string{"/docs", "/docs/", "/docs/next/", "/docs/next/roles", "/docs/1.1/roles", "/docs/compare", "/docs/go?version=next"} {
		wantRedirect(t, c.get(p), "/login")
	}
}

func TestDocsLandOnTheNewestVersionForDevBuilds(t *testing.T) {
	app := newApp(t)
	c := app.login(app.owner("1101"))
	wantRedirect(t, c.get("/docs"), "/docs/next/")
	wantRedirect(t, c.get("/docs/"), "/docs/next/")
}

func TestDocsPageRendersInTheAppLayout(t *testing.T) {
	app := newApp(t)
	c := app.login(app.owner("1102"))
	page := body(c.get("/docs/next/"))

	for name, want := range map[string]string{
		"app stylesheet (same theme)":        `href="/static/css/app.css"`,
		"top bar of the main site":           `class="topbar"`,
		"help link in the main nav":          `href="/docs">Help</a>`,
		"link back to the planner":           `href="/">&larr; Back to the planner</a>`,
		"page title":                         "Home",
		"docs page link rewritten":           `href="/docs/next/roles"`,
		"app: link rewritten to the app":     `href="/squadron"`,
		"unreleased banner":                  "Unreleased",
		"version notes":                      "first appeared in version",
		"recorded change shown":              "Reworded.",
		"nav marks the current page":         `aria-current="page"`,
		"version selector lists both":        `<option value="1.1"`,
		"compare link present (2+ versions)": "/docs/compare?page=index",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("%s: %q not found", name, want)
		}
	}
	// Raw HTML from a docs page never reaches the browser.
	if strings.Contains(page, "<script>alert(1)") {
		t.Error("raw <script> from the markdown was output")
	}
}

func TestDocsOldVersionBannerPointsToLatest(t *testing.T) {
	app := newApp(t)
	c := app.login(app.owner("1103"))
	page := body(c.get("/docs/1.1/roles"))
	if !strings.Contains(page, "documentation for version <strong>1.1</strong>") || !strings.Contains(page, `href="/docs/next/roles"`) {
		t.Errorf("no pointer to the latest version of the page:\n%.1500s", page)
	}
	if strings.Contains(page, "Reworded.") || strings.Contains(page, "Viewers can export PDFs.") {
		t.Error("old version shows notes from a newer version")
	}
	if !strings.Contains(page, "Viewers read.") || strings.Contains(page, "Viewers read and export.") {
		t.Error("old version does not show its own text")
	}
}

func TestDocsAnyRoleCanRead(t *testing.T) {
	app := newApp(t)
	ownerEmail := app.owner("1104")
	app.member(ownerEmail, "viewer@example.org", "viewer")
	wantStatus(t, app.login("viewer@example.org").get("/docs/next/roles"), 200)
}

func TestDocsUnknownTargets(t *testing.T) {
	app := newApp(t)
	c := app.login(app.owner("1105"))
	for _, p := range []string{
		"/docs/9.9/", "/docs/next/nope", "/docs/1.1/extra", // extra exists only in next
		"/docs/next/..%2f..%2fetc%2fpasswd", "/docs/next/index.md", "/docs/..%2f/roles", "/docs/next/ROLES",
		"/docs/compare?from=nope&to=next", "/docs/compare?from=1.1&to=9.9",
		"/docs/go?version=nope",
	} {
		resp := c.get(p)
		if resp.StatusCode != 404 {
			t.Errorf("GET %s -> %d, want 404", p, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestDocsVersionSelector(t *testing.T) {
	app := newApp(t)
	c := app.login(app.owner("1106"))

	// Same page exists in the chosen version: stay on it.
	wantRedirect(t, c.get("/docs/go?version=1.1&page=roles"), "/docs/1.1/roles")
	wantRedirect(t, c.get("/docs/go?version=next&page=index"), "/docs/next/")
	// Page only exists in next: land on 1.1's home with an explanation.
	resp := c.get("/docs/go?version=1.1&page=extra")
	wantStatus(t, resp, 303)
	loc := resp.Header.Get("Location")
	if loc != "/docs/1.1/?missing=extra" {
		t.Fatalf("redirect = %q", loc)
	}
	home := body(c.get(loc))
	if !strings.Contains(home, "doesn’t exist in version 1.1") {
		t.Errorf("no explanation shown:\n%.800s", home)
	}
	// What the reader typed is escaped, and absurdly long values are dropped.
	evil := body(c.get("/docs/1.1/?missing=" + url.QueryEscape(`<script>alert(1)</script>`)))
	if strings.Contains(evil, "<script>alert(1)") {
		t.Error("missing= parameter reflected unescaped")
	}
	long := c.get("/docs/go?version=1.1&page=" + strings.Repeat("a", 500))
	wantRedirect(t, long, "/docs/1.1/")
	// No page given: version home.
	wantRedirect(t, c.get("/docs/go?version=1.1"), "/docs/1.1/")
}

func TestDocsCompare(t *testing.T) {
	app := newApp(t)
	c := app.login(app.owner("1107"))

	t.Run("overall summary", func(t *testing.T) {
		page := body(c.get("/docs/compare"))
		for _, want := range []string{"Extra", "Added", "Roles", "Changed", "Home", "/docs/compare?from=1.1&amp;to=next&amp;page=roles"} {
			if !strings.Contains(page, want) {
				t.Errorf("summary missing %q:\n%.2500s", want, page)
			}
		}
	})

	t.Run("page diff", func(t *testing.T) {
		page := body(c.get("/docs/compare?from=1.1&to=next&page=roles"))
		for _, want := range []string{
			"Comparing <strong>1.1</strong> with <strong>Unreleased</strong>",
			`class="diff-add"`, `class="diff-del"`, `class="diff-same"`,
			"Viewers read and export.", "Editors edit.", "Viewers can export PDFs.", // text and recorded change
		} {
			if !strings.Contains(page, want) {
				t.Errorf("diff missing %q", want)
			}
		}
	})

	t.Run("from and to are put in order", func(t *testing.T) {
		a := body(c.get("/docs/compare?from=next&to=1.1&page=roles"))
		if !strings.Contains(a, "Comparing <strong>1.1</strong> with <strong>Unreleased</strong>") {
			t.Error("reversed arguments not normalised")
		}
	})

	t.Run("page missing from one side", func(t *testing.T) {
		page := body(c.get("/docs/compare?from=1.1&to=next&page=extra"))
		if !strings.Contains(page, "did not exist in 1.1") || !strings.Contains(page, `class="diff-add"`) {
			t.Error("added page not explained/shown as additions")
		}
	})

	t.Run("identical text", func(t *testing.T) {
		page := body(c.get("/docs/compare?from=next&to=next&page=roles"))
		if !strings.Contains(page, "identical") {
			t.Error("no 'identical' message")
		}
	})

	t.Run("a page in neither version", func(t *testing.T) {
		wantStatus(t, c.get("/docs/compare?from=1.1&to=next&page=nope"), 404)
	})
}

func TestDocsWithASingleVersionHasNoCompare(t *testing.T) {
	one, err := docs.New(fstest.MapFS{"next/index.md": {Data: []byte("---\ntitle: Home\norder: 0\nsummary: s\nsince: 1.0\n---\nHello\n")}})
	if err != nil {
		t.Fatal(err)
	}
	app := newAppWithDocs(t, one)
	c := app.login(app.owner("1108"))
	page := body(c.get("/docs/next/"))
	if strings.Contains(page, "/docs/compare") {
		t.Error("compare link shown with only one version")
	}
	wantStatus(t, c.get("/docs/compare"), 404)
}

func TestDocsHonourTheBasePath(t *testing.T) {
	app := newApp(t, func(c *config.Config) { c.BasePath = "/planner" })
	c := app.anon()
	wantRedirect(t, c.get("/docs"), "/planner/login")
	// Logged-in checks use the prefix on every generated link.
	own := app.owner("1109")
	cl := app.anon()
	tok := scrapeCSRF(t, cl.get("/login"))
	cl.postRaw("/login", url.Values{"csrf": {tok}, "email": {own}, "password": {testPassword}}).Body.Close()
	page := body(cl.get("/docs/next/"))
	for _, want := range []string{`href="/planner/docs/next/roles"`, `href="/planner/squadron"`, `href="/planner/"`, `action="/planner/docs/go"`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, m := range regexpHref.FindAllStringSubmatch(page, -1) {
		if strings.HasPrefix(m[1], "/") && !strings.HasPrefix(m[1], "/planner/") && m[1] != "/planner" {
			t.Errorf("unprefixed internal link %q", m[1])
		}
	}
}

func TestDocsAreServedWithTheStrictHeaders(t *testing.T) {
	app := newApp(t)
	c := app.login(app.owner("1110"))
	resp := c.get("/docs/next/")
	resp.Body.Close()
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "script-src 'self'") || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("docs lack the app's security headers: %v", resp.Header)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status %d", resp.StatusCode)
	}
}
