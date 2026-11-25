// Package reminders emails a squadron's owners and editors about parade nights
// that still have empty Activity or Instructor boxes. An owner turns it on under
// Squadron by choosing how many days ahead; each night is reminded about once.
package reminders

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"rafactools-planner/internal/programme"
)

// Deps is what the job needs.
type Deps struct {
	Store *programme.Store
	// Recipients returns the addresses to email for a squadron: its active owners
	// and editors with a confirmed address.
	Recipients func(squadronID int64) ([]string, error)
	Send       func(to, subject, body string) error
	BaseURL    string // the site's address, for the links
	Now        func() time.Time
}

// FirstHour is the earliest local hour a reminder is sent, so they arrive in the day.
const FirstHour = 8

var weekdayFormat = "Monday 2 January"

// RunOnce sends what is due. It does nothing before FirstHour, UK time. It returns
// the number of emails sent.
func RunOnce(d Deps) (int, error) {
	now := d.Now().In(programme.Local)
	if now.Hour() < FirstHour {
		return 0, nil
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, programme.Local)
	due, err := d.Store.DueReminders(today)
	if err != nil {
		return 0, err
	}
	_ = d.Store.ForgetOldReminders(today)
	sent := 0
	for _, q := range due {
		to, err := d.Recipients(q.SquadronID)
		if err != nil {
			log.Printf("reminders: recipients for squadron %d: %v", q.SquadronID, err)
			continue
		}
		if len(to) == 0 {
			continue
		}
		subject, body := Message(q, d.BaseURL)
		ok := 0
		for _, addr := range to {
			if err := d.Send(addr, subject, body); err != nil {
				log.Printf("reminders: send to %s: %v", addr, err)
				continue
			}
			ok++
		}
		if ok == 0 {
			continue // try again next time
		}
		sent += ok
		var dates []string
		for _, n := range q.Nights {
			dates = append(dates, n.Date)
		}
		if err := d.Store.MarkReminded(q.SquadronID, dates); err != nil {
			log.Printf("reminders: %v", err)
		}
	}
	return sent, nil
}

// Message writes the email for one squadron.
func Message(q programme.ReminderDue, baseURL string) (subject, body string) {
	n := len(q.Nights)
	plural := "s"
	if n == 1 {
		plural = ""
	}
	subject = fmt.Sprintf("%s: %d parade night%s with blank boxes", q.SquadronName, n, plural)
	var b strings.Builder
	fmt.Fprintf(&b, "These parade nights of %s are coming up and still have empty boxes in the programme:\n\n", q.SquadronName)
	for _, night := range q.Nights {
		day, _ := time.ParseInLocation("2006-01-02", night.Date, programme.Local)
		var parts []string
		if night.BlankActivity > 0 {
			parts = append(parts, fmt.Sprintf("%d empty Activity box%s", night.BlankActivity, es(night.BlankActivity)))
		}
		if night.BlankIns > 0 {
			parts = append(parts, fmt.Sprintf("%d empty Instructor box%s", night.BlankIns, es(night.BlankIns)))
		}
		fmt.Fprintf(&b, "- %s: %s\n", day.Format(weekdayFormat), strings.Join(parts, ", "))
	}
	fmt.Fprintf(&b, "\nOpen the planner to fill them in:\n%s/\n\n", strings.TrimRight(baseURL, "/"))
	b.WriteString("Each night is reminded about once. An owner can change or turn off these reminders under Squadron.\n")
	return subject, b.String()
}

func es(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

// Run checks every CheckEvery until ctx ends.
func Run(ctx context.Context, d Deps) {
	tick := func() {
		if n, err := RunOnce(d); err != nil {
			log.Printf("reminders: %v", err)
		} else if n > 0 {
			log.Printf("reminders: sent %d emails", n)
		}
	}
	tick()
	t := time.NewTicker(CheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}

// CheckEvery is how often the job looks for anything due.
const CheckEvery = 30 * time.Minute
