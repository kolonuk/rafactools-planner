package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ipLimiter is a small fixed-window rate limiter keyed by client IP, used on
// the auth endpoints (login/claim/invite-accept) to slow down credential
// stuffing and brute-force guessing. It's in-memory and per-process — fine
// at this app's scale (a handful of squadrons), and resets on restart, which
// is an acceptable trade for not adding an external dependency (Redis etc.)
// for a handful of requests per minute.
type ipLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	counters map[string]*windowCount
}

type windowCount struct {
	count      int
	windowFrom time.Time
}

func newIPLimiter(limit int, window time.Duration) *ipLimiter {
	l := &ipLimiter{limit: limit, window: window, counters: make(map[string]*windowCount)}
	go l.sweepLoop()
	return l
}

// Allow reports whether ip is still under limit within the current window,
// incrementing its count as a side effect.
func (l *ipLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	c, ok := l.counters[ip]
	if !ok || now.Sub(c.windowFrom) > l.window {
		l.counters[ip] = &windowCount{count: 1, windowFrom: now}
		return true
	}
	if c.count >= l.limit {
		return false
	}
	c.count++
	return true
}

// sweepLoop evicts stale entries periodically so a long-running process
// doesn't accumulate one entry per distinct IP forever.
func (l *ipLimiter) sweepLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-10 * l.window)
		l.mu.Lock()
		for ip, c := range l.counters {
			if c.windowFrom.Before(cutoff) {
				delete(l.counters, ip)
			}
		}
		l.mu.Unlock()
	}
}

// rateLimited wraps a handler so repeated requests from one IP within the
// window past the limit get a 429 instead of reaching auth logic. Applied to
// POST /login, /claim, and /invite/{token} — the endpoints an attacker would
// actually script against.
func (l *ipLimiter) rateLimited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(clientIP(r)) {
			http.Error(w, "Too many attempts — try again in a few minutes.", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// clientIP prefers X-Forwarded-For/X-Real-IP, set by the Traefik reverse
// proxy every deployment of this app runs behind (see dadock's
// docker-compose.yml) — without it every request would appear to come from
// Traefik's own container IP, making per-IP limiting useless. Falls back to
// RemoteAddr for local dev, where there's no proxy in front.
//
// The LAST X-Forwarded-For entry is used: the proxy appends the address it
// actually saw, whereas earlier entries are whatever the client sent and are
// trivially spoofed to dodge the rate limit.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return real
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
