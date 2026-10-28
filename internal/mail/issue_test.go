package mail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
)

// parse reads back the message the fake SMTP server received.
func parseMessage(t *testing.T, raw string) (*mail.Message, []part) {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("not a valid email: %v\n%s", err, raw)
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/mixed" {
		t.Fatalf("content type = %q (%v), want multipart/mixed", msg.Header.Get("Content-Type"), err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var parts []part
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var data []byte
		switch strings.ToLower(p.Header.Get("Content-Transfer-Encoding")) {
		case "base64":
			b, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
			data = b
		case "quoted-printable":
			b, _ := io.ReadAll(quotedprintable.NewReader(p))
			data = b
		default:
			data, _ = io.ReadAll(p)
		}
		_, dparams, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		parts = append(parts, part{Type: p.Header.Get("Content-Type"), Filename: dparams["filename"], Data: data})
	}
	return msg, parts
}

type part struct {
	Type, Filename string
	Data           []byte
}

func TestSendIssueReportBuildsAMultipartEmail(t *testing.T) {
	f := newFakeSMTP(t)
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0, 1, 2, 250, 251, 252}, 500)...) // binary, > one base64 line
	r := IssueReport{
		ID: "ab12cd34", Subject: "Bug: grid won't save",
		Body:    "What happened:\nLine one.\n.\nLine with a lone dot above and ünïcödé.\n",
		ReplyTo: "cadet@example.org",
		Attachments: []Attachment{
			{Name: "context.json", ContentType: "application/json", Data: []byte(`{"a":1}`)},
			{Name: "screenshot.png", ContentType: "image/png", Data: png},
		},
	}
	if err := f.sender().SendIssueReport("issues@example.org", r); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	raw := f.data
	f.mu.Unlock()

	msg, parts := parseMessage(t, raw)
	if got := msg.Header.Get("To"); got != "issues@example.org" {
		t.Errorf("To = %q", got)
	}
	if got := msg.Header.Get("Reply-To"); got != "cadet@example.org" {
		t.Errorf("Reply-To = %q", got)
	}
	subj, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if subj != "[RAFACTools issue ab12cd34] Bug: grid won't save" {
		t.Errorf("Subject = %q", subj)
	}
	if len(parts) != 3 {
		t.Fatalf("%d parts, want text + 2 attachments", len(parts))
	}
	if !strings.HasPrefix(parts[0].Type, "text/plain") || strings.ReplaceAll(string(parts[0].Data), "\r\n", "\n") != r.Body {
		t.Errorf("body part = %q / %q (lone-dot line and UTF-8 must survive)", parts[0].Type, parts[0].Data)
	}
	if parts[1].Filename != "context.json" || string(parts[1].Data) != `{"a":1}` {
		t.Errorf("context part = %+v", parts[1])
	}
	if parts[2].Filename != "screenshot.png" || !bytes.Equal(parts[2].Data, png) || !strings.HasPrefix(parts[2].Type, "image/png") {
		t.Errorf("screenshot part wrong: %q %q len %d", parts[2].Type, parts[2].Filename, len(parts[2].Data))
	}
}

func TestSendIssueReportWithoutReplyToOrAttachments(t *testing.T) {
	f := newFakeSMTP(t)
	if err := f.sender().SendIssueReport("issues@example.org", IssueReport{ID: "1", Subject: "x", Body: "hello"}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	raw := f.data
	f.mu.Unlock()
	msg, parts := parseMessage(t, raw)
	if msg.Header.Get("Reply-To") != "" || len(parts) != 1 {
		t.Errorf("reply-to %q, %d parts", msg.Header.Get("Reply-To"), len(parts))
	}
}

// Everything in a report comes from a user, so none of it may reach a header
// unchecked, and attachment names must not be able to break out either.
func TestSendIssueReportRefusesHeaderInjection(t *testing.T) {
	f := newFakeSMTP(t)
	s := f.sender()
	bad := []IssueReport{
		{ID: "1", Subject: "hi\r\nBcc: evil@example.org", Body: "b"},
		{ID: "1", Subject: "hi", Body: "b", ReplyTo: "a@example.org\r\nBcc: evil@example.org"},
		{ID: "1\r\nBcc: x", Subject: "hi", Body: "b"},
		{ID: "1", Subject: "hi", Body: "b", Attachments: []Attachment{{Name: "a\"\r\nX: y", ContentType: "image/png", Data: []byte("x")}}},
		{ID: "1", Subject: "hi", Body: "b", Attachments: []Attachment{{Name: "a.png", ContentType: "image/png\r\nX: y", Data: []byte("x")}}},
	}
	for i, r := range bad {
		if err := s.SendIssueReport("issues@example.org", r); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	if err := s.SendIssueReport("issues@example.org\r\nBcc: evil@example.org", IssueReport{ID: "1", Subject: "s", Body: "b"}); err == nil {
		t.Error("recipient with a line break accepted")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data != "" || len(f.rcpt) != 0 {
		t.Errorf("something reached the server: %v %q", f.rcpt, f.data)
	}
}

func TestSendIssueReportUnconfiguredLogsInsteadOfFailing(t *testing.T) {
	if err := (Sender{From: "a@b.c"}).SendIssueReport("issues@example.org", IssueReport{ID: "1", Subject: "x", Body: "b"}); err != nil {
		t.Errorf("dev-mode sender failed: %v", err)
	}
}
