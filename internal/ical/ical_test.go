package ical

import (
	"strings"
	"testing"
	"time"
)

func TestBuildWritesTimedAndAllDayEvents(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/London")
	mod := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	out := string(Build("2491 Sqn", []Event{
		{UID: "a@x", Start: time.Date(2026, 11, 5, 19, 0, 0, 0, loc), End: time.Date(2026, 11, 5, 21, 30, 0, 0, loc),
			Summary: "Parade night, 2491; Sqn", Description: "Uniform: No 3\nFirst Aid", URL: "https://x.example/share/t/2026-11.pdf", Modified: mod},
		{UID: "b@x", AllDay: true, Start: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), Summary: "Air show"},
	}, mod))
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n", "VERSION:2.0\r\n", "X-WR-CALNAME:2491 Sqn\r\n",
		"DTSTART:20261105T190000Z\r\n", // November: London is on GMT, so UTC is the same
		"DTEND:20261105T213000Z\r\n",
		`SUMMARY:Parade night\, 2491\; Sqn` + "\r\n",
		`DESCRIPTION:Uniform: No 3\nFirst Aid` + "\r\n",
		"URL:https://x.example/share/t/2026-11.pdf\r\n",
		"DTSTART;VALUE=DATE:20261108\r\n", "DTEND;VALUE=DATE:20261109\r\n",
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, "BEGIN:VEVENT") != 2 || strings.Count(out, "END:VEVENT") != 2 {
		t.Errorf("want two events:\n%s", out)
	}
}

func TestSummerTimeIsConvertedToUTC(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/London")
	out := string(Build("x", []Event{{UID: "a", Start: time.Date(2026, 6, 4, 19, 0, 0, 0, loc), End: time.Date(2026, 6, 4, 21, 30, 0, 0, loc), Summary: "s"}}, time.Now()))
	if !strings.Contains(out, "DTSTART:20260604T180000Z") {
		t.Errorf("19:00 BST should be 18:00 UTC:\n%s", out)
	}
}

func TestLongLinesFoldWithoutSplittingCharacters(t *testing.T) {
	long := strings.Repeat("é", 100) // 200 octets
	out := string(Build("x", []Event{{UID: "a", Start: time.Now(), End: time.Now(), Summary: long}}, time.Now()))
	for _, l := range strings.Split(out, "\r\n") {
		if len(l) > 75 {
			t.Errorf("line of %d octets: %q", len(l), l)
		}
	}
	unfolded := strings.ReplaceAll(out, "\r\n ", "")
	if !strings.Contains(unfolded, "SUMMARY:"+long) {
		t.Error("folding changed the text")
	}
}
