package mail

import (
	"errors"
	"testing"
)

func TestDynamicRefusesToSendOrLogWithoutAServer(t *testing.T) {
	d := Dynamic{Resolve: func() (Sender, error) { return Sender{}, nil }}
	for name, err := range map[string]error{
		"verification": d.SendVerification("a@example.org", "http://x", "tok"),
		"invite":       d.SendInvite("a@example.org", "http://x", "tok", "Sqn", "o@example.org"),
		"reset":        d.SendPasswordReset("a@example.org", "http://x", "tok"),
		"report":       d.SendIssueReport("a@example.org", IssueReport{ID: "1", Subject: "s", Body: "b"}),
		"test":         d.SendTest("a@example.org"),
	} {
		if !errors.Is(err, ErrNotConfigured) {
			t.Errorf("%s: err = %v, want ErrNotConfigured", name, err)
		}
	}
}

func TestDynamicMayLogWhenAskedButTestNeverDoes(t *testing.T) {
	d := Dynamic{Resolve: func() (Sender, error) { return Sender{}, nil }, AllowLog: true}
	if err := d.SendVerification("a@example.org", "http://x", "tok"); err != nil {
		t.Errorf("allowed log mode: %v", err)
	}
	if err := d.SendTest("a@example.org"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("a test must not succeed by logging: %v", err)
	}
}

func TestDynamicUsesTheCurrentSettingsOnEverySend(t *testing.T) {
	var n int
	d := Dynamic{Resolve: func() (Sender, error) { n++; return Sender{Host: "127.0.0.1", Port: 1, From: "a@example.org"}, nil }}
	d.SendTest("a@example.org") // connection refused is fine: only the lookup is measured
	d.SendTest("a@example.org")
	if n != 2 {
		t.Errorf("settings resolved %d times for 2 sends", n)
	}
	bad := Dynamic{Resolve: func() (Sender, error) { return Sender{}, errors.New("MAIL_PROVIDER=ses needs SES_REGION") }}
	if err := bad.SendTest("a@example.org"); err == nil || errors.Is(err, ErrNotConfigured) {
		t.Errorf("a settings error must be reported as such: %v", err)
	}
}
