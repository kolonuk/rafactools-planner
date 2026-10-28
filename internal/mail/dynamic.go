package mail

import (
	"errors"
	"fmt"
	"log"
)

// ErrNotConfigured is returned when no mail server is set and logging mail was
// not asked for, so nothing is sent (and no link is written to the log).
var ErrNotConfigured = errors.New("mail: no mail server is configured")

// Dynamic sends through whatever settings are current at the moment of each
// send, so a change saved by the master admin takes effect without a restart.
// Resolve returns the sender to use; allowLog says whether an unconfigured
// server may fall back to logging (development only).
type Dynamic struct {
	Resolve  func() (Sender, error)
	AllowLog bool
}

func (d Dynamic) sender() (Sender, error) {
	s, err := d.Resolve()
	if err != nil {
		return Sender{}, fmt.Errorf("mail settings: %w", err)
	}
	if !s.configured() && !d.AllowLog {
		return Sender{}, ErrNotConfigured
	}
	return s, nil
}

func (d Dynamic) SendVerification(to, baseURL, token string) error {
	s, err := d.sender()
	if err != nil {
		log.Printf("mail not sent: %v", err)
		return err
	}
	return s.SendVerification(to, baseURL, token)
}

func (d Dynamic) SendInvite(to, baseURL, token, squadronName, invitedByEmail string) error {
	s, err := d.sender()
	if err != nil {
		log.Printf("mail not sent: %v", err)
		return err
	}
	return s.SendInvite(to, baseURL, token, squadronName, invitedByEmail)
}

func (d Dynamic) SendPasswordReset(to, baseURL, token string) error {
	s, err := d.sender()
	if err != nil {
		log.Printf("mail not sent: %v", err)
		return err
	}
	return s.SendPasswordReset(to, baseURL, token)
}

func (d Dynamic) SendIssueReport(to string, r IssueReport) error {
	s, err := d.sender()
	if err != nil {
		log.Printf("mail not sent: %v", err)
		return err
	}
	return s.SendIssueReport(to, r)
}

// SendTest sends a short message so the master admin can check the settings.
// Unlike the others it never falls back to logging: a test that "succeeds" by
// writing to a log would be misleading.
func (d Dynamic) SendTest(to string) error {
	s, err := d.Resolve()
	if err != nil {
		return fmt.Errorf("mail settings: %w", err)
	}
	if !s.configured() {
		return ErrNotConfigured
	}
	return s.SendTest(to)
}

// SendTest sends the test message.
func (s Sender) SendTest(to string) error {
	return s.send(to, "RAFACTools Planner test email",
		"This is a test message from RAFACTools Planner.\r\n\r\n"+
			"If you can read it, the site can send email with the settings you saved.\r\n")
}
