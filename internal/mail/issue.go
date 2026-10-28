package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strings"
)

// Attachment is one file attached to an issue report.
type Attachment struct {
	Name        string
	ContentType string
	Data        []byte
}

// IssueReport is a problem report to email to the people who run the site.
// Everything in it was supplied by a user (or their browser), so every value
// that reaches a header is checked.
type IssueReport struct {
	ID          string // short reference, put in the subject
	Subject     string
	Body        string // plain-text readable report
	ReplyTo     string // optional: only set when the person agreed to be contacted
	Attachments []Attachment
}

var errBadIssueField = errors.New("mail: issue report field contains a line break or control character")

func headerSafe(vs ...string) bool {
	for _, v := range vs {
		if strings.ContainsAny(v, "\r\n\x00") {
			return false
		}
	}
	return true
}

// SendIssueReport emails an issue report as multipart/mixed: the readable
// text, then each attachment (a context file and optionally a screenshot).
func (s Sender) SendIssueReport(to string, r IssueReport) error {
	fields := []string{to, s.From, r.ID, r.Subject, r.ReplyTo}
	for _, a := range r.Attachments {
		fields = append(fields, a.Name, a.ContentType)
	}
	if !headerSafe(fields...) {
		return errBadIssueField
	}
	subject := fmt.Sprintf("[RAFACTools issue %s] %s", r.ID, r.Subject)
	if !s.configured() {
		log.Printf("[mail] SMTP not configured: issue report %q for %s (%d attachment(s)):\n%s", subject, to, len(r.Attachments), r.Body)
		return nil
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	var rnd [12]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return err
	}
	if err := mw.SetBoundary("rt-" + hex.EncodeToString(rnd[:])); err != nil {
		return err
	}

	text, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/plain; charset=UTF-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return err
	}
	qp := quotedprintable.NewWriter(text)
	if _, err := qp.Write([]byte(r.Body)); err != nil {
		return err
	}
	if err := qp.Close(); err != nil {
		return err
	}

	for _, a := range r.Attachments {
		p, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {mime.FormatMediaType(a.ContentType, map[string]string{"name": a.Name})},
			"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": a.Name})},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return fmt.Errorf("attachment %q: %w", a.Name, err)
		}
		enc := base64.StdEncoding.EncodeToString(a.Data)
		for len(enc) > 76 {
			if _, err := p.Write([]byte(enc[:76] + "\r\n")); err != nil {
				return err
			}
			enc = enc[76:]
		}
		if _, err := p.Write([]byte(enc + "\r\n")); err != nil {
			return err
		}
	}
	if err := mw.Close(); err != nil {
		return err
	}

	var head bytes.Buffer
	fmt.Fprintf(&head, "From: %s\r\nTo: %s\r\n%s", s.From, to, s.stdHeaders())
	if r.ReplyTo != "" {
		fmt.Fprintf(&head, "Reply-To: %s\r\n", r.ReplyTo)
	}
	fmt.Fprintf(&head, "Subject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%q\r\n\r\n",
		encodeSubject(subject), mw.Boundary())
	return s.deliver(to, append(head.Bytes(), buf.Bytes()...))
}
