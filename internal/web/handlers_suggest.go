package web

import (
	"errors"
	"log"
	"net/http"
	"strings"

	qrcode "github.com/skip2/go-qrcode"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/pdfgen"
	"rafactools-planner/internal/programme"
)

// Activity suggestions: a public form behind a link only the squadron has (the QR code
// on the printed programme), an inbox for editors on the Activities page, and the owner's
// controls on the Squadron page. Only the name of an activity is collected.

func (s *server) suggestURL(token string) string {
	return strings.TrimSuffix(s.Cfg.BaseURL, "/") + s.url("/suggest/"+token)
}

// printOptions is everything printed at the foot of a PDF: the calendar's QR code for a
// published month and, if the squadron chose it, the QR code for suggesting an activity.
func (s *server) printOptions(squadronID int64, published bool) pdfgen.Options {
	var o pdfgen.Options
	if published {
		o = s.pdfOptions(squadronID)
	}
	if set, err := s.Store.GetSuggestSettings(squadronID); err == nil && set.QROnPDF {
		o.SuggestURL = s.suggestURL(set.Token)
	}
	return o
}

// --- The public form --------------------------------------------------------

func (s *server) suggestSquadron(w http.ResponseWriter, r *http.Request) (*programme.Squadron, bool) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	sq, err := s.Store.SquadronBySuggestToken(r.PathValue("token"))
	if err != nil {
		if !errors.Is(err, programme.ErrNotFound) {
			log.Printf("suggestion link lookup: %v", err)
		}
		s.renderError(w, r, http.StatusNotFound, "Suggest an activity", "That link is not valid. Ask the squadron for a new one.")
		return nil, false
	}
	return sq, true
}

func (s *server) suggestForm(w http.ResponseWriter, r *http.Request) {
	sq, ok := s.suggestSquadron(w, r)
	if !ok {
		return
	}
	s.render(w, r, "suggest", "Suggest an activity", map[string]any{
		"SquadronName": sq.Name, "Token": r.PathValue("token"), "Thanks": r.URL.Query().Get("thanks") == "1",
	})
}

func (s *server) suggestSubmit(w http.ResponseWriter, r *http.Request) {
	sq, ok := s.suggestSquadron(w, r)
	if !ok {
		return
	}
	token := r.PathValue("token")
	again := func(status int, msg string) {
		w.WriteHeader(status)
		s.render(w, r, "suggest", "Suggest an activity", map[string]any{"SquadronName": sq.Name, "Token": token, "Error": msg})
	}
	if !s.checkCSRF(r) {
		again(http.StatusBadRequest, "The page was too old. Try again.")
		return
	}
	if !s.turnstileOK(r) {
		again(http.StatusBadRequest, turnstileFailMsg)
		return
	}
	// A box real people never see: a script that fills in every field fills this one in too.
	if r.FormValue("website") != "" {
		http.Redirect(w, r, s.url("/suggest/"+token+"?thanks=1"), http.StatusSeeOther)
		return
	}
	if _, err := s.Store.AddSuggestion(sq.ID, r.FormValue("name")); err != nil {
		if errors.Is(err, programme.ErrInvalidInput) {
			again(http.StatusBadRequest, errorText(err))
			return
		}
		log.Printf("add suggestion: %v", err)
		again(http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}
	http.Redirect(w, r, s.url("/suggest/"+token+"?thanks=1"), http.StatusSeeOther)
}

// --- Editors: the inbox -----------------------------------------------------

func (s *server) suggestionAction(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Activities", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid suggestion.")
		return
	}
	var err error
	switch r.PathValue("action") {
	case "add":
		_, err = s.Store.MakeSuggestionActivity(u.SquadronID, id)
	case "delete":
		err = s.Store.DeleteSuggestion(u.SquadronID, id)
	default:
		s.renderError(w, r, http.StatusNotFound, "Not found", "Unknown action.")
		return
	}
	switch {
	case err == nil:
		http.Redirect(w, r, s.url("/activities#suggestions"), http.StatusSeeOther)
	case errors.Is(err, programme.ErrNotFound):
		s.renderError(w, r, http.StatusNotFound, "Not found", "That suggestion isn't there any more.")
	case errors.Is(err, programme.ErrInvalidInput):
		s.renderActivities(w, r, http.StatusBadRequest, errorText(err))
	default:
		log.Printf("suggestion %s: %v", r.PathValue("action"), err)
		s.renderError(w, r, http.StatusInternalServerError, "Activities", "Could not do that. Try again.")
	}
}

// --- Owners: the link and the QR code ---------------------------------------

func (s *server) suggestSettings(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	var err error
	detail := ""
	switch r.FormValue("action") {
	case "new":
		_, err = s.Store.NewSuggestToken(u.SquadronID)
		detail = "made the suggestion link"
	case "off":
		err = s.Store.DisableSuggestions(u.SquadronID)
		detail = "turned the suggestion link off"
	case "pdf":
		err = s.Store.SetSuggestQROnPDF(u.SquadronID, r.FormValue("qr_on_pdf") == "1")
		detail = "changed the suggestion QR code on the PDF"
	default:
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Unknown action.")
		return
	}
	if err != nil {
		if errors.Is(err, programme.ErrInvalidInput) {
			s.renderError(w, r, http.StatusBadRequest, "Squadron settings", errorText(err))
			return
		}
		log.Printf("suggestion settings: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Squadron settings", "Could not save. Try again.")
		return
	}
	s.audit(r, u.SquadronID, auth.AuditSquadronEdit, detail)
	http.Redirect(w, r, s.url("/squadron#suggestions"), http.StatusSeeOther)
}

// suggestQR serves the suggestion link's QR code to the squadron's owners.
func (s *server) suggestQR(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	set, err := s.Store.GetSuggestSettings(u.SquadronID)
	if err != nil || !set.HasToken {
		http.NotFound(w, r)
		return
	}
	png, err := qrcode.Encode(s.suggestURL(set.Token), qrcode.Medium, 280)
	if err != nil {
		http.Error(w, "Could not make the QR code.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, no-cache")
	_, _ = w.Write(png)
}
