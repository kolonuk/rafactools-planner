package mail

import "testing"

func TestSendRejectsHeaderInjection(t *testing.T) {
	s := Sender{From: "a@example.org"} // no Host: would only log, but guard runs first
	cases := map[string][2]string{
		"to":      {"victim@example.org\r\nBcc: evil@example.org", "subject"},
		"subject": {"victim@example.org", "hi\r\nBcc: evil@example.org"},
	}
	for name, c := range cases {
		if err := s.send(c[0], c[1], "body"); err != errHeaderInjection {
			t.Errorf("%s: err = %v, want errHeaderInjection", name, err)
		}
	}
	if err := s.send("ok@example.org", "fine", "body"); err != nil {
		t.Errorf("clean message rejected: %v", err)
	}
}
