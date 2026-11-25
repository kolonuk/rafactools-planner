// Package holidays keeps a copy of the UK government's list of bank holidays
// (https://www.gov.uk/bank-holidays.json) in the database. It is fetched about
// once a month, every year the government lists is kept, and when the list
// changes every squadron's current and future months are brought in line.
package holidays

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"rafactools-planner/internal/programme"
)

// DefaultURL is the government's list.
const DefaultURL = "https://www.gov.uk/bank-holidays.json"

const (
	// RefreshEvery is how old the copy may get before it is fetched again.
	RefreshEvery = 28 * 24 * time.Hour
	// RetryAfter is how long to wait after a failed try.
	RetryAfter = 6 * time.Hour
	// CheckEvery is how often the loop looks at those two.
	CheckEvery = time.Hour
	maxBody    = 2 << 20
)

var client = &http.Client{Timeout: 20 * time.Second}

// Parse reads the government's JSON: a map of region to {events: [{title, date}]}.
func Parse(r io.Reader) ([]programme.Holiday, error) {
	var raw map[string]struct {
		Events []struct {
			Title string `json:"title"`
			Date  string `json:"date"`
		} `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(r, maxBody)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("read bank holidays: %w", err)
	}
	var out []programme.Holiday
	for region, v := range raw {
		if !programme.ValidRegion(region) || region == programme.RegionNone {
			continue
		}
		for _, e := range v.Events {
			title := strings.Join(strings.Fields(e.Title), " ")
			if title == "" || len(title) > 120 {
				continue
			}
			out = append(out, programme.Holiday{Region: region, Date: e.Date, Title: title})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("read bank holidays: the list has no events")
	}
	return out, nil
}

// Fetch downloads and reads the list.
func Fetch(ctx context.Context, url string) ([]programme.Holiday, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bank holidays: %s answered %s", url, resp.Status)
	}
	return Parse(resp.Body)
}

// Due reports whether the copy should be fetched now.
func Due(fetched, attempted, now time.Time) bool {
	if !attempted.IsZero() && now.Sub(attempted) < RetryAfter && now.Sub(fetched) >= RefreshEvery {
		return false // tried lately and failed
	}
	return fetched.IsZero() || now.Sub(fetched) >= RefreshEvery
}

// SyncOnce fetches the list if it is due, stores it, and if it changed brings
// every squadron's months in line. It reports whether it fetched.
func SyncOnce(ctx context.Context, store *programme.Store, url string, now time.Time) (bool, error) {
	fetched, attempted, err := store.HolidaySync()
	if err != nil {
		return false, err
	}
	if !Due(fetched, attempted, now) {
		return false, nil
	}
	hols, err := Fetch(ctx, url)
	if err != nil {
		_ = store.MarkHolidaySync(now, false)
		return true, err
	}
	changed, err := store.ReplaceBankHolidays(hols)
	if err != nil {
		_ = store.MarkHolidaySync(now, false)
		return true, err
	}
	if changed || fetched.IsZero() {
		if err := store.ApplyBankHolidays(); err != nil {
			return true, err
		}
	}
	if changed {
		log.Printf("bank holidays: the list changed; %d dates kept", len(hols))
	}
	return true, store.MarkHolidaySync(now, true)
}

// Run keeps the copy fresh until ctx ends. An empty url (or "off") does nothing.
func Run(ctx context.Context, store *programme.Store, url string) {
	if url == "" || strings.EqualFold(url, "off") {
		return
	}
	tick := func() {
		if _, err := SyncOnce(ctx, store, url, time.Now()); err != nil {
			log.Printf("bank holidays: %v", err)
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
