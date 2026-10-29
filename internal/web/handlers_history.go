package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"rafactools-planner/internal/programme"
)

// recordVersion keeps a copy of a programme after a change, for its history.
// A failure is logged and otherwise ignored: the change itself has been made
// and must not be reported as failed because its history line could not be
// written.
func (s *server) recordVersion(squadronID, programmeID, userID int64, email, action string) {
	if _, _, err := s.Store.RecordVersion(squadronID, programmeID, userID, email, action); err != nil && !errors.Is(err, programme.ErrNotFound) {
		log.Printf("record version of programme %d failed: %v", programmeID, err)
	}
}

// versioned wraps a handler that changes a programme: if the programme has no
// history yet its starting point is kept first, and after the handler succeeds
// (any status below 400) the result is kept with who did it and action.
func (s *server) versioned(action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		id, ok := parseID(r, "id")
		if ok && u != nil && !s.Store.HasVersions(id) {
			s.recordVersion(u.SquadronID, id, u.ID, u.Email, "Starting point (before the first recorded change)")
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		if ok && u != nil && rec.status < 400 {
			s.recordVersion(u.SquadronID, id, u.ID, u.Email, action)
		}
	}
}

// historyPage lists a programme's kept versions: number, date and time, who,
// and what they did, with a way back to any of them for people who can edit.
func (s *server) historyPage(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid programme.")
		return
	}
	fp, err := s.Store.GetFullProgramme(u.SquadronID, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	versions, err := s.Store.ListVersions(u.SquadronID, id)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "History", "Could not load the history.")
		return
	}
	extra := map[string]any{
		"ProgrammeID": id, "MonthName": monthNames[fp.Month], "Year": fp.Year,
		"Versions": versions, "CanEdit": u.CanEdit(),
	}
	if r.URL.Query().Get("restored") != "" {
		extra["Flash"] = "Restored version " + r.URL.Query().Get("restored") + ". The programme as it was before is still in the history."
	}
	s.render(w, r, "history", monthNames[fp.Month]+" "+strconv.Itoa(fp.Year)+" history", extra)
}

func (s *server) restoreConfirm(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	id, ok1 := parseID(r, "id")
	ver, ok2 := parseID(r, "version")
	if !ok1 || !ok2 {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid request.")
		return
	}
	fp, err := s.Store.GetFullProgramme(u.SquadronID, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	info, err := s.Store.GetVersionInfo(u.SquadronID, id, int(ver))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That version doesn't exist.")
		return
	}
	s.render(w, r, "confirm_restore", "Restore this version?", map[string]any{
		"ProgrammeID": id, "MonthName": monthNames[fp.Month], "Year": fp.Year, "Version": info,
	})
}

func (s *server) restoreSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, ok1 := parseID(r, "id")
	ver, ok2 := parseID(r, "version")
	if !ok1 || !ok2 {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid request.")
		return
	}
	// Keep what is there now first, so the restore can itself be undone.
	if !s.Store.HasVersions(id) {
		s.recordVersion(u.SquadronID, id, u.ID, u.Email, "Starting point (before the first recorded change)")
	} else {
		s.recordVersion(u.SquadronID, id, u.ID, u.Email, "Before restoring version "+strconv.FormatInt(ver, 10))
	}
	if err := s.Store.RestoreVersion(u.SquadronID, id, int(ver)); err != nil {
		if errors.Is(err, programme.ErrNotFound) || errors.Is(err, programme.ErrVersionNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Not found", "That version doesn't exist.")
			return
		}
		log.Printf("restore programme %d to version %d failed: %v", id, ver, err)
		s.renderError(w, r, http.StatusInternalServerError, "History", "Could not restore that version. Nothing was changed.")
		return
	}
	s.recordVersion(u.SquadronID, id, u.ID, u.Email, "Restored version "+strconv.FormatInt(ver, 10))
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)+"/history?restored="+strconv.FormatInt(ver, 10)), http.StatusSeeOther)
}
