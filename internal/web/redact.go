package web

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// Everything below exists so that a problem report can carry enough to
// diagnose a fault without carrying secrets or personal data: tokens in URLs
// (password-reset and invite links), query strings, email addresses and
// control characters are removed before anything is stored or emailed.

var (
	// A path segment that looks like a secret token: long and made only of
	// URL-safe characters (the app's tokens are 64 hex characters).
	tokenSegment = regexp.MustCompile(`^[A-Za-z0-9_-]{20,}$`)
	emailLike    = regexp.MustCompile(`[A-Za-z0-9._%+'-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	longToken    = regexp.MustCompile(`[A-Za-z0-9_-]{24,}`)
)

// cleanText trims, removes control characters (keeping newlines when
// multiline is set) and limits the length in runes.
func cleanText(s string, max int, multiline bool) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r == '\r' {
			continue
		}
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t')) {
			if r == '\n' || r == '\t' {
				r = ' '
			} else {
				continue
			}
		}
		if n >= max {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// sanitizePath turns a URL or path into a path that is safe to record: no
// scheme/host, no query string or fragment, no token-like segments.
func sanitizePath(raw string) string {
	raw = cleanText(raw, 300, false)
	if u, err := url.Parse(raw); err == nil {
		raw = u.Path
	} else if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	segs := strings.Split(raw, "/")
	for i, s := range segs {
		if tokenSegment.MatchString(s) {
			segs[i] = "[token]"
		}
	}
	out := strings.Join(segs, "/")
	if out == "" {
		return "/"
	}
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	return cleanText(out, 200, false)
}

// redactLine removes email addresses and token-like strings from free text
// such as a log line or a browser error message.
func redactLine(s string) string {
	s = emailLike.ReplaceAllString(s, "[email]")
	s = longToken.ReplaceAllString(s, "[token]")
	return cleanText(s, 300, false)
}
