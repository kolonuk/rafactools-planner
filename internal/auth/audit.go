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
)

const (
	maxAuditDetail = 200
	auditKeepFor   = 365 * 24 * time.Hour
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
	_, err := s.db.Exec(`DELETE FROM audit_log WHERE at < ?`, time.Now().Add(-auditKeepFor))
	return err
}

// ListAudit returns a squadron's newest entries first.
func (s *Service) ListAudit(squadronID int64, limit int) ([]AuditEntry, error) {
	return s.queryAudit(`SELECT a.at, '', a.actor_email, a.action, a.detail FROM audit_log a
		WHERE a.squadron_id = ? ORDER BY a.id DESC LIMIT ?`, squadronID, limit)
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
	}
	return "Changed something"
}
