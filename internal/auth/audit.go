package auth

import (
	"time"
	"unicode/utf8"
)

// Audit actions. Only these are recorded, and only these are displayed.
const (
	AuditMasterView   = "master.view"   // the master admin opened a squadron
	AuditMemberRole   = "member.role"   // an owner changed someone's role
	AuditMemberRemove = "member.remove" // an owner removed someone
	AuditInviteSent   = "invite.sent"   // an owner invited someone
	AuditSettingsMail = "settings.mail" // the master admin changed the email settings

	AuditLogin          = "login"                  // someone signed in
	AuditLoginFailed    = "login.fail"             // a wrong password for an existing account
	AuditPasswordChange = "password.change"        // a signed-in user changed their password
	AuditPasswordReset  = "password.reset"         // a password was reset from the emailed link
	AuditSignOutOthers  = "session.signout_others" // someone signed out their other sessions
	AuditMemberInactive = "member.inactive"        // an owner marked a member inactive
	AuditMemberActive   = "member.active"          // an owner made a member active again
	AuditTempPassword   = "member.temp_password"   // an owner emailed a member a temporary password
	AuditEmailChange    = "email.change"           // someone changed their email address
	AuditSquadronEdit   = "squadron.settings"      // an owner saved the squadron settings
)

const (
	maxAuditDetail = 200
	// A wrong password for the same account is recorded at most this often, so
	// guessing cannot flood the log.
	failedLoginWindow = 5 * time.Minute
	auditKeepFor      = 365 * 24 * time.Hour
)

// AuditEntry is one line of the trail.
type AuditEntry struct {
	At          time.Time
	SquadronNum string // set only by ListAuditAll
	ActorEmail  string
	Action      string
	Detail      string
}

// Audit records an event. squadronID 0 means it belongs to no squadron.
// Failure is returned for the caller to log; it must never stop the action
// being audited from completing.
func (s *Service) Audit(squadronID, actorID int64, actorEmail, action, detail string) error {
	if utf8.RuneCountInString(detail) > maxAuditDetail {
		detail = string([]rune(detail)[:maxAuditDetail])
	}
	var sq any
	if squadronID != 0 {
		sq = squadronID
	}
	if _, err := s.db.Exec(`INSERT INTO audit_log (squadron_id, actor_id, actor_email, action, detail) VALUES (?, ?, ?, ?, ?)`,
		sq, actorID, actorEmail, action, detail); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM audit_log WHERE at < ?`, time.Now().UTC().Add(-auditKeepFor))
	return err
}

// AuditUser records an event done by an existing user, looking up their
// squadron and address. A repeated wrong password within a minute is recorded
// once, so guessing cannot flood the log.
func (s *Service) AuditUser(userID int64, action, detail string) error {
	var sq int64
	var email string
	if err := s.db.QueryRow(`SELECT squadron_id, email FROM users WHERE id = ?`, userID).Scan(&sq, &email); err != nil {
		return err
	}
	if action == AuditLoginFailed {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE actor_id = ? AND action = ? AND at > ?`,
			userID, action, time.Now().UTC().Add(-failedLoginWindow)).Scan(&n); err == nil && n > 0 {
			return nil
		}
	}
	return s.Audit(sq, userID, email, action, detail)
}

// RecentAudit reports whether a squadron has an entry of this action newer
// than d, for throttling notifications.
func (s *Service) RecentAudit(squadronID int64, action string, d time.Duration) bool {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE squadron_id = ? AND action = ? AND at > ?`,
		squadronID, action, time.Now().UTC().Add(-d)).Scan(&n)
	return n > 0
}

// ListAudit returns a squadron's newest entries first.
func (s *Service) ListAudit(squadronID int64, limit int) ([]AuditEntry, error) {
	return s.queryAudit(`SELECT a.at, '', a.actor_email, a.action, a.detail FROM audit_log a
		WHERE a.squadron_id = ? ORDER BY a.id DESC LIMIT ?`, squadronID, limit)
}

// ListAuditSecurity returns a squadron's newest entries other than ordinary
// sign-ins. Sign-ins are frequent (and wrong passwords can be provoked by
// anyone), so they are kept out of this list where they could push the rarer,
// more important events off the screen. ListSignIns lists them.
func (s *Service) ListAuditSecurity(squadronID int64, limit int) ([]AuditEntry, error) {
	return s.queryAudit(`SELECT a.at, '', a.actor_email, a.action, a.detail FROM audit_log a
		WHERE a.squadron_id = ? AND a.action NOT IN (?, ?) ORDER BY a.id DESC LIMIT ?`, squadronID, AuditLogin, AuditLoginFailed, limit)
}

// ListSignIns returns a squadron's newest sign-ins and wrong passwords.
func (s *Service) ListSignIns(squadronID int64, limit int) ([]AuditEntry, error) {
	return s.queryAudit(`SELECT a.at, '', a.actor_email, a.action, a.detail FROM audit_log a
		WHERE a.squadron_id = ? AND a.action IN (?, ?) ORDER BY a.id DESC LIMIT ?`, squadronID, AuditLogin, AuditLoginFailed, limit)
}

// ListAuditAll returns the newest entries of one action across every squadron,
// for the master admin.
func (s *Service) ListAuditAll(action string, limit int) ([]AuditEntry, error) {
	return s.queryAudit(`SELECT a.at, COALESCE(sq.number, ''), a.actor_email, a.action, a.detail FROM audit_log a
		LEFT JOIN squadrons sq ON sq.id = a.squadron_id
		WHERE a.action = ? ORDER BY a.id DESC LIMIT ?`, action, limit)
}

func (s *Service) queryAudit(q string, args ...any) ([]AuditEntry, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.At, &e.SquadronNum, &e.ActorEmail, &e.Action, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AuditText describes an entry in words for the page. The action is one of the
// constants above; anything else is shown as a plain "changed something".
func AuditText(action string) string {
	switch action {
	case AuditMasterView:
		return "Site administrator viewed this squadron (read-only)"
	case AuditMemberRole:
		return "Changed a member's role"
	case AuditMemberRemove:
		return "Removed a member"
	case AuditInviteSent:
		return "Sent an invitation"
	case AuditSettingsMail:
		return "Changed the site's email settings"
	case AuditLogin:
		return "Signed in"
	case AuditLoginFailed:
		return "Wrong password entered"
	case AuditPasswordChange:
		return "Changed their password"
	case AuditPasswordReset:
		return "Reset their password from the emailed link"
	case AuditSignOutOthers:
		return "Signed out their other sessions"
	case AuditSquadronEdit:
		return "Saved the squadron settings"
	case AuditMemberInactive:
		return "Marked a member inactive"
	case AuditMemberActive:
		return "Made a member active again"
	case AuditTempPassword:
		return "Sent a member a temporary password"
	case AuditEmailChange:
		return "Changed their email address"
	}
	return "Changed something"
}
