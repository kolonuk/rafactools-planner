package mail

import "testing"

func TestNoticeRefusesLineBreaksInHeaders(t *testing.T) {
	s := Sender{From: "no-reply@example.org"}
	for _, c := range [][2]string{{"a@example.org\r\nBcc: x@example.org", "Hi"}, {"a@example.org", "Hi\nBcc: x@example.org"}} {
		if err := s.SendNotice(c[0], c[1], "body"); err == nil {
			t.Errorf("SendNotice(%q, %q) was accepted", c[0], c[1])
		}
	}
	// With no mail server it logs and succeeds, whatever the body holds.
	if err := s.SendNotice("a@example.org", "Hi", "line\n.\nQUIT\n"); err != nil {
		t.Errorf("a body with SMTP-looking lines: %v", err)
	}
}
