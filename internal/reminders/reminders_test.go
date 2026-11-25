package reminders

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rafactools-planner/internal/db"
	"rafactools-planner/internal/programme"
)

func setup(t *testing.T) (*programme.Store, int64) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if _, err := d.Exec(`INSERT INTO squadrons (number, name, parade_weekdays) VALUES ('7', '7 Sqn', '[1]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO users (squadron_id, email, password_hash, role) VALUES (1, 'o@x', 'x', 'owner')`); err != nil {
		t.Fatal(err)
	}
	return programme.NewStore(d), 1
}

type outbox struct{ sent []string }

func (o *outbox) send(to, subject, body string) error {
	o.sent = append(o.sent, to+"|"+subject+"|"+body)
	return nil
}

func at(s string) func() time.Time {
	return func() time.Time {
		v, _ := time.ParseInLocation("2006-01-02 15:04", s, programme.Local)
		return v
	}
}

func TestRemindersAreSentOnceForBlankNightsAhead(t *testing.T) {
	st, sq := setup(t)
	old := programme.Now
	programme.Now = at("2030-04-01 09:00")
	defer func() { programme.Now = old }()
	p, err := st.CreateProgramme(sq, 2030, 4, 1) // Mondays: 1 8 15 22 29
	if err != nil {
		t.Fatal(err)
	}
	box := &outbox{}
	deps := func(now string) Deps {
		return Deps{Store: st, BaseURL: "https://planner.example/", Now: at(now), Send: box.send,
			Recipients: func(int64) ([]string, error) { return []string{"owner@example.org", "editor@example.org"}, nil }}
	}

	t.Run("off by default", func(t *testing.T) {
		if n, err := RunOnce(deps("2030-04-01 09:00")); err != nil || n != 0 {
			t.Errorf("sent %d, %v", n, err)
		}
	})

	if err := st.SetReminderDays(sq, 7); err != nil {
		t.Fatal(err)
	}
	t.Run("not before the morning", func(t *testing.T) {
		if n, _ := RunOnce(deps("2030-04-01 06:30")); n != 0 {
			t.Errorf("sent %d at 06:30", n)
		}
	})

	t.Run("a night within the days ahead with blank boxes is reminded once, to every recipient", func(t *testing.T) {
		n, err := RunOnce(deps("2030-04-01 09:00")) // covers 1 April to 8 April: two nights
		if err != nil || n != 2 {
			t.Fatalf("sent %d, %v", n, err)
		}
		msg := box.sent[0]
		for _, want := range []string{"owner@example.org", "7 Sqn: 2 parade nights with blank boxes", "Monday 1 April", "Monday 8 April", "4 empty Activity boxes, 4 empty Instructor boxes", "https://planner.example/\n"} {
			if !strings.Contains(msg, want) {
				t.Errorf("the email lacks %q:\n%s", want, msg)
			}
		}
		if strings.Contains(msg, "15 April") {
			t.Error("a night beyond the days ahead was included")
		}
		if again, _ := RunOnce(deps("2030-04-01 12:00")); again != 0 {
			t.Errorf("the same nights were reminded about again (%d)", again)
		}
	})

	t.Run("the next night is reminded about when it comes within range", func(t *testing.T) {
		box.sent = nil
		if n, _ := RunOnce(deps("2030-04-08 09:00")); n != 2 {
			t.Fatalf("sent %d on 8 April", n)
		}
		if !strings.Contains(box.sent[0], "1 parade night with blank boxes") || !strings.Contains(box.sent[0], "Monday 15 April") {
			t.Errorf("wrong email:\n%s", box.sent[0])
		}
	})

	t.Run("a full night and a closed night are left alone", func(t *testing.T) {
		fp, _ := st.GetFullProgramme(sq, p.ID)
		var in []programme.DayInput
		for _, d := range fp.Days {
			if d.Date == "2030-04-22" {
				act, _ := d.RowByHalfType(1, "activity")
				ins, _ := d.RowByHalfType(1, "instructor")
				full := []string{"a", "b", "c", "d"}
				in = append(in, programme.DayInput{ID: d.ID, IsParade: true, Rows: []programme.RowInput{{ID: act.ID, Texts: full}, {ID: ins.ID, Texts: full}}})
			}
		}
		if err := st.SaveDaysAndRows(sq, p.ID, in); err != nil {
			t.Fatal(err)
		}
		if err := st.AddClosure(p.ID, "2030-04-29", "2030-04-29", "Closed"); err != nil {
			t.Fatal(err)
		}
		box.sent = nil
		if n, _ := RunOnce(deps("2030-04-22 09:00")); n != 0 {
			t.Errorf("sent %d for a full night and a closed one: %v", n, box.sent)
		}
	})

	t.Run("a failed send is tried again", func(t *testing.T) {
		st2, sq2 := setup(t)
		programme.Now = at("2030-05-01 09:00")
		if _, err := st2.CreateProgramme(sq2, 2030, 5, 1); err != nil {
			t.Fatal(err)
		}
		st2.SetReminderDays(sq2, 14)
		fail := true
		d := Deps{Store: st2, BaseURL: "x", Now: at("2030-05-01 09:00"),
			Recipients: func(int64) ([]string, error) { return []string{"a@example.org"}, nil },
			Send: func(string, string, string) error {
				if fail {
					return errFail
				}
				return nil
			}}
		if n, _ := RunOnce(d); n != 0 {
			t.Errorf("counted %d sent when sending failed", n)
		}
		fail = false
		if n, _ := RunOnce(d); n != 1 {
			t.Errorf("did not retry: sent %d", n)
		}
	})
}

type failError struct{}

func (failError) Error() string { return "mail server down" }

var errFail = failError{}
