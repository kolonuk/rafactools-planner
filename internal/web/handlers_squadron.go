package web

import (
	"encoding/csv"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/programme"
)

var weekdayNames = []struct {
	Value int
	Label string
}{
	{1, "Mon"}, {2, "Tue"}, {3, "Wed"}, {4, "Thu"}, {5, "Fri"}, {6, "Sat"}, {7, "Sun"},
}

func (s *server) squadronSettings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	sq, err := s.Store.GetSquadron(u.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Squadron settings", "Could not load squadron.")
		return
	}
	members, err := s.Auth.ListMembers(u.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Squadron settings", "Could not load members.")
		return
	}
	invites, err := s.Auth.PendingInvites(u.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Squadron settings", "Could not load invites.")
		return
	}

	auditEntries, err := s.Auth.ListAuditSecurity(u.SquadronID, 100)
	if err != nil {
		log.Printf("list audit for squadron %d: %v", u.SquadronID, err)
	}
	signIns, err := s.Auth.ListSignIns(u.SquadronID, 30)
	if err != nil {
		log.Printf("list sign-ins for squadron %d: %v", u.SquadronID, err)
	}

	paradeSet := map[int]bool{}
	for _, d := range sq.ParadeWeekdays {
		paradeSet[d] = true
	}
	var weekdayOptions []map[string]any
	for _, wd := range weekdayNames {
		weekdayOptions = append(weekdayOptions, map[string]any{"Value": wd.Value, "Label": wd.Label, "Checked": paradeSet[wd.Value]})
	}

	type inviteRow struct {
		Token, Email, Role, ExpiresAt string
	}
	var inviteRows []inviteRow
	for _, inv := range invites {
		inviteRows = append(inviteRows, inviteRow{inv.Token, inv.Email, inv.Role, inv.ExpiresAt.Format("2006-01-02")})
	}

	s.render(w, r, "squadron_settings", "Squadron settings", map[string]any{
		"Squadron":            sq,
		"Members":             members,
		"Invites":             inviteRows,
		"Audit":               auditEntries,
		"SignIns":             signIns,
		"WeekdayOptions":      weekdayOptions,
		"ClassificationsText": strings.Join(sq.Classifications, "\n"),
	})
}

func (s *server) squadronSave(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	sq, err := s.Store.GetSquadron(u.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Squadron settings", "Could not load squadron.")
		return
	}

	// A blank name would leave the PDF and every page without a title; keep
	// the existing one instead.
	if name := strings.TrimSpace(r.FormValue("name")); name != "" {
		sq.Name = name
	}
	sq.Subtitle = r.FormValue("subtitle")
	sq.ProtectiveMarking = r.FormValue("protective_marking")
	// The address goes into a clickable link on the dashboard and in the PDF,
	// so only a plain http(s) address is accepted (never javascript:, data:, file:).
	website := strings.TrimSpace(r.FormValue("website_url"))
	if website != "" && !validWebsiteURL(website) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "The website must be a full address starting with http:// or https://.")
		return
	}
	sq.WebsiteURL = website

	var classifications []string
	for _, line := range strings.Split(r.FormValue("classifications"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			classifications = append(classifications, line)
		}
	}
	if len(classifications) > 0 {
		sq.Classifications = classifications
	}

	r.ParseForm()
	var weekdays []int
	for _, v := range r.PostForm["parade_weekdays"] {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 7 {
			weekdays = append(weekdays, n)
		}
	}
	sq.ParadeWeekdays = weekdays

	if err := s.Store.UpdateSquadronSettings(sq); err != nil {
		if errors.Is(err, programme.ErrInvalidInput) {
			// e.g. removing a classification that still has text in a programme
			s.renderError(w, r, http.StatusBadRequest, "Squadron settings", errorText(err))
			return
		}
		log.Printf("save squadron settings: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Squadron settings", "Could not save.")
		return
	}
	s.audit(r, u.SquadronID, auth.AuditSquadronEdit, "")
	http.Redirect(w, r, s.url("/squadron"), http.StatusSeeOther)
}

func (s *server) inviteCreate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	email := r.FormValue("email")
	role := r.FormValue("role")

	token, err := s.Auth.CreateInvite(u.SquadronID, u.ID, email, role)
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", err.Error())
		return
	}
	sq, _ := s.Store.GetSquadron(u.SquadronID)
	squadronName := "your squadron"
	if sq != nil {
		squadronName = sq.Name
	}
	s.audit(r, u.SquadronID, auth.AuditInviteSent, strings.TrimSpace(email)+" as "+role)
	if err := s.Mail.SendInvite(email, s.Cfg.BaseURL, token, squadronName, u.Email); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Squadron settings", "Invite created, but the email failed to send.")
		return
	}
	http.Redirect(w, r, s.url("/squadron"), http.StatusSeeOther)
}

func (s *server) inviteRevoke(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	_ = s.Auth.RevokeInvite(u.SquadronID, r.PathValue("token"))
	http.Redirect(w, r, s.url("/squadron"), http.StatusSeeOther)
}

func (s *server) memberSetRole(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	memberID, ok := parseID(r, "memberID")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid member.")
		return
	}
	target := s.memberEmail(u.SquadronID, memberID)
	if err := s.Auth.SetMemberRole(u.SquadronID, memberID, r.FormValue("role")); err != nil {
		s.memberError(w, r, err)
		return
	}
	s.audit(r, u.SquadronID, auth.AuditMemberRole, target+" to "+r.FormValue("role"))
	http.Redirect(w, r, s.url("/squadron"), http.StatusSeeOther)
}

func (s *server) memberRemove(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	memberID, ok := parseID(r, "memberID")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid member.")
		return
	}
	target := s.memberEmail(u.SquadronID, memberID)
	if err := s.Auth.RemoveMember(u.SquadronID, memberID); err != nil {
		s.memberError(w, r, err)
		return
	}
	s.audit(r, u.SquadronID, auth.AuditMemberRemove, target)
	http.Redirect(w, r, s.url("/squadron"), http.StatusSeeOther)
}

// memberError shows the reason for a refused member change when it is one of
// the known, user-meaningful ones, and a generic message (never the raw
// database error) for anything else.
func (s *server) memberError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrLastOwner), errors.Is(err, auth.ErrInvalidRole):
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", err.Error())
	case errors.Is(err, auth.ErrNotMember):
		s.renderError(w, r, http.StatusNotFound, "Squadron settings", err.Error())
	default:
		log.Printf("member change: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Squadron settings", "Could not change that member. Try again.")
	}
}

// validWebsiteURL accepts only absolute http(s) URLs of sensible length with
// a host and no spaces or control characters.
func validWebsiteURL(raw string) bool {
	if len(raw) > 200 {
		return false
	}
	for _, r := range raw {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// audit records a security-relevant event for a squadron's trail. A failure is
// logged and otherwise ignored: the action being recorded has already happened
// and must not be reported as failed because its log line could not be written.
func (s *server) audit(r *http.Request, squadronID int64, action, detail string) {
	u := userFrom(r.Context())
	if err := s.Auth.Audit(squadronID, u.ID, u.Email, action, detail); err != nil {
		log.Printf("audit %s for squadron %d: %v", action, squadronID, err)
	}
}

// memberEmail is a member's address for the audit trail, or "a member" if the
// id is not in the squadron.
func (s *server) memberEmail(squadronID, memberID int64) string {
	members, err := s.Auth.ListMembers(squadronID)
	if err == nil {
		for _, m := range members {
			if m.ID == memberID {
				return m.Email
			}
		}
	}
	return "a member"
}

// squadronDelete permanently deletes the owner's whole squadron. It is a
// separate, deliberately awkward form: the owner must retype the squadron
// number and their password. It shares the sign-in rate limit.
func (s *server) squadronDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	err := s.Auth.DeleteSquadron(u.SquadronID, u.ID, r.FormValue("password"), strings.TrimSpace(r.FormValue("confirm_number")))
	switch {
	case err == nil:
	case errors.Is(err, auth.ErrWrongPassword), errors.Is(err, auth.ErrConfirmMismatch):
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Nothing was deleted: "+err.Error()+".")
		return
	case errors.Is(err, auth.ErrCannotDeleteHome), errors.Is(err, auth.ErrNoSuchSquadron):
		s.renderError(w, r, http.StatusForbidden, "Squadron settings", "That squadron cannot be deleted.")
		return
	default:
		log.Printf("delete squadron %d: %v", u.SquadronID, err)
		s.renderError(w, r, http.StatusInternalServerError, "Squadron settings", "Could not delete the squadron. Nothing was changed.")
		return
	}
	log.Printf("AUDIT squadron %d deleted by %s", u.SquadronID, u.Email)
	s.setSessionCookie(w, "", time.Time{})
	http.Redirect(w, r, s.url("/login?deleted=1"), http.StatusSeeOther)
}

// noticeOwners emails each owner of a squadron that the site administrator
// opened it. The caller sends at most one per hour per squadron. It runs in the
// background and only logs failures: looking at a squadron must not wait on, or
// fail because of, mail.
func (s *server) noticeOwners(squadronID int64) {
	sq, err := s.Store.GetSquadron(squadronID)
	if err != nil {
		return
	}
	members, err := s.Auth.ListMembers(squadronID)
	if err != nil {
		return
	}
	when := time.Now().UTC().Format("2 Jan 2006 15:04")
	for _, m := range members {
		if m.Role != "owner" {
			continue
		}
		to := m.Email
		go func() {
			if err := s.Mail.SendMasterViewNotice(to, sq.Name, when); err != nil {
				log.Printf("notify owner of squadron %d: %v", squadronID, err)
			}
		}()
	}
}

// securityLogCSV lets an owner download the squadron's whole security log
// (up to a year). Cells that a spreadsheet would run as a formula are made
// harmless first.
func (s *server) securityLogCSV(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	entries, err := s.Auth.ListAudit(u.SquadronID, 100000)
	if err != nil {
		log.Printf("export audit for squadron %d: %v", u.SquadronID, err)
		s.renderError(w, r, http.StatusInternalServerError, "Security log", "Could not read the log.")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="security-log.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"when_utc", "who", "action", "detail"})
	for _, e := range entries {
		_ = cw.Write([]string{e.At.UTC().Format("2006-01-02 15:04:05"), csvSafe(e.ActorEmail), csvSafe(auth.AuditText(e.Action)), csvSafe(e.Detail)})
	}
	cw.Flush()
}

// csvSafe stops a spreadsheet treating text as a formula (CSV injection).
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
