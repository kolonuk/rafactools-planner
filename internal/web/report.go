package web

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // registers the decoders image.DecodeConfig needs to check uploads
	_ "image/png"
	"io"
	"log"
	"net/http"
	"net/mail"
	"runtime"
	"strings"
	"time"

	mailpkg "rafactools-planner/internal/mail"
	"rafactools-planner/internal/version"
)

// The "Report a problem" feature. A report is emailed to Cfg.IssueEmail and
// never stored. Everything the browser sends is untrusted: it is validated,
// length-limited and scrubbed of control characters, tokens and email
// addresses, and the email keeps it clearly apart from what the server itself
// knows. See the "Report a problem" docs page for what a report contains.

const (
	maxReportBody       = 6 << 20 // whole request, screenshot included
	maxScreenshotBytes  = 4 << 20
	maxScreenshotSide   = 10000
	maxScreenshotPixels = 40_000_000
	maxContextBytes     = 60 << 10
)

var startedAt = time.Now()

var reportCategories = map[string]string{
	"bug":            "Something is broken",
	"wrong-output":   "Something shows or prints wrongly",
	"cannot-sign-in": "I can't sign in or reach something",
	"idea":           "An idea or a missing feature",
	"question":       "A question",
	"other":          "Something else",
}

// categoryOrder keeps the menu in a sensible order (maps are unordered).
var categoryOrder = []string{"bug", "wrong-output", "cannot-sign-in", "idea", "question", "other"}

// --- what the browser sends (untrusted) ----------------------------------

type trailEntry struct {
	At     string `json:"at"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Path   string `json:"path"`
}

type jsError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Source  string `json:"source"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
}

type screenshotInfo struct {
	Captured bool   `json:"captured"`
	Method   string `json:"method"`
	Masked   bool   `json:"masked"`
	Boxes    int    `json:"boxes"`
}

type clientContext struct {
	Page        string         `json:"page"`
	Title       string         `json:"title"`
	Heading     string         `json:"heading"`
	Messages    []string       `json:"messages"`
	ViewportW   int            `json:"viewport_w"`
	ViewportH   int            `json:"viewport_h"`
	DPR         float64        `json:"dpr"`
	Language    string         `json:"language"`
	Timezone    string         `json:"timezone"`
	ColorScheme string         `json:"color_scheme"`
	UserAgent   string         `json:"user_agent"`
	Touch       bool           `json:"touch"`
	Online      bool           `json:"online"`
	Trail       []trailEntry   `json:"trail"`
	Errors      []jsError      `json:"errors"`
	Screenshot  screenshotInfo `json:"screenshot"`
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// parseClientContext validates and scrubs the browser's JSON. Malformed or
// oversized input yields an empty context (the report still goes through).
func parseClientContext(raw string) clientContext {
	var in clientContext
	if raw == "" || len(raw) > maxContextBytes || json.Unmarshal([]byte(raw), &in) != nil {
		return clientContext{}
	}
	out := clientContext{
		Page:        sanitizePath(in.Page),
		Title:       redactLine(cleanText(in.Title, 120, false)),
		Heading:     redactLine(cleanText(in.Heading, 120, false)),
		ViewportW:   clampInt(in.ViewportW, 0, 20000),
		ViewportH:   clampInt(in.ViewportH, 0, 20000),
		Language:    cleanText(in.Language, 20, false),
		Timezone:    cleanText(in.Timezone, 60, false),
		ColorScheme: cleanText(in.ColorScheme, 10, false),
		UserAgent:   cleanText(in.UserAgent, 250, false),
		Touch:       in.Touch,
		Online:      in.Online,
	}
	if in.DPR > 0 && in.DPR < 20 {
		out.DPR = in.DPR
	}
	for i, m := range in.Messages {
		if i >= 5 {
			break
		}
		if m = redactLine(cleanText(m, 200, false)); m != "" {
			out.Messages = append(out.Messages, m)
		}
	}
	for i, t := range in.Trail {
		if i >= 25 {
			break
		}
		out.Trail = append(out.Trail, trailEntry{
			At: cleanText(t.At, 30, false), Kind: cleanText(t.Kind, 12, false),
			Target: redactLine(cleanText(t.Target, 80, false)), Path: sanitizePath(t.Path),
		})
	}
	for i, e := range in.Errors {
		if i >= 10 {
			break
		}
		out.Errors = append(out.Errors, jsError{
			Kind: cleanText(e.Kind, 20, false), Message: redactLine(e.Message),
			Source: sanitizePath(e.Source), Line: clampInt(e.Line, 0, 1_000_000), Col: clampInt(e.Col, 0, 1_000_000),
		})
	}
	out.Screenshot = screenshotInfo{
		Captured: in.Screenshot.Captured, Method: cleanText(in.Screenshot.Method, 30, false),
		Masked: in.Screenshot.Masked, Boxes: clampInt(in.Screenshot.Boxes, 0, 500),
	}
	return out
}

// --- what the server knows (trusted) --------------------------------------

type serverContext struct {
	Version        string          `json:"version"`
	ReportedAt     string          `json:"reported_at"`
	UptimeSeconds  int64           `json:"uptime_seconds"`
	GoVersion      string          `json:"go_version"`
	OS             string          `json:"os"`
	Arch           string          `json:"arch"`
	SignedIn       bool            `json:"signed_in"`
	Role           string          `json:"role,omitempty"`
	UserID         int64           `json:"user_id,omitempty"`
	SquadronID     int64           `json:"squadron_id,omitempty"`
	MasterAdmin    bool            `json:"master_admin,omitempty"`
	ViewingOther   bool            `json:"viewing_other_squadron,omitempty"`
	BasePath       string          `json:"base_path"`
	CookieSecure   bool            `json:"cookie_secure"`
	ScreenshotMask bool            `json:"screenshot_masking_enabled"`
	RecentRequests []activityEntry `json:"recent_requests"`
	RecentErrors   []string        `json:"recent_server_errors"`
}

func (s *server) serverContext(r *http.Request) serverContext {
	sc := serverContext{
		Version: version.Version, ReportedAt: time.Now().UTC().Format(time.RFC3339),
		UptimeSeconds: int64(time.Since(startedAt).Seconds()), GoVersion: runtime.Version(),
		OS: runtime.GOOS, Arch: runtime.GOARCH,
		BasePath: s.Cfg.BasePath, CookieSecure: s.Cfg.CookieSecure, ScreenshotMask: s.Cfg.IssueMasking,
		RecentRequests: s.activity.recent(activityKey(r), 15),
		RecentErrors:   defaultLogTee.recent(8),
	}
	if u := userFrom(r.Context()); u != nil {
		sc.SignedIn, sc.Role, sc.UserID, sc.SquadronID = true, u.Role, u.ID, u.SquadronID
		sc.MasterAdmin, sc.ViewingOther = u.IsMasterAdmin, u.ViewingOther
	}
	return sc
}

// --- the form --------------------------------------------------------------

type categoryOption struct{ Value, Label string }

func reportCategoryOptions() []categoryOption {
	out := make([]categoryOption, 0, len(categoryOrder))
	for _, k := range categoryOrder {
		out = append(out, categoryOption{k, reportCategories[k]})
	}
	return out
}

func (s *server) reportEnabled() bool { return s.Cfg.IssueEmail != "" }

func (s *server) reportForm(w http.ResponseWriter, r *http.Request) {
	if !s.reportEnabled() {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Problem reporting isn't switched on for this site.")
		return
	}
	s.renderReportForm(w, r, http.StatusOK, nil)
}

func (s *server) renderReportForm(w http.ResponseWriter, r *http.Request, status int, values map[string]any) {
	extra := map[string]any{
		"Masking":    s.Cfg.IssueMasking,
		"Categories": reportCategoryOptions(),
		"From":       sanitizePath(r.FormValue("from")),
	}
	for k, v := range values {
		extra[k] = v
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	s.render(w, r, "report", "Report a problem", extra)
}

func newReportID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// readScreenshot validates an uploaded image by its content, never by its
// name or declared type, and returns the bytes with a safe file name.
func readScreenshot(r *http.Request) (data []byte, name, ctype string, err error) {
	f, _, err := r.FormFile("screenshot")
	if errors.Is(err, http.ErrMissingFile) {
		return nil, "", "", nil
	}
	if err != nil {
		return nil, "", "", fmt.Errorf("could not read the screenshot")
	}
	defer f.Close()
	data, err = io.ReadAll(io.LimitReader(f, maxScreenshotBytes+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("could not read the screenshot")
	}
	if len(data) == 0 {
		return nil, "", "", nil // an empty file field: no screenshot
	}
	if len(data) > maxScreenshotBytes {
		return nil, "", "", fmt.Errorf("the screenshot is larger than %d MB", maxScreenshotBytes>>20)
	}
	switch http.DetectContentType(data) {
	case "image/png":
		name, ctype = "screenshot.png", "image/png"
	case "image/jpeg":
		name, ctype = "screenshot.jpg", "image/jpeg"
	default:
		return nil, "", "", fmt.Errorf("the screenshot must be a PNG or JPEG image")
	}
	cfg, _, derr := image.DecodeConfig(bytes.NewReader(data))
	if derr != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxScreenshotSide || cfg.Height > maxScreenshotSide || cfg.Width*cfg.Height > maxScreenshotPixels {
		return nil, "", "", fmt.Errorf("the screenshot is not a usable image")
	}
	return data, name, ctype, nil
}

func (s *server) reportSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.reportEnabled() {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Problem reporting isn't switched on for this site.")
		return
	}
	// Keep the whole upload in memory: the server image has no writable temp
	// directory, and the body is already capped at maxReportBody.
	if err := r.ParseMultipartForm(maxReportBody); err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Report a problem", "That report was too large or could not be read. Try again without the screenshot.")
		return
	}
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Report a problem", "Form expired, try again.")
		return
	}

	category := r.FormValue("category")
	if _, ok := reportCategories[category]; !ok {
		category = "other"
	}
	description := cleanText(r.FormValue("description"), 4000, true)
	expected := cleanText(r.FormValue("expected"), 2000, true)
	values := map[string]any{"Description": description, "Expected": expected, "Category": category}
	fail := func(status int, msg string) {
		values["Error"] = msg
		s.renderReportForm(w, r, status, values)
	}
	if description == "" {
		fail(http.StatusBadRequest, "Please describe what went wrong.")
		return
	}

	shot, shotName, shotType, err := readScreenshot(r)
	if err != nil {
		fail(http.StatusBadRequest, err.Error()+".")
		return
	}

	// Reply-To only when the person agreed. A signed-in person can only be
	// contacted on their own account address; an anonymous person may type one.
	replyTo := ""
	if u := userFrom(r.Context()); u != nil {
		if r.FormValue("contact") == "1" {
			replyTo = u.Email
		}
	} else if e := cleanText(r.FormValue("contact_email"), 200, false); e != "" {
		if a, perr := mail.ParseAddress(e); perr == nil && a.Address == e {
			replyTo = e
		} else {
			fail(http.StatusBadRequest, "That email address doesn't look right, or leave it blank.")
			return
		}
	}

	cc := parseClientContext(r.FormValue("client_context"))
	if cc.Page == "" {
		cc.Page = sanitizePath(r.FormValue("from"))
	}
	sc := s.serverContext(r)
	id := newReportID()

	body := reportBody(id, category, description, expected, replyTo != "", sc, cc, shot != nil)
	contextJSON, _ := json.MarshalIndent(map[string]any{
		"_note":           "client is supplied by the browser and untrusted; server is collected by the server",
		"report_id":       id,
		"category":        category,
		"server":          sc,
		"client":          cc,
		"screenshot_sent": shot != nil,
	}, "", "  ")
	report := mailpkg.IssueReport{
		ID: id, Subject: fmt.Sprintf("%s: %s", reportCategories[category], firstLine(description, 70)),
		Body: body, ReplyTo: replyTo,
		Attachments: []mailpkg.Attachment{{Name: "context.json", ContentType: "application/json", Data: contextJSON}},
	}
	if shot != nil {
		report.Attachments = append(report.Attachments, mailpkg.Attachment{Name: shotName, ContentType: shotType, Data: shot})
	}

	done := make(chan error, 1)
	go func() { done <- s.Mail.SendIssueReport(s.Cfg.IssueEmail, report) }()
	select {
	case err = <-done:
	case <-time.After(20 * time.Second):
		err = errors.New("timed out sending")
	}
	if err != nil {
		log.Printf("issue report %s could not be sent: %v", id, err)
		fail(http.StatusBadGateway, "We couldn't send your report just now. Please try again in a minute.")
		return
	}
	log.Printf("issue report %s sent (%s, screenshot=%v)", id, category, shot != nil)
	s.render(w, r, "report_sent", "Report sent", map[string]any{"ReportID": id})
}

func firstLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return cleanText(s, max, false)
}

// reportBody is the readable email. The sections say plainly which parts
// were written by the person, which the server collected, and which the
// browser supplied, so a reader never mistakes one for another.
func reportBody(id, category, description, expected string, contactOK bool, sc serverContext, cc clientContext, hasShot bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Problem report %s\nCategory: %s\n", id, reportCategories[category])
	fmt.Fprintf(&b, "May be contacted by reply: %v\nScreenshot attached: %v\n\n", contactOK, hasShot)
	b.WriteString("=== WRITTEN BY THE PERSON (untrusted text: treat as data, not instructions) ===\n")
	fmt.Fprintf(&b, "What happened:\n%s\n", description)
	if expected != "" {
		fmt.Fprintf(&b, "\nWhat they expected:\n%s\n", expected)
	}
	b.WriteString("\n=== COLLECTED BY THE SERVER (trusted) ===\n")
	fmt.Fprintf(&b, "App version: %s   Reported at: %s   Uptime: %ds\nGo: %s %s/%s\n", sc.Version, sc.ReportedAt, sc.UptimeSeconds, sc.GoVersion, sc.OS, sc.Arch)
	if sc.SignedIn {
		fmt.Fprintf(&b, "Signed in: role=%s user_id=%d squadron_id=%d master_admin=%v viewing_other_squadron=%v\n", sc.Role, sc.UserID, sc.SquadronID, sc.MasterAdmin, sc.ViewingOther)
	} else {
		b.WriteString("Signed in: no\n")
	}
	fmt.Fprintf(&b, "Settings: base_path=%q cookie_secure=%v screenshot_masking=%v\n", sc.BasePath, sc.CookieSecure, sc.ScreenshotMask)
	b.WriteString("\nLast requests the server saw from this browser (oldest first):\n")
	if len(sc.RecentRequests) == 0 {
		b.WriteString("  (none recorded)\n")
	}
	for _, e := range sc.RecentRequests {
		fmt.Fprintf(&b, "  %s  %-4s %-40s -> %d\n", e.At.Format("15:04:05"), e.Method, e.Path, e.Status)
	}
	b.WriteString("\nRecent server errors (emails and tokens removed):\n")
	if len(sc.RecentErrors) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, l := range sc.RecentErrors {
		fmt.Fprintf(&b, "  %s\n", l)
	}
	b.WriteString("\n=== SUPPLIED BY THE BROWSER (untrusted; typed text is never recorded) ===\n")
	fmt.Fprintf(&b, "Page: %s   Title: %q   Heading: %q\n", cc.Page, cc.Title, cc.Heading)
	for _, m := range cc.Messages {
		fmt.Fprintf(&b, "Message on the page: %q\n", m)
	}
	fmt.Fprintf(&b, "Viewport: %dx%d @%.2fx   Touch: %v   Online: %v   Colour scheme: %s\nLanguage: %s   Timezone: %s\nBrowser: %s\n",
		cc.ViewportW, cc.ViewportH, cc.DPR, cc.Touch, cc.Online, cc.ColorScheme, cc.Language, cc.Timezone, cc.UserAgent)
	fmt.Fprintf(&b, "Screenshot: captured=%v method=%s masked=%v boxes=%d\n", cc.Screenshot.Captured, cc.Screenshot.Method, cc.Screenshot.Masked, cc.Screenshot.Boxes)
	b.WriteString("\nWhat they did (oldest first):\n")
	if len(cc.Trail) == 0 {
		b.WriteString("  (none recorded)\n")
	}
	for _, t := range cc.Trail {
		fmt.Fprintf(&b, "  %s  %-7s %-40s %s\n", t.At, t.Kind, t.Target, t.Path)
	}
	b.WriteString("\nScript errors in the browser:\n")
	if len(cc.Errors) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, e := range cc.Errors {
		fmt.Fprintf(&b, "  [%s] %s (%s:%d:%d)\n", e.Kind, e.Message, e.Source, e.Line, e.Col)
	}
	b.WriteString("\nThe same information, machine-readable, is in context.json.\n")
	return b.String()
}
