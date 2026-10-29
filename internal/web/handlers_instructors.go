package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/programme"
)

// instructorsPage lists the squadron's instructors and lets editors and
// owners add or remove them. They feed the Instructor suggestions in the
// programme grid.
func (s *server) instructorsPage(w http.ResponseWriter, r *http.Request) {
	s.renderInstructors(w, r, http.StatusOK, "")
}

func (s *server) renderInstructors(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	u := userFrom(r.Context())
	list, err := s.Store.ListInstructors(u.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Instructors", "Could not load the list.")
		return
	}
	type row struct {
		ID      int64
		Display string
	}
	var rows []row
	for _, i := range list {
		rows = append(rows, row{i.ID, i.Display()})
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	s.render(w, r, "instructors", "Instructors", map[string]any{"Instructors": rows, "Error": errMsg})
}

func (s *server) instructorAdd(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Instructors", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	err := s.Store.AddInstructor(u.SquadronID, r.FormValue("name"), r.FormValue("rank"))
	switch {
	case errors.Is(err, programme.ErrInvalidInput):
		s.renderInstructors(w, r, http.StatusBadRequest, errorText(err))
		return
	case err != nil:
		s.renderError(w, r, http.StatusInternalServerError, "Instructors", "Could not add that person. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/instructors"), http.StatusSeeOther)
}

func (s *server) instructorDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Instructors", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid entry.")
		return
	}
	if err := s.Store.DeleteInstructor(u.SquadronID, id); errors.Is(err, programme.ErrNotFound) {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That person isn't in your list.")
		return
	} else if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Instructors", "Could not remove that person. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/instructors"), http.StatusSeeOther)
}

// errorText is the part of a wrapped validation error that is meant for the
// user ("name is longer than 80 characters"), without the sentinel prefix.
func errorText(err error) string {
	msg := err.Error()
	const prefix = "invalid input: "
	if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
		return msg[len(prefix):]
	}
	return msg
}

// accountPage lets anyone signed in set the name shown for them in member
// lists and in the grid's Instructor suggestions.
func (s *server) accountPage(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	extra := s.accountExtra(r, u)
	if u.IsMasterAdmin {
		squadrons, err := s.Auth.ListSwitchableSquadrons()
		if err != nil {
			log.Printf("list squadrons for the master admin: %v", err)
		}
		type option struct {
			ID       int64
			Label    string
			Selected bool
		}
		var opts []option
		for _, q := range squadrons {
			opts = append(opts, option{q.ID, q.Number + " – " + q.Name, u.ViewingOther && q.ID == u.SquadronID})
		}
		extra["Squadrons"] = opts
		views, err := s.Auth.ListAuditAll(auth.AuditMasterView, 50)
		if err != nil {
			log.Printf("list master views: %v", err)
		}
		extra["MasterViews"] = views
	}
	if r.URL.Query().Get("saved") == "1" {
		extra["Flash"] = "Saved."
	}
	if r.URL.Query().Get("changed") == "1" {
		extra["Flash"] = "Password changed. Your other sessions were signed out."
	}
	switch {
	case r.URL.Query().Get("emailsent") == "1":
		extra["Flash"] = "We've sent a confirmation link to your new address. Your address changes when you open it."
	case r.URL.Query().Get("emailchanged") == "1":
		extra["Flash"] = "Your email address has been changed."
	}
	switch r.URL.Query().Get("twostep") {
	case "on":
		extra["Flash"] = "Two-step sign-in is on. You will need a code each time you log in."
	case "off":
		extra["Flash"] = "Two-step sign-in is off."
	}
	if r.URL.Query().Get("signedout") == "1" {
		extra["Flash"] = "Signed out."
	}
	s.render(w, r, "account", "Your details", extra)
}

func (s *server) accountSave(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Your details", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	if err := s.Auth.SetDisplayName(u.ID, r.FormValue("display_name")); err != nil {
		if errors.Is(err, auth.ErrDisplayName) {
			w.WriteHeader(http.StatusBadRequest)
			extra := s.accountExtra(r, u)
			extra["DisplayName"], extra["Error"] = r.FormValue("display_name"), err.Error()
			s.render(w, r, "account", "Your details", extra)
			return
		}
		s.renderError(w, r, http.StatusInternalServerError, "Your details", "Could not save. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/account?saved=1"), http.StatusSeeOther)
}

// accountPassword changes the signed-in user's password. It shares the auth
// rate limit, so a stolen session cannot be used to guess the current password.
func (s *server) accountPassword(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Your details", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	err := s.Auth.ChangePassword(u.ID, s.sessionToken(r), r.FormValue("current_password"), r.FormValue("new_password"))
	switch {
	case err == nil:
		http.Redirect(w, r, s.url("/account?changed=1"), http.StatusSeeOther)
		return
	case errors.Is(err, auth.ErrWrongPassword), errors.Is(err, auth.ErrPasswordShort), errors.Is(err, auth.ErrPasswordLong):
		w.WriteHeader(http.StatusBadRequest)
		if u.MustChangePassword {
			s.render(w, r, "force_password", "Choose your password", map[string]any{"Error": err.Error()})
			return
		}
		extra := s.accountExtra(r, u)
		extra["PasswordError"] = err.Error()
		s.render(w, r, "account", "Your details", extra)
	default:
		log.Printf("change password: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Your details", "Could not change the password. Try again.")
	}
}

// accountSwitch lets the master admin look at another squadron (read-only) or
// go back to their own. Anyone else is refused, even if they post the form.
func (s *server) accountSwitch(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Your details", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	if !u.IsMasterAdmin {
		s.renderError(w, r, http.StatusForbidden, "Forbidden", "Only the master admin can switch squadrons.")
		return
	}
	var target int64 // 0 = back to the master admin's own squadron
	if r.FormValue("back") != "1" {
		id, err := strconv.ParseInt(r.FormValue("squadron_id"), 10, 64)
		if err != nil {
			s.renderError(w, r, http.StatusBadRequest, "Your details", "Choose a squadron.")
			return
		}
		target = id
	}
	switch err := s.Auth.SwitchSquadron(s.sessionToken(r), u.ID, target); {
	case errors.Is(err, auth.ErrNoSuchSquadron):
		s.renderError(w, r, http.StatusNotFound, "Not found", "That squadron doesn't exist.")
		return
	case err != nil:
		log.Printf("switch squadron: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Your details", "Could not switch. Try again.")
		return
	}
	// An audit trail: looking at another squadron's data is always logged.
	log.Printf("AUDIT master admin %s switched to squadron id %d (0 = own)", u.Email, target)
	if target != 0 {
		recent := s.Auth.RecentAudit(target, auth.AuditMasterView, time.Hour)
		s.audit(r, target, auth.AuditMasterView, "")
		if !recent {
			s.noticeOwners(target)
		}
	}
	if target == 0 {
		http.Redirect(w, r, s.url("/account"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, s.url("/"), http.StatusSeeOther)
}

// accountExtra is what every render of the account page needs: the user's name
// and the places they are signed in.
func (s *server) accountExtra(r *http.Request, u *auth.User) map[string]any {
	extra := map[string]any{"DisplayName": u.DisplayName}
	sessions, err := s.Auth.ListSessions(u.ID, s.sessionToken(r))
	if err != nil {
		log.Printf("list sessions for user %d: %v", u.ID, err)
	}
	extra["Sessions"] = sessions
	if !u.IsMasterAdmin {
		if s.Auth.CanChangeEmail(u.ID) {
			extra["EmailChangeAllowed"] = true
		} else {
			extra["EmailChangeFixed"] = true
		}
	}
	if u.IsMasterAdmin {
		extra["TwoStepOffered"] = true
		extra["TwoStepOn"] = s.Auth.TOTPEnabled(u.ID)
		if secret, uri := s.Auth.PendingTOTP(u.ID); secret != "" {
			extra["TwoStepSecret"], extra["TwoStepURI"] = groupSecret(secret), uri
		}
	}
	extra["OtherSessions"] = len(sessions) > 1
	return extra
}

// accountSessionSignOut ends one of the user's other sessions.
func (s *server) accountSessionSignOut(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Your details", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	if err := s.Auth.RevokeSession(u.ID, r.PathValue("id"), s.sessionToken(r)); err != nil {
		if errors.Is(err, auth.ErrNoSuchSession) {
			s.renderError(w, r, http.StatusNotFound, "Not found", "That session doesn't exist (it may already have ended).")
			return
		}
		log.Printf("sign out session for user %d: %v", u.ID, err)
		s.renderError(w, r, http.StatusInternalServerError, "Your details", "Could not sign that session out. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/account?signedout=1"), http.StatusSeeOther)
}

// accountSignOutOthers ends every session except this browser's.
func (s *server) accountSignOutOthers(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Your details", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	n, err := s.Auth.RevokeOtherSessions(u.ID, s.sessionToken(r))
	if err == nil && n > 0 && !u.ViewingOther {
		s.audit(r, u.SquadronID, auth.AuditSignOutOthers, "")
	}
	if err != nil {
		log.Printf("sign out other sessions for user %d: %v", u.ID, err)
		s.renderError(w, r, http.StatusInternalServerError, "Your details", "Could not sign out your other sessions. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/account?signedout=1"), http.StatusSeeOther)
}

// groupSecret shows a base32 key in groups of four, which is easier to type.
func groupSecret(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Two-step sign-in is offered to the master admin only for now: the account
// that can read every squadron. Everyone else is refused, even if they post.
func (s *server) twoStepGuard(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Your details", "Form expired, try again.")
		return nil, false
	}
	u := userFrom(r.Context())
	if !u.IsMasterAdmin {
		s.renderError(w, r, http.StatusForbidden, "Forbidden", "Two-step sign-in is only available to the master admin for now.")
		return nil, false
	}
	return u, true
}

func (s *server) twoStepError(w http.ResponseWriter, r *http.Request, u *auth.User, msg string) {
	extra := s.accountExtra(r, u)
	extra["TwoStepError"] = msg
	w.WriteHeader(http.StatusBadRequest)
	s.render(w, r, "account", "Your details", extra)
}

func (s *server) twoStepStart(w http.ResponseWriter, r *http.Request) {
	u, ok := s.twoStepGuard(w, r)
	if !ok {
		return
	}
	if _, _, err := s.Auth.BeginTOTP(u.ID); err != nil {
		s.twoStepError(w, r, u, err.Error()+".")
		return
	}
	http.Redirect(w, r, s.url("/account"), http.StatusSeeOther)
}

func (s *server) twoStepConfirm(w http.ResponseWriter, r *http.Request) {
	u, ok := s.twoStepGuard(w, r)
	if !ok {
		return
	}
	if err := s.Auth.ConfirmTOTP(u.ID, r.FormValue("code")); err != nil {
		s.twoStepError(w, r, u, "Two-step sign-in is still off: "+err.Error()+".")
		return
	}
	log.Printf("AUDIT master admin %s turned two-step sign-in on", u.Email)
	http.Redirect(w, r, s.url("/account?twostep=on"), http.StatusSeeOther)
}

func (s *server) twoStepDisable(w http.ResponseWriter, r *http.Request) {
	u, ok := s.twoStepGuard(w, r)
	if !ok {
		return
	}
	if err := s.Auth.DisableTOTP(u.ID, r.FormValue("password"), r.FormValue("code")); err != nil {
		s.twoStepError(w, r, u, "Two-step sign-in is still on: "+err.Error()+".")
		return
	}
	log.Printf("AUDIT master admin %s turned two-step sign-in off", u.Email)
	http.Redirect(w, r, s.url("/account?twostep=off"), http.StatusSeeOther)
}

// instructorEditForm and instructorEditSave rename an entry (name and rank).
func (s *server) instructorEditForm(w http.ResponseWriter, r *http.Request) {
	s.renderInstructorEdit(w, r, http.StatusOK, "", nil)
}

func (s *server) renderInstructorEdit(w http.ResponseWriter, r *http.Request, status int, errMsg string, typed map[string]string) {
	u := userFrom(r.Context())
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid entry.")
		return
	}
	in, err := s.Store.GetInstructor(u.SquadronID, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That person isn't in your list.")
		return
	}
	data := map[string]any{"ID": in.ID, "Name": in.Name, "Rank": in.Rank, "Error": errMsg}
	if typed != nil {
		data["Name"], data["Rank"] = typed["name"], typed["rank"]
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	s.render(w, r, "instructor_edit", "Edit instructor", data)
}

func (s *server) instructorEditSave(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Instructors", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid entry.")
		return
	}
	err := s.Store.UpdateInstructor(u.SquadronID, id, r.FormValue("name"), r.FormValue("rank"))
	switch {
	case errors.Is(err, programme.ErrNotFound):
		s.renderError(w, r, http.StatusNotFound, "Not found", "That person isn't in your list.")
		return
	case errors.Is(err, programme.ErrInvalidInput):
		s.renderInstructorEdit(w, r, http.StatusBadRequest, errorText(err), map[string]string{"name": r.FormValue("name"), "rank": r.FormValue("rank")})
		return
	case err != nil:
		s.renderError(w, r, http.StatusInternalServerError, "Instructors", "Could not save. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/instructors"), http.StatusSeeOther)
}

// newPasswordForm is the only page open to someone who signed in with a
// temporary password: it asks them to choose their own.
func (s *server) newPasswordForm(w http.ResponseWriter, r *http.Request) {
	if !userFrom(r.Context()).MustChangePassword {
		http.Redirect(w, r, s.url("/account"), http.StatusSeeOther)
		return
	}
	s.render(w, r, "force_password", "Choose your password", nil)
}

// emailChangeRequest starts a change of the account's address: the link goes
// to the NEW address and nothing changes until it is opened and confirmed.
func (s *server) emailChangeRequest(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Your details", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	fail := func(status int, msg string) {
		extra := s.accountExtra(r, u)
		extra["EmailError"] = msg
		w.WriteHeader(status)
		s.render(w, r, "account", "Your details", extra)
	}
	token, newEmail, err := s.Auth.RequestEmailChange(u.ID, strings.TrimSpace(r.FormValue("new_email")), r.FormValue("current_password"))
	switch {
	case err == nil:
	case errors.Is(err, auth.ErrEmailChangeNotAllowed), errors.Is(err, auth.ErrNewEmailInvalid), errors.Is(err, auth.ErrNewEmailOnClaimDomain),
		errors.Is(err, auth.ErrWrongPassword), errors.Is(err, auth.ErrEmailTaken):
		fail(http.StatusBadRequest, err.Error()+".")
		return
	default:
		log.Printf("email change request for user %d: %v", u.ID, err)
		s.renderError(w, r, http.StatusInternalServerError, "Your details", "Could not start the change. Try again.")
		return
	}
	link := strings.TrimSuffix(s.Cfg.BaseURL, "/") + "/email/confirm/" + token
	if err := s.Mail.SendEmailChangeVerify(newEmail, link); err != nil {
		log.Printf("email change verification mail failed: %v", err)
		fail(http.StatusBadGateway, "The confirmation email could not be sent, so nothing was changed. Try again later.")
		return
	}
	http.Redirect(w, r, s.url("/account?emailsent=1"), http.StatusSeeOther)
}

// emailConfirmForm shows what the link would do and asks for a click, so
// mail scanners that open links cannot confirm a change on their own.
func (s *server) emailConfirmForm(w http.ResponseWriter, r *http.Request) {
	to, err := s.Auth.EmailChangeTarget(r.PathValue("token"))
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Confirm email address", "That link is invalid or has expired.")
		return
	}
	s.render(w, r, "email_confirm", "Confirm your new email address", map[string]any{"Token": r.PathValue("token"), "NewEmail": to})
}

func (s *server) emailConfirmSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Confirm email address", "Form expired, try again.")
		return
	}
	userID, oldEmail, newEmail, err := s.Auth.ConfirmEmailChange(r.PathValue("token"))
	switch {
	case err == nil:
	case errors.Is(err, auth.ErrEmailChangeInvalid), errors.Is(err, auth.ErrEmailChangeNotAllowed), errors.Is(err, auth.ErrEmailTaken):
		s.renderError(w, r, http.StatusBadRequest, "Confirm email address", "That link is invalid or has expired, or the address can no longer be used.")
		return
	default:
		log.Printf("confirm email change: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Confirm email address", "Something went wrong. Try again.")
		return
	}
	if err := s.Auth.AuditUser(userID, auth.AuditEmailChange, oldEmail+" to "+newEmail); err != nil {
		log.Printf("audit email change: %v", err)
	}
	go func() {
		if err := s.Mail.SendEmailChanged(oldEmail, newEmail); err != nil {
			log.Printf("email-changed notice failed: %v", err)
		}
	}()
	if userFrom(r.Context()) != nil {
		http.Redirect(w, r, s.url("/account?emailchanged=1"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, s.url("/login?emailchanged=1"), http.StatusSeeOther)
}
