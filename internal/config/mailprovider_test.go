package config

import (
	"strings"
	"testing"
)

func lookup(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestResolveMailPresets(t *testing.T) {
	cases := map[string]struct {
		env               map[string]string
		host, user, from  string
		port              int
		wantErrContaining string
	}{
		"plain smtp keeps what is given": {
			env:  map[string]string{"SMTP_HOST": "mail.example.org", "SMTP_USER": "u", "SMTP_PASS": "p", "MAIL_FROM": "a@example.org"},
			host: "mail.example.org", port: 587, user: "u", from: "a@example.org"},
		"cloudflare": {
			env:  map[string]string{"MAIL_PROVIDER": "cloudflare", "SMTP_PASS": "tok", "MAIL_FROM": "no-reply@example.org"},
			host: "smtp.mx.cloudflare.net", port: 465, user: "api_token", from: "no-reply@example.org"},
		"cloudflare needs a token": {
			env:               map[string]string{"MAIL_PROVIDER": "cloudflare", "MAIL_FROM": "no-reply@example.org"},
			wantErrContaining: "SMTP_PASS"},
		"cloudflare needs a sender": {
			env:               map[string]string{"MAIL_PROVIDER": "cloudflare", "SMTP_PASS": "tok"},
			wantErrContaining: "MAIL_FROM"},
		"gmail sends as the login": {
			env:  map[string]string{"MAIL_PROVIDER": "gmail", "SMTP_USER": "me@gmail.com", "SMTP_PASS": "app password"},
			host: "smtp.gmail.com", port: 465, user: "me@gmail.com", from: "me@gmail.com"},
		"gmail needs a login": {
			env:               map[string]string{"MAIL_PROVIDER": "gmail", "SMTP_PASS": "x"},
			wantErrContaining: "SMTP_USER"},
		"google workspace relay needs no login": {
			env:  map[string]string{"MAIL_PROVIDER": "google-workspace", "MAIL_FROM": "no-reply@example.org"},
			host: "smtp-relay.gmail.com", port: 587, from: "no-reply@example.org"},
		"ses": {
			env:  map[string]string{"MAIL_PROVIDER": "SES", "SES_REGION": "eu-west-2", "SMTP_USER": "AKIA", "SMTP_PASS": "s", "MAIL_FROM": "no-reply@example.org"},
			host: "email-smtp.eu-west-2.amazonaws.com", port: 465, user: "AKIA", from: "no-reply@example.org"},
		"ses needs a region": {
			env:               map[string]string{"MAIL_PROVIDER": "ses", "SMTP_USER": "a", "SMTP_PASS": "s", "MAIL_FROM": "a@example.org"},
			wantErrContaining: "SES_REGION"},
		"ses region must look like a region": {
			env:               map[string]string{"MAIL_PROVIDER": "ses", "SES_REGION": "evil.example.org/x", "SMTP_USER": "a", "SMTP_PASS": "s", "MAIL_FROM": "a@example.org"},
			wantErrContaining: "SES_REGION"},
		"explicit settings beat the preset": {
			env:  map[string]string{"MAIL_PROVIDER": "cloudflare", "SMTP_HOST": "relay.example.org", "SMTP_PORT": "2525", "SMTP_USER": "me", "SMTP_PASS": "p", "MAIL_FROM": "a@example.org"},
			host: "relay.example.org", port: 2525, user: "me", from: "a@example.org"},
		"unknown provider": {
			env:               map[string]string{"MAIL_PROVIDER": "carrier-pigeon"},
			wantErrContaining: "MAIL_PROVIDER"},
		"bad port": {
			env:               map[string]string{"SMTP_HOST": "m.example.org", "SMTP_PORT": "abc"},
			wantErrContaining: "SMTP_PORT"},
		"port out of range": {
			env:               map[string]string{"SMTP_HOST": "m.example.org", "SMTP_PORT": "70000"},
			wantErrContaining: "SMTP_PORT"},
		"host with a space": {
			env:               map[string]string{"SMTP_HOST": "mail example.org"},
			wantErrContaining: "SMTP_HOST"},
		"from that is not an address": {
			env:               map[string]string{"SMTP_HOST": "m.example.org", "MAIL_FROM": "Bob <b@example.org>"},
			wantErrContaining: "MAIL_FROM"},
		"line break in the password": {
			env:               map[string]string{"SMTP_HOST": "m.example.org", "SMTP_PASS": "a\r\nRCPT TO:<x@y>"},
			wantErrContaining: "line breaks"},
		"nothing set is allowed here": {
			env:  map[string]string{},
			port: 587, from: "no-reply@rafactools-planner.local"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m, err := ResolveMail(lookup(tc.env))
			if tc.wantErrContaining != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrContaining) {
					t.Fatalf("err = %v, want one containing %q", err, tc.wantErrContaining)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if m.Host != tc.host || m.Port != tc.port || m.User != tc.user || m.From != tc.from {
				t.Errorf("got host=%q port=%d user=%q from=%q; want %q %d %q %q", m.Host, m.Port, m.User, m.From, tc.host, tc.port, tc.user, tc.from)
			}
		})
	}
}

func TestRequireMail(t *testing.T) {
	if err := RequireMail(MailSettings{}, false); err == nil || !strings.Contains(err.Error(), "ALLOW_LOG_MAIL") {
		t.Errorf("no host, not allowed: %v", err)
	}
	if err := RequireMail(MailSettings{}, true); err != nil {
		t.Errorf("no host, allowed: %v", err)
	}
	if err := RequireMail(MailSettings{Host: "m.example.org"}, false); err != nil {
		t.Errorf("host set: %v", err)
	}
}

func TestLoadKeepsEnvironmentMailAsDefaultsAndTolerantOfPartialSettings(t *testing.T) {
	// A half-set provider is not fatal at Load: the master admin may complete
	// it in the database. It is recorded for the caller to judge.
	env(t, map[string]string{"SESSION_KEY": goodKey, "MAIL_PROVIDER": "ses", "SMTP_HOST": ""})
	c, err := Load()
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	if c.MailEnvError == nil {
		t.Error("the incomplete provider was not recorded")
	}
	env(t, map[string]string{"SESSION_KEY": goodKey, "MAIL_PROVIDER": "cloudflare", "SMTP_PASS": "tok", "MAIL_FROM": "a@example.org", "SMTP_HOST": ""})
	c, err = Load()
	if err != nil || c.SMTPHost != "smtp.mx.cloudflare.net" || c.SMTPPort != 465 || c.MailEnvError != nil {
		t.Errorf("preset not applied: %+v %v", c, err)
	}
}
