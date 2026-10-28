// Package web is the server-rendered website: plain HTML forms posting back
// to full-page handlers, no JS framework. Deliberately simple — the user
// base is a handful of squadron staff per unit, not a public product.
package web

import (
	"context"
	"crypto/subtle"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/config"
	"rafactools-planner/internal/docs"
	"rafactools-planner/internal/mail"
	"rafactools-planner/internal/programme"
	"rafactools-planner/internal/version"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Mailer is what the handlers need to send. mail.Sender implements it; tests
// substitute a recorder so they can read the links that would be emailed.
type Mailer interface {
	SendVerification(to, baseURL, token string) error
	SendInvite(to, baseURL, token, squadronName, invitedByEmail string) error
	SendPasswordReset(to, baseURL, token string) error
	SendIssueReport(to string, r mail.IssueReport) error
}

var _ Mailer = mail.Sender{}

type Deps struct {
	Auth  *auth.Service
	Store *programme.Store
	Mail  Mailer
	Docs  *docs.Library
	Cfg   config.Config
}

type server struct {
	*Deps
	pages    map[string]*template.Template
	activity *activityLog
}

// pageNames are the content templates under templates/, one per file
// (excluding layout.html and error.html, handled separately). Each is
// parsed together with layout.html into its own template set — parsing them
// all into one set would make every file's {{define "content"}} collide,
// since html/template keeps a single flat namespace per set.
var pageNames = []string{
	"claim", "login", "check_email", "invite_accept", "forgot", "reset",
	"dashboard", "squadron_settings", "programme_edit", "confirm_remove_half",
	"docs_page", "docs_compare", "instructors", "account", "report", "report_sent",
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
	s := &server{Deps: d, pages: loadPages(d.Cfg.BasePath), activity: newActivityLog()}

	// 20 attempts per 5 minutes per IP on the endpoints worth scripting an
	// attack against (credential stuffing on login, mass squadron-claim
	// spam, invite-token guessing). Generous enough not to lock out a
	// fumbling human, tight enough to blunt automation.
	authLimiter := newIPLimiter(20, 5*time.Minute)
	// Reports carry an upload and send an email, and can be filed without
	// signing in, so they get a tighter limit of their own.
	reportLimiter := newIPLimiter(5, 10*time.Minute)

	mux := http.NewServeMux()

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	mux.HandleFunc("GET /health", s.health)

	mux.HandleFunc("GET /report", s.withUserOptional(s.reportForm))
	mux.HandleFunc("POST /report", reportLimiter.rateLimited(s.withUserOptional(s.reportSubmit)))

	mux.HandleFunc("GET /claim", s.withUserOptional(s.claimForm))
	mux.HandleFunc("POST /claim", authLimiter.rateLimited(s.withUserOptional(s.claimSubmit)))
	mux.HandleFunc("GET /check-email", s.withUserOptional(s.checkEmail))
	mux.HandleFunc("GET /verify", s.withUserOptional(s.verify))
	mux.HandleFunc("GET /login", s.withUserOptional(s.loginForm))
	mux.HandleFunc("POST /login", authLimiter.rateLimited(s.withUserOptional(s.loginSubmit)))
	mux.HandleFunc("POST /logout", s.requireAuth(s.logout))
	mux.HandleFunc("GET /forgot", s.withUserOptional(s.forgotForm))
	mux.HandleFunc("POST /forgot", authLimiter.rateLimited(s.withUserOptional(s.forgotSubmit)))
	mux.HandleFunc("GET /reset/{token}", s.withUserOptional(s.resetForm))
	mux.HandleFunc("POST /reset/{token}", authLimiter.rateLimited(s.withUserOptional(s.resetSubmit)))
	mux.HandleFunc("GET /invite/{token}", s.withUserOptional(s.inviteForm))
	mux.HandleFunc("POST /invite/{token}", authLimiter.rateLimited(s.withUserOptional(s.inviteSubmit)))

	mux.HandleFunc("GET /{$}", s.requireAuth(s.dashboard))

	// Documentation: behind login like everything else. Version labels are
	// validated against the library, never used as file paths.
	mux.HandleFunc("GET /docs", s.requireAuth(s.docsRoot))
	mux.HandleFunc("GET /docs/{$}", s.requireAuth(s.docsRoot))
	mux.HandleFunc("GET /docs/go", s.requireAuth(s.docsGo))
	mux.HandleFunc("GET /docs/compare", s.requireAuth(s.docsCompare))
	mux.HandleFunc("GET /docs/{version}/{$}", s.requireAuth(s.docsPage))
	mux.HandleFunc("GET /docs/{version}/{slug}", s.requireAuth(s.docsPage))

	mux.HandleFunc("GET /squadron", s.requireOwner(s.squadronSettings))
	mux.HandleFunc("POST /squadron", s.requireOwner(s.squadronSave))
	mux.HandleFunc("POST /squadron/invites", s.requireOwner(s.inviteCreate))
	mux.HandleFunc("POST /squadron/invites/{token}/revoke", s.requireOwner(s.inviteRevoke))
	mux.HandleFunc("POST /squadron/members/{memberID}/role", s.requireOwner(s.memberSetRole))
	mux.HandleFunc("POST /squadron/members/{memberID}/remove", s.requireOwner(s.memberRemove))

	mux.HandleFunc("GET /instructors", s.requireEditor(s.instructorsPage))
	mux.HandleFunc("POST /instructors", s.requireEditor(s.instructorAdd))
	mux.HandleFunc("POST /instructors/{id}/delete", s.requireEditor(s.instructorDelete))
	mux.HandleFunc("GET /account", s.requireAuth(s.accountPage))
	mux.HandleFunc("POST /account", s.requireAuth(s.accountSave))
	mux.HandleFunc("POST /account/switch", s.requireAuth(s.accountSwitch))
	mux.HandleFunc("POST /account/password", authLimiter.rateLimited(s.requireAuth(s.accountPassword)))

	mux.HandleFunc("POST /programmes", s.requireEditor(s.programmeCreate))
	mux.HandleFunc("GET /programmes/{id}", s.requireAuth(s.programmeEdit))
	mux.HandleFunc("POST /programmes/{id}", s.requireEditor(s.programmeSave))
	mux.HandleFunc("GET /programmes/{id}/pdf", s.requireAuth(s.programmePDF))
	mux.HandleFunc("POST /programmes/{id}/days/{dayID}/add-half", s.requireEditor(s.dayAddHalf))
	mux.HandleFunc("POST /programmes/{id}/days/{dayID}/move", s.requireEditor(s.dayMove))
	mux.HandleFunc("POST /programmes/{id}/days/{dayID}/remove-half", s.requireEditor(s.dayRemoveHalf))
	mux.HandleFunc("POST /programmes/{id}/closures", s.requireEditor(s.closureAdd))
	mux.HandleFunc("POST /programmes/{id}/closures/{closureID}/delete", s.requireEditor(s.closureDelete))
	mux.HandleFunc("POST /programmes/{id}/weekend-notes", s.requireEditor(s.weekendNoteAdd))
	mux.HandleFunc("POST /programmes/{id}/weekend-notes/{noteID}/delete", s.requireEditor(s.weekendNoteDelete))
	mux.HandleFunc("POST /programmes/{id}/requirements", s.requireEditor(s.reqCategoryAdd))
	mux.HandleFunc("POST /programmes/{id}/requirements/{catID}/delete", s.requireEditor(s.reqCategoryDelete))
	mux.HandleFunc("POST /programmes/{id}/requirements/{catID}/entries", s.requireEditor(s.reqEntryAdd))
	mux.HandleFunc("POST /programmes/{id}/requirements/{catID}/entries/{entryID}/delete", s.requireEditor(s.reqEntryDelete))

	// 1MB for every form; only the problem report (which may carry a
	// screenshot) is allowed more.
	return s.securityHeaders(s.recordActivity(maxBody(1<<20, map[string]int64{"/report": maxReportBody}, mux)))
}

// maxBody caps request bodies so a POST can't exhaust memory/disk parsing an
// oversized form — there's nothing in this app (no file uploads) that
// legitimately needs more than a tiny form payload.
func maxBody(limit int64, larger map[string]int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := limit
		if r.Method == http.MethodPost {
			if v, ok := larger[r.URL.Path]; ok {
				l = v
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, l)
		next.ServeHTTP(w, r)
	})
}

// securityHeaders sets a conservative baseline on every response. There are
// no third-party requests anywhere in this app (see layout.html's comment on
// why Google Fonts was dropped), so everything is limited to this origin.
// JavaScript and TypeScript are welcome where they are needed, but only as
// files served from /static/ (script-src 'self'): no inline <script>, no
// inline event handlers, no eval and no third-party scripts. Do not add
// 'unsafe-inline', 'unsafe-eval' or a host to script-src — a test fails if
// you do.
func (s *server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), payment=(), usb=(), display-capture=(self)")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data:; "+
			"script-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		// The router sees paths with any reverse-proxy prefix already stripped.
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			h.Set("Cache-Control", "no-store")
		}
		// Only when the operator has told us we're reached over TLS
		// (COOKIE_SECURE, which defaults true) — sending this over plain
		// HTTP would be a lie the browser then enforces incorrectly.
		if s.Auth.CookieSecure() {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
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
		"Title":         title,
		"User":          userFrom(r.Context()),
		"CSRF":          s.csrfFor(r),
		"Flash":         "",
		"Error":         "",
		"Version":       version.Version,
		"ReportEnabled": s.reportEnabled(),
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
	return err == nil && c.Value != "" && subtle.ConstantTimeCompare([]byte(c.Value), []byte(submitted)) == 1
}

func userFrom(ctx context.Context) *auth.User {
	u, _ := ctx.Value(ctxUser).(*auth.User)
	return u
}
