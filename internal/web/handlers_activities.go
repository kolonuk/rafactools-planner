package web

import (
	"errors"
	"log"
	"net/http"

	"rafactools-planner/internal/programme"
)

// The activity list: the built-in activities plus the squadron's own, offered as
// suggestions in the grid. Editors add, rename and remove the squadron's own.

func (s *server) activitiesPage(w http.ResponseWriter, r *http.Request) {
	s.renderActivities(w, r, http.StatusOK, "")
}

func (s *server) renderActivities(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	u := userFrom(r.Context())
	custom, err := s.Store.ListCustomActivities(u.SquadronID)
	if err != nil {
		log.Printf("list activities: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Activities", "Could not load the list.")
		return
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	suggestions, err := s.Store.ListSuggestions(u.SquadronID)
	if err != nil {
		log.Printf("list suggestions: %v", err)
	}
	set, _ := s.Store.GetSuggestSettings(u.SquadronID)
	s.render(w, r, "activities", "Activities", map[string]any{
		"Defaults": programme.DefaultActivities(), "Custom": custom, "Error": errMsg,
		"Suggestions": suggestions, "SuggestOn": set.HasToken,
	})
}

func (s *server) activityResult(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case err == nil:
		http.Redirect(w, r, s.url("/activities"), http.StatusSeeOther)
	case errors.Is(err, programme.ErrInvalidInput):
		s.renderActivities(w, r, http.StatusBadRequest, errorText(err))
	case errors.Is(err, programme.ErrNotFound):
		s.renderError(w, r, http.StatusNotFound, "Not found", "That activity isn't in your list.")
	default:
		log.Printf("activity change: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Activities", "Could not save that. Try again.")
	}
}

func (s *server) activityAdd(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Activities", "Form expired, try again.")
		return
	}
	s.activityResult(w, r, s.Store.AddActivity(userFrom(r.Context()).SquadronID, r.FormValue("title")))
}

func (s *server) activityEdit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Activities", "Form expired, try again.")
		return
	}
	id, _ := parseID(r, "id")
	s.activityResult(w, r, s.Store.UpdateActivity(userFrom(r.Context()).SquadronID, id, r.FormValue("title")))
}

func (s *server) activityDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Activities", "Form expired, try again.")
		return
	}
	id, _ := parseID(r, "id")
	s.activityResult(w, r, s.Store.DeleteActivity(userFrom(r.Context()).SquadronID, id))
}
