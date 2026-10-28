// Package config loads runtime configuration from the environment. There is
// no config file: dev/test/prod are the same binary with different env vars,
// set by docker-compose (dev) or whatever runs the image.
package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port         string
	DBPath       string
	BaseURL      string // public URL used in verification/invite emails, e.g. https://cadets.example.org
	BasePath     string // path component of BaseURL (e.g. "/rafactools"), derived — see Load
	SessionKey   string // HMAC secret for CSRF tokens; also seeds cookie signing
	CookieSecure bool
	ClaimDomain  string // email domain allowed to claim a squadron, e.g. "rafac.mod.gov.uk"

	// SkipEmailVerification marks every new claim as verified immediately,
	// skipping the confirmation-email step entirely — for dev only. Real
	// environments (test/prod) must never set this: it removes the one
	// check that a claim's email address is real and reachable.
	SkipEmailVerification bool

	// MasterAdminEmail / MasterAdminPassword configure the one site-wide
	// administrator account, created and kept in step at every startup. Both
	// must be set for it to exist. The password belongs in a secret, never in
	// a plain variable or the repository.
	MasterAdminEmail    string
	MasterAdminPassword string

	// IssueEmail is where "Report a problem" reports are emailed (for example
	// issues@your-domain). Empty switches the feature off entirely.
	IssueEmail string

	// MasterAdminReset2FA clears the master admin's two-step sign-in at the next
	// start, for an operator who has lost the authenticator app. Set it for one
	// start, then remove it.
	MasterAdminReset2FA bool
	// IssueMasking lets people black out parts of their screenshot before
	// sending it (default true). A site run for other people, whose operators
	// need to see exactly what the user saw, sets it false; the report form
	// then says plainly that the screenshot is sent as it is.
	IssueMasking bool

	// AllowLogMail lets the site start without SMTP, writing the emails it would
	// send (confirmation, invite and password-reset links, problem reports) to
	// the log instead. Those links are credentials: anyone who can read the log
	// can use them, so this is for local development only and is off by default.
	AllowLogMail bool

	// DisableSignups closes the claim-a-squadron form, for example on a copy
	// that holds real data. Existing accounts, invites and password resets
	// still work.
	DisableSignups bool

	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	MailFrom string
}

func Load() (Config, error) {
	c := Config{
		Port:         getenv("PORT", "8080"),
		DBPath:       getenv("DB_PATH", "/data/rafactools.db"),
		BaseURL:      getenv("BASE_URL", "http://localhost:8080"),
		SessionKey:   os.Getenv("SESSION_KEY"),
		CookieSecure: getenvBool("COOKIE_SECURE", true),
		ClaimDomain:  getenv("CLAIM_DOMAIN", "rafac.mod.gov.uk"),

		SkipEmailVerification: getenvBool("SKIP_EMAIL_VERIFICATION", false),
		MasterAdminEmail:      strings.TrimSpace(os.Getenv("MASTER_ADMIN_EMAIL")),
		MasterAdminPassword:   os.Getenv("MASTER_ADMIN_PASSWORD"),
		MasterAdminReset2FA:   getenvBool("MASTER_ADMIN_RESET_2FA", false),
		IssueEmail:            strings.TrimSpace(os.Getenv("ISSUE_EMAIL")),
		IssueMasking:          getenvBool("ISSUE_MASKING", true),
		AllowLogMail:          getenvBool("ALLOW_LOG_MAIL", false),
		DisableSignups:        getenvBool("DISABLE_SIGNUPS", false),
		SMTPHost:              os.Getenv("SMTP_HOST"),
		SMTPUser:              os.Getenv("SMTP_USER"),
		SMTPPass:              os.Getenv("SMTP_PASS"),
		MailFrom:              getenv("MAIL_FROM", "no-reply@rafactools-planner.local"),
	}

	if c.IssueEmail != "" {
		if a, err := mail.ParseAddress(c.IssueEmail); err != nil || a.Address != c.IssueEmail {
			return c, fmt.Errorf("ISSUE_EMAIL %q is not a valid email address", c.IssueEmail)
		}
	}

	if c.SMTPHost == "" && !c.AllowLogMail {
		return c, fmt.Errorf("SMTP_HOST is not set: without it, sign-in and reset links would be written to the log where anyone who can read it could use them; configure SMTP, or set ALLOW_LOG_MAIL=true for local development only")
	}

	if c.SessionKey == "" {
		return c, fmt.Errorf("SESSION_KEY must be set (random 32+ byte string used to sign CSRF tokens)")
	}

	if len(c.SessionKey) < 32 {
		return c, fmt.Errorf("SESSION_KEY must be at least 32 characters (try `openssl rand -hex 32`)")
	}

	// When this app is reverse-proxied under a path prefix (e.g. a
	// Traefik PathPrefix(`/rafactools`) + stripprefix — the app itself always
	// sees unprefixed paths, but any URL IT generates for the browser to
	// re-request (a link, a form action, a redirect) must include the prefix
	// or Traefik routes it to the wrong service. BASE_URL is already the one
	// place that prefix has to be configured correctly (for email links), so
	// BasePath is derived from it instead of needing its own env var.
	parsedBase, err := url.Parse(c.BaseURL)
	if err != nil {
		return c, fmt.Errorf("BASE_URL: %w", err)
	}
	c.BasePath = strings.TrimSuffix(parsedBase.Path, "/")

	port := getenv("SMTP_PORT", "587")
	p, err := strconv.Atoi(port)
	if err != nil {
		return c, fmt.Errorf("SMTP_PORT: %w", err)
	}
	c.SMTPPort = p

	return c, nil
}

// HasSMTP reports whether outgoing mail is configured. When false, the mail
// package logs verification links instead, which Load only allows when
// ALLOW_LOG_MAIL is set (local development).
func (c Config) HasSMTP() bool {
	return c.SMTPHost != ""
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
