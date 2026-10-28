package web

import (
	"strings"
	"testing"
)

func TestSanitizePath(t *testing.T) {
	tok := strings.Repeat("ab12", 16) // 64 hex characters, like the app's real tokens
	for in, want := range map[string]string{
		"/programmes/3":                  "/programmes/3",
		"/programmes/3/pdf?x=1#top":      "/programmes/3/pdf",
		"/reset/" + tok:                  "/reset/[token]",
		"/invite/" + tok + "?foo=bar":    "/invite/[token]",
		"/verify?token=" + tok:           "/verify",
		"https://example.org/a/b?c=d":    "/a/b",
		"//evil.example.org/x":           "/x",
		"/check-email?email=a%40b.org":   "/check-email",
		"/planner/reset/" + tok:          "/planner/reset/[token]",
		"":                               "/",
		"no-leading-slash":               "/no-leading-slash",
		"/with\x00control\x1b[31m/chars": "/withcontrol[31m/chars",
		"/docs/next/roles-and-invites":   "/docs/next/roles-and-invites", // long but readable words are kept
		"/a/" + strings.Repeat("x", 400): "/a/[token]",
	} {
		got := sanitizePath(in)
		if in == "/a/"+strings.Repeat("x", 400) {
			if got != "/a/[token]" {
				t.Errorf("long segment kept: %q", got)
			}
			continue
		}
		if got != want {
			t.Errorf("sanitizePath(%q) = %q, want %q", in, got, want)
		}
		if strings.Contains(got, tok) || strings.ContainsAny(got, "?#") {
			t.Errorf("sanitizePath(%q) leaked: %q", in, got)
		}
	}
}

func TestRedactLine(t *testing.T) {
	tok := strings.Repeat("0f", 32)
	for _, tc := range []struct{ in, notWant string }{
		{"mail to oc.1234@rafac.mod.gov.uk failed", "rafac.mod.gov.uk"},
		{"link https://x/reset/" + tok + " sent", tok},
		{"ERROR master admin NOT configured: master admin email admin@example.org", "admin@example.org"},
	} {
		got := redactLine(tc.in)
		if strings.Contains(got, tc.notWant) {
			t.Errorf("redactLine(%q) = %q still contains %q", tc.in, got, tc.notWant)
		}
	}
	if got := redactLine("save programme 3: database is locked"); got != "save programme 3: database is locked" {
		t.Errorf("ordinary text altered: %q", got)
	}
}

func TestCleanText(t *testing.T) {
	if got := cleanText("  a\x00b\r\nc\td  ", 100, false); got != "ab c d" {
		t.Errorf("single-line = %q", got)
	}
	if got := cleanText("a\r\nb\x07c", 100, true); got != "a\nbc" {
		t.Errorf("multiline = %q", got)
	}
	if got := cleanText("héllo wörld", 5, false); got != "héllo" {
		t.Errorf("limit counts runes, not bytes: %q", got)
	}
}
