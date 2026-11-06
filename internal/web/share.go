package web

import (
	"encoding/base64"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/ical"
	"rafactools-planner/internal/pdfgen"
	"rafactools-planner/internal/programme"
)

// Public sharing. A squadron owner makes a public link (a long random token
// that works for one month and can be renewed). Behind it, with no sign-in,
// are the calendar feed and the PDF of each month an editor has published.
// Only dates, uniform and activity titles leave the app: never the
// requirement tracker (cadet names), notes or, in the calendar, instructors.

// shareURL is the full address of a file at the squadron's public link.
func (s *server) shareURL(token, file string) string {
	return strings.TrimSuffix(s.Cfg.BaseURL, "/") + "/share/" + token + "/" + file
}

// monthFile is the file name of a month's public PDF, e.g. "2026-11.pdf".
func monthFile(year, month int, ext string) string {
	return strconv.Itoa(year) + "-" + pad2(month) + "." + ext
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// parseMonthFile reads "2026-11.pdf" back into a year and month.
func parseMonthFile(name, ext string) (year, month int, ok bool) {
	base, found := strings.CutSuffix(name, "."+ext)
	if !found || len(base) != 7 || base[4] != '-' {
		return 0, 0, false
	}
	y, err1 := strconv.Atoi(base[:4])
	m, err2 := strconv.Atoi(base[5:])
	if err1 != nil || err2 != nil || m < 1 || m > 12 || y < 2000 || y > 2200 {
		return 0, 0, false
	}
	return y, m, true
}

// shareSquadron finds the squadron behind the public link in the request, or
// writes the answer: 404 for an unknown link, 410 for one that has run out.
func (s *server) shareSquadron(w http.ResponseWriter, r *http.Request) (*programme.Squadron, bool) {
	// The link is a secret in the address: keep it out of referrers and search engines.
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	sq, err := s.Store.SquadronByShareToken(r.PathValue("token"))
	switch {
	case err == nil:
		return sq, true
	case errors.Is(err, programme.ErrExpired):
		http.Error(w, "This link has expired. Ask the squadron for a new one.", http.StatusGone)
	case errors.Is(err, programme.ErrNotFound):
		http.NotFound(w, r)
	default:
		log.Printf("share lookup: %v", err)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
	}
	return nil, false
}

// shareFile serves a file behind the public link: the calendar or a month's PDF or PNG.
func (s *server) shareFile(w http.ResponseWriter, r *http.Request) {
	switch name := r.PathValue("file"); {
	case name == "calendar.ics":
		s.shareCalendar(w, r)
	case strings.HasSuffix(name, ".pdf"):
		s.shareFileOfKind(w, r, "pdf")
	case strings.HasSuffix(name, ".png"):
		s.shareFileOfKind(w, r, "png")
	default:
		http.NotFound(w, r)
	}
}

func (s *server) shareCalendar(w http.ResponseWriter, r *http.Request) {
	sq, ok := s.shareSquadron(w, r)
	if !ok {
		return
	}
	months, err := s.Store.PublishedProgrammes(sq.ID)
	if err != nil {
		log.Printf("share calendar: %v", err)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	token := r.PathValue("token")
	now := programme.Now()
	events := calendarEvents(sq, months, now, func(year, month int) string {
		return s.shareURL(token, monthFile(year, month, "pdf"))
	})
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="programme.ics"`)
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write(ical.Build(sq.Name, events, now))
}

func (s *server) shareFileOfKind(w http.ResponseWriter, r *http.Request, kind string) {
	year, month, ok := parseMonthFile(r.PathValue("file"), kind)
	if !ok {
		http.NotFound(w, r)
		return
	}
	sq, ok := s.shareSquadron(w, r)
	if !ok {
		return
	}
	fp, err := s.Store.PublishedProgramme(sq.ID, year, month)
	if errors.Is(err, programme.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		log.Printf("share %s: %v", kind, err)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	// The requirement tracker names cadets: it never goes into a public file.
	data, key, err := s.monthFileData(kind, sq, fp, true)
	if err != nil {
		log.Printf("share %s render for programme %d: %v", kind, fp.ID, err)
		http.Error(w, "Could not make the file.", http.StatusInternalServerError)
		return
	}
	serveMonthFile(w, r, kind, data, key, year, month, "private, max-age=300")
}

// calendarEvents makes one event per parade day from today on, in published
// months, plus an all-day event for each weekend note. A day covered by a
// closure is left out.
func calendarEvents(sq *programme.Squadron, months []*programme.FullProgramme, now time.Time, pdfURL func(year, month int) string) []ical.Event {
	today := now.In(programme.Local).Format("2006-01-02")
	uniform := map[string]string{}
	for _, c := range sq.UniformColours {
		uniform[c.Key] = c.Label
	}
	var out []ical.Event
	for _, fp := range months {
		link := pdfURL(fp.Year, fp.Month)
		for _, d := range fp.Days {
			if d.Date < today {
				continue
			}
			if _, closed := programme.ClosureOn(d.Date, fp.Closures); closed {
				continue
			}
			if !d.IsParade {
				continue
			}
			day, err := time.ParseInLocation("2006-01-02", d.Date, programme.Local)
			if err != nil {
				continue
			}
			start, ok1 := atClock(day, sq.ParadeStart)
			end, ok2 := atClock(day, sq.ParadeEnd)
			if !ok1 || !ok2 {
				continue
			}
			out = append(out, ical.Event{
				UID:         "parade-" + sq.Number + "-" + d.Date + "@rafactools-planner",
				Start:       start,
				End:         end,
				Summary:     sq.Name + " parade night",
				Description: eventDescription(sq, d, uniform[d.UniformKey], link),
				URL:         link,
				Modified:    fp.UpdatedAt,
			})
		}
		for _, n := range fp.WeekendNotes {
			if n.Date < today {
				continue
			}
			day, err := time.Parse("2006-01-02", n.Date)
			if err != nil {
				continue
			}
			out = append(out, ical.Event{
				UID:      "note-" + sq.Number + "-" + strconv.FormatInt(n.ID, 10) + "@rafactools-planner",
				AllDay:   true,
				Start:    day,
				End:      day.AddDate(0, 0, 1),
				Summary:  n.Text,
				URL:      link,
				Modified: fp.UpdatedAt,
			})
		}
	}
	return out
}

// atClock returns the given local day at an HH:MM time.
func atClock(day time.Time, hhmm string) (time.Time, bool) {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, programme.Local), true
}

// eventDescription lists the uniform and the night's activities, grouped so
// that classes doing the same thing share one line. Instructors are left out.
func eventDescription(sq *programme.Squadron, d programme.Day, uniformLabel, link string) string {
	var lines []string
	if uniformLabel != "" {
		lines = append(lines, "Uniform: "+uniformLabel)
	}
	for half := 1; half <= max(d.Halves, 1); half++ {
		row, ok := d.RowByHalfType(half, "activity")
		if !ok {
			continue
		}
		groups := groupActivities(sq.Classifications, row.Texts)
		if len(groups) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		if d.Halves > 1 {
			lines = append(lines, "Part "+strconv.Itoa(half)+":")
		}
		lines = append(lines, groups...)
	}
	if link != "" {
		lines = append(lines, "", "Programme: "+link)
	}
	return strings.Join(lines, "\n")
}

// groupActivities turns one activity per class into lines of "Activity: Class,
// Class", in the order each activity first appears. Blank boxes are skipped; an
// activity shared by every class reads "Activity (all classes)".
func groupActivities(classes, texts []string) []string {
	type group struct {
		text    string
		classes []string
	}
	var groups []*group
	seen := map[string]*group{}
	for i, raw := range texts {
		text := strings.TrimSpace(raw)
		if text == "" || i >= len(classes) {
			continue
		}
		key := strings.ToLower(text)
		g := seen[key]
		if g == nil {
			g = &group{text: text}
			seen[key] = g
			groups = append(groups, g)
		}
		g.classes = append(g.classes, classes[i])
	}
	var out []string
	for _, g := range groups {
		if len(g.classes) == len(classes) {
			out = append(out, g.text+" (all classes)")
		} else {
			out = append(out, g.text+": "+strings.Join(g.classes, ", "))
		}
	}
	return out
}

// shareRenew makes or renews the squadron's public link (owners only).
func (s *server) shareRenew(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	if _, err := s.Store.NewShare(u.SquadronID); err != nil {
		log.Printf("renew share link: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Squadron settings", "Could not make the link.")
		return
	}
	s.audit(r, u.SquadronID, auth.AuditShareRenew, "")
	http.Redirect(w, r, s.url("/squadron#sharing"), http.StatusSeeOther)
}

// programmePublish publishes or unpublishes one month (editors and owners).
func (s *server) programmePublish(w http.ResponseWriter, r *http.Request) {
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
	on := r.FormValue("published") == "1"
	if err := s.Store.SetPublished(u.SquadronID, id, on); err != nil {
		if errors.Is(err, programme.ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Not found", "That programme doesn't exist.")
			return
		}
		log.Printf("set published: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Error", "Could not change that.")
		return
	}
	detail := "unpublished"
	if on {
		detail = "published"
		// Publishing needs a public link to be of any use: make one if there is
		// none or it has run out (anyone who can publish may do this; changing or
		// renewing it later stays with the owners).
		if sh, err := s.Store.GetShare(u.SquadronID); err == nil && (sh.Token == "" || sh.Expired(programme.Now())) {
			if _, err := s.Store.NewShare(u.SquadronID); err != nil {
				log.Printf("make public link on publish: %v", err)
			} else {
				s.audit(r, u.SquadronID, auth.AuditShareRenew, "made when a month was published")
			}
		}
	}
	s.audit(r, u.SquadronID, auth.AuditPublish, detail)
	if r.FormValue("return") == "dashboard" { // the buttons on the list of months
		http.Redirect(w, r, s.url("/"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, s.url("/programmes/"+strconv.FormatInt(id, 10)), http.StatusSeeOther)
}

// pdfOptions adds the calendar QR code to a PDF when the squadron has a public
// link that is still valid. The code uses the webcal scheme so a phone offers to
// subscribe rather than download.
func (s *server) pdfOptions(squadronID int64) pdfgen.Options {
	sh, err := s.Store.GetShare(squadronID)
	if err != nil || sh.Token == "" || sh.Expired(programme.Now()) {
		return pdfgen.Options{}
	}
	link := s.shareURL(sh.Token, "calendar.ics")
	for _, scheme := range []string{"https://", "http://"} {
		if rest, ok := strings.CutPrefix(link, scheme); ok {
			link = "webcal://" + rest
			break
		}
	}
	return pdfgen.Options{CalendarURL: link, CalendarExpires: sh.ExpiresAt}
}

// sharePanel is what the month page shows once a month is published: the
// public addresses, when they stop working and a QR code for the calendar.
func (s *server) sharePanel(squadronID int64, fp *programme.FullProgramme) map[string]any {
	out := map[string]any{"Has": false}
	if !fp.Published {
		return out
	}
	sh, err := s.Store.GetShare(squadronID)
	if err != nil || sh.Token == "" {
		return out
	}
	out["Has"] = true
	out["Expired"] = sh.Expired(programme.Now())
	out["Expires"] = sh.ExpiresAt.In(programme.Local).Format("2 January 2006")
	out["CalendarURL"] = s.shareURL(sh.Token, "calendar.ics")
	out["PDFURL"] = s.shareURL(sh.Token, monthFile(fp.Year, fp.Month, "pdf"))
	out["PNGURL"] = s.shareURL(sh.Token, monthFile(fp.Year, fp.Month, "png"))
	if opts := s.pdfOptions(squadronID); opts.CalendarURL != "" {
		if png, err := qrcode.Encode(opts.CalendarURL, qrcode.Medium, 280); err == nil {
			out["QR"] = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png))
		}
	}
	return out
}
