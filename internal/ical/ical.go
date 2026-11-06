// Package ical writes a calendar feed (RFC 5545) that phones and calendar apps
// can subscribe to. It writes only what the planner needs: timed and all-day
// events with a title, description and link.
package ical

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// Event is one calendar entry. Times are absolute; they are written in UTC so
// no time-zone definitions are needed and every calendar shows local time.
type Event struct {
	UID         string
	Start, End  time.Time // for an all-day event, the first day and the day after the last
	AllDay      bool
	Summary     string
	Description string
	URL         string
	Modified    time.Time // when the content last changed; also the sequence number
}

// Build returns the whole calendar. name is shown by the calendar app.
func Build(name string, events []Event, now time.Time) []byte {
	var b bytes.Buffer
	line := func(s string) { writeFolded(&b, s) }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//RAFACTools Planner//Programme//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:" + escape(name))
	for _, e := range events {
		stamp := e.Modified
		if stamp.IsZero() {
			stamp = now
		}
		line("BEGIN:VEVENT")
		line("UID:" + e.UID)
		line("DTSTAMP:" + utc(stamp))
		line("LAST-MODIFIED:" + utc(stamp))
		line(fmt.Sprintf("SEQUENCE:%d", stamp.Unix()))
		if e.AllDay {
			line("DTSTART;VALUE=DATE:" + e.Start.Format("20060102"))
			line("DTEND;VALUE=DATE:" + e.End.Format("20060102"))
		} else {
			line("DTSTART:" + utc(e.Start))
			line("DTEND:" + utc(e.End))
		}
		line("SUMMARY:" + escape(e.Summary))
		if e.Description != "" {
			line("DESCRIPTION:" + escape(e.Description))
		}
		if e.URL != "" {
			line("URL:" + e.URL)
		}
		line("TRANSP:TRANSPARENT")
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.Bytes()
}

func utc(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// escape applies the text escaping of RFC 5545 section 3.3.11.
func escape(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)
	return r.Replace(s)
}

// writeFolded writes one content line, folded at 75 octets without splitting a
// UTF-8 character, ending in CRLF.
func writeFolded(b *bytes.Buffer, s string) {
	const limit = 75
	n := 0
	for _, r := range s {
		size := len(string(r))
		if n+size > limit {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += size
	}
	b.WriteString("\r\n")
}
