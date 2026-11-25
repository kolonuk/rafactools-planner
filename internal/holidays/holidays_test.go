package holidays

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestSyncOnceFetchesAMonthAtATimeAndAppliesChanges(t *testing.T) {
	var hits atomic.Int32
	body := sample
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(body))
	}))
	defer srv.Close()

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
	programme.Now = func() time.Time { return time.Date(2030, 6, 1, 9, 0, 0, 0, time.UTC) }
	defer func() { programme.Now = old }()
	p, err := st.CreateProgramme(1, 2030, 12, 1)
	if err != nil {
		t.Fatal(err)
	}
	closures := func() int {
		fp, _ := st.GetFullProgramme(1, p.ID)
		return len(fp.Closures)
	}
	if closures() != 0 {
		t.Fatal("closures before any list was fetched")
	}

	now := time.Date(2030, 6, 1, 9, 0, 0, 0, time.UTC)
	ctx := context.Background()
	if fetched, err := SyncOnce(ctx, st, srv.URL, now); err != nil || !fetched {
		t.Fatalf("first sync: fetched=%v err=%v", fetched, err)
	}
	if closures() != 2 {
		t.Errorf("after the first fetch the existing month has %d closures, want 2", closures())
	}
	if fetched, _ := SyncOnce(ctx, st, srv.URL, now.Add(7*24*time.Hour)); fetched || hits.Load() != 1 {
		t.Errorf("fetched again within a month (hits %d)", hits.Load())
	}
	body = strings.Replace(sample, `"2030-12-26"`, `"2030-12-27"`, 1)
	if fetched, err := SyncOnce(ctx, st, srv.URL, now.Add(30*24*time.Hour)); err != nil || !fetched {
		t.Fatalf("monthly sync: fetched=%v err=%v", fetched, err)
	}
	fp, _ := st.GetFullProgramme(1, p.ID)
	if len(fp.Closures) != 2 || fp.Closures[1].StartDate != "2030-12-27" {
		t.Errorf("the changed date was not picked up: %+v", fp.Closures)
	}

	// A failing download keeps what is kept and is not retried at once.
	srv.Close()
	later := now.Add(70 * 24 * time.Hour)
	if _, err := SyncOnce(ctx, st, srv.URL, later); err == nil {
		t.Error("an unreachable list was not an error")
	}
	if closures() != 2 {
		t.Error("a failed fetch changed the closures")
	}
	if fetched, _ := SyncOnce(ctx, st, srv.URL, later.Add(time.Hour)); fetched {
		t.Error("retried within the wait after a failure")
	}
}
