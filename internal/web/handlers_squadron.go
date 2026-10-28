package web

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
	if err := s.Auth.SetMemberRole(u.SquadronID, memberID, r.FormValue("role")); err != nil {
		s.memberError(w, r, err)
		return
	}
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
	if err := s.Auth.RemoveMember(u.SquadronID, memberID); err != nil {
		s.memberError(w, r, err)
		return
	}
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
