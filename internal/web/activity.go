package web

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// activityLog keeps the last few requests per browser session, in memory
// only, so a problem report can say what the server saw just before
// ("POST /programmes/3 -> 500"). It records the method, a sanitised path and
// the status, never bodies, queries, cookies or form values, and it holds at
// most maxSessions sessions (the least recently active is dropped).
type activityLog struct {
	mu       sync.Mutex
	sessions map[string]*activityRing
}

const (
	activityPerSession = 20
	maxActivitySession = 500
)

type activityEntry struct {
	At     time.Time `json:"at"`
	Method string    `json:"method"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
	ID     string    `json:"request_id,omitempty"`
}

type activityRing struct {
	entries []activityEntry
	last    time.Time
}

func newActivityLog() *activityLog {
	return &activityLog{sessions: map[string]*activityRing{}}
}

func (a *activityLog) add(key string, e activityEntry) {
	if key == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	r, ok := a.sessions[key]
	if !ok {
		if len(a.sessions) >= maxActivitySession {
			var oldest string
			var oldestAt time.Time
			for k, v := range a.sessions {
				if oldest == "" || v.last.Before(oldestAt) {
					oldest, oldestAt = k, v.last
				}
			}
			delete(a.sessions, oldest)
		}
		r = &activityRing{}
		a.sessions[key] = r
	}
	r.entries = append(r.entries, e)
	if len(r.entries) > activityPerSession {
		r.entries = r.entries[len(r.entries)-activityPerSession:]
	}
	r.last = e.At
}

// recent returns up to n of the newest entries, oldest first.
func (a *activityLog) recent(key string, n int) []activityEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	r, ok := a.sessions[key]
	if !ok {
		return nil
	}
	e := r.entries
	if len(e) > n {
		e = e[len(e)-n:]
	}
	return append([]activityEntry(nil), e...)
}

// statusRecorder remembers the status code a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// activityKey identifies a browser session for the trail without ever
// putting the cookie value anywhere it could be read back out.
func activityKey(r *http.Request) string {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return "s:" + c.Value
	}
	if c, err := r.Cookie(anonCSRFCookieName); err == nil && c.Value != "" {
		return "a:" + c.Value
	}
	return ""
}

// recordActivity wraps the router and notes each request once it is done.
func (s *server) recordActivity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/static/") || path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.activity.add(activityKey(r), activityEntry{
			At: time.Now().UTC(), Method: r.Method, Path: sanitizePath(path), Status: rec.status, ID: requestIDFrom(r.Context()),
		})
	})
}
