package docs

import (
	"fmt"
	"strconv"
	"strings"
)

// parsePage splits front matter from the body. The format is deliberately a
// strict subset of YAML so no YAML dependency is needed and mistakes are
// errors, not silent omissions:
//
//	---
//	title: Inviting people
//	order: 20
//	summary: One line for the page list.
//	since: 1.0
//	changed:
//	  - next: What changed.
//	  - 1.42: An earlier change.
//	---
func parsePage(slug, raw string) (*Page, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return nil, fmt.Errorf("missing front matter (file must start with ---)")
	}
	rest := raw[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return nil, fmt.Errorf("front matter is not closed with ---")
	}
	fm, body := rest[:end], rest[end+5:]

	p := &Page{Slug: slug, Source: strings.TrimLeft(body, "\n")}
	seen := map[string]bool{}
	inChanged := false
	for i, line := range strings.Split(fm, "\n") {
		lineNo := i + 2
		if strings.TrimSpace(line) == "" {
			return nil, fmt.Errorf("front matter line %d: blank lines are not allowed", lineNo)
		}
		if strings.HasPrefix(line, "  - ") {
			if !inChanged {
				return nil, fmt.Errorf("front matter line %d: list item outside 'changed:'", lineNo)
			}
			ver, note, ok := strings.Cut(strings.TrimPrefix(line, "  - "), ": ")
			ver, note = strings.TrimSpace(ver), strings.TrimSpace(note)
			if !ok || note == "" {
				return nil, fmt.Errorf("front matter line %d: change must look like '  - <version>: <note>'", lineNo)
			}
			if !versionRe.MatchString(ver) {
				return nil, fmt.Errorf("front matter line %d: %q is not a version (next or 1.<number>)", lineNo, ver)
			}
			if n := len(p.Changed); n > 0 && CompareVersions(p.Changed[n-1].Version, ver) < 0 {
				return nil, fmt.Errorf("front matter line %d: changes must be newest first", lineNo)
			}
			p.Changed = append(p.Changed, Change{Version: ver, Note: note})
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			return nil, fmt.Errorf("front matter line %d: unexpected indentation", lineNo)
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("front matter line %d: expected 'key: value'", lineNo)
		}
		val = strings.TrimSpace(val)
		if seen[key] {
			return nil, fmt.Errorf("front matter: %q appears twice", key)
		}
		seen[key] = true
		inChanged = false
		switch key {
		case "title":
			p.Title = val
		case "summary":
			p.Summary = val
		case "order":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("front matter: order must be a non-negative number")
			}
			p.Order = n
		case "since":
			if !versionRe.MatchString(val) {
				return nil, fmt.Errorf("front matter: since %q is not a version (next or 1.<number>)", val)
			}
			p.Since = val
		case "changed":
			if val != "" {
				return nil, fmt.Errorf("front matter line %d: 'changed:' takes a list on the following lines", lineNo)
			}
			inChanged = true
		default:
			return nil, fmt.Errorf("front matter: unknown key %q", key)
		}
	}
	for _, req := range []string{"title", "order", "summary", "since"} {
		if !seen[req] {
			return nil, fmt.Errorf("front matter: %q is required", req)
		}
	}
	if p.Title == "" || p.Summary == "" {
		return nil, fmt.Errorf("front matter: title and summary must not be empty")
	}
	if strings.TrimSpace(p.Source) == "" {
		return nil, fmt.Errorf("page has no content")
	}
	return p, nil
}
