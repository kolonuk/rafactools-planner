package docs

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// URLs turns the two kinds of internal link a page may contain into real
// URLs. Pages never contain hand-written paths, so the app can move (e.g.
// under a reverse-proxy prefix) without touching the docs.
type URLs struct {
	// Page returns the URL of another page in the same version.
	Page func(slug string) string
	// App returns the URL of an app screen, from a path like "/squadron".
	App func(path string) string
}

var identityURLs = URLs{
	Page: func(slug string) string { return slug },
	App:  func(path string) string { return path },
}

// Render converts a page to HTML. Raw HTML in the Markdown is never passed
// through (goldmark's default), images are not supported, and every link
// must be a page slug, an app: link, an in-page #anchor, or https:/mailto:.
func (v *Version) Render(p *Page, urls URLs) (template.HTML, error) {
	var linkErr error
	t := &linkTransformer{v: v, urls: urls, err: &linkErr}
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(util.Prioritized(t, 100)),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(p.Source), &buf); err != nil {
		return "", fmt.Errorf("render %s/%s: %w", v.Label, p.Slug, err)
	}
	if linkErr != nil {
		return "", fmt.Errorf("%s/%s: %w", v.Label, p.Slug, linkErr)
	}
	// Safe: goldmark escapes text and drops raw HTML unless WithUnsafe is
	// set, which it is not; link destinations are rewritten or rejected by
	// the transformer above.
	// Table headers need a scope for screen readers; goldmark does not add one.
	// Raw HTML is disabled, so "<th>" can only come from a Markdown table.
	out := strings.ReplaceAll(buf.String(), "<th>", `<th scope="col">`)
	return template.HTML(out), nil // #nosec G203
}

type linkTransformer struct {
	v    *Version
	urls URLs
	err  *error
}

func (t *linkTransformer) fail(format string, a ...any) {
	if *t.err == nil {
		*t.err = fmt.Errorf(format, a...)
	}
}

func (t *linkTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Image:
			t.fail("images are not supported (%q)", n.Destination)
		case *ast.AutoLink:
			// <https://...> style links: only https and mailto, same as links.
			u := string(n.URL(src))
			if n.AutoLinkType != ast.AutoLinkEmail && !strings.HasPrefix(u, "https://") {
				t.fail("autolink %q must be https: or an email address", u)
			}
		case *ast.Link:
			dest := string(n.Destination)
			switch {
			case strings.HasPrefix(dest, "#"):
			case strings.HasPrefix(dest, "app:/"):
				n.Destination = []byte(t.urls.App(strings.TrimPrefix(dest, "app:")))
			case strings.HasPrefix(dest, "https://"), strings.HasPrefix(dest, "mailto:"):
			default:
				slug, frag, _ := strings.Cut(dest, "#")
				if _, ok := t.v.Page(slug); !ok {
					t.fail("link %q: no page %q in version %s (use a page slug, app:/path, or https://)", dest, slug, t.v.Label)
					break
				}
				u := t.urls.Page(slug)
				if frag != "" {
					u += "#" + frag
				}
				n.Destination = []byte(u)
			}
		}
		return ast.WalkContinue, nil
	})
}

// validateLinks renders every page with identity URLs so broken links and
// unsupported constructs surface when the library is built.
func (v *Version) validateLinks() error {
	for _, p := range v.Pages {
		if _, err := v.Render(p, identityURLs); err != nil {
			return fmt.Errorf("docs: %w", err)
		}
	}
	return nil
}
