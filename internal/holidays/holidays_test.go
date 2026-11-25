package holidays

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"rafactools-planner/internal/db"
	"rafactools-planner/internal/programme"
)

const sample = `{
  "england-and-wales": {"division": "england-and-wales", "events": [
    {"title": "Christmas Day", "date": "2030-12-25", "notes": "", "bunting": true},
    {"title": "Boxing Day", "date": "2030-12-26", "notes": "", "bunting": true}]},
  "scotland": {"division": "scotland", "events": [{"title": "2nd January", "date": "2031-01-02"}]},
  "northern-ireland": {"division": "northern-ireland", "events": []},
  "somewhere-else": {"events": [{"title": "Ignored", "date": "2030-01-01"}]}
}`

func TestParse(t *testing.T) {
	got, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d holidays, want 3: %+v", len(got), got)
	}
	for _, bad := range []string{"", "not json", "{}", `{"scotland":{"events":[]}}`} {
		if _, err := Parse(strings.NewReader(bad)); err == nil {
			t.Errorf("Parse(%q) did not fail", bad)
		}
	}
}

func TestDue(t *testing.T) {
	now := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name               string
		fetched, attempted time.Time
		want               bool
	}{
		{"never fetched", time.Time{}, time.Time{}, true},
		{"fetched yesterday", now.Add(-24 * time.Hour), now.Add(-24 * time.Hour), false},
		{"fetched a month ago", now.Add(-30 * 24 * time.Hour), now.Add(-30 * 24 * time.Hour), true},
		{"a month old, tried an hour ago and failed", now.Add(-30 * 24 * time.Hour), now.Add(-time.Hour), false},
		{"a month old, tried a day ago and failed", now.Add(-30 * 24 * time.Hour), now.Add(-24 * time.Hour), true},
	} {
		if got := Due(tc.fetched, tc.attempted, now); got != tc.want {
			t.Errorf("%s: Due = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The government's own list, fetched for real: this is what the site will read.
func liveList(t *testing.T) []programme.Holiday {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := Fetch(ctx, DefaultURL)
	if err != nil {
		t.Fatalf("fetching %s: %v", DefaultURL, err)
	}
	return list
}

func TestTheGovernmentListReadsAsExpected(t *testing.T) {
	list := liveList(t)
	regions := map[string]int{}
	var christmas bool
	for _, h := range list {
		regions[h.Region]++
		christmas = christmas || (h.Region == programme.RegionEnglandWales && strings.HasSuffix(h.Date, "-12-25") && h.Title == "Christmas Day")
	}
	for _, r := range []string{programme.RegionEnglandWales, programme.RegionScotland, programme.RegionNorthernIE} {
		if regions[r] < 10 {
			t.Errorf("only %d holidays for %s", regions[r], r)
		}
	}
	if !christmas {
		t.Error("England and Wales has no Christmas Day")
	}
}

func TestSyncOnceFetchesAMonthAtATimeAndKeepsMonthsInLine(t *testing.T) {
	// The first England and Wales holiday after the clock below, from the real list.
	var target programme.Holiday
	for _, h := range liveList(t) {
		if h.Region == programme.RegionEnglandWales && h.Date > "2027-01-01" && (target.Date == "" || h.Date < target.Date) {
			target = h
		}
	}
	if target.Date == "" {
		t.Fatal("the list has no England and Wales holiday after 2027-01-01")
	}
	year, month := int(mustAtoi(t, target.Date[:4])), int(mustAtoi(t, target.Date[5:7]))

	d, err := db.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(`INSERT INTO squadrons (number, name) VALUES ('1', '1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO users (squadron_id, email, password_hash, role) VALUES (1, 'a@x', 'x', 'owner')`); err != nil {
		t.Fatal(err)
	}
	st := programme.NewStore(d)
	old := programme.Now
	start := time.Date(year, time.Month(month), 1, 9, 0, 0, 0, time.UTC).AddDate(0, 0, -40)
	programme.Now = func() time.Time { return start }
	defer func() { programme.Now = old }()
	p, err := st.CreateProgramme(1, year, month, 1)
	if err != nil {
		t.Fatal(err)
	}
	closures := func() []programme.Closure {
		fp, _ := st.GetFullProgramme(1, p.ID)
		return fp.Closures
	}
	has := func() bool {
		for _, c := range closures() {
			if c.StartDate == target.Date && c.Message == target.Title && c.Source == "bank" {
				return true
			}
		}
		return false
	}
	if len(closures()) != 0 {
		t.Fatal("closures before any list was fetched")
	}

	ctx := context.Background()
	if fetched, err := SyncOnce(ctx, st, DefaultURL, start); err != nil || !fetched {
		t.Fatalf("first sync: fetched=%v err=%v", fetched, err)
	}
	if !has() {
		t.Fatalf("the existing month has no closure for %s %s: %+v", target.Date, target.Title, closures())
	}
	if fetched, _ := SyncOnce(ctx, st, DefaultURL, start.Add(7*24*time.Hour)); fetched {
		t.Error("fetched again within a month")
	}

	// The kept list drifts from the government's (a date moved and a day added): the next
	// monthly fetch puts it right, and the month follows.
	moved := []programme.Holiday{{Region: programme.RegionEnglandWales, Date: target.Date[:8] + "28", Title: "Moved day"}}
	if changed, err := st.ReplaceBankHolidays(moved); err != nil || !changed {
		t.Fatalf("replace: %v %v", changed, err)
	}
	if err := st.ApplyBankHolidays(); err != nil {
		t.Fatal(err)
	}
	if has() {
		t.Fatal("the month did not follow the kept list")
	}
	if fetched, err := SyncOnce(ctx, st, DefaultURL, start.Add(30*24*time.Hour)); err != nil || !fetched {
		t.Fatalf("monthly sync: fetched=%v err=%v", fetched, err)
	}
	if !has() {
		t.Errorf("the monthly fetch did not restore the government's date: %+v", closures())
	}

	// An address that cannot be reached is an error, changes nothing and is not retried at once.
	later := start.Add(70 * 24 * time.Hour)
	if _, err := SyncOnce(ctx, st, "http://127.0.0.1:1/bank-holidays.json", later); err == nil {
		t.Error("an unreachable address was not an error")
	}
	if !has() {
		t.Error("a failed fetch changed the closures")
	}
	if fetched, _ := SyncOnce(ctx, st, DefaultURL, later.Add(time.Hour)); fetched {
		t.Error("retried within the wait after a failure")
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
