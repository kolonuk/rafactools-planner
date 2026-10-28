package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"

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
	extra := map[string]any{"DisplayName": u.DisplayName}
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
	}
	if r.URL.Query().Get("saved") == "1" {
		extra["Flash"] = "Saved."
	}
	if r.URL.Query().Get("changed") == "1" {
		extra["Flash"] = "Password changed. Your other sessions were signed out."
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
			s.render(w, r, "account", "Your details", map[string]any{"DisplayName": r.FormValue("display_name"), "Error": err.Error()})
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
		s.render(w, r, "account", "Your details", map[string]any{"DisplayName": u.DisplayName, "PasswordError": err.Error()})
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
	if target == 0 {
		http.Redirect(w, r, s.url("/account"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, s.url("/"), http.StatusSeeOther)
}
