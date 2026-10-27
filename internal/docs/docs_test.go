package docs

import (
	"strings"
	"testing"
	"testing/fstest"
)

func page(extraFM, body string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("---\ntitle: T\norder: 1\nsummary: S\nsince: 1.0\n" + extraFM + "---\n" + body)}
}

func mustLib(t *testing.T, fsys fstest.MapFS) *Library {
	t.Helper()
	lib, err := New(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

// The docs compiled into the binary are what users read, and what CI gates:
// this is "the docs check". It fails on bad front matter, unknown version
// labels, broken links, images, raw schemes, or a version with no index.
func TestEmbeddedDocsAreValid(t *testing.T) {
	lib, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	next, ok := lib.Version(Next)
	if !ok {
		t.Fatal("no content/next working copy")
	}
	if len(next.Pages) < 5 {
		t.Errorf("only %d pages in next/", len(next.Pages))
	}
	// Every page must be reachable from the index (directly linked).
	idx, _ := next.Page("index")
	linked := map[string]bool{"index": true}
	for _, p := range next.Pages {
		if strings.Contains(idx.Source, "]("+p.Slug+")") {
			linked[p.Slug] = true
		}
	}
	for _, p := range next.Pages {
		if !linked[p.Slug] {
			t.Errorf("page %q is not linked from index.md", p.Slug)
		}
	}
	// Rendering with real-looking URLs must not fail either.
	for _, v := range lib.versions {
		for _, p := range v.Pages {
			if _, err := v.Render(p, URLs{Page: func(s string) string { return "/docs/" + v.Label + "/" + s }, App: func(s string) string { return "/x" + s }}); err != nil {
				t.Errorf("%s/%s: %v", v.Label, p.Slug, err)
			}
		}
	}
}

func TestFrontMatterErrors(t *testing.T) {
	good := "title: T\norder: 1\nsummary: S\nsince: 1.0\n"
	for name, raw := range map[string]string{
		"no front matter":      "just text",
		"unclosed":             "---\n" + good + "body",
		"missing title":        "---\norder: 1\nsummary: S\nsince: 1.0\n---\nbody",
		"missing since":        "---\ntitle: T\norder: 1\nsummary: S\n---\nbody",
		"bad since":            "---\ntitle: T\norder: 1\nsummary: S\nsince: 2.0\n---\nbody",
		"since not a version":  "---\ntitle: T\norder: 1\nsummary: S\nsince: soon\n---\nbody",
		"bad order":            "---\ntitle: T\norder: first\nsummary: S\nsince: 1.0\n---\nbody",
		"unknown key":          "---\n" + good + "tags: x\n---\nbody",
		"duplicate key":        "---\n" + good + "title: again\n---\nbody",
		"blank line in fm":     "---\ntitle: T\n\norder: 1\nsummary: S\nsince: 1.0\n---\nbody",
		"change bad version":   "---\n" + good + "changed:\n  - tomorrow: x\n---\nbody",
		"change no note":       "---\n" + good + "changed:\n  - next:\n---\nbody",
		"change oldest first":  "---\n" + good + "changed:\n  - 1.1: a\n  - next: b\n---\nbody",
		"list outside changed": "---\n" + good + "  - next: x\n---\nbody",
		"changed with value":   "---\n" + good + "changed: next\n---\nbody",
		"empty body":           "---\n" + good + "---\n\n",
	} {
		if _, err := New(fstest.MapFS{"next/index.md": {Data: []byte(raw)}}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := New(fstest.MapFS{"next/index.md": page("changed:\n  - next: a\n  - 1.2: b\n", "ok")}); err != nil {
		t.Errorf("valid page rejected: %v", err)
	}
}

func TestLibraryStructureErrors(t *testing.T) {
	ok := page("", "Hello")
	for name, fsys := range map[string]fstest.MapFS{
		"empty":                {},
		"stray top-level file": {"README.md": ok, "next/index.md": ok},
		"bad version dir":      {"v2/index.md": ok},
		"two-part only":        {"1.x/index.md": ok},
		"no index":             {"next/other.md": ok},
		"non-md file":          {"next/index.md": ok, "next/logo.png": {Data: []byte("x")}},
		"bad slug":             {"next/index.md": ok, "next/Bad_Slug.md": ok},
		"nested dir":           {"next/index.md": ok, "next/sub/x.md": ok},
		"since newer than ver": {"1.1/index.md": {Data: []byte("---\ntitle: T\norder: 1\nsummary: S\nsince: 1.5\n---\nx")}},
	} {
		if _, err := New(fsys); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestVersionOrderingAndDefault(t *testing.T) {
	lib := mustLib(t, fstest.MapFS{
		"1.9/index.md": page("", "a"), "1.10/index.md": page("", "a"), "1.2/index.md": page("", "a"), "next/index.md": page("", "a"),
	})
	got := strings.Join(lib.Versions(), ",")
	if got != "next,1.10,1.9,1.2" {
		t.Errorf("order = %s (1.10 must sort after 1.9 numerically)", got)
	}
	if lib.Latest() != "next" {
		t.Error("latest should be next")
	}
	for app, want := range map[string]string{"1.9": "1.9", "1.10": "1.10", "1.3": "next", "dev": "next", "": "next", "2.0": "next"} {
		if g := lib.DefaultFor(app); g != want {
			t.Errorf("DefaultFor(%q) = %q, want %q", app, g, want)
		}
	}
	if CompareVersions("next", "1.999") <= 0 || CompareVersions("1.2", "1.10") >= 0 || CompareVersions("1.4", "1.4") != 0 {
		t.Error("CompareVersions wrong")
	}
	if DisplayLabel("next") != "Unreleased" || DisplayLabel("1.4") != "1.4" {
		t.Error("DisplayLabel wrong")
	}
}

func TestPageOrdering(t *testing.T) {
	mk := func(order, body string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("---\ntitle: T\norder: " + order + "\nsummary: S\nsince: 1.0\n---\n" + body)}
	}
	lib := mustLib(t, fstest.MapFS{"next/index.md": mk("0", "x"), "next/b.md": mk("5", "x"), "next/a.md": mk("5", "x"), "next/z.md": mk("1", "x")})
	v, _ := lib.Version("next")
	var got []string
	for _, p := range v.Pages {
		got = append(got, p.Slug)
	}
	if strings.Join(got, ",") != "index,z,a,b" {
		t.Errorf("order = %v", got)
	}
}

func TestChangesBetween(t *testing.T) {
	p, err := parsePage("x", "---\ntitle: T\norder: 1\nsummary: S\nsince: 1.0\nchanged:\n  - next: n\n  - 1.5: five\n  - 1.2: two\n---\nbody")
	if err != nil {
		t.Fatal(err)
	}
	notes := func(from, to string) string {
		var s []string
		for _, c := range p.ChangesBetween(from, to) {
			s = append(s, c.Note)
		}
		return strings.Join(s, ",")
	}
	for _, tc := range []struct{ from, to, want string }{
		{"1.0", "next", "n,five,two"},
		{"1.2", "next", "n,five"}, // from is exclusive: you already have 1.2
		{"1.2", "1.5", "five"},    // to is inclusive
		{"1.5", "next", "n"},
		{"1.0", "1.1", ""},
	} {
		if got := notes(tc.from, tc.to); got != tc.want {
			t.Errorf("%s..%s = %q, want %q", tc.from, tc.to, got, tc.want)
		}
	}
}

var testURLs = URLs{
	Page: func(s string) string { return "/docs/next/" + s },
	App:  func(s string) string { return "/base" + s },
}

func renderOne(t *testing.T, body string) (string, error) {
	t.Helper()
	lib, err := New(fstest.MapFS{"next/index.md": page("", "Home\n"), "next/other.md": page("", "Other\n")})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := lib.Version("next")
	p := &Page{Slug: "t", Source: body}
	h, err := v.Render(p, testURLs)
	return string(h), err
}

func TestRenderRewritesLinks(t *testing.T) {
	html, err := renderOne(t, "[a](other) [b](other#sec) [c](app:/squadron) [d](#local) [e](https://example.org/x) [f](mailto:a@b.c)")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`href="/docs/next/other"`, `href="/docs/next/other#sec"`, `href="/base/squadron"`, `href="#local"`, `href="https://example.org/x"`, `href="mailto:a@b.c"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in %s", want, html)
		}
	}
}

func TestRenderRejectsBadLinksAndImages(t *testing.T) {
	for name, body := range map[string]string{
		"unknown page":  "[x](nope)",
		"http":          "[x](http://example.org)",
		"javascript":    "[x](javascript:alert(1))",
		"data":          "[x](data:text/html,hi)",
		"absolute path": "[x](/etc/passwd)",
		"parent path":   "[x](../secret)",
		"page with ext": "[x](other.md)",
		"image":         "![x](https://example.org/a.png)",
		"autolink http": "<http://example.org>",
		"app no slash":  "[x](app:squadron)",
		"empty":         "[x]()",
	} {
		if _, err := renderOne(t, body); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Raw HTML in a page must never reach the browser.
func TestRenderDoesNotPassRawHTML(t *testing.T) {
	html, err := renderOne(t, "Hello <script>alert(1)</script> <img src=x onerror=alert(2)>\n\n<div onclick=\"x()\">block</div>\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script", "<img", "onerror=", "onclick=", "<div"} {
		if strings.Contains(html, bad) {
			t.Errorf("raw HTML %q passed through: %s", bad, html)
		}
	}
}

func TestRenderFeatures(t *testing.T) {
	html, err := renderOne(t, "# Title\n\n## Sub heading\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n`code` and **bold**\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<h2 id="sub-heading">`, "<table>", `<th scope="col">a</th>`, "<code>code</code>", "<strong>bold</strong>"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in %s", want, html)
		}
	}
}

func TestDiff(t *testing.T) {
	d := Diff("a\nb\nc\n", "a\nB\nc\nd\n")
	var got []string
	for _, l := range d {
		got = append(got, l.Kind[:1]+":"+l.Text)
	}
	want := "s:a d:b a:B s:c a:d"
	if strings.Join(got, " ") != want {
		t.Errorf("diff = %v, want %s", got, want)
	}
	if !Changed(d) || Changed(Diff("x\ny", "x\ny")) {
		t.Error("Changed wrong")
	}
	if got := Diff("", "a"); len(got) != 1 || got[0].Kind != "add" {
		t.Errorf("diff from empty = %v", got)
	}
	if got := Diff("a", ""); len(got) != 1 || got[0].Kind != "del" {
		t.Errorf("diff to empty = %v", got)
	}
	// CRLF vs LF is not a change.
	if Changed(Diff("a\r\nb\r\n", "a\nb\n")) {
		t.Error("line endings reported as a change")
	}
	// Huge input is bounded, not quadratic-explosive.
	big := strings.Repeat("line\n", 50000)
	if n := len(Diff(big, big+"x")); n > 2*maxDiffLines+1 {
		t.Errorf("unbounded diff: %d lines", n)
	}
}
