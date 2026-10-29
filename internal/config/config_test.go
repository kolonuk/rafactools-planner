package config

import (
	"strings"
	"testing"
)

const goodKey = "0123456789abcdef0123456789abcdef"

// env clears every variable Load reads, then applies overrides, so tests are
// independent of the machine they run on.
func env(t *testing.T, set map[string]string) {
	t.Helper()
	for _, k := range []string{"PORT", "DB_PATH", "BASE_URL", "SESSION_KEY", "COOKIE_SECURE", "CLAIM_DOMAIN",
		"SITE_DOMAIN", "ISSUE_EMAIL", "SKIP_EMAIL_VERIFICATION", "MAIL_PROVIDER", "SES_REGION", "ALLOW_LOG_MAIL", "DISABLE_SIGNUPS", "MASTER_ADMIN_RESET_2FA", "SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASS", "MAIL_FROM"} {
		t.Setenv(k, "")
	}
	// Mail is configured unless a test says otherwise: Load refuses to run
	// without it (see TestLoadRefusesToLogMailUnlessAllowed).
	t.Setenv("SMTP_HOST", "smtp.test.invalid")
	for k, v := range set {
		t.Setenv(k, v)
	}
}

func TestLoadDefaultsAreSecure(t *testing.T) {
	env(t, map[string]string{"SESSION_KEY": goodKey, "SMTP_HOST": "", "ALLOW_LOG_MAIL": "true"})
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.DisableSignups {
		t.Error("DISABLE_SIGNUPS must default to false")
	}
	if !c.CookieSecure {
		t.Error("COOKIE_SECURE must default to true")
	}
	if c.SkipEmailVerification {
		t.Error("SKIP_EMAIL_VERIFICATION must default to false")
	}
	if c.Port != "8080" || c.DBPath != "/data/rafactools.db" || c.ClaimDomain != "rafac.mod.gov.uk" || c.SMTPPort != 587 {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.HasSMTP() {
		t.Error("HasSMTP true with no host")
	}
}

func TestLoadSessionKeyValidation(t *testing.T) {
	for name, key := range map[string]string{"missing": "", "short": "too-short", "31 chars": strings.Repeat("k", 31)} {
		env(t, map[string]string{"SESSION_KEY": key})
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_KEY") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	env(t, map[string]string{"SESSION_KEY": strings.Repeat("k", 32)})
	if _, err := Load(); err != nil {
		t.Errorf("32-char key rejected: %v", err)
	}
}

func TestLoadBasePathDerivedFromBaseURL(t *testing.T) {
	for url, want := range map[string]string{
		"https://example.org":             "",
		"https://example.org/":            "",
		"https://example.org/rafactools":  "/rafactools",
		"https://example.org/rafactools/": "/rafactools",
		"https://example.org/a/b/":        "/a/b",
		"http://localhost:8080":           "",
	} {
		env(t, map[string]string{"SESSION_KEY": goodKey, "BASE_URL": url})
		c, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if c.BasePath != want {
			t.Errorf("BASE_URL %q -> BasePath %q, want %q", url, c.BasePath, want)
		}
	}
	env(t, map[string]string{"SESSION_KEY": goodKey, "BASE_URL": "http://[::1"})
	if _, err := Load(); err == nil {
		t.Error("malformed BASE_URL accepted")
	}
}

func TestLoadParsing(t *testing.T) {
	env(t, map[string]string{
		"SESSION_KEY": goodKey, "COOKIE_SECURE": "false", "SKIP_EMAIL_VERIFICATION": "1",
		"SMTP_HOST": "mail.example.org", "SMTP_PORT": "465", "PORT": "9000",
	})
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.CookieSecure || !c.SkipEmailVerification || !c.HasSMTP() || c.SMTPPort != 465 || c.Port != "9000" {
		t.Errorf("parsed wrong: %+v", c)
	}

	env(t, map[string]string{"SESSION_KEY": goodKey, "SMTP_PORT": "not-a-number"})
	if _, err := Load(); err == nil {
		t.Error("bad SMTP_PORT accepted")
	}

	// An unparseable boolean falls back to the (secure) default rather than
	// silently turning a protection off.
	env(t, map[string]string{"SESSION_KEY": goodKey, "COOKIE_SECURE": "flase"})
	c, _ = Load()
	if !c.CookieSecure {
		t.Error("typo in COOKIE_SECURE disabled secure cookies")
	}
	env(t, map[string]string{"SESSION_KEY": goodKey, "SKIP_EMAIL_VERIFICATION": "yess"})
	c, _ = Load()
	if c.SkipEmailVerification {
		t.Error("typo in SKIP_EMAIL_VERIFICATION enabled the bypass")
	}
}

func TestLoadDisableSignups(t *testing.T) {
	env(t, map[string]string{"SESSION_KEY": goodKey, "DISABLE_SIGNUPS": "true"})
	c, err := Load()
	if err != nil || !c.DisableSignups {
		t.Fatalf("DisableSignups=%v err=%v", c.DisableSignups, err)
	}
}

func TestLoadMasterAdminReset2FA(t *testing.T) {
	env(t, map[string]string{"SESSION_KEY": goodKey})
	if c, err := Load(); err != nil || c.MasterAdminReset2FA {
		t.Fatalf("default: reset=%v err=%v", c.MasterAdminReset2FA, err)
	}
	env(t, map[string]string{"SESSION_KEY": goodKey, "MASTER_ADMIN_RESET_2FA": "true"})
	if c, err := Load(); err != nil || !c.MasterAdminReset2FA {
		t.Fatalf("set: reset=%v err=%v", c.MasterAdminReset2FA, err)
	}
}

func TestSiteDomainDefaultsTheMailboxes(t *testing.T) {
	key := strings.Repeat("k", 32)
	for _, tc := range []struct {
		name                 string
		set                  map[string]string
		domain, from, issues string
	}{
		{"host of BASE_URL", map[string]string{"BASE_URL": "https://planner.example.org/app"}, "planner.example.org", "no-reply@planner.example.org", "issues@planner.example.org"},
		{"SITE_DOMAIN wins", map[string]string{"BASE_URL": "https://planner.example.org", "SITE_DOMAIN": "Example.ORG"}, "example.org", "no-reply@example.org", "issues@example.org"},
		{"explicit settings win", map[string]string{"BASE_URL": "https://planner.example.org", "MAIL_FROM": "hello@x.org", "ISSUE_EMAIL": "help@x.org"}, "planner.example.org", "hello@x.org", "help@x.org"},
		{"none switches reports off", map[string]string{"BASE_URL": "https://planner.example.org", "ISSUE_EMAIL": "none"}, "planner.example.org", "no-reply@planner.example.org", ""},
		{"localhost has no domain", map[string]string{"BASE_URL": "http://localhost:8080"}, "", "no-reply@rafactools-planner.local", ""},
		{"an IP has no domain", map[string]string{"BASE_URL": "http://10.0.0.5:8080"}, "", "no-reply@rafactools-planner.local", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := map[string]string{"SESSION_KEY": key}
			for k, v := range tc.set {
				set[k] = v
			}
			env(t, set)
			c, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if c.SiteDomain != tc.domain || c.MailFrom != tc.from || c.IssueEmail != tc.issues {
				t.Errorf("got domain %q from %q issues %q; want %q %q %q", c.SiteDomain, c.MailFrom, c.IssueEmail, tc.domain, tc.from, tc.issues)
			}
		})
	}
}
