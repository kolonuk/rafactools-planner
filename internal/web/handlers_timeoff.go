package web

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/programme"
)

// Time off for instructors and other staff. Everyone in the squadron can see the
// list, and everyone (viewers too) can add, change and delete anyone's entries. It is shown in
// the planning grid as a warning and is never in the PDF, the PNG or the calendar.

func ownName(u *auth.User) string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Email
}

// canChangeAbsence says whether u may edit or delete an entry: anyone in the
// squadron may, except the master admin looking at it read-only.
func canChangeAbsence(u *auth.User, _ programme.Absence) bool {
	return !u.ViewingOther
}

func (s *server) timeOffPage(w http.ResponseWriter, r *http.Request) {
	s.renderTimeOff(w, r, http.StatusOK, "")
}

func (s *server) renderTimeOff(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	u := userFrom(r.Context())
	list, err := s.Store.ListAbsences(u.SquadronID, programme.Now().In(programme.Local).Format("2006-01-02"))
	if err != nil {
		log.Printf("list time off: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Time off", "Could not load the list.")
		return
	}
	type row struct {
		programme.Absence
		CanChange bool
	}
	rows := make([]row, 0, len(list))
	for _, a := range list {
		rows = append(rows, row{a, canChangeAbsence(u, a)})
	}
	var options []string
	for _, o := range s.instructorOptions(u.SquadronID) {
		if o != "All Staff" {
			options = append(options, o)
		}
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	s.render(w, r, "time_off", "Time off", map[string]any{
		"Rows": rows, "Options": options, "OwnName": ownName(u),
		"CanAddForOthers": !u.ViewingOther, "CanAdd": !u.ViewingOther, "Error": errMsg,
	})
}

func (s *server) timeOffAdd(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Time off", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	if u.ViewingOther {
		s.renderError(w, r, http.StatusForbidden, "Forbidden", "You are looking at this squadron read-only.")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = ownName(u)
	}
	var userID int64
	if strings.EqualFold(name, ownName(u)) {
		userID = u.ID
	}
	err := s.Store.AddAbsence(u.SquadronID, name, userID, u.ID, r.FormValue("start_date"), r.FormValue("end_date"), r.FormValue("note"))
	switch {
	case errors.Is(err, programme.ErrInvalidInput):
		s.renderTimeOff(w, r, http.StatusBadRequest, errorText(err))
		return
	case err != nil:
		log.Printf("add time off: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Time off", "Could not save that. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/time-off"), http.StatusSeeOther)
}

// timeOffEntry loads the entry named in the path and checks u may change it.
func (s *server) timeOffEntry(w http.ResponseWriter, r *http.Request) (*auth.User, programme.Absence, bool) {
	u := userFrom(r.Context())
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Time off", "Form expired, try again.")
		return nil, programme.Absence{}, false
	}
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid entry.")
		return nil, programme.Absence{}, false
	}
	a, err := s.Store.GetAbsence(u.SquadronID, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That entry doesn't exist.")
		return nil, programme.Absence{}, false
	}
	if !canChangeAbsence(u, a) {
		s.renderError(w, r, http.StatusForbidden, "Forbidden", "You are looking at this squadron read-only.")
		return nil, programme.Absence{}, false
	}
	return u, a, true
}

func (s *server) timeOffEdit(w http.ResponseWriter, r *http.Request) {
	u, a, ok := s.timeOffEntry(w, r)
	if !ok {
		return
	}
	// The name is not changed here: delete and add again to give it to someone else.
	err := s.Store.UpdateAbsence(u.SquadronID, a.ID, a.Name, r.FormValue("start_date"), r.FormValue("end_date"), r.FormValue("note"))
	switch {
	case errors.Is(err, programme.ErrInvalidInput):
		s.renderTimeOff(w, r, http.StatusBadRequest, errorText(err))
		return
	case err != nil:
		log.Printf("edit time off: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Time off", "Could not save that. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/time-off"), http.StatusSeeOther)
}

func (s *server) timeOffDelete(w http.ResponseWriter, r *http.Request) {
	u, a, ok := s.timeOffEntry(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteAbsence(u.SquadronID, a.ID); err != nil {
		log.Printf("delete time off: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Time off", "Could not delete that. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/time-off"), http.StatusSeeOther)
}

// awayMatcher builds what the grid uses to flag an Instructor box naming someone
// who is away. A person from the list is also matched by name without the rank.
func (s *server) awayMatcher(squadronID int64) *programme.AwayMatcher {
	from := programme.Now().In(programme.Local).AddDate(0, 0, -1).Format("2006-01-02")
	list, err := s.Store.ListAbsences(squadronID, from)
	if err != nil {
		log.Printf("list time off for the grid: %v", err)
		return nil
	}
	if len(list) == 0 {
		return nil
	}
	aliases := map[string][]string{}
	if people, err := s.Store.ListInstructors(squadronID); err == nil {
		for _, p := range people {
			aliases[p.Display()] = append(aliases[p.Display()], p.Name)
		}
	}
	return programme.NewAwayMatcher(list, aliases)
}
