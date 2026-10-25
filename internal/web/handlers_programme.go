package web

import (
	"net/http"
	"strconv"
	"time"

	"rafactools-planner/internal/pdfgen"
	"rafactools-planner/internal/programme"
)

var monthNames = []string{"", "January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}

func parseID(r *http.Request, name string) (int64, bool) {
	v := r.PathValue(name)
	id, err := strconv.ParseInt(v, 10, 64)
	return id, err == nil
}

// --- Dashboard -----------------------------------------------------------

func (s *server) dashboard(w http.ResponseWriter, r *http.Request) {
	s.renderDashboard(w, r, "")
}

func (s *server) renderDashboard(w http.ResponseWriter, r *http.Request, errMsg string) {
	u := userFrom(r.Context())
	sq, err := s.Store.GetSquadron(u.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Dashboard", "Could not load squadron.")
		return
	}
	progs, err := s.Store.ListProgrammes(u.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Dashboard", "Could not load programmes.")
		return
	}

	type progView struct {
		ID        int64
		MonthName string
		Year      int
		Status    string
	}
	var views []progView
	for _, p := range progs {
		views = append(views, progView{p.ID, monthNames[p.Month], p.Year, p.Status})
	}

	now := time.Now()
	var monthOptions []map[string]any
	for m := 1; m <= 12; m++ {
		monthOptions = append(monthOptions, map[string]any{"Value": m, "Label": monthNames[m], "Selected": m == int(now.Month())})
	}

	s.render(w, r, "dashboard", "Dashboard", map[string]any{
		"Squadron":     sq,
		"Programmes":   views,
		"CanEdit":      u.CanEdit(),
		"MonthOptions": monthOptions,
		"DefaultYear":  now.Year(),
		"Error":        errMsg,
	})
}

func (s *server) programmeCreate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderDashboard(w, r, "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	year, err1 := strconv.Atoi(r.FormValue("year"))
	month, err2 := strconv.Atoi(r.FormValue("month"))
	if err1 != nil || err2 != nil || month < 1 || month > 12 {
		s.renderDashboard(w, r, "Invalid month/year.")
		return
	}
	p, err := s.Store.CreateProgramme(u.SquadronID, year, month, u.ID)
	if err != nil {
		s.renderDashboard(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(p.ID, 10), http.StatusSeeOther)
}

// --- Programme edit --------------------------------------------------------

type subRowView struct {
	RowType string
	RowID   int64
	Texts   []string
}

type dayView struct {
	ID          int64
	DateLabel   string
	DayLabel    string
	IsParade    bool
	UniformKey  string
	Notes       string
	SubRows     []subRowView
	ShowAddHalf bool
}

func buildDayViews(days []programme.Day, canEdit bool) []dayView {
	var out []dayView
	for _, d := range days {
		t, _ := time.Parse("2006-01-02", d.Date)
		dv := dayView{
			ID: d.ID, DateLabel: strconv.Itoa(t.Day()), DayLabel: d.DayLabel,
			IsParade: d.IsParade, UniformKey: d.UniformKey, Notes: d.Notes,
		}
		if d.IsParade {
			for half := 1; half <= d.Halves; half++ {
				for _, rt := range []struct{ key, label string }{{"activity", "Activity"}, {"instructor", "Instructor"}} {
					row, _ := d.RowByHalfType(half, rt.key)
					dv.SubRows = append(dv.SubRows, subRowView{RowType: rt.label, RowID: row.ID, Texts: row.Texts})
				}
			}
			dv.ShowAddHalf = canEdit && d.Halves == 1
		}
		out = append(out, dv)
	}
	return out
}

func (s *server) loadProgrammeAndSquadron(w http.ResponseWriter, r *http.Request) (*programme.Squadron, *programme.FullProgramme, bool) {
	u := userFrom(r.Context())
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid programme.")
		return nil, nil, false
	}
	sq, err := s.Store.GetSquadron(u.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not load squadron.")
		return nil, nil, false
	}
	fp, err := s.Store.GetFullProgramme(u.SquadronID, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return nil, nil, false
	}
	return sq, fp, true
}

func (s *server) programmeEdit(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	sq, fp, ok := s.loadProgrammeAndSquadron(w, r)
	if !ok {
		return
	}
	canEdit := u.CanEdit()

	var uniformOptions []map[string]any
	for _, uc := range sq.UniformColours {
		uniformOptions = append(uniformOptions, map[string]any{"Key": uc.Key, "Label": uc.Label})
	}

	s.render(w, r, "programme_edit", monthNames[fp.Month]+" "+strconv.Itoa(fp.Year), map[string]any{
		"ProgrammeID":         fp.ID,
		"MonthName":           monthNames[fp.Month],
		"Year":                fp.Year,
		"CanEdit":             canEdit,
		"Classifications":     sq.Classifications,
		"LabelClassColsCount": 1 + len(sq.Classifications),
		"UniformOptions":      uniformOptions,
		"Days":                buildDayViews(fp.Days, canEdit),
		"Closures":            fp.Closures,
		"WeekendNotes":        fp.WeekendNotes,
		"Requirements":        fp.Requirements,
	})
}

func (s *server) programmeSave(w http.ResponseWriter, r *http.Request) {
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
	fp, err := s.Store.GetFullProgramme(u.SquadronID, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Could not read form.")
		return
	}

	var days []programme.DayInput
	for _, d := range fp.Days {
		prefix := "day_" + strconv.FormatInt(d.ID, 10) + "_"
		di := programme.DayInput{
			ID:         d.ID,
			IsParade:   r.PostForm.Get(prefix+"parade") == "1",
			DayLabel:   r.PostForm.Get(prefix + "label"),
			UniformKey: r.PostForm.Get(prefix + "uniform"),
			Notes:      r.PostForm.Get(prefix + "notes"),
		}
		for _, row := range d.Rows {
			rp := "row_" + strconv.FormatInt(row.ID, 10) + "_text_"
			texts := make([]string, len(row.Texts))
			for i := range texts {
				texts[i] = r.PostForm.Get(rp + strconv.Itoa(i))
			}
			di.Rows = append(di.Rows, programme.RowInput{ID: row.ID, Texts: texts})
		}
		days = append(days, di)
	}

	if err := s.Store.SaveDaysAndRows(u.SquadronID, id, days); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not save: "+err.Error())
		return
	}
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *server) dayAddHalf(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	progID, ok1 := parseID(r, "id")
	dayID, ok2 := parseID(r, "dayID")
	if !ok1 || !ok2 {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid request.")
		return
	}
	sq, err := s.Store.GetSquadron(u.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not load squadron.")
		return
	}
	if err := s.Store.AddHalf(u.SquadronID, progID, dayID, len(sq.Classifications)); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not add half.")
		return
	}
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(progID, 10), http.StatusSeeOther)
}

func (s *server) programmePDF(w http.ResponseWriter, r *http.Request) {
	sq, fp, ok := s.loadProgrammeAndSquadron(w, r)
	if !ok {
		return
	}
	data, err := pdfgen.Render(sq, fp)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not render PDF: "+err.Error())
		return
	}
	filename := monthNames[fp.Month] + "_" + strconv.Itoa(fp.Year) + "_Programme.pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+filename+`"`)
	w.Write(data)
}

// --- Closures / weekend notes / requirements -------------------------------

func (s *server) closureAdd(w http.ResponseWriter, r *http.Request) {
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
	if _, err := s.Store.GetFullProgramme(u.SquadronID, id); err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	if err := s.Store.AddClosure(id, r.FormValue("start_date"), r.FormValue("end_date"), r.FormValue("message")); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not add closure.")
		return
	}
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *server) closureDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, _ := parseID(r, "id")
	closureID, _ := parseID(r, "closureID")
	if _, err := s.Store.GetFullProgramme(u.SquadronID, id); err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	_ = s.Store.DeleteClosure(id, closureID)
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *server) weekendNoteAdd(w http.ResponseWriter, r *http.Request) {
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
	if _, err := s.Store.GetFullProgramme(u.SquadronID, id); err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	if err := s.Store.AddWeekendNote(id, r.FormValue("date"), r.FormValue("text")); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not add note.")
		return
	}
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *server) weekendNoteDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, _ := parseID(r, "id")
	noteID, _ := parseID(r, "noteID")
	if _, err := s.Store.GetFullProgramme(u.SquadronID, id); err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	_ = s.Store.DeleteWeekendNote(id, noteID)
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *server) reqCategoryAdd(w http.ResponseWriter, r *http.Request) {
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
	if _, err := s.Store.GetFullProgramme(u.SquadronID, id); err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	if _, err := s.Store.AddRequirementCategory(id, r.FormValue("name"), r.FormValue("due_label")); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not add category.")
		return
	}
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *server) reqCategoryDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, _ := parseID(r, "id")
	catID, _ := parseID(r, "catID")
	if _, err := s.Store.GetFullProgramme(u.SquadronID, id); err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	_ = s.Store.DeleteRequirementCategory(id, catID)
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *server) reqEntryAdd(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, _ := parseID(r, "id")
	catID, _ := parseID(r, "catID")
	if _, err := s.Store.GetFullProgramme(u.SquadronID, id); err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	if err := s.Store.AddRequirementEntry(catID, r.FormValue("cadet_name"), r.FormValue("note")); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not add entry.")
		return
	}
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *server) reqEntryDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, _ := parseID(r, "id")
	catID, _ := parseID(r, "catID")
	entryID, _ := parseID(r, "entryID")
	if _, err := s.Store.GetFullProgramme(u.SquadronID, id); err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	_ = s.Store.DeleteRequirementEntry(catID, entryID)
	http.Redirect(w, r, "/programmes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
