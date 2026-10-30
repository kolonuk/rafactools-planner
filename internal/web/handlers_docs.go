package web

import (
	"net/http"
	"net/url"
	"sort"

	"rafactools-planner/internal/docs"
	"rafactools-planner/internal/version"
)

func (s *server) docsPageURL(ver, slug string) string {
	if slug == "index" {
		return s.url("/docs/" + ver + "/")
	}
	return s.url("/docs/" + ver + "/" + slug)
}

func (s *server) docsURLs(ver string) docs.URLs {
	return docs.URLs{
		Page: func(slug string) string { return s.docsPageURL(ver, slug) },
		App:  s.url,
	}
}

type docsVersionOption struct {
	Label, Display string
	Selected       bool
}

func (s *server) docsVersionOptions(selected string) []docsVersionOption {
	var out []docsVersionOption
	for _, v := range s.Docs.Versions() {
		out = append(out, docsVersionOption{Label: v, Display: docs.DisplayLabel(v), Selected: v == selected})
	}
	return out
}

func (s *server) docsRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, s.docsPageURL(s.Docs.DefaultFor(version.Version), "index"), http.StatusSeeOther)
}

// docsGo is the target of the version-selector form (a plain GET form, so no
// JavaScript is needed): it keeps the reader on the same page when the chosen
// version has it, and falls back to that version's home otherwise.
func (s *server) docsGo(w http.ResponseWriter, r *http.Request) {
	ver, page := r.URL.Query().Get("version"), r.URL.Query().Get("page")
	v, ok := s.Docs.Version(ver)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That documentation version doesn't exist.")
		return
	}
	if _, ok := v.Page(page); ok && page != "" {
		http.Redirect(w, r, s.docsPageURL(ver, page), http.StatusSeeOther)
		return
	}
	target := s.docsPageURL(ver, "index")
	if page != "" && page != "index" && len(page) <= 60 {
		target += "?missing=" + url.QueryEscape(page)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *server) docsPage(w http.ResponseWriter, r *http.Request) {
	ver := r.PathValue("version")
	slug := r.PathValue("slug")
	if slug == "" {
		slug = "index"
	}
	v, ok := s.Docs.Version(ver)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That documentation version doesn't exist.")
		return
	}
	page, ok := v.Page(slug)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That documentation page doesn't exist in this version.")
		return
	}
	body, err := v.Render(page, s.docsURLs(ver))
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Documentation", "This page could not be shown.")
		return
	}

	type navSection struct{ Title, URL string }
	type navItem struct {
		Title, URL string
		Current    bool
		Sections   []navSection // only for the current page
	}
	type navGroup struct {
		Title string
		Open  bool
		Items []navItem
	}
	var nav []navGroup
	for _, g := range v.Groups() {
		ng := navGroup{Title: g.Title}
		for _, p := range g.Pages {
			item := navItem{Title: p.Title, URL: s.docsPageURL(ver, p.Slug), Current: p.Slug == slug}
			if item.Current {
				ng.Open = true
				for _, h := range p.Outline() {
					item.Sections = append(item.Sections, navSection{h.Text, item.URL + "#" + h.ID})
				}
			}
			ng.Items = append(ng.Items, item)
		}
		nav = append(nav, ng)
	}
	// On the help home nothing is current, so open the first branch.
	if slug == "index" && len(nav) > 0 {
		nav[0].Open = true
	}
	type changeView struct{ Version, Note string }
	var changes []changeView
	for _, c := range page.Changed {
		changes = append(changes, changeView{docs.DisplayLabel(c.Version), c.Note})
	}

	versions := s.Docs.Versions()
	extra := map[string]any{
		"DocVersion":  docs.DisplayLabel(ver),
		"DocIsLatest": ver == s.Docs.Latest(),
		"DocLatest":   s.docsPageURL(s.Docs.Latest(), slug),
		"DocIsNext":   ver == docs.Next,
		"Versions":    s.docsVersionOptions(ver),
		"Nav":         nav,
		"HomeURL":     s.docsPageURL(ver, "index"),
		"HomeCurrent": slug == "index",
		"Page":        page,
		"Body":        body,
		"Since":       docs.DisplayLabel(page.Since),
		"Changes":     changes,
		"CanCompare":  len(versions) > 1,
		"CompareURL":  s.url("/docs/compare"),
		"GoURL":       s.url("/docs/go"),
		"CompareFrom": versions[min(1, len(versions)-1)],
		"CompareTo":   versions[0],
		"CompareHref": s.url("/docs/compare?page=" + url.QueryEscape(slug)),
	}
	if m := r.URL.Query().Get("missing"); m != "" && len(m) <= 60 {
		extra["Flash"] = "The page “" + m + "” doesn’t exist in version " + docs.DisplayLabel(ver) + ", so you’re on the home page."
	}
	s.render(w, r, "docs_page", page.Title+" · Help", extra)
}

type compareRow struct {
	Slug, Title, Status, URL string
}

func (s *server) docsCompare(w http.ResponseWriter, r *http.Request) {
	versions := s.Docs.Versions()
	if len(versions) < 2 {
		s.renderError(w, r, http.StatusNotFound, "Compare versions", "There is only one documentation version so far, so there is nothing to compare yet.")
		return
	}
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" {
		from = versions[1]
	}
	if to == "" {
		to = versions[0]
	}
	fv, ok1 := s.Docs.Version(from)
	tv, ok2 := s.Docs.Version(to)
	if !ok1 || !ok2 {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That documentation version doesn't exist.")
		return
	}
	// Always present oldest -> newest so "added"/"removed" read naturally.
	if docs.CompareVersions(from, to) > 0 {
		from, to, fv, tv = to, from, tv, fv
	}

	extra := map[string]any{
		"CompareURL": s.url("/docs/compare"),
		"Versions":   s.docsVersionOptions(""),
		"From":       from, "To": to,
		"FromDisplay": docs.DisplayLabel(from), "ToDisplay": docs.DisplayLabel(to),
		"FromOptions": s.docsVersionOptions(from), "ToOptions": s.docsVersionOptions(to),
		"HomeURL": s.docsPageURL(to, "index"),
	}

	if slug := q.Get("page"); slug != "" {
		fp, inFrom := fv.Page(slug)
		tp, inTo := tv.Page(slug)
		if !inFrom && !inTo {
			s.renderError(w, r, http.StatusNotFound, "Not found", "That page doesn't exist in either version.")
			return
		}
		var a, b, title string
		if inFrom {
			a, title = fp.Source, fp.Title
		}
		if inTo {
			b, title = tp.Source, tp.Title
		}
		lines := docs.Diff(a, b)
		type change struct{ Version, Note string }
		var notes []change
		if inTo {
			for _, c := range tp.ChangesBetween(from, to) {
				notes = append(notes, change{docs.DisplayLabel(c.Version), c.Note})
			}
		}
		extra["PageSlug"], extra["PageTitle"] = slug, title
		extra["Lines"] = lines
		extra["Differs"] = docs.Changed(lines)
		extra["Notes"] = notes
		extra["MissingFrom"], extra["MissingTo"] = !inFrom, !inTo
		extra["SummaryURL"] = s.url("/docs/compare?from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to))
		s.render(w, r, "docs_compare", "Compare · "+title, extra)
		return
	}

	slugs := map[string]bool{}
	for _, p := range fv.Pages {
		slugs[p.Slug] = true
	}
	for _, p := range tv.Pages {
		slugs[p.Slug] = true
	}
	var rows []compareRow
	for slug := range slugs {
		fp, inFrom := fv.Page(slug)
		tp, inTo := tv.Page(slug)
		var status, title string
		switch {
		case !inFrom:
			status, title = "Added", tp.Title
		case !inTo:
			status, title = "Removed", fp.Title
		default:
			title = tp.Title
			if docs.Changed(docs.Diff(fp.Source, tp.Source)) || len(tp.ChangesBetween(from, to)) > 0 {
				status = "Changed"
			}
		}
		if status == "" {
			continue
		}
		rows = append(rows, compareRow{slug, title, status,
			s.url("/docs/compare?from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to) + "&page=" + url.QueryEscape(slug))})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Title < rows[j].Title })
	extra["Rows"] = rows
	s.render(w, r, "docs_compare", "Compare versions", extra)
}
