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

// SendMasterViewNotice tells a squadron owner that the site administrator
// opened their squadron.
func (d Dynamic) SendMasterViewNotice(to, squadronName, whenUTC string) error {
	s, err := d.sender()
	if err != nil {
		log.Printf("mail not sent: %v", err)
		return err
	}
	return s.SendMasterViewNotice(to, squadronName, whenUTC)
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

// SendMasterViewNotice is the owner's notice that the site administrator
// opened the squadron (read-only). It carries no link and no data.
func (s Sender) SendMasterViewNotice(to, squadronName, whenUTC string) error {
	return s.send(to, "The site administrator viewed "+squadronName+" on RAFACTools Planner",
		"The site administrator of RAFACTools Planner opened your squadron ("+squadronName+") at "+whenUTC+" UTC, to help with a problem.\r\n\r\n"+
			"They could only look (read-only); nothing was changed. The visit is also listed in your squadron's security log under Squadron settings.\r\n\r\n"+
			"You are sent this once an hour at most, however many times they look.\r\n")
}

// SendTest sends the test message.
func (s Sender) SendTest(to string) error {
	return s.send(to, "RAFACTools Planner test email",
		"This is a test message from RAFACTools Planner.\r\n\r\n"+
			"If you can read it, the site can send email with the settings you saved.\r\n")
}

// SendTempPassword emails a member the temporary password an owner asked for.
func (d Dynamic) SendTempPassword(to, squadronName, loginURL, password, expiresUTC string) error {
	s, err := d.sender()
	if err != nil {
		log.Printf("mail not sent: %v", err)
		return err
	}
	return s.SendTempPassword(to, squadronName, loginURL, password, expiresUTC)
}

// SendEmailChangeVerify sends the link that confirms a new address.
func (d Dynamic) SendEmailChangeVerify(to, link string) error {
	s, err := d.sender()
	if err != nil {
		log.Printf("mail not sent: %v", err)
		return err
	}
	return s.SendEmailChangeVerify(to, link)
}

// SendEmailChanged tells the OLD address that the account's address was changed.
func (d Dynamic) SendEmailChanged(to, newEmail string) error {
	s, err := d.sender()
	if err != nil {
		log.Printf("mail not sent: %v", err)
		return err
	}
	return s.SendEmailChanged(to, newEmail)
}

// SendTempPassword is the temporary-password message. It is the one email that
// carries a password, so it says plainly that it works once and for a week.
func (s Sender) SendTempPassword(to, squadronName, loginURL, password, expiresUTC string) error {
	return s.send(to, "A temporary password for RAFACTools Planner",
		"An owner of "+squadronName+" has sent you a temporary password for RAFACTools Planner.\r\n\r\n"+
			"Temporary password: "+password+"\r\n\r\n"+
			"Sign in at "+loginURL+" with your email address and this password. You will be asked to choose your own password straight away.\r\n\r\n"+
			"It stops working on "+expiresUTC+" UTC. After that, use \"Forgot your password?\" on the sign-in page.\r\n\r\n"+
			"If you didn't expect this, tell an owner of your squadron. Your old password no longer works.\r\n")
}

// SendEmailChangeVerify asks the new address to confirm.
func (s Sender) SendEmailChangeVerify(to, link string) error {
	return s.send(to, "Confirm your new email address for RAFACTools Planner",
		"Someone (you, we hope) asked to use this address for a RAFACTools Planner account.\r\n\r\n"+
			"To confirm, open:\r\n"+link+"\r\n\r\n"+
			"This link works once and expires in 24 hours. If you didn't ask for this, ignore this email and nothing changes.\r\n")
}

// SendEmailChanged tells the old address, so a hijacked change is noticed.
func (s Sender) SendEmailChanged(to, newEmail string) error {
	return s.send(to, "Your RAFACTools Planner email address was changed",
		"The email address on your RAFACTools Planner account was changed to "+newEmail+".\r\n\r\n"+
			"If that was you, nothing more is needed. If it wasn't, tell an owner of your squadron straight away.\r\n")
}
