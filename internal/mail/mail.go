// Package mail sends the email-confirmation link. With no SMTP configured
// (local dev) it logs the link to stdout instead of sending it, so signup
// works out of the box without a mail server — see docker-compose.yml, which
// wires Mailpit for dev so you can actually see the email render.
package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/smtp"
	"strings"
)

type Sender struct {
	Host, User, Pass, From string
	Port                   int
}

func (s Sender) configured() bool { return s.Host != "" }

func (s Sender) SendVerification(to, baseURL, token string) error {
	link := fmt.Sprintf("%s/verify?token=%s", baseURL, token)
	subject := "Confirm your RAFACTools Planner account"
	body := fmt.Sprintf("Welcome to RAFACTools Planner.\r\n\r\n"+
		"Confirm your email address to finish signing up:\r\n%s\r\n\r\n"+
		"This link expires in 24 hours. If you didn't request this, ignore this email.\r\n", link)
	return s.send(to, subject, body)
}

func (s Sender) SendInvite(to, baseURL, token, squadronName, invitedByEmail string) error {
	link := fmt.Sprintf("%s/invite/%s", baseURL, token)
	subject := fmt.Sprintf("You've been invited to %s on RAFACTools Planner", squadronName)
	body := fmt.Sprintf("%s has invited you to join %s on RAFACTools Planner.\r\n\r\n"+
		"Set up your account:\r\n%s\r\n\r\n"+
		"This link expires in 7 days.\r\n", invitedByEmail, squadronName, link)
	return s.send(to, subject, body)
}

// errHeaderInjection is returned when a header value contains CR/LF, which
// would let a caller smuggle extra headers (e.g. Bcc:) or SMTP commands into
// the message. Recipient addresses and the squadron name in the subject are
// user-supplied, so every header value is checked here, at the one place
// they are assembled.
var errHeaderInjection = errors.New("mail: header value contains a line break")

func (s Sender) send(to, subject, body string) error {
	for _, v := range []string{to, subject, s.From} {
		if strings.ContainsAny(v, "\r\n") {
			return errHeaderInjection
		}
	}
	if !s.configured() {
		log.Printf("[mail] SMTP not configured — %q for %s:\n%s", subject, to, body)
		return nil
	}

	msg := fmt.Appendf(nil, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		s.From, to, subject, body)
	return s.deliver(to, msg)
}

// deliver hands a complete, already-built message to the SMTP server.
func (s Sender) deliver(to string, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	// Mailpit (dev) and most relays on 587/25 accept plain SMTP with
	// optional AUTH; only reach for implicit TLS on 465.
	if s.Port == 465 {
		return s.sendImplicitTLS(addr, to, msg)
	}

	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Pass, s.Host)
	}
	return smtp.SendMail(addr, auth, s.From, []string{to}, msg)
}

func (s Sender) sendImplicitTLS(addr, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if s.User != "" {
		if err := client.Auth(smtp.PlainAuth("", s.User, s.Pass, s.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(s.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (s Sender) SendPasswordReset(to, baseURL, token string) error {
	link := fmt.Sprintf("%s/reset/%s", baseURL, token)
	subject := "Reset your RAFACTools Planner password"
	body := fmt.Sprintf("Someone asked to reset the password for this email address on RAFACTools Planner.\r\n\r\n"+
		"To choose a new password, open:\r\n%s\r\n\r\n"+
		"This link works once and expires in 1 hour. Using it signs you out everywhere.\r\n\r\n"+
		"If you didn't ask for this, ignore this email: your password has not changed.\r\n", link)
	return s.send(to, subject, body)
}
