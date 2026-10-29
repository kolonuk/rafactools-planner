package web

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"rafactools-planner/internal/auth"
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
	s.ensureThisMonth(userFrom(r.Context()))
	s.renderDashboard(w, r, "")
}

// ensureThisMonth makes the current month's programme for the person's
// squadron if it does not exist yet. It runs when anyone signs in (and when the
// dashboard opens, for sessions that last across a month end). The master admin
// has no squadron of their own to plan, and only looks at others read-only.
func (s *server) ensureThisMonth(u *auth.User) {
	if u == nil || u.IsMasterAdmin {
		return
	}
	if made, err := s.Store.EnsureCurrentProgramme(u.SquadronID, u.ID); err != nil {
		log.Printf("make this month's programme for squadron %d failed: %v", u.SquadronID, err)
	} else if made {
		log.Printf("made this month's programme for squadron %d", u.SquadronID)
		year, month := programme.CurrentMonth()
		if p, err := s.Store.FindProgramme(u.SquadronID, year, month); err == nil {
			s.recordVersion(u.SquadronID, p, u.ID, u.Email, "Created the month automatically when someone signed in")
		}
	}
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
	}
	var views []progView
	for _, p := range progs {
		views = append(views, progView{p.ID, monthNames[p.Month], p.Year})
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
		if errors.Is(err, programme.ErrInvalidInput) {
			err = errors.New(errorText(err))
		}
		s.renderDashboard(w, r, err.Error())
		return
	}
	if r.FormValue("copy_categories") == "1" {
		if _, err := s.Store.CopyRequirementCategories(u.SquadronID, p.ID); err != nil {
			log.Printf("copy requirement categories into programme %d: %v", p.ID, err)
		}
	}
	s.recordVersion(u.SquadronID, p.ID, u.ID, u.Email, "Created the month")
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(p.ID, 10)), http.StatusSeeOther)
}

// --- Programme edit --------------------------------------------------------

type subRowView struct {
	RowType string
	RowID   int64
	Texts   []string
}

type dayView struct {
	ID             int64
	Date           string // ISO yyyy-mm-dd
	FullLabel      string // e.g. "Mon 7", for the move picker and drag confirmation
	DateLabel      string
	DayLabel       string
	Closed         bool   // inside a closure: shown locked, nothing to edit
	ClosedMessage  string // the closure's message
	IsParade       bool
	UniformKey     string
	Notes          string
	SubRows        []subRowView
	ShowAddHalf    bool
	ShowRemoveHalf bool

	// For a night that is not a parade night yet: the boxes that appear when
	// its Parade tickbox is ticked, so the row looks as it will after saving.
	// They are named for rows the night already has (kept from an earlier
	// time it was a parade night) or, if it has none, as new ones.
	PendingActivity   []pendingCell
	PendingInstructor []pendingCell
}

type pendingCell struct{ Name, Value string }

func buildDayViews(days []programme.Day, canEdit bool, classes int, closures []programme.Closure) []dayView {
	var out []dayView
	for _, d := range days {
		t, _ := time.Parse("2006-01-02", d.Date)
		dv := dayView{
			ID: d.ID, Date: d.Date, FullLabel: t.Format("Mon 2"), DateLabel: strconv.Itoa(t.Day()), DayLabel: d.DayLabel,
			IsParade: d.IsParade, UniformKey: d.UniformKey, Notes: d.Notes,
		}
		if c, closed := programme.ClosureOn(d.Date, closures); closed {
			dv.Closed, dv.ClosedMessage = true, c.Message
			out = append(out, dv)
			continue
		}
		if d.IsParade {
			for half := 1; half <= d.Halves; half++ {
				for _, rt := range []struct{ key, label string }{{"activity", "Activity"}, {"instructor", "Instructor"}} {
					row, _ := d.RowByHalfType(half, rt.key)
					dv.SubRows = append(dv.SubRows, subRowView{RowType: rt.label, RowID: row.ID, Texts: row.Texts})
				}
			}
			dv.ShowAddHalf = canEdit && d.Halves == 1
			dv.ShowRemoveHalf = canEdit && d.Halves == 2
		} else if canEdit {
			dv.PendingActivity = pendingCells(d, "activity", classes)
			dv.PendingInstructor = pendingCells(d, "instructor", classes)
		}
		out = append(out, dv)
	}
	return out
}

// pendingCells names the boxes shown when a non-parade night is ticked. If the
// night kept a half-1 row of this kind its real names and text are used, so
// what was typed before comes back; otherwise they are new ones, filled in on
// save.
func pendingCells(d programme.Day, rowType string, classes int) []pendingCell {
	if row, ok := d.RowByHalfType(1, rowType); ok {
		out := make([]pendingCell, classes)
		for i := range out {
			out[i].Name = "row_" + strconv.FormatInt(row.ID, 10) + "_text_" + strconv.Itoa(i)
			if i < len(row.Texts) {
				out[i].Value = row.Texts[i]
			}
		}
		return out
	}
	out := make([]pendingCell, classes)
	for i := range out {
		out[i].Name = "day_" + strconv.FormatInt(d.ID, 10) + "_new_" + rowType + "_" + strconv.Itoa(i)
	}
	return out
}

// instructorOptions is the datalist content for the grid's Instructor row:
// a generic "All Staff" catch-all plus one entry per registered squadron
// member (their display name if they've set one, else their email — see
// TODO.md on why email is the common case right now). Purely a suggestion
// list — the input stays free text, so anything not listed still works.
func (s *server) instructorOptions(squadronID int64) []string {
	options := []string{"All Staff"}
	seen := map[string]bool{"All Staff": true}
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			options = append(options, name)
		}
	}
	if list, err := s.Store.ListInstructors(squadronID); err != nil {
		log.Printf("list instructors for options: %v", err)
	} else {
		for _, i := range list {
			add(i.Display())
		}
	}
	members, err := s.Auth.ListMembers(squadronID)
	if err != nil {
		log.Printf("list members for instructor options: %v", err)
		return options
	}
	for _, m := range members {
		if !m.Active {
			continue // inactive members are not suggested as instructors
		}
		name := m.DisplayName
		if name == "" {
			name = m.Email
		}
		add(name)
	}
	return options
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
		"Days":                buildDayViews(fp.Days, canEdit, len(sq.Classifications), fp.Closures),
		"ClosedColspan":       4 + len(sq.Classifications),
		"DayOptions":          dayOptions(fp.Days, fp.Closures),
		"Closures":            fp.Closures,
		"WeekendNotes":        fp.WeekendNotes,
		"Requirements":        fp.Requirements,
		"InstructorOptions":   s.instructorOptions(u.SquadronID),
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
	sq, err := s.Store.GetSquadron(u.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not load squadron.")
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
			UniformKey: r.PostForm.Get(prefix + "uniform"),
			Notes:      r.PostForm.Get(prefix + "notes"),
		}
		for _, row := range d.Rows {
			rp := "row_" + strconv.FormatInt(row.ID, 10) + "_text_"
			// A row that was not on the page (a night that is not a parade
			// night shows none) sent nothing: leave its text alone rather than
			// overwrite it with blanks.
			if !r.PostForm.Has(rp + "0") {
				continue
			}
			texts := make([]string, len(row.Texts))
			for i := range texts {
				texts[i] = r.PostForm.Get(rp + strconv.Itoa(i))
			}
			di.Rows = append(di.Rows, programme.RowInput{ID: row.ID, Texts: texts})
		}
		// A night that has just been ticked as a parade night and had no rows:
		// the text typed into its new boxes.
		if di.IsParade && !d.IsParade {
			di.NewActivity = newRowTexts(r.PostForm, prefix+"new_activity_", len(sq.Classifications))
			di.NewInstructor = newRowTexts(r.PostForm, prefix+"new_instructor_", len(sq.Classifications))
		}
		days = append(days, di)
	}

	if err := s.Store.SaveDaysAndRows(u.SquadronID, id, days); err != nil {
		log.Printf("save programme %d: %v", id, err)
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not save — try again, and if it keeps happening ask an admin to check the server logs.")
		return
	}
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
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
		if errors.Is(err, programme.ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Not found", "That night doesn't exist.")
			return
		}
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not add half.")
		return
	}
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(progID, 10)), http.StatusSeeOther)
}

// dayRemoveHalf drops a parade night from 2 halves back to 1. If half 2's
// content actually differs from half 1's, it doesn't delete anything on the
// first request — it shows a confirmation page instead (see
// confirm_remove_half.html), since that's the case where removal silently
// discards something.
func (s *server) dayRemoveHalf(w http.ResponseWriter, r *http.Request) {
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

	fp, err := s.Store.GetFullProgramme(u.SquadronID, progID)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	var day *programme.Day
	for i := range fp.Days {
		if fp.Days[i].ID == dayID {
			day = &fp.Days[i]
			break
		}
	}
	if day == nil || day.Halves != 2 {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That night doesn't have a second half to remove.")
		return
	}

	activity1, _ := day.RowByHalfType(1, "activity")
	instructor1, _ := day.RowByHalfType(1, "instructor")
	activity2, _ := day.RowByHalfType(2, "activity")
	instructor2, _ := day.RowByHalfType(2, "instructor")
	halvesMatch := stringSlicesEqual(activity1.Texts, activity2.Texts) && stringSlicesEqual(instructor1.Texts, instructor2.Texts)

	if r.FormValue("confirm") != "1" && !halvesMatch {
		s.render(w, r, "confirm_remove_half", "Remove second half?", map[string]any{
			"ProgrammeID": progID,
			"DayID":       dayID,
			"DateLabel":   day.Date,
			"Activity1":   activity1.Texts,
			"Instructor1": instructor1.Texts,
			"Activity2":   activity2.Texts,
			"Instructor2": instructor2.Texts,
		})
		return
	}

	if err := s.Store.RemoveHalf(u.SquadronID, progID, dayID); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not remove half.")
		return
	}
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(progID, 10)), http.StatusSeeOther)
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *server) programmePDF(w http.ResponseWriter, r *http.Request) {
	sq, fp, ok := s.loadProgrammeAndSquadron(w, r)
	if !ok {
		return
	}
	data, err := pdfgen.Render(sq, fp)
	if err != nil {
		log.Printf("render pdf failed for programme %d: %v", fp.ID, err)
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not render the PDF — try again, and if it keeps happening ask an admin to check the server logs.")
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
		if errors.Is(err, programme.ErrInvalidInput) {
			s.renderError(w, r, http.StatusBadRequest, "Error", "Closure dates must be valid yyyy-mm-dd dates, at most a year apart, with the end on or after the start.")
			return
		}
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not add closure.")
		return
	}
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
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
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
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
		if errors.Is(err, programme.ErrInvalidInput) {
			s.renderError(w, r, http.StatusBadRequest, "Error", "The note date must be a valid yyyy-mm-dd date.")
			return
		}
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not add note.")
		return
	}
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
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
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
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
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
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
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
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
	if err := s.Store.AddRequirementEntry(id, catID, r.FormValue("cadet_name"), r.FormValue("note")); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not add entry.")
		return
	}
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
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
	_ = s.Store.DeleteRequirementEntry(id, catID, entryID)
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
}

// dayMove moves one parade night's activities to another date (a swap, so
// nothing is overwritten). It backs both the "Move" control, which works
// without JavaScript, and drag-and-drop in static/js/gen/programme-grid.js.
func (s *server) dayMove(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	progID, ok1 := parseID(r, "id")
	fromID, ok2 := parseID(r, "dayID")
	toID, err := strconv.ParseInt(r.FormValue("target_day"), 10, 64)
	if !ok1 || !ok2 || err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Choose the date to move the activities to.")
		return
	}
	switch err := s.Store.MoveDayContent(u.SquadronID, progID, fromID, toID); {
	case errors.Is(err, programme.ErrInvalidInput):
		s.renderError(w, r, http.StatusBadRequest, "Error", "Can't move the activities: choose a different date, and move from a parade night.")
		return
	case errors.Is(err, programme.ErrNotFound):
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme or date doesn't exist.")
		return
	case err != nil:
		log.Printf("move activities in programme %d: %v", progID, err)
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not move the activities — try again.")
		return
	}
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(progID, 10)), http.StatusSeeOther)
}

type dayOption struct {
	ID    int64
	Label string
}

// dayOptions lists every date of the month for the "move activities to" picker.
func dayOptions(days []programme.Day, closures []programme.Closure) []dayOption {
	out := make([]dayOption, 0, len(days))
	for _, d := range days {
		if _, closed := programme.ClosureOn(d.Date, closures); closed {
			continue
		}
		t, _ := time.Parse("2006-01-02", d.Date)
		out = append(out, dayOption{ID: d.ID, Label: t.Format("Mon 2 Jan")})
	}
	return out
}

// programmeResetConfirm asks before a month is emptied; the reset itself is
// the POST below. Two steps because it cannot be undone.
func (s *server) programmeResetConfirm(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid request.")
		return
	}
	fp, err := s.Store.GetFullProgramme(u.SquadronID, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
		return
	}
	s.render(w, r, "confirm_reset_programme", "Reset programme?", map[string]any{
		"ProgrammeID": id,
		"MonthName":   monthNames[fp.Month],
		"Year":        fp.Year,
	})
}

func (s *server) programmeReset(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Error", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid request.")
		return
	}
	if err := s.Store.ResetProgramme(u.SquadronID, id); err != nil {
		if errors.Is(err, programme.ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
			return
		}
		log.Printf("reset programme %d: %v", id, err)
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not reset the programme.")
		return
	}
	log.Printf("programme %d reset by user %d (squadron %d)", id, u.ID, u.SquadronID)
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
}

// newRowTexts reads the boxes of a row that does not exist yet; nil if none was
// filled in.
func newRowTexts(form url.Values, prefix string, n int) []string {
	out := make([]string, n)
	any := false
	for i := range out {
		out[i] = form.Get(prefix + strconv.Itoa(i))
		any = any || out[i] != ""
	}
	if !any {
		return nil
	}
	return out
}
