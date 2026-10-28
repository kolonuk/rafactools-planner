package config

import (
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
)

// MAIL_PROVIDER picks a preset so an operator only supplies what is theirs
// (credentials, sender address, region) and not the provider's host and port.
// Every preset is plain SMTP, which is all the mailer speaks; anything set
// explicitly (SMTP_HOST, SMTP_PORT, SMTP_USER) overrides the preset. The
// default, "smtp", applies nothing and leaves every SMTP_* setting as given.
const (
	ProviderSMTP            = "smtp"
	ProviderCloudflare      = "cloudflare"
	ProviderGmail           = "gmail"
	ProviderGoogleWorkspace = "google-workspace"
	ProviderSES             = "ses"
)

// MailProviders lists the accepted MAIL_PROVIDER values, for messages and docs.
var MailProviders = []string{ProviderSMTP, ProviderCloudflare, ProviderGmail, ProviderGoogleWorkspace, ProviderSES}

// MailKeys are the settings that make up the mail configuration. Each can come
// from the environment (the default) or be set in the database from the master
// admin's page; a value in the database wins, an empty one falls back to the
// environment. SMTP_PASS is the only secret.
var MailKeys = []string{"MAIL_PROVIDER", "SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASS", "MAIL_FROM", "SES_REGION"}

// MailSettings is the resolved mail configuration.
type MailSettings struct {
	Provider  string
	Host      string
	Port      int
	User      string
	Pass      string
	From      string
	SESRegion string
}

var (
	sesRegionRe = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]$`)
	hostRe      = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
)

// ResolveMail turns raw settings (looked up by their environment names through
// get) into a validated configuration, applying the provider preset and
// defaults. It is a pure function of get, so the same code resolves the
// environment alone at startup and environment plus database at send time.
func ResolveMail(get func(string) string) (MailSettings, error) {
	val := func(k string) string { return strings.TrimSpace(get(k)) }

	m := MailSettings{
		Provider:  strings.ToLower(val("MAIL_PROVIDER")),
		Host:      val("SMTP_HOST"),
		User:      val("SMTP_USER"),
		Pass:      get("SMTP_PASS"),
		From:      val("MAIL_FROM"),
		SESRegion: strings.ToLower(val("SES_REGION")),
	}
	if m.Provider == "" {
		m.Provider = ProviderSMTP
	}
	explicitFrom := m.From != ""
	if m.From == "" {
		m.From = "no-reply@rafactools-planner.local"
	}

	portText := val("SMTP_PORT")
	if portText != "" {
		p, err := strconv.Atoi(portText)
		if err != nil || p < 1 || p > 65535 {
			return m, fmt.Errorf("SMTP_PORT %q is not a port number", portText)
		}
		m.Port = p
	}

	var host, user string
	var port int
	var needUser, needPass, needFrom bool
	switch m.Provider {
	case ProviderSMTP:
	case ProviderCloudflare:
		// Cloudflare Email Service: SMTP on 465 only, user is the literal
		// "api_token", password is an API token with Email Sending: Edit, and
		// the sender must be on an onboarded domain.
		host, port, user = "smtp.mx.cloudflare.net", 465, "api_token"
		needPass, needFrom = true, true
	case ProviderGmail:
		// A personal Google account (or a Workspace user) signing in with an
		// app password. Google only sends as that account or its aliases.
		host, port = "smtp.gmail.com", 465
		needUser, needPass = true, true
	case ProviderGoogleWorkspace:
		// Google Workspace SMTP relay: set up in the Admin console, usually
		// allowing the server's fixed IP address, so no login is needed.
		// SMTP_USER and SMTP_PASS are used if the relay is set to require them.
		host, port = "smtp-relay.gmail.com", 587
		needFrom = true
	case ProviderSES:
		// Amazon SES: SMTP credentials (not the IAM access keys themselves),
		// a verified sender and the region they were made in.
		if !sesRegionRe.MatchString(m.SESRegion) {
			return m, fmt.Errorf("MAIL_PROVIDER=ses needs SES_REGION set to the AWS region of your SES account, for example eu-west-2")
		}
		host, port = "email-smtp."+m.SESRegion+".amazonaws.com", 465
		needUser, needPass, needFrom = true, true, true
	default:
		return m, fmt.Errorf("MAIL_PROVIDER %q is not one of: %s", m.Provider, strings.Join(MailProviders, ", "))
	}
	if m.Host == "" {
		m.Host = host
	}
	if m.Port == 0 {
		m.Port = port
	}
	if m.Port == 0 {
		m.Port = 587
	}
	if m.User == "" {
		m.User = user
	}

	if m.Host != "" && !hostRe.MatchString(m.Host) {
		return m, fmt.Errorf("SMTP_HOST %q is not a host name", m.Host)
	}
	if needUser && m.User == "" {
		return m, fmt.Errorf("MAIL_PROVIDER=%s needs SMTP_USER", m.Provider)
	}
	if needPass && m.Pass == "" {
		return m, fmt.Errorf("MAIL_PROVIDER=%s needs SMTP_PASS (a secret)", m.Provider)
	}
	if !explicitFrom {
		switch {
		case m.Provider == ProviderGmail:
			m.From = m.User // Gmail only sends as the signed-in account
		case needFrom:
			return m, fmt.Errorf("MAIL_PROVIDER=%s needs MAIL_FROM set to a verified sender address on your domain", m.Provider)
		}
	}
	if a, err := mail.ParseAddress(m.From); err != nil || a.Address != m.From {
		return m, fmt.Errorf("MAIL_FROM %q is not a plain email address", m.From)
	}
	if strings.ContainsAny(m.User+m.Pass, "\r\n") {
		return m, fmt.Errorf("SMTP_USER and SMTP_PASS must not contain line breaks")
	}
	return m, nil
}

// RequireMail refuses a configuration with no mail server unless logging mail
// has been asked for by name: without a server the site would write
// confirmation, invite and reset links to its log, and anyone who can read the
// log could use them.
func RequireMail(m MailSettings, allowLogMail bool) error {
	if m.Host == "" && !allowLogMail {
		return fmt.Errorf("SMTP_HOST is not set: without it, sign-in and reset links would be written to the log where anyone who can read it could use them; set MAIL_PROVIDER or SMTP_HOST (or save mail settings as the master admin), or ALLOW_LOG_MAIL=true for local development only")
	}
	return nil
}
