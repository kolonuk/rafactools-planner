package web

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestActivityLogKeepsTheNewestPerSessionAndBoundsMemory(t *testing.T) {
	a := newActivityLog()
	for i := 0; i < 50; i++ {
		a.add("s:one", activityEntry{At: time.Now(), Method: "GET", Path: fmt.Sprintf("/p/%d", i), Status: 200})
	}
	got := a.recent("s:one", 100)
	if len(got) != activityPerSession || got[len(got)-1].Path != "/p/49" || got[0].Path != "/p/30" {
		t.Errorf("kept %d entries, first %q last %q", len(got), got[0].Path, got[len(got)-1].Path)
	}
	if r := a.recent("s:one", 5); len(r) != 5 || r[4].Path != "/p/49" {
		t.Errorf("recent(5) = %+v", r)
	}
	if a.recent("s:missing", 5) != nil {
		t.Error("an unknown session returned entries")
	}
	a.add("", activityEntry{Path: "/x"}) // no session key: nothing is kept
	if len(a.sessions) != 1 {
		t.Error("an entry without a key was stored")
	}
	for i := 0; i < maxActivitySession+50; i++ {
		a.add(fmt.Sprintf("s:%d", i), activityEntry{At: time.Now().Add(time.Duration(i) * time.Millisecond)})
	}
	if len(a.sessions) > maxActivitySession {
		t.Errorf("%d sessions held, cap is %d", len(a.sessions), maxActivitySession)
	}
}

func TestLogTeeKeepsOnlyRedactedErrors(t *testing.T) {
	l := &logTee{}
	tok := strings.Repeat("ab", 32)
	l.Write([]byte("listening on :8080\nERROR master admin NOT configured: admin@example.org\nsave programme 3 failed: database is locked\nreset link /reset/" + tok + " error\n"))
	got := strings.Join(l.recent(10), "\n")
	if strings.Contains(got, "listening") {
		t.Error("an ordinary line was kept")
	}
	for _, bad := range []string{"admin@example.org", tok} {
		if strings.Contains(got, bad) {
			t.Errorf("kept %q: %s", bad, got)
		}
	}
	if !strings.Contains(got, "database is locked") {
		t.Errorf("the useful line is missing: %s", got)
	}
	for i := 0; i < 100; i++ {
		l.Write([]byte(fmt.Sprintf("error %d\n", i)))
	}
	if len(l.recent(1000)) != keepErrorLines {
		t.Errorf("kept %d lines, cap is %d", len(l.recent(1000)), keepErrorLines)
	}
}
