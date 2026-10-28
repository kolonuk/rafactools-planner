package web

import (
	"database/sql"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"rafactools-planner/internal/docs"
	"regexp"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/config"
	"rafactools-planner/internal/db"
	"rafactools-planner/internal/programme"
)

const testPassword = "correct horse battery staple"

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard) // the mailer logs links when SMTP is unset
	os.Exit(m.Run())
}

// testApp is the real router over a real (temp-file) SQLite database, served
// by httptest. Nothing is mocked: tests exercise middleware, handlers,
// templates, auth and storage together, the way a browser would.
type testApp struct {
	t     *testing.T
	srv   *httptest.Server
	db    *sql.DB
	auth  *auth.Service
	store *programme.Store
	cfg   config.Config
	mail  *recordingMail
}

func newApp(t *testing.T, tweak ...func(*config.Config)) *testApp {
	t.Helper()
	return newAppWithDocs(t, testDocs(t), tweak...)
}

func newAppWithDocs(t *testing.T, lib *docs.Library, tweak ...func(*config.Config)) *testApp {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		SessionKey:  "test-session-key-0123456789abcdef0123",
		ClaimDomain: "rafac.mod.gov.uk",
		BaseURL:     "http://example.test",
	}
	for _, f := range tweak {
		f(&cfg)
	}
	a := &testApp{t: t, db: database, cfg: cfg, mail: &recordingMail{}}
	a.auth = auth.NewService(database, cfg.SessionKey, cfg.CookieSecure, cfg.ClaimDomain, cfg.SkipEmailVerification)
	a.store = programme.NewStore(database)
	a.srv = httptest.NewServer(NewRouter(&Deps{Auth: a.auth, Store: a.store, Mail: a.mail, Docs: lib, Cfg: cfg}))
	t.Cleanup(func() { a.srv.Close(); database.Close() })
	return a
}

// client is a cookie-keeping, non-redirect-following HTTP client bound to one
// user (or none). csrf is the token that user's forms would carry.
type client struct {
	t    *testing.T
	base string
	hc   *http.Client
	csrf string
	xff  string
}

func (a *testApp) anon() *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: a.t, base: a.srv.URL, hc: &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// owner claims squadron `number`, confirms the emailed token and returns the
// owner's email.
func (a *testApp) owner(number string) string {
	a.t.Helper()
	email := "oc." + number + "@rafac.mod.gov.uk"
	_, token, err := a.auth.ClaimSquadron(email, testPassword, number+" Sqn")
	if err != nil {
		a.t.Fatal(err)
	}
	if err := a.auth.VerifyEmail(token); err != nil {
		a.t.Fatal(err)
	}
	return email
}

// member adds a user with the given role to the squadron of `ownerEmail`
// through the real invite path.
func (a *testApp) member(ownerEmail, email, role string) {
	a.t.Helper()
	var sqID, ownerID int64
	if err := a.db.QueryRow(`SELECT squadron_id, id FROM users WHERE email = ?`, ownerEmail).Scan(&sqID, &ownerID); err != nil {
		a.t.Fatal(err)
	}
	tok, err := a.auth.CreateInvite(sqID, ownerID, email, role)
	if err != nil {
		a.t.Fatal(err)
	}
	if _, _, err := a.auth.AcceptInvite(tok, testPassword); err != nil {
		a.t.Fatal(err)
	}
}

func (a *testApp) squadronID(email string) int64 {
	a.t.Helper()
	var id int64
	if err := a.db.QueryRow(`SELECT squadron_id FROM users WHERE email = ?`, email).Scan(&id); err != nil {
		a.t.Fatal(err)
	}
	return id
}

// login drives the real login form (anon CSRF cookie and all) and returns a
// client carrying the resulting session.
func (a *testApp) login(email string) *client {
	a.t.Helper()
	c := a.anon()
	resp := c.get("/login")
	token := scrapeCSRF(a.t, resp)
	resp = c.postRaw("/login", url.Values{"csrf": {token}, "email": {email}, "password": {testPassword}})
	if resp.StatusCode != http.StatusSeeOther {
		a.t.Fatalf("login as %s: status %d, want 303", email, resp.StatusCode)
	}
	for _, ck := range c.hc.Jar.Cookies(mustURL(c.base)) {
		if ck.Name == sessionCookieName {
			c.csrf = a.auth.CSRFToken(ck.Value)
		}
	}
	if c.csrf == "" {
		a.t.Fatal("no session cookie after login")
	}
	return c
}

func mustURL(s string) *url.URL { u, _ := url.Parse(s); return u }

var csrfInput = regexp.MustCompile(`name="csrf" value="([0-9a-f]+)"`)

func scrapeCSRF(t *testing.T, resp *http.Response) string {
	t.Helper()
	m := csrfInput.FindStringSubmatch(body(resp))
	if m == nil {
		t.Fatal("no csrf hidden input in page")
	}
	return m[1]
}

func body(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(b)
}

func (c *client) do(req *http.Request) *http.Response {
	c.t.Helper()
	if c.xff != "" {
		req.Header.Set("X-Forwarded-For", c.xff)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

func (c *client) get(path string) *http.Response {
	c.t.Helper()
	req, _ := http.NewRequest("GET", c.base+path, nil)
	return c.do(req)
}

// postRaw posts form exactly as given.
func (c *client) postRaw(path string, form url.Values) *http.Response {
	c.t.Helper()
	req, _ := http.NewRequest("POST", c.base+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

// post adds this client's CSRF token unless the caller set one.
func (c *client) post(path string, form url.Values) *http.Response {
	c.t.Helper()
	if form == nil {
		form = url.Values{}
	}
	if _, ok := form["csrf"]; !ok {
		form.Set("csrf", c.csrf)
	}
	return c.postRaw(path, form)
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d\n%.300s", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want, body(resp))
	}
	resp.Body.Close()
}

func wantRedirect(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	wantStatus(t, resp, http.StatusSeeOther)
	if got := resp.Header.Get("Location"); got != want {
		t.Fatalf("redirect to %q, want %q", got, want)
	}
}

func count(t *testing.T, d *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := d.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

var regexpHref = regexp.MustCompile(`(?:href|action)="([^"]+)"`)

// testDocs is a two-version docs library so version selection and
// comparison are exercised; the real embedded docs are validated separately
// in internal/docs.
func testDocs(t *testing.T) *docs.Library {
	t.Helper()
	page := func(title, since, changed, body string) *fstest.MapFile {
		fm := "---\ntitle: " + title + "\norder: 1\nsummary: s\nsince: " + since + "\n" + changed + "---\n" + body
		return &fstest.MapFile{Data: []byte(fm)}
	}
	lib, err := docs.New(fstest.MapFS{
		"1.1/index.md":  page("Home", "1.0", "", "Welcome one.\n\nSee [roles](roles).\n"),
		"1.1/roles.md":  page("Roles", "1.0", "", "Owners edit.\nViewers read.\n"),
		"next/index.md": page("Home", "1.0", "changed:\n  - next: Reworded.\n", "Welcome two.\n\nSee [roles](roles) and [settings](app:/squadron).\n\nRaw <script>alert(1)</script> html.\n"),
		"next/roles.md": page("Roles", "1.0", "changed:\n  - next: Viewers can export PDFs.\n", "Owners edit.\nEditors edit.\nViewers read and export.\n"),
		"next/extra.md": page("Extra", "next", "", "A new page.\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

// recordingMail stands in for the SMTP sender and keeps what would have been
// emailed, so tests can follow the real links. Sending happens in the
// background in the handlers, so reads wait briefly.
type recordingMail struct {
	mu   sync.Mutex
	sent []sentMail
}

type sentMail struct{ Kind, To, Token string }

func (m *recordingMail) add(kind, to, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMail{kind, to, token})
	return nil
}

func (m *recordingMail) SendVerification(to, _, token string) error {
	return m.add("verify", to, token)
}
func (m *recordingMail) SendInvite(to, _, token, _, _ string) error {
	return m.add("invite", to, token)
}
func (m *recordingMail) SendPasswordReset(to, _, token string) error {
	return m.add("reset", to, token)
}

// waitFor returns the first matching message, waiting up to two seconds.
func (m *recordingMail) waitFor(t *testing.T, kind, to string) (sentMail, bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		m.mu.Lock()
		for _, s := range m.sent {
			if s.Kind == kind && s.To == to {
				m.mu.Unlock()
				return s, true
			}
		}
		m.mu.Unlock()
		if time.Now().After(deadline) {
			return sentMail{}, false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// count returns how many messages of a kind went out (after a short settle,
// so a wrongly-sent background email would have arrived).
func (m *recordingMail) count(kind string) int {
	time.Sleep(150 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.sent {
		if s.Kind == kind {
			n++
		}
	}
	return n
}
