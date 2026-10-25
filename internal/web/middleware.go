package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

const sessionCookieName = "session"
const anonCSRFCookieName = "csrf_anon"

func (s *server) sessionToken(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// setSessionCookie sets (or, with a zero expiresAt, clears) the session
// cookie. maxAge is derived from expiresAt here rather than taken as a
// parameter so callers can't accidentally pass a Unix timestamp where
// net/http wants a duration in seconds.
func (s *server) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	maxAge := 0
	if token == "" {
		maxAge = -1
	} else {
		maxAge = int(time.Until(expiresAt).Seconds())
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.Auth.CookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// withUserOptional attaches the current user (if any) to the request context
// without requiring one — used by public pages like /login and /claim so the
// layout can still show "logged in as ..." when relevant.
//
// There's no session yet on these pages, so the usual session-derived CSRF
// token (see auth.Service.CSRFToken) has nothing to key off. Instead we
// issue a one-off random "csrf_anon" cookie on first visit and use it as a
// plain double-submit token: the hidden form field must match the cookie.
func (s *server) withUserOptional(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := s.sessionToken(r)
		user, err := s.Auth.UserBySession(token)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, "Error", "Something went wrong.")
			return
		}
		ctx := context.WithValue(r.Context(), ctxSessionToken, token)
		if user != nil {
			ctx = context.WithValue(ctx, ctxUser, user)
		} else {
			ctx = context.WithValue(ctx, ctxAnonCSRF, s.ensureAnonCSRFCookie(w, r))
		}
		next(w, r.WithContext(ctx))
	}
}

func (s *server) ensureAnonCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(anonCSRFCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	value := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name: anonCSRFCookieName, Value: value, Path: "/",
		HttpOnly: true, Secure: s.Auth.CookieSecure(), SameSite: http.SameSiteLaxMode, MaxAge: 3600,
	})
	return value
}

func (s *server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return s.withUserOptional(func(w http.ResponseWriter, r *http.Request) {
		if userFrom(r.Context()) == nil {
			http.Redirect(w, r, s.url("/login"), http.StatusSeeOther)
			return
		}
		next(w, r)
	})
}

func (s *server) requireEditor(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		if !u.CanEdit() {
			s.renderError(w, r, http.StatusForbidden, "Forbidden", "Viewers can't make changes.")
			return
		}
		next(w, r)
	})
}

func (s *server) requireOwner(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		if !u.IsOwner() {
			s.renderError(w, r, http.StatusForbidden, "Forbidden", "Only the squadron owner can do that.")
			return
		}
		next(w, r)
	})
}
