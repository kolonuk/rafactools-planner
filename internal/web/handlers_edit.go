package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"rafactools-planner/internal/programme"
)

// Editing entries in place: closures, weekend notes and the requirement
// tracker's categories and entries. Each handler checks the form token, that
// the month belongs to the signed-in person's squadron, and then asks the store
// to change the entry (which checks that the entry belongs to the month).

// editEntry runs one in-place edit and sends the person back to the month.
func (s *server) editEntry(w http.ResponseWriter, r *http.Request, change func(progID int64) error) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	progID, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid programme.")
		return
	}
	if _, err := s.Store.GetFullProgramme(u.SquadronID, progID); err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	switch err := change(progID); {
	case err == nil:
	case errors.Is(err, programme.ErrInvalidInput):
		s.renderError(w, r, http.StatusBadRequest, "Error", errorText(err))
		return
	case errors.Is(err, programme.ErrNotFound):
		s.renderError(w, r, http.StatusNotFound, "Not found", "That entry doesn't exist any more.")
		return
	default:
		log.Printf("edit entry in programme %d: %v", progID, err)
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not save that change. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(progID, 10)), http.StatusSeeOther)
}

func (s *server) closureEdit(w http.ResponseWriter, r *http.Request) {
	closureID, _ := parseID(r, "closureID")
	s.editEntry(w, r, func(progID int64) error {
		return s.Store.UpdateClosure(progID, closureID, r.FormValue("start_date"), r.FormValue("end_date"), r.FormValue("message"))
	})
}

func (s *server) weekendNoteEdit(w http.ResponseWriter, r *http.Request) {
	noteID, _ := parseID(r, "noteID")
	s.editEntry(w, r, func(progID int64) error {
		return s.Store.UpdateWeekendNote(progID, noteID, r.FormValue("date"), r.FormValue("text"))
	})
}

func (s *server) reqCategoryEdit(w http.ResponseWriter, r *http.Request) {
	catID, _ := parseID(r, "catID")
	s.editEntry(w, r, func(progID int64) error {
		return s.Store.UpdateRequirementCategory(progID, catID, r.FormValue("name"), r.FormValue("due_label"))
	})
}

func (s *server) reqEntryEdit(w http.ResponseWriter, r *http.Request) {
	catID, _ := parseID(r, "catID")
	entryID, _ := parseID(r, "entryID")
	s.editEntry(w, r, func(progID int64) error {
		return s.Store.UpdateRequirementEntry(progID, catID, entryID, r.FormValue("cadet_name"), r.FormValue("note"))
	})
}
