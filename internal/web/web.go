// Package web is the server-rendered website: plain HTML forms posting back
// to full-page handlers, no JS framework. Deliberately simple — the user
// base is a handful of squadron staff per unit, not a public product.
package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/config"
	"rafactools-planner/internal/mail"
	"rafactools-planner/internal/programme"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type Deps struct {
	Auth  *auth.Service
	Store *programme.Store
	Mail  mail.Sender
	Cfg   config.Config
}

type server struct {
	*Deps
	pages map[string]*template.Template
}

// pageNames are the content templates under templates/, one per file
// (excluding layout.html and error.html, handled separately). Each is
// parsed together with layout.html into its own template set — parsing them
// all into one set would make every file's {{define "content"}} collide,
// since html/template keeps a single flat namespace per set.
var pageNames = []string{
	"claim", "login", "check_email", "invite_accept",
	"dashboard", "squadron_settings", "programme_edit",
}

// urlFunc builds the "url" template function: it joins basePath with the
// stringified concatenation of its arguments, e.g. {{url "/programmes/" .ID}}.
// Every internal href/form-action in the templates must go through this (or
// the equivalent server.url helper for redirects) instead of a bare "/path"
// literal — see the comment on config.Config.BasePath for why.
func urlFunc(basePath string) func(...any) string {
	return func(parts ...any) string {
		var b strings.Builder
		b.WriteString(basePath)
		for _, p := range parts {
			fmt.Fprint(&b, p)
		}
		return b.String()
	}
}

func loadPages(basePath string) map[string]*template.Template {
	funcs := template.FuncMap{"url": urlFunc(basePath)}
	pages := make(map[string]*template.Template, len(pageNames)+1)
	for _, name := range pageNames {
		pages[name] = template.Must(template.New("layout.html").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html"))
	}
	pages["error"] = template.Must(template.New("layout.html").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/error.html"))
	return pages
}

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSessionToken
	ctxAnonCSRF
)

func NewRouter(d *Deps) http.Handler {
	s := &server{Deps: d, pages: loadPages(d.Cfg.BasePath)}

	mux := http.NewServeMux()

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	mux.HandleFunc("GET /health", s.health)

	mux.HandleFunc("GET /claim", s.withUserOptional(s.claimForm))
	mux.HandleFunc("POST /claim", s.withUserOptional(s.claimSubmit))
	mux.HandleFunc("GET /check-email", s.withUserOptional(s.checkEmail))
	mux.HandleFunc("GET /verify", s.withUserOptional(s.verify))
	mux.HandleFunc("GET /login", s.withUserOptional(s.loginForm))
	mux.HandleFunc("POST /login", s.withUserOptional(s.loginSubmit))
	mux.HandleFunc("POST /logout", s.requireAuth(s.logout))
	mux.HandleFunc("GET /invite/{token}", s.withUserOptional(s.inviteForm))
	mux.HandleFunc("POST /invite/{token}", s.withUserOptional(s.inviteSubmit))

	mux.HandleFunc("GET /", s.requireAuth(s.dashboard))

	mux.HandleFunc("GET /squadron", s.requireOwner(s.squadronSettings))
	mux.HandleFunc("POST /squadron", s.requireOwner(s.squadronSave))
	mux.HandleFunc("POST /squadron/invites", s.requireOwner(s.inviteCreate))
	mux.HandleFunc("POST /squadron/invites/{token}/revoke", s.requireOwner(s.inviteRevoke))

	mux.HandleFunc("POST /programmes", s.requireEditor(s.programmeCreate))
	mux.HandleFunc("GET /programmes/{id}", s.requireAuth(s.programmeEdit))
	mux.HandleFunc("POST /programmes/{id}", s.requireEditor(s.programmeSave))
	mux.HandleFunc("GET /programmes/{id}/pdf", s.requireAuth(s.programmePDF))
	mux.HandleFunc("POST /programmes/{id}/days/{dayID}/add-half", s.requireEditor(s.dayAddHalf))
	mux.HandleFunc("POST /programmes/{id}/closures", s.requireEditor(s.closureAdd))
	mux.HandleFunc("POST /programmes/{id}/closures/{closureID}/delete", s.requireEditor(s.closureDelete))
	mux.HandleFunc("POST /programmes/{id}/weekend-notes", s.requireEditor(s.weekendNoteAdd))
	mux.HandleFunc("POST /programmes/{id}/weekend-notes/{noteID}/delete", s.requireEditor(s.weekendNoteDelete))
	mux.HandleFunc("POST /programmes/{id}/requirements", s.requireEditor(s.reqCategoryAdd))
	mux.HandleFunc("POST /programmes/{id}/requirements/{catID}/delete", s.requireEditor(s.reqCategoryDelete))
	mux.HandleFunc("POST /programmes/{id}/requirements/{catID}/entries", s.requireEditor(s.reqEntryAdd))
	mux.HandleFunc("POST /programmes/{id}/requirements/{catID}/entries/{entryID}/delete", s.requireEditor(s.reqEntryDelete))

	return mux
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "ok"}`))
}

// --- rendering ---------------------------------------------------------

func (s *server) render(w http.ResponseWriter, r *http.Request, page, title string, extra map[string]any) {
	tmpl, ok := s.pages[page]
	if !ok {
		log.Printf("unknown page template %q", page)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := map[string]any{
		"Title": title,
		"User":  userFrom(r.Context()),
		"CSRF":  s.csrfFor(r),
		"Flash": "",
		"Error": "",
	}
	for k, v := range extra {
		data[k] = v
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (s *server) renderError(w http.ResponseWriter, r *http.Request, status int, title, message string) {
	w.WriteHeader(status)
	s.render(w, r, "error", title, map[string]any{"Error": message})
}

// url prefixes an internal path with BasePath for use in redirects — the Go
// equivalent of the "url" template func (urlFunc). Every http.Redirect
// target in this package must go through this; see config.Config.BasePath.
func (s *server) url(path string) string {
	return s.Cfg.BasePath + path
}

// csrfFor returns the token to embed in a page's forms: HMAC-derived from
// the session when logged in, otherwise the request's anon double-submit
// cookie (see ensureAnonCSRFCookie).
func (s *server) csrfFor(r *http.Request) string {
	if tok, _ := r.Context().Value(ctxSessionToken).(string); tok != "" {
		return s.Auth.CSRFToken(tok)
	}
	anon, _ := r.Context().Value(ctxAnonCSRF).(string)
	return anon
}

func (s *server) checkCSRF(r *http.Request) bool {
	submitted := r.FormValue("csrf")
	if tok, _ := r.Context().Value(ctxSessionToken).(string); tok != "" {
		return s.Auth.ValidCSRF(tok, submitted)
	}
	c, err := r.Cookie(anonCSRFCookieName)
	return err == nil && c.Value != "" && c.Value == submitted
}

func userFrom(ctx context.Context) *auth.User {
	u, _ := ctx.Value(ctxUser).(*auth.User)
	return u
}
