package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"rafactools-planner/internal/programme"
)

// Copy an earlier month into this one (uniform, activities and instructors only;
// see programme.CopyMonth for how the nights are lined up).

type copySource struct {
	ID    int64
	Label string
	Plan  programme.CopyPlan
}

func (s *server) copyOptions(squadronID int64, dst *programme.FullProgramme) []copySource {
	list, err := s.Store.ListProgrammes(squadronID)
	if err != nil {
		return nil
	}
	var out []copySource
	for _, p := range list {
		if p.Year*12+p.Month >= dst.Year*12+dst.Month {
			continue // only earlier months
		}
		src, err := s.Store.GetFullProgramme(squadronID, p.ID)
		if err != nil {
			continue
		}
		out = append(out, copySource{ID: p.ID, Label: monthNames[p.Month] + " " + strconv.Itoa(p.Year), Plan: programme.PlanCopy(dst, src)})
	}
	return out
}

func (s *server) renderCopy(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
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
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	s.render(w, r, "copy_month", "Copy a month", map[string]any{
		"ProgrammeID": id, "MonthName": monthNames[fp.Month], "Year": fp.Year,
		"Sources": s.copyOptions(u.SquadronID, fp), "HasEntries": programme.CopyableNights(fp) > 0,
		"Selected": r.FormValue("source"), "Error": errMsg,
	})
}

func (s *server) copyForm(w http.ResponseWriter, r *http.Request) {
	s.renderCopy(w, r, http.StatusOK, "")
}

// copySubmit copies the chosen month in. If this month already has entries, the
// person must have ticked the box that says they will be replaced.
func (s *server) copySubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid programme.")
		return
	}
	dst, err := s.Store.GetFullProgramme(u.SquadronID, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	srcID, err := strconv.ParseInt(r.FormValue("source"), 10, 64)
	if err != nil {
		s.renderCopy(w, r, http.StatusBadRequest, "Choose a month to copy from.")
		return
	}
	src, err := s.Store.GetFullProgramme(u.SquadronID, srcID)
	if err != nil || src.Year*12+src.Month >= dst.Year*12+dst.Month {
		s.renderCopy(w, r, http.StatusBadRequest, "Choose one of the earlier months in the list.")
		return
	}
	if plan := programme.PlanCopy(dst, src); plan.Replaced > 0 && r.FormValue("confirm") != "1" {
		s.renderCopy(w, r, http.StatusBadRequest, fmt.Sprintf("%d of this month's nights already have entries. Tick the box to replace them.", plan.Replaced))
		return
	}
	_, err = s.Store.CopyMonth(u.SquadronID, id, srcID)
	switch {
	case errors.Is(err, programme.ErrInvalidInput):
		s.renderCopy(w, r, http.StatusBadRequest, errorText(err))
		return
	case errors.Is(err, programme.ErrNotFound):
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	case err != nil:
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not copy the month. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
}
