package mail

import (
	"bufio"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a minimal in-process SMTP server that records what it is sent,
// so the tests check the real wire output of Sender rather than its inputs.
type fakeSMTP struct {
	ln   net.Listener
	mu   sync.Mutex
	from string
	rcpt []string
	data string
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	say := func(s string) { c.Write([]byte(s + "\r\n")) }
	say("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		switch cmd := strings.ToUpper(line); {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			say("250 fake")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			f.mu.Lock()
			f.from = line[len("MAIL FROM:"):]
			f.mu.Unlock()
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			f.mu.Lock()
			f.rcpt = append(f.rcpt, line[len("RCPT TO:"):])
			f.mu.Unlock()
			say("250 ok")
		case cmd == "DATA":
			say("354 go")
			var sb strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				// a real server removes the extra dot SMTP adds to lines starting with "."
				l = strings.TrimPrefix(l, ".")
				sb.WriteString(l)
			}
			f.mu.Lock()
			f.data = sb.String()
			f.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func (f *fakeSMTP) sender() Sender {
	host, port, _ := net.SplitHostPort(f.ln.Addr().String())
	p, _ := strconv.Atoi(port)
	return Sender{Host: host, Port: p, From: "no-reply@rafactools.example"}
}

func TestSendVerificationOnTheWire(t *testing.T) {
	f := newFakeSMTP(t)
	if err := f.sender().SendVerification("oc.1@rafac.mod.gov.uk", "https://x.example/rafactools", "tok123"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.Contains(f.from, "no-reply@rafactools.example") || len(f.rcpt) != 1 || !strings.Contains(f.rcpt[0], "oc.1@rafac.mod.gov.uk") {
		t.Errorf("envelope from=%q rcpt=%q", f.from, f.rcpt)
	}
	for _, want := range []string{
		"To: oc.1@rafac.mod.gov.uk\r\n", "Subject: Confirm your RAFACTools Planner account\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n", "https://x.example/rafactools/verify?token=tok123",
	} {
		if !strings.Contains(f.data, want) {
			t.Errorf("message missing %q:\n%s", want, f.data)
		}
	}
}

func TestSendInviteOnTheWire(t *testing.T) {
	f := newFakeSMTP(t)
	if err := f.sender().SendInvite("new@example.org", "https://x.example", "invtok", "2491 Sqn", "boss@rafac.mod.gov.uk"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, want := range []string{"Subject: You've been invited to 2491 Sqn on RAFACTools Planner\r\n", "https://x.example/invite/invtok", "boss@rafac.mod.gov.uk has invited you", "7 days"} {
		if !strings.Contains(f.data, want) {
			t.Errorf("invite missing %q:\n%s", want, f.data)
		}
	}
}

// The squadron name is user-controlled and goes into the Subject header.
// Whatever it contains, the server must never see an extra header.
func TestInjectionAttemptsNeverReachTheWire(t *testing.T) {
	f := newFakeSMTP(t)
	s := f.sender()
	if err := s.SendInvite("a@example.org", "https://x", "t", "Sqn\r\nBcc: evil@example.org", "o@example.org"); err == nil {
		t.Error("CRLF in squadron name was sent")
	}
	if err := s.SendInvite("a@example.org\r\nBcc: evil@example.org", "https://x", "t", "Sqn", "o@example.org"); err == nil {
		t.Error("CRLF in recipient was sent")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data != "" || len(f.rcpt) != 0 {
		t.Errorf("something reached the server: rcpt=%v data=%q", f.rcpt, f.data)
	}
}

func TestSMTPFailureIsReported(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().(*net.TCPAddr)
	ln.Close() // nothing listening
	s := Sender{Host: "127.0.0.1", Port: addr.Port, From: "a@b.c"}
	if err := s.SendVerification("x@y.z", "https://x", "t"); err == nil {
		t.Error("connection failure swallowed: callers would tell users the email was sent")
	}
}

func TestUnconfiguredSenderDoesNotFail(t *testing.T) {
	if err := (Sender{From: "a@b.c"}).SendVerification("x@y.z", "https://x", "t"); err != nil {
		t.Errorf("dev-mode logging sender errored: %v", err)
	}
}

func TestSendPasswordResetOnTheWire(t *testing.T) {
	f := newFakeSMTP(t)
	if err := f.sender().SendPasswordReset("cadet@example.org", "https://x.example/planner", "resettok"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, want := range []string{
		"Subject: Reset your RAFACTools Planner password\r\n", "https://x.example/planner/reset/resettok",
		"expires in 1 hour", "signs you out everywhere", "If you didn't ask for this, ignore this email",
	} {
		if !strings.Contains(f.data, want) {
			t.Errorf("reset email missing %q:\n%s", want, f.data)
		}
	}
}

func TestEveryMessageCarriesDateAndAUniqueMessageIDInTheSendersDomain(t *testing.T) {
	f := newFakeSMTP(t)
	ids := map[string]bool{}
	for i := 0; i < 3; i++ {
		if err := f.sender().SendTest("a@example.org"); err != nil {
			t.Fatal(err)
		}
		f.mu.Lock()
		data := f.data
		f.mu.Unlock()
		m := regexp.MustCompile(`(?m)^Message-ID: <([0-9a-f]{24})@rafactools\.example>\r$`).FindStringSubmatch(data)
		if m == nil {
			t.Fatalf("no Message-ID in the sender's domain:\n%s", data)
		}
		if ids[m[1]] {
			t.Errorf("Message-ID %s repeated", m[1])
		}
		ids[m[1]] = true
		dm := regexp.MustCompile(`(?m)^Date: (.+)\r$`).FindStringSubmatch(data)
		if dm == nil {
			t.Fatalf("no Date header:\n%s", data)
		}
		if _, err := time.Parse(time.RFC1123Z, dm[1]); err != nil {
			t.Errorf("Date %q is not RFC 5322: %v", dm[1], err)
		}
	}
}

func TestNonASCIISubjectsAreEncodedNotSentRaw(t *testing.T) {
	f := newFakeSMTP(t)
	if err := f.sender().SendInvite("a@example.org", "https://x.example", "tok", "Squadron Zoë – Cadets", "o@example.org"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	head, _, _ := strings.Cut(f.data, "\r\n\r\n")
	for i := 0; i < len(head); i++ {
		if head[i] > 126 {
			t.Fatalf("a raw non-ASCII byte in the headers:\n%s", head)
		}
	}
	if !strings.Contains(head, "Subject: =?utf-8?q?") {
		t.Errorf("subject not Q-encoded:\n%s", head)
	}
}

func TestSendTestOnTheWire(t *testing.T) {
	f := newFakeSMTP(t)
	if err := f.sender().SendTest("me@example.org"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.Contains(f.data, "Subject: RAFACTools Planner test email") || !strings.Contains(f.data, "the site can send email") {
		t.Errorf("test message:\n%s", f.data)
	}
}

func TestAServerThatNeverAnswersTimesOutInsteadOfHanging(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close() // accept, then say nothing at all
		}
	}()
	old := ioTimeout
	ioTimeout = 300 * time.Millisecond
	defer func() { ioTimeout = old }()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(port)
	done := make(chan error, 1)
	go func() { done <- Sender{Host: host, Port: p, From: "a@example.org"}.SendTest("b@example.org") }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a silent server was reported as success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sending hung on a server that never answers")
	}
}

func TestConnectionRefusedIsAnError(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	p, _ := strconv.Atoi(port)
	if err := (Sender{Host: host, Port: p, From: "a@example.org"}).SendTest("b@example.org"); err == nil {
		t.Error("connection refused was not reported")
	}
}
