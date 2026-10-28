package web

import (
	"strings"
	"sync"
)

// LogTee keeps the last few server log lines that look like errors, with
// emails and tokens removed, so a problem report can include what the server
// complained about. Install it with log.SetOutput(io.MultiWriter(os.Stderr,
// web.LogTee())) at startup.
type logTee struct {
	mu    sync.Mutex
	lines []string
}

const keepErrorLines = 30

var defaultLogTee = &logTee{}

// LogTee returns the process-wide tee to add to the standard logger's output.
func LogTee() *logTee { return defaultLogTee }

func (l *logTee) Write(p []byte) (int, error) {
	for _, line := range strings.Split(string(p), "\n") {
		low := strings.ToLower(line)
		if strings.Contains(low, "error") || strings.Contains(low, "panic") || strings.Contains(low, "failed") {
			l.mu.Lock()
			l.lines = append(l.lines, redactLine(line))
			if len(l.lines) > keepErrorLines {
				l.lines = l.lines[len(l.lines)-keepErrorLines:]
			}
			l.mu.Unlock()
		}
	}
	return len(p), nil
}

// recent returns up to n of the newest error-like lines, oldest first.
func (l *logTee) recent(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.lines
	if len(e) > n {
		e = e[len(e)-n:]
	}
	return append([]string(nil), e...)
}
