// Package docs is the in-app documentation: Markdown pages embedded in the
// binary, versioned with the app, rendered inside the app's own layout.
//
// Layout of the embedded tree:
//
//	content/next/<slug>.md   working copy, shown as "Unreleased"
//	content/1.42/<slug>.md   frozen snapshot taken at release 1.42
//
// Everything is validated when the library is built (front matter, version
// labels, links), so a bad page fails the tests in CI and fails startup
// rather than failing for a reader. See .claude/skills/docs/SKILL.md for the
// authoring rules.
package docs

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed all:content
var embedded embed.FS

// Next is the label of the unreleased working copy.
const Next = "next"

var (
	versionRe = regexp.MustCompile(`^(?:next|1\.[0-9]+)$`)
	slugRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

// Audience says who a page is written for. The help is shown as a tree with
// one branch per audience.
type Audience string

const (
	// AudienceUser is for everyone who uses the planner day to day.
	AudienceUser Audience = "user"
	// AudienceAdmin is for squadron owners, who manage members and settings.
	AudienceAdmin Audience = "admin"
	// AudienceDev is for whoever runs the site, and for contributors.
	AudienceDev Audience = "dev"
)

// audienceOrder is the order of the branches in the tree.
var audienceOrder = []Audience{AudienceUser, AudienceAdmin, AudienceDev}

// AudienceTitle is the heading of an audience's branch.
func AudienceTitle(a Audience) string {
	switch a {
	case AudienceAdmin:
		return "Squadron admins"
	case AudienceDev:
		return "Running the site (dev and power users)"
	}
	return "Using the planner"
}

// legacyAudience places pages whose front matter has no audience key. Frozen
// snapshots (which are never edited) predate the key; every page in next/
// must set it (a test enforces that).
var legacyAudience = map[string]Audience{
	"roles-and-invites": AudienceAdmin,
	"squadron-settings": AudienceAdmin,
	"run-your-own":      AudienceDev,
}

func audienceFor(slug string, declared Audience) Audience {
	if declared != "" {
		return declared
	}
	if a, ok := legacyAudience[slug]; ok {
		return a
	}
	if strings.HasPrefix(slug, "operators-") {
		return AudienceDev
	}
	return AudienceUser
}

// Change is one behaviour change recorded on a page, keyed by the version it
// arrived in.
type Change struct {
	Version string
	Note    string
}

// Page is one Markdown document within one version.
type Page struct {
	Slug    string
	Title   string
	Summary string
	Order   int
	Since   string
	// Audience is who the page is for; empty for the index, which belongs to
	// no branch of the tree.
	Audience Audience
	Changed  []Change // newest first
	Source   string   // Markdown body, front matter removed
}

// Version is the full set of pages for one release (or "next").
type Version struct {
	Label string
	Pages []*Page // by Order, then Slug
	index map[string]*Page
}

// Page returns the page with the given slug.
func (v *Version) Page(slug string) (*Page, bool) {
	p, ok := v.index[slug]
	return p, ok
}

// Library holds every version, newest first ("next" leads).
type Library struct {
	versions []*Version
	byLabel  map[string]*Version
}

// Embedded loads the docs compiled into the binary.
func Embedded() (*Library, error) {
	sub, err := fs.Sub(embedded, "content")
	if err != nil {
		return nil, err
	}
	return New(sub)
}

// New builds a library from a filesystem whose top-level directories are
// version labels. It validates everything it loads.
func New(fsys fs.FS) (*Library, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("docs: read root: %w", err)
	}
	lib := &Library{byLabel: map[string]*Version{}}
	for _, e := range entries {
		if !e.IsDir() {
			return nil, fmt.Errorf("docs: unexpected file %q at the top level (only version directories belong there)", e.Name())
		}
		if !versionRe.MatchString(e.Name()) {
			return nil, fmt.Errorf("docs: directory %q is not a version label (next or 1.<number>)", e.Name())
		}
		v, err := loadVersion(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		lib.versions = append(lib.versions, v)
		lib.byLabel[v.Label] = v
	}
	if len(lib.versions) == 0 {
		return nil, fmt.Errorf("docs: no versions found")
	}
	sort.Slice(lib.versions, func(i, j int) bool { return CompareVersions(lib.versions[i].Label, lib.versions[j].Label) > 0 })

	for _, v := range lib.versions {
		if err := v.validateLinks(); err != nil {
			return nil, err
		}
	}
	return lib, nil
}

func loadVersion(fsys fs.FS, label string) (*Version, error) {
	files, err := fs.ReadDir(fsys, label)
	if err != nil {
		return nil, fmt.Errorf("docs: read %s: %w", label, err)
	}
	v := &Version{Label: label, index: map[string]*Page{}}
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || !strings.HasSuffix(name, ".md") {
			return nil, fmt.Errorf("docs: %s/%s: only .md pages are allowed", label, name)
		}
		slug := strings.TrimSuffix(name, ".md")
		if !slugRe.MatchString(slug) {
			return nil, fmt.Errorf("docs: %s/%s: slug must be lower-case letters, digits and hyphens", label, name)
		}
		raw, err := fs.ReadFile(fsys, label+"/"+name)
		if err != nil {
			return nil, err
		}
		p, err := parsePage(slug, string(raw))
		if err != nil {
			return nil, fmt.Errorf("docs: %s/%s: %w", label, name, err)
		}
		if CompareVersions(p.Since, label) > 0 {
			return nil, fmt.Errorf("docs: %s/%s: since %s is newer than the version it is in", label, name, p.Since)
		}
		for _, c := range p.Changed {
			if CompareVersions(c.Version, label) > 0 {
				return nil, fmt.Errorf("docs: %s/%s: change %s is newer than the version it is in", label, name, c.Version)
			}
		}
		if slug != "index" {
			p.Audience = audienceFor(slug, p.Audience)
		}
		v.Pages = append(v.Pages, p)
		v.index[slug] = p
	}
	if _, ok := v.index["index"]; !ok {
		return nil, fmt.Errorf("docs: %s has no index.md", label)
	}
	sort.Slice(v.Pages, func(i, j int) bool {
		if v.Pages[i].Order != v.Pages[j].Order {
			return v.Pages[i].Order < v.Pages[j].Order
		}
		return v.Pages[i].Slug < v.Pages[j].Slug
	})
	return v, nil
}

// Group is one branch of the help tree.
type Group struct {
	Audience Audience
	Title    string
	Pages    []*Page
}

// Groups returns the pages other than the index, grouped by audience in tree
// order. Empty groups are left out.
func (v *Version) Groups() []Group {
	var out []Group
	for _, a := range audienceOrder {
		g := Group{Audience: a, Title: AudienceTitle(a)}
		for _, p := range v.Pages {
			if p.Slug != "index" && p.Audience == a {
				g.Pages = append(g.Pages, p)
			}
		}
		if len(g.Pages) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// Versions returns the version labels, newest first.
func (l *Library) Versions() []string {
	out := make([]string, len(l.versions))
	for i, v := range l.versions {
		out[i] = v.Label
	}
	return out
}

// Version returns one version by label.
func (l *Library) Version(label string) (*Version, bool) {
	v, ok := l.byLabel[label]
	return v, ok
}

// Latest is the newest version label (normally "next").
func (l *Library) Latest() string { return l.versions[0].Label }

// DefaultFor picks the version a reader should land on: the docs for the
// release they are running if a snapshot exists, otherwise the newest.
func (l *Library) DefaultFor(appVersion string) string {
	if _, ok := l.byLabel[appVersion]; ok {
		return appVersion
	}
	return l.Latest()
}

// DisplayLabel is the human name of a version label.
func DisplayLabel(label string) string {
	if label == Next {
		return "Unreleased"
	}
	return label
}

// CompareVersions orders version labels: "next" is newest, then 1.<n> by n.
// It returns <0, 0 or >0.
func CompareVersions(a, b string) int {
	rank := func(s string) int {
		if s == Next {
			return int(^uint(0) >> 1)
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(s, "1."))
		return n
	}
	ra, rb := rank(a), rank(b)
	switch {
	case ra < rb:
		return -1
	case ra > rb:
		return 1
	}
	return 0
}

// ChangesBetween lists a page's recorded changes that arrived after `from`
// up to and including `to`, newest first.
func (p *Page) ChangesBetween(from, to string) []Change {
	var out []Change
	for _, c := range p.Changed {
		if CompareVersions(c.Version, from) > 0 && CompareVersions(c.Version, to) <= 0 {
			out = append(out, c)
		}
	}
	return out
}
