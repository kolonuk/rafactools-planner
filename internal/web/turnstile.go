package web

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	turnstileOrigin  = "https://challenges.cloudflare.com"
	turnstileVerify  = turnstileOrigin + "/turnstile/v0/siteverify"
	turnstileFailMsg = "The human check failed or timed out. Reload the page and try again."
)

var turnstileClient = &http.Client{Timeout: 5 * time.Second}

// turnstileOK reports whether the request may proceed. It is true when
// Turnstile is not configured. When it is, a missing, invalid or unverifiable
// token is a failure: an unreachable Cloudflare does not let anyone through.
func (s *server) turnstileOK(r *http.Request) bool {
	if s.Cfg.TurnstileSiteKey == "" {
		return true
	}
	token := r.FormValue("cf-turnstile-response")
	if token == "" || len(token) > 2048 {
		return false
	}
	verify := s.Turnstile
	if verify == nil {
		verify = s.cloudflareVerify
	}
	ok, err := verify(r.Context(), token, clientIP(r))
	if err != nil {
		log.Printf("turnstile verify: %v", err)
		return false
	}
	return ok
}

func (s *server) cloudflareVerify(ctx context.Context, token, remoteIP string) (bool, error) {
	form := url.Values{"secret": {s.Cfg.TurnstileSecret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerify, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := turnstileClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return false, err
	}
	return out.Success, nil
}
