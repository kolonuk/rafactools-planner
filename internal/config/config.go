// Package config loads runtime configuration from the environment. There is
// no config file: dev/test/prod are the same binary with different env vars,
// set by docker-compose (dev) or the dadock stack (test/prod).
package config

import (
	"fmt"
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
		SMTPHost:              os.Getenv("SMTP_HOST"),
		SMTPUser:              os.Getenv("SMTP_USER"),
		SMTPPass:              os.Getenv("SMTP_PASS"),
		MailFrom:              getenv("MAIL_FROM", "no-reply@rafactools-planner.local"),
	}

	if c.SessionKey == "" {
		return c, fmt.Errorf("SESSION_KEY must be set (random 32+ byte string used to sign CSRF tokens)")
	}

	// When this app is reverse-proxied under a path prefix (e.g. dadock's
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
// package logs verification links to stdout instead — fine for local dev,
// never acceptable in test/prod (see TODO.md).
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
