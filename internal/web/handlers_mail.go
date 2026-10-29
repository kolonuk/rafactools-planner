package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/config"
)

// The master admin's page for the site's email settings. Each value is the
// environment's default unless one is saved here, and a saved value wins. The
// password is write-only: it is never sent back to the browser.

const (
	maxMailValue    = 200
	maxMailPassword = 500
	mailTestTimeout = 25 * time.Second
)

type mailField struct {
	Key, Name, Label, Help string
	Secret                 bool
	Saved                  string // the database value (never the password)
	Effective              string // what is in use (never the password)
	Source                 string // database | environment | default
	HasValue               bool   // for the password: is one in use
	Options                []string
}

var mailFieldDefs = []mailField{
	{Key: "MAIL_PROVIDER", Name: "mail_provider", Label: "Provider", Options: config.MailProviders,
		Help: "A preset that fills in the host, port and login name for you. Leave as \"smtp\" for any other mail server."},
	{Key: "SMTP_HOST", Name: "smtp_host", Label: "Mail server (host)", Help: "Needed for \"smtp\"; the other presets fill it in."},
	{Key: "SMTP_PORT", Name: "smtp_port", Label: "Port", Help: "465 uses TLS from the start; 587 upgrades with STARTTLS."},
	{Key: "SMTP_USER", Name: "smtp_user", Label: "Login name", Help: "Cloudflare uses the fixed name api_token; Gmail needs your full address; SES uses its SMTP username."},
	{Key: "SMTP_PASS", Name: "smtp_pass", Label: "Password / token", Secret: true, Help: "An app password, API token or SMTP password. Stored encrypted. Leave blank to keep the current one."},
	{Key: "MAIL_FROM", Name: "mail_from", Label: "Send as (From address)", Help: "Must be on a domain your provider has verified."},
	{Key: "SES_REGION", Name: "ses_region", Label: "Amazon SES region", Help: "Only for the ses provider, for example eu-west-2."},
}

func (s *server) mailEnv() func(string) string {
	if s.Env != nil {
		return s.Env
	}
	return os.Getenv
}

// mailView is everything the page shows.
func (s *server) mailView(saved map[string]string) map[string]any {
	env := s.mailEnv()
	var fields []mailField
	for _, f := range mailFieldDefs {
		f.Saved = saved[f.Key]
		switch {
		case saved[f.Key] != "":
			f.Source = "database"
		case env(f.Key) != "":
			f.Source = "environment"
		default:
			f.Source = "default"
		}
		if f.Secret {
			f.HasValue = f.Source != "default"
		} else {
			f.Saved = saved[f.Key]
			f.Effective = env(f.Key)
			if f.Saved != "" {
				f.Effective = f.Saved
			}
		}
		fields = append(fields, f)
	}
	view := map[string]any{"Fields": fields}

	get := func(k string) string {
		if v := saved[k]; v != "" {
			return v
		}
		return env(k)
	}
	if m, err := config.ResolveMail(get); err != nil {
		view["MailProblem"] = err.Error()
	} else {
		view["Resolved"] = map[string]any{"Provider": m.Provider, "Host": m.Host, "Port": m.Port, "User": m.User, "From": m.From, "HasPass": m.Pass != ""}
		if m.Host == "" {
			view["MailProblem"] = "No mail server is set, so the site cannot send email."
		}
	}
	if entries, err := s.Auth.ListAuditAll(auth.AuditSettingsMail, 20); err == nil {
		view["Changes"] = entries
	}
	return view
}

func (s *server) mailPage(w http.ResponseWriter, r *http.Request, extra map[string]any) {
	if s.Settings == nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Site settings are not available.")
		return
	}
	saved, err := s.Settings.All()
	if err != nil {
		log.Printf("WARNING %v", err)
	}
	if sub, ok := extra["Submitted"].(map[string]string); ok {
		saved = sub // show what was typed, not what is stored
	}
	view := s.mailView(saved)
	for k, v := range extra {
		view[k] = v
	}
	if err != nil {
		view["Error"] = err.Error()
	}
	s.render(w, r, "admin_mail", "Email settings", view)
}

func (s *server) mailShow(w http.ResponseWriter, r *http.Request) {
	extra := map[string]any{}
	switch r.URL.Query().Get("done") {
	case "saved":
		extra["Flash"] = "Saved. The new settings are used for the next email."
	case "reset":
		extra["Flash"] = "Database settings removed. The environment's settings are used."
	}
	s.mailPage(w, r, extra)
}

// mailSave validates the proposed settings as a whole before writing anything,
// so a half-valid set can never leave the site unable to send mail.
func (s *server) mailSave(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Email settings", "Form expired, try again.")
		return
	}
	if s.Settings == nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Site settings are not available.")
		return
	}
	u := userFrom(r.Context())
	current, _ := s.Settings.All()
	proposed := map[string]string{}
	for k, v := range current {
		proposed[k] = v
	}
	var changed []string
	for _, f := range mailFieldDefs {
		val := strings.TrimSpace(r.FormValue(f.Name))
		limit := maxMailValue
		if f.Secret {
			limit = maxMailPassword
			if r.FormValue("clear_"+f.Name) == "1" {
				val = ""
			} else if val == "" {
				continue // blank keeps the saved password
			}
		}
		if len(val) > limit {
			s.mailPage(w, r, map[string]any{"Error": f.Label + " is too long."})
			return
		}
		if val == "" {
			delete(proposed, f.Key)
		} else {
			proposed[f.Key] = val
		}
		if proposed[f.Key] != current[f.Key] {
			changed = append(changed, f.Key)
		}
	}

	env := s.mailEnv()
	get := func(k string) string {
		if v := proposed[k]; v != "" {
			return v
		}
		return env(k)
	}
	m, err := config.ResolveMail(get)
	if err == nil {
		err = config.RequireMail(m, s.Cfg.AllowLogMail)
	}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.mailPage(w, r, map[string]any{"Error": "Not saved: " + err.Error() + ".", "Submitted": proposed})
		return
	}

	for _, f := range mailFieldDefs {
		v, had := proposed[f.Key]
		if !had {
			v = ""
		}
		if v == current[f.Key] {
			continue
		}
		if err := s.Settings.Set(f.Key, v, u.Email); err != nil {
			log.Printf("save setting %s: %v", f.Key, err)
			s.renderError(w, r, http.StatusInternalServerError, "Email settings", "Could not save. Nothing may have been changed; check the page.")
			return
		}
	}
	if len(changed) > 0 {
		s.audit(r, 0, auth.AuditSettingsMail, "changed "+strings.Join(changed, ", "))
		log.Printf("AUDIT master admin %s changed mail settings: %s", u.Email, strings.Join(changed, ", "))
	}
	http.Redirect(w, r, s.url("/admin/mail?done=saved"), http.StatusSeeOther)
}

// mailReset removes every saved mail setting so the environment applies again.
func (s *server) mailReset(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Email settings", "Form expired, try again.")
		return
	}
	if s.Settings == nil {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Site settings are not available.")
		return
	}
	u := userFrom(r.Context())
	// Refuse if the environment alone could not send mail: that would strand
	// the site without email.
	get := s.mailEnv()
	m, err := config.ResolveMail(get)
	if err == nil {
		err = config.RequireMail(m, s.Cfg.AllowLogMail)
	}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.mailPage(w, r, map[string]any{"Error": "Not reset: the environment's settings would not work (" + err.Error() + ")."})
		return
	}
	for _, f := range mailFieldDefs {
		if err := s.Settings.Delete(f.Key); err != nil {
			log.Printf("reset setting %s: %v", f.Key, err)
			s.renderError(w, r, http.StatusInternalServerError, "Email settings", "Could not reset.")
			return
		}
	}
	s.audit(r, 0, auth.AuditSettingsMail, "removed all database mail settings")
	log.Printf("AUDIT master admin %s removed the database mail settings", u.Email)
	http.Redirect(w, r, s.url("/admin/mail?done=reset"), http.StatusSeeOther)
}

// mailTest sends a short message to the master admin's own address using the
// settings in force, and shows exactly what happened.
func (s *server) mailTest(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Email settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), mailTestTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Mail.SendTest(u.Email) }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		err = errors.New("timed out waiting for the mail server")
	}
	extra := map[string]any{}
	if err != nil {
		log.Printf("mail test by %s failed: %v", u.Email, err)
		extra["TestFailed"] = true
		extra["TestResult"] = "The test email was not sent: " + cleanText(redactLine(err.Error()), 300, false)
		w.WriteHeader(http.StatusBadGateway)
	} else {
		extra["TestResult"] = "A test email was handed to the mail server for " + u.Email + ". Check that inbox (and spam)."
	}
	s.mailPage(w, r, extra)
}

// mailSummary says whether mail is set up and by which provider, for problem
// reports. It reads the settings but never returns a credential.
func (s *server) mailSummary() (configured bool, provider string) {
	env := s.mailEnv()
	get := env
	if s.Settings != nil {
		if g, _ := s.Settings.Getter(env); g != nil {
			get = g
		}
	}
	m, err := config.ResolveMail(get)
	if err != nil {
		return false, "invalid settings"
	}
	return m.Host != "", m.Provider
}
