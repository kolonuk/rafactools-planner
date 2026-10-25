package web

import (
	"net/http"
	"strconv"
	"strings"
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

	sq.Name = r.FormValue("name")
	sq.Subtitle = r.FormValue("subtitle")
	sq.ProtectiveMarking = r.FormValue("protective_marking")

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
		if n, err := strconv.Atoi(v); err == nil {
			weekdays = append(weekdays, n)
		}
	}
	sq.ParadeWeekdays = weekdays

	if err := s.Store.UpdateSquadronSettings(sq); err != nil {
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
