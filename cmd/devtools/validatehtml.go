package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/config"
	"rafactools-planner/internal/db"
	"rafactools-planner/internal/docs"
	"rafactools-planner/internal/mail"
	"rafactools-planner/internal/programme"
	"rafactools-planner/internal/settings"
	"rafactools-planner/internal/web"
)

var csrfRe = regexp.MustCompile(`name="csrf" value="([0-9a-f]+)"`)

// runValidateHTML serves the real site in-process against a throwaway
// database, fetches the public pages and (after signing in) every docs
// page, and validates the rendered HTML with html-validate. The templates
// are Go templates, which HTML linters cannot read, so rendered output is
// what gets checked. Needs node/npx for html-validate.
func runValidateHTML() error {
	tmp, err := os.MkdirTemp("", "validate-html")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	database, err := db.Open(filepath.Join(tmp, "t.db"))
	if err != nil {
		return err
	}
	defer database.Close()
	cfg := config.Config{
		SessionKey: "html-validate-session-key-0123456789abcdef", ClaimDomain: "rafac.mod.gov.uk",
		BaseURL: "http://localhost", SkipEmailVerification: true, IssueEmail: "issues@example.org", IssueMasking: true,
	}
	lib, err := docs.Embedded()
	if err != nil {
		return err
	}
	authSvc := auth.NewService(database, cfg.SessionKey, false, cfg.ClaimDomain, true)
	siteSettings, err := settings.New(database, cfg.SessionKey, config.MailKeys, []string{"SMTP_PASS"})
	if err != nil {
		return err
	}
	srv := httptest.NewServer(web.NewRouter(&web.Deps{
		Auth:  authSvc,
		Store: programme.NewStore(database), Mail: mail.Sender{}, Docs: lib, Cfg: cfg,
		Settings: siteSettings, Env: func(string) string { return "" },
	}))
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(path string) (string, error) {
		resp, err := client.Get(srv.URL + path)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("GET %s -> %d", path, resp.StatusCode)
		}
		return string(b), nil
	}
	token := func(path string) (string, error) {
		page, err := get(path)
		if err != nil {
			return "", err
		}
		m := csrfRe.FindStringSubmatch(page)
		if m == nil {
			return "", fmt.Errorf("no csrf token on %s", path)
		}
		return m[1], nil
	}
	post := func(path string) error {
		tok, err := token(path)
		if err != nil {
			return err
		}
		resp, err := client.PostForm(srv.URL+path, url.Values{"csrf": {tok}, "email": {"oc.1@rafac.mod.gov.uk"}, "password": {"a long enough password"}})
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			return fmt.Errorf("POST %s -> %d", path, resp.StatusCode)
		}
		return nil
	}

	pagesDir := filepath.Join(tmp, "pages")
	if err := os.Mkdir(pagesDir, 0o700); err != nil {
		return err
	}
	save := func(name, path string) error {
		page, err := get(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(pagesDir, name+".html"), []byte(page), 0o600)
	}

	for _, p := range []string{"login", "claim", "forgot"} {
		if err := save(p, "/"+p); err != nil {
			return err
		}
	}
	// Claim a squadron (dev mode skips the confirmation email) and sign
	// in, so the programme pages below can be validated.
	if err := post("/claim"); err != nil {
		return err
	}
	if err := post("/login"); err != nil {
		return err
	}
	// The programme editor is the most complex page (a large grid form with
	// controls that belong to separate forms), so validate it too.
	tok, err := token("/")
	if err != nil {
		return err
	}
	resp, err := client.PostForm(srv.URL+"/programmes", url.Values{"csrf": {tok}, "year": {"2030"}, "month": {"4"}})
	if err != nil {
		return err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusSeeOther || loc == "" {
		return fmt.Errorf("create programme -> %d", resp.StatusCode)
	}
	if err := save("programme-edit", loc); err != nil {
		return err
	}
	if err := save("confirm-reset-programme", loc+"/reset"); err != nil {
		return err
	}
	for name, path := range map[string]string{"instructors": "/instructors", "account": "/account", "squadron-settings": "/squadron", "report": "/report", "privacy": "/privacy"} {
		if err := save(name, path); err != nil {
			return err
		}
	}
	next, _ := lib.Version(docs.Next)
	for _, p := range next.Pages {
		path := "/docs/next/" + p.Slug
		if p.Slug == "index" {
			path = "/docs/next/"
		}
		if err := save("docs-"+p.Slug, path); err != nil {
			return err
		}
	}

	// The master admin's pages (a separate sign-in, so a separate cookie jar).
	const masterPW = "a long master admin password"
	if _, err := authSvc.EnsureMasterAdmin("master@example.org", masterPW); err != nil {
		return err
	}
	mjar, _ := cookiejar.New(nil)
	mclient := &http.Client{Jar: mjar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	mget := func(path string) (string, error) {
		resp, err := mclient.Get(srv.URL + path)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("GET %s as master admin -> %d", path, resp.StatusCode)
		}
		return string(b), nil
	}
	loginPage, err := mget("/login")
	if err != nil {
		return err
	}
	m := csrfRe.FindStringSubmatch(loginPage)
	if m == nil {
		return fmt.Errorf("no csrf token on /login")
	}
	resp, err = mclient.PostForm(srv.URL+"/login", url.Values{"csrf": {m[1]}, "email": {"master@example.org"}, "password": {masterPW}})
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		return fmt.Errorf("master admin login -> %d", resp.StatusCode)
	}
	for name, path := range map[string]string{"master-account": "/account", "admin-mail": "/admin/mail"} {
		page, err := mget(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(pagesDir, name+".html"), []byte(page), 0o600); err != nil {
			return err
		}
	}

	files, _ := filepath.Glob(filepath.Join(pagesDir, "*.html"))
	args := append([]string{"--yes", "html-validate@9.5.5", "-c", ".htmlvalidate.json"}, files...)
	cmd := exec.Command("npx", args...) // #nosec G204 -- fixed tool and flags; the file list is our own temp directory
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("html-validate reported problems: %w", err)
	}
	fmt.Printf("validated %d rendered pages\n", len(files))
	return nil
}
