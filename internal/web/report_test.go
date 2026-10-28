package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"rafactools-planner/internal/config"
	"rafactools-planner/internal/mail"
)

func withReports(masking bool) func(*config.Config) {
	return func(c *config.Config) { c.IssueEmail = "issues@example.org"; c.IssueMasking = masking }
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(i % 251)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for x := 0; x < 40; x++ {
		img.Set(x, x%30, color.RGBA{200, 10, 10, 255})
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

var nextReportIP int

type upload struct {
	name, ctype string
	data        []byte
}

// postReport submits the real multipart form the browser would. Anonymous
// clients first load the form so they hold the anonymous form-token cookie.
func postReport(t *testing.T, c *client, fields map[string]string, file *upload) *http.Response {
	t.Helper()
	// Every test client shares one IP; give each submission its own address so
	// the (real) per-address report limit only applies where a test wants it.
	if c.xff == "" {
		nextReportIP++
		c.xff = fmt.Sprintf("10.%d.%d.%d", nextReportIP>>16&255, nextReportIP>>8&255, nextReportIP&255)
		defer func() { c.xff = "" }()
	}
	if _, ok := fields["csrf"]; !ok {
		if c.csrf != "" {
			fields["csrf"] = c.csrf
		} else {
			fields["csrf"] = scrapeCSRF(t, c.get("/report"))
		}
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	if file != nil {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="screenshot"; filename="`+file.name+`"`)
		h.Set("Content-Type", file.ctype)
		p, _ := mw.CreatePart(h)
		p.Write(file.data)
	}
	mw.Close()
	req, _ := http.NewRequest("POST", c.base+"/report", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return c.do(req)
}

func (m *recordingMail) lastReport(t *testing.T) (mail.IssueReport, string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.reports) == 0 {
		t.Fatal("no report was emailed")
	}
	return m.reports[len(m.reports)-1], m.reportTo[len(m.reportTo)-1]
}

func (m *recordingMail) reportCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.reports)
}

func attachmentNamed(r mail.IssueReport, name string) (mail.Attachment, bool) {
	for _, a := range r.Attachments {
		if a.Name == name {
			return a, true
		}
	}
	return mail.Attachment{}, false
}

func TestReportingIsOffUnlessAnAddressIsConfigured(t *testing.T) {
	app := newApp(t) // no IssueEmail
	c := app.anon()
	wantStatus(t, c.get("/report"), 404)
	resp := c.postRaw("/report", url.Values{})
	wantStatus(t, resp, 404)
	page := body(c.get("/login"))
	if strings.Contains(page, "Report a problem") || strings.Contains(page, "report-capture.js") {
		t.Error("the link or script is present although reporting is off")
	}
}

func TestReportLinkAndScriptAppearOnEveryKindOfPage(t *testing.T) {
	app := newApp(t, withReports(true))
	owner := app.login(app.owner("1701"))
	for name, resp := range map[string]*http.Response{
		"login":     app.anon().get("/login"),
		"claim":     app.anon().get("/claim"),
		"dashboard": owner.get("/"),
		"docs":      owner.get("/docs/next/"),
		"account":   owner.get("/account"),
		"not found": owner.get("/programmes/999"),
	} {
		page := body(resp)
		if !strings.Contains(page, `data-report-link`) || !strings.Contains(page, `/static/js/gen/report-capture.js`) {
			t.Errorf("%s: no report link / script", name)
		}
	}
}

func TestMaskingOptionFollowsTheSetting(t *testing.T) {
	on := body(newApp(t, withReports(true)).anon().get("/report"))
	for _, want := range []string{`data-masking="1"`, `id="mask-undo"`, `id="mask-clear"`, "black out"} {
		if !strings.Contains(on, want) {
			t.Errorf("masking on: page lacks %q", want)
		}
	}
	off := body(newApp(t, withReports(false)).anon().get("/report"))
	for _, bad := range []string{`data-masking`, `mask-undo`, `mask-clear`, "black out"} {
		if strings.Contains(off, bad) {
			t.Errorf("masking off: page still offers %q", bad)
		}
	}
	if !strings.Contains(off, "sent exactly as it is") {
		t.Error("masking off: the page does not say the screenshot is sent as it is")
	}
}

func TestReportFormIsAMultipartFormWithAToken(t *testing.T) {
	app := newApp(t, withReports(true))
	page := body(app.anon().get("/report?from=" + url.QueryEscape("/programmes/3?token=secret#x")))
	for _, want := range []string{`enctype="multipart/form-data"`, `name="csrf"`, `name="description"`, `name="screenshot"`, `accept="image/png,image/jpeg"`, `name="from" value="/programmes/3"`} {
		if !strings.Contains(page, want) {
			t.Errorf("form lacks %s", want)
		}
	}
	if strings.Contains(page, "secret") {
		t.Error("the query string of the reported page leaked into the form")
	}
	evil := body(app.anon().get("/report?from=" + url.QueryEscape(`javascript:alert(1)//"><script>x</script>`)))
	if strings.Contains(evil, "<script>x") {
		t.Error("from= reflected unescaped")
	}
}

func TestReportIsEmailedWithTheRightParts(t *testing.T) {
	app := newApp(t, withReports(true))
	c := app.anon()
	resp := postReport(t, c, map[string]string{
		"category": "bug", "description": "The grid will not save.\nIt just spins.", "expected": "It saves.",
		"contact_email": "cadet@example.org", "from": "/programmes/3",
		"client_context": `{"page":"/programmes/3","title":"April 2030","viewport_w":1280,"viewport_h":800,"dpr":2,"user_agent":"TestBrowser/1.0","online":true,
			"trail":[{"at":"2030-04-01T10:00:00Z","kind":"click","target":"button \"Save programme\"","path":"/programmes/3"}],
			"errors":[{"kind":"error","message":"x is not defined","source":"/static/js/gen/report-form.js","line":10,"col":4}],
			"screenshot":{"captured":false,"method":"none"}}`,
	}, nil)
	page := body(resp)
	if resp.StatusCode != 200 || !strings.Contains(page, "your report was sent") || !strings.Contains(page, "data-report-sent") {
		t.Fatalf("status %d\n%.400s", resp.StatusCode, page)
	}
	r, to := app.mail.lastReport(t)
	if to != "issues@example.org" {
		t.Errorf("sent to %q", to)
	}
	if r.ReplyTo != "cadet@example.org" {
		t.Errorf("ReplyTo = %q", r.ReplyTo)
	}
	if !strings.Contains(r.Subject, "Something is broken") || !strings.Contains(r.Subject, "The grid will not save.") {
		t.Errorf("Subject = %q", r.Subject)
	}
	if !strings.Contains(page, r.ID) {
		t.Error("the thank-you page does not show the reference")
	}
	for _, want := range []string{
		"WRITTEN BY THE PERSON", "untrusted", "COLLECTED BY THE SERVER (trusted)", "SUPPLIED BY THE BROWSER",
		"The grid will not save.", "It saves.", "App version:", "Signed in: no",
		`button "Save programme"`, "x is not defined", "TestBrowser/1.0", "1280x800",
	} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("report body lacks %q:\n%s", want, r.Body)
		}
	}
	ctx, ok := attachmentNamed(r, "context.json")
	if !ok {
		t.Fatal("no context.json attached")
	}
	var parsed map[string]any
	if err := json.Unmarshal(ctx.Data, &parsed); err != nil || parsed["report_id"] != r.ID {
		t.Errorf("context.json invalid or wrong id: %v", err)
	}
	if _, has := attachmentNamed(r, "screenshot.png"); has {
		t.Error("a screenshot appeared from nowhere")
	}
	if len(r.Attachments) != 1 {
		t.Errorf("%d attachments, want only context.json", len(r.Attachments))
	}
}

func TestScreenshotsAreAcceptedOnlyIfTheyAreRealSmallImages(t *testing.T) {
	app := newApp(t, withReports(true))
	fields := func() map[string]string { return map[string]string{"description": "see picture"} }

	t.Run("png and jpeg are attached under safe names", func(t *testing.T) {
		for name, tc := range map[string]struct {
			up   upload
			want string
		}{
			"png":  {upload{"..\\..\\evil name.exe", "application/octet-stream", pngBytes(t, 50, 40)}, "screenshot.png"},
			"jpeg": {upload{"x.png", "image/png", jpegBytes(t)}, "screenshot.jpg"}, // the content decides, not the name or declared type
		} {
			before := app.mail.reportCount()
			up := tc.up
			wantStatus(t, postReport(t, app.anon(), fields(), &up), 200)
			if app.mail.reportCount() != before+1 {
				t.Fatalf("%s: not sent", name)
			}
			r, _ := app.mail.lastReport(t)
			a, ok := attachmentNamed(r, tc.want)
			if !ok || !bytes.Equal(a.Data, up.data) {
				t.Errorf("%s: attachment %q missing or altered", name, tc.want)
			}
		}
	})
	t.Run("refused", func(t *testing.T) {
		var gifBuf bytes.Buffer
		gif.Encode(&gifBuf, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}), nil)
		big := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, bytes.Repeat([]byte{0}, maxScreenshotBytes+10)...)
		for name, up := range map[string]upload{
			"gif":               {"a.png", "image/png", gifBuf.Bytes()},
			"html as png":       {"a.png", "image/png", []byte("<html><script>alert(1)</script></html>")},
			"svg":               {"a.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
			"png header only":   {"a.png", "image/png", []byte("\x89PNG\r\n\x1a\nnotreallyapng")},
			"too many pixels":   {"a.png", "image/png", pngBytes(t, maxScreenshotSide+1, 1)},
			"over the size cap": {"a.png", "image/png", big},
			"truncated jpeg":    {"a.jpg", "image/jpeg", jpegBytes(t)[:20]},
			"executable":        {"a.png", "image/png", []byte("MZ\x90\x00\x03\x00\x00\x00")},
		} {
			before := app.mail.reportCount()
			up := up
			resp := postReport(t, app.anon(), fields(), &up)
			page := body(resp)
			if resp.StatusCode != 400 || !strings.Contains(page, `class="error"`) {
				t.Errorf("%s: status %d", name, resp.StatusCode)
			}
			if app.mail.reportCount() != before {
				t.Errorf("%s: a report with a bad screenshot was emailed", name)
			}
			if !strings.Contains(page, "see picture") {
				t.Errorf("%s: the person's text was lost on the error page", name)
			}
		}
	})
	t.Run("an empty file field means no screenshot", func(t *testing.T) {
		up := upload{"", "application/octet-stream", nil}
		wantStatus(t, postReport(t, app.anon(), fields(), &up), 200)
		r, _ := app.mail.lastReport(t)
		if len(r.Attachments) != 1 {
			t.Errorf("%d attachments", len(r.Attachments))
		}
	})
}

func TestReportValidation(t *testing.T) {
	app := newApp(t, withReports(true))
	before := app.mail.reportCount()
	for name, fields := range map[string]map[string]string{
		"no description":     {"description": ""},
		"blank description":  {"description": "   \n  "},
		"bad email":          {"description": "x", "contact_email": "not an email"},
		"email with header":  {"description": "x", "contact_email": "a@example.org\r\nBcc: evil@example.org"},
		"display-name email": {"description": "x", "contact_email": "Evil <evil@example.org>"},
	} {
		resp := postReport(t, app.anon(), fields, nil)
		if resp.StatusCode != 400 {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
	if app.mail.reportCount() != before {
		t.Error("an invalid report was emailed")
	}
	// An unknown category is treated as "other", not an error; text is length-limited.
	wantStatus(t, postReport(t, app.anon(), map[string]string{"description": strings.Repeat("a", 9000), "category": "<script>"}, nil), 200)
	r, _ := app.mail.lastReport(t)
	if !strings.Contains(r.Subject, "Something else") {
		t.Errorf("category fallback: %q", r.Subject)
	}
	if strings.Count(r.Body, "a") > 4400 {
		t.Error("a 9000-character description was not cut to 4000")
	}
}

func TestSignedInReportsCanOnlyBeRepliedToAtTheirOwnAddress(t *testing.T) {
	app := newApp(t, withReports(true))
	email := app.owner("1702")
	c := app.login(email)

	wantStatus(t, postReport(t, c, map[string]string{"description": "x", "contact": "1", "contact_email": "attacker@example.org"}, nil), 200)
	r, _ := app.mail.lastReport(t)
	if r.ReplyTo != email {
		t.Errorf("ReplyTo = %q, want the person's own %q (a typed address must be ignored when signed in)", r.ReplyTo, email)
	}
	wantStatus(t, postReport(t, c, map[string]string{"description": "x"}, nil), 200)
	r, _ = app.mail.lastReport(t)
	if r.ReplyTo != "" {
		t.Errorf("ReplyTo = %q without consent", r.ReplyTo)
	}
	if strings.Contains(r.Body, email) {
		t.Error("the person's email appears in the report body without consent")
	}
}

func TestReportRequiresAValidFormToken(t *testing.T) {
	app := newApp(t, withReports(true))
	c := app.anon()
	c.get("/report")
	before := app.mail.reportCount()
	wantStatus(t, postReport(t, c, map[string]string{"description": "x", "csrf": "forged"}, nil), 400)
	wantStatus(t, postReport(t, app.anon(), map[string]string{"description": "x", "csrf": ""}, nil), 400)
	if app.mail.reportCount() != before {
		t.Error("a forged report was emailed")
	}
}

func TestReportsAreRateLimitedPerAddress(t *testing.T) {
	app := newApp(t, withReports(true))
	c := app.anon()
	c.xff = "203.0.113.9"
	tok := scrapeCSRF(t, c.get("/report"))
	var last int
	for i := 0; i < 7; i++ {
		resp := postReport(t, c, map[string]string{"description": "spam", "csrf": tok}, nil)
		last = resp.StatusCode
		resp.Body.Close()
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("7th report in a row -> %d, want 429", last)
	}
	if n := app.mail.reportCount(); n != 5 {
		t.Errorf("%d reports emailed, want exactly 5 before the limit", n)
	}
	other := app.anon()
	other.xff = "198.51.100.4"
	wantStatus(t, postReport(t, other, map[string]string{"description": "from elsewhere"}, nil), 200)
}

func TestAMailFailureIsReportedAndNothingIsLost(t *testing.T) {
	app := newApp(t, withReports(true))
	app.mail.reportErr = errors.New("smtp down")
	resp := postReport(t, app.anon(), map[string]string{"description": "my long careful description"}, nil)
	page := body(resp)
	if resp.StatusCode != http.StatusBadGateway || strings.Contains(page, "was sent") {
		t.Errorf("status %d: a failed send must not say it succeeded", resp.StatusCode)
	}
	if !strings.Contains(page, "my long careful description") || !strings.Contains(page, "try again") {
		t.Error("the person's text was not kept for a retry")
	}
}

func TestBrowserSuppliedContextIsScrubbed(t *testing.T) {
	app := newApp(t, withReports(true))
	token := strings.Repeat("ab12", 16)
	bad := map[string]any{
		"page":       "/reset/" + token + "?next=/x",
		"title":      "Page for oc.1234@rafac.mod.gov.uk\x00\x1b[31m",
		"messages":   []string{"bad link /invite/" + token, "a@b.org"},
		"user_agent": "UA\r\nBcc: evil@example.org",
		"trail": func() []map[string]string {
			var out []map[string]string
			for i := 0; i < 80; i++ {
				out = append(out, map[string]string{"at": "t", "kind": "click", "target": "button Save " + token, "path": "/verify?token=" + token})
			}
			return out
		}(),
		"errors": []map[string]any{{"kind": "error", "message": "failed for cadet@example.org " + token, "source": "https://evil.example/x.js?k=" + token, "line": -5, "col": 99999999}},
	}
	raw, _ := json.Marshal(bad)
	wantStatus(t, postReport(t, app.anon(), map[string]string{"description": "x", "client_context": string(raw)}, nil), 200)
	r, _ := app.mail.lastReport(t)
	ctx, _ := attachmentNamed(r, "context.json")
	all := r.Body + string(ctx.Data)
	for _, leaked := range []string{token, "oc.1234@rafac.mod.gov.uk", "cadet@example.org", "a@b.org", "\x1b", "next=/x", "k="} {
		if strings.Contains(all, leaked) {
			t.Errorf("report leaks %q", strings.ReplaceAll(leaked, "\x1b", `\x1b`))
		}
	}
	for _, line := range strings.Split(r.Body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Bcc:") {
			t.Errorf("a line break in the user agent started a new line: %q", line)
		}
	}
	if n := strings.Count(r.Body, "button Save"); n > 25 {
		t.Errorf("%d trail entries kept, cap is 25", n)
	}
	for name, junk := range map[string]string{"not json": "{{{", "huge": strings.Repeat("a", maxContextBytes+10), "wrong types": `{"trail":"nope","viewport_w":"wide"}`} {
		before := app.mail.reportCount()
		wantStatus(t, postReport(t, app.anon(), map[string]string{"description": "x", "client_context": junk}, nil), 200)
		if app.mail.reportCount() != before+1 {
			t.Errorf("%s: a bad context stopped the report going through", name)
		}
	}
}

func TestReportCarriesTheServersOwnViewOfRecentActivity(t *testing.T) {
	app := newApp(t, withReports(true))
	email := app.owner("1703")
	c := app.login(email)
	other := app.login(app.owner("1704"))

	// things this person did, including a failed save and a request with a secret in the URL
	c.post("/programmes", url.Values{"year": {"2030"}, "month": {"4"}}).Body.Close()
	c.post("/programmes", url.Values{"year": {"2030"}, "month": {"4"}}).Body.Close() // duplicate month -> error page
	c.get("/reset/" + strings.Repeat("cd34", 16) + "?x=1").Body.Close()
	other.get("/squadron").Body.Close() // someone else's activity must not appear

	wantStatus(t, postReport(t, c, map[string]string{"description": "what happened?"}, nil), 200)
	r, _ := app.mail.lastReport(t)
	for _, want := range []string{"Signed in: role=owner", "POST", "/programmes", "/reset/[token]"} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("report lacks %q:\n%s", want, r.Body)
		}
	}
	if !strings.Contains(r.Body, "-> 303") || !strings.Contains(r.Body, "-> 400") && !strings.Contains(r.Body, "-> 200") {
		t.Errorf("statuses of the requests are missing:\n%s", r.Body)
	}
	if strings.Contains(r.Body, "/squadron") && !strings.Contains(r.Body, "squadron_id") {
		t.Error("another person's request shows up")
	}
	for _, leaked := range []string{strings.Repeat("cd34", 16), "?x=1"} {
		if strings.Contains(r.Body, leaked) {
			t.Errorf("leaked %q", leaked)
		}
	}
}

// Nothing that could take over an account may ever be emailed: not session
// cookies, not form tokens, not the person's address or password.
func TestReportsNeverContainSecrets(t *testing.T) {
	app := newApp(t, withReports(true))
	email := app.owner("1705")
	c := app.login(email)
	var secrets []string
	for _, ck := range c.hc.Jar.Cookies(mustURL(c.base)) {
		secrets = append(secrets, ck.Value)
	}
	secrets = append(secrets, c.csrf, testPassword, app.cfg.SessionKey)

	c.get("/account").Body.Close()
	wantStatus(t, postReport(t, c, map[string]string{"description": "check", "client_context": `{"page":"/account","trail":[{"kind":"click","target":"link Account","path":"/account"}]}`}, nil), 200)
	r, _ := app.mail.lastReport(t)
	var all strings.Builder
	all.WriteString(r.Subject + r.Body + r.ReplyTo)
	for _, a := range r.Attachments {
		all.Write(a.Data)
	}
	if len(secrets) < 4 {
		t.Fatalf("test did not collect the secrets: %v", secrets)
	}
	for _, s := range secrets {
		if s != "" && strings.Contains(all.String(), s) {
			t.Errorf("the report contains a secret value (%d chars)", len(s))
		}
	}
	if strings.Contains(all.String(), email) {
		t.Error("the person's email address is in the report although they did not ask to be contacted")
	}
}

func TestReportBodyLimitIsHigherOnlyForReports(t *testing.T) {
	app := newApp(t, withReports(true))
	c := app.anon()
	tok := scrapeCSRF(t, c.get("/report"))
	huge := upload{"a.png", "image/png", bytes.Repeat([]byte{1}, maxReportBody+1)}
	resp := postReport(t, c, map[string]string{"description": "x", "csrf": tok}, &huge)
	if resp.StatusCode != 400 {
		t.Errorf("a %d MB upload -> %d, want 400", maxReportBody>>20+1, resp.StatusCode)
	}
	resp.Body.Close()
	// every other form keeps the 1 MB cap
	resp = c.postRaw("/login", url.Values{"csrf": {tok}, "email": {strings.Repeat("a", 2<<20)}, "password": {"x"}})
	if resp.StatusCode == 401 || resp.StatusCode == 200 {
		t.Errorf("a 2 MB login form was accepted (%d)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPrivateAreasAreMarkedForMasking(t *testing.T) {
	app := newApp(t, withReports(true))
	email := app.owner("1706")
	c := app.login(email)
	p, _ := app.store.CreateProgramme(app.squadronID(email), 2030, 4, 1)
	cat, _ := app.store.AddRequirementCategory(p.ID, "Duty", "")
	app.store.AddRequirementEntry(p.ID, cat, "Cadet Secretname", "")
	app.store.AddInstructor(app.squadronID(email), "Sgt Hidden", "")

	for path, name := range map[string]string{
		"/programmes/" + itoaID(p.ID): "Secretname",
		"/instructors":                "Hidden",
		"/squadron":                   email,
		"/":                           email, // the signed-in address in the top bar
	} {
		page := body(c.get(path))
		i := strings.Index(page, name)
		if i < 0 {
			t.Errorf("%s: %q not shown", path, name)
			continue
		}
		if !strings.Contains(page[max0(i-200):i], "data-private") {
			t.Errorf("%s: %q is not inside a data-private element", path, name)
		}
	}
}

func itoaID(n int64) string { return strconv.FormatInt(n, 10) }
func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
