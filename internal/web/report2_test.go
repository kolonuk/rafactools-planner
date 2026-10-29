package web

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestEveryResponseCarriesARequestIDMatchingThePageAndTheTrail(t *testing.T) {
	app := newApp(t, withReports(true))
	c := app.login(app.owner("1703"))
	resp := c.get("/instructors")
	id := resp.Header.Get("X-Request-ID")
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("X-Request-ID = %q", id)
	}
	if page := body(resp); !strings.Contains(page, `<meta name="request-id" content="`+id+`">`) {
		t.Error("the page does not carry its own request id")
	}
	if other := c.get("/instructors").Header.Get("X-Request-ID"); other == id {
		t.Error("two requests share an id")
	}

	// An error page shows it as a reference the person can quote.
	e := c.get("/programmes/999999")
	eid := e.Header.Get("X-Request-ID")
	if page := body(e); !strings.Contains(page, "(reference "+eid+")") {
		t.Errorf("error page lacks its reference:\n%s", page)
	}

	// And the report names the ids of the requests the server saw.
	postReport(t, c, map[string]string{"description": "x", "category": "bug"}, nil)
	rep, _ := app.mail.lastReport(t)
	if !strings.Contains(rep.Body, "["+id+"]") || !strings.Contains(rep.Body, "["+eid+"]") {
		t.Errorf("recent requests do not list their ids:\n%s", rep.Body)
	}
}

func TestReportCarriesProgrammeShapeSchemaAndMailFacts(t *testing.T) {
	app := newApp(t, withReports(false))
	app.env["SMTP_HOST"] = "mail.example.org"
	email := app.owner("1701")
	c := app.login(email)
	sq := app.squadronID(email)
	p, _ := app.store.CreateProgramme(sq, 2032, 4, 1)
	cat, _ := app.store.AddRequirementCategory(p.ID, "Fieldcraft", "")
	app.store.AddRequirementEntry(p.ID, cat, "Cdt Very Private", "private note")
	path := "/programmes/" + itoaID(p.ID)

	cc, _ := json.Marshal(map[string]any{"page": path, "request_id": "abc123def456", "timing_ms": map[string]int{"ttfb": 12, "dom_content_loaded": 80, "load": 140}})
	postReport(t, c, map[string]string{"description": "the grid looks wrong", "category": "wrong-output", "severity": "annoying", "repeat": "always", "client_context": string(cc)}, nil)
	rep, _ := app.mail.lastReport(t)

	for _, want := range []string{
		"How much it matters: It's in the way, but I can carry on", "How often: Every time I try",
		"Programme being viewed (numbers only): 2032/4,", "1 requirement categories with 1 entries",
		"mail_configured=true (smtp)", "migrations applied, newest", "Request id of the page: abc123def456", "first byte 12, DOM ready 80, loaded 140",
	} {
		if !strings.Contains(rep.Body, want) {
			t.Errorf("report body lacks %q:\n%s", want, rep.Body)
		}
	}
	if strings.Contains(rep.Body, "Cdt Very Private") || strings.Contains(rep.Body, "private note") {
		t.Error("cadet text leaked into the report")
	}
	ctx, ok := attachmentNamed(rep, "context.json")
	if !ok || !strings.Contains(string(ctx.Data), `"programme_shape"`) || !strings.Contains(string(ctx.Data), `"severity": "annoying"`) {
		t.Errorf("context.json lacks the new facts:\n%s", ctx.Data)
	}

	// A programme of another squadron gives no shape.
	other := app.login(app.owner("1702"))
	postReport(t, other, map[string]string{"description": "x", "client_context": string(cc)}, nil)
	rep2, _ := app.mail.lastReport(t)
	if strings.Contains(rep2.Body, "Programme being viewed") {
		t.Error("another squadron's programme was described")
	}
}

func TestSeverityRepeatAndRequestIDInputsAreAllowListed(t *testing.T) {
	app := newApp(t, withReports(true))
	c := app.anon()
	cc, _ := json.Marshal(map[string]any{"request_id": "<script>alert(1)</script>", "timing_ms": map[string]int{"ttfb": -5, "load": 999999999}})
	postReport(t, c, map[string]string{"description": "x", "severity": "<b>evil</b>\nBcc: x@y", "repeat": "whenever", "client_context": string(cc)}, nil)
	rep, _ := app.mail.lastReport(t)
	for _, bad := range []string{"<script>", "<b>evil", "Bcc: x@y", "whenever"} {
		if strings.Contains(rep.Body, bad) {
			t.Errorf("unvalidated input reached the report: %q", bad)
		}
	}
	if strings.Contains(rep.Body, "How much it matters") || strings.Contains(rep.Body, "How often") {
		t.Error("an invalid choice was treated as an answer")
	}
	if strings.Contains(rep.Body, "Request id of the page") {
		t.Error("an invalid request id was accepted")
	}
	if strings.Contains(rep.Body, "999999999") {
		t.Errorf("timing was not clamped:\n%s", rep.Body)
	}
}

func TestReportFormOffersSeverityAndRepeat(t *testing.T) {
	app := newApp(t, withReports(true))
	page := body(app.anon().get("/report"))
	for _, want := range []string{`name="severity"`, `name="repeat"`, "Every time I try", "get my work done"} {
		if !strings.Contains(page, want) {
			t.Errorf("form lacks %q", want)
		}
	}
	if got := app.anon().get("/report").Header.Get("X-Request-ID"); got == "" {
		t.Error("no request id on the form")
	}
	_ = http.StatusOK
}

func TestScreenshotStepIsSpelledOutAndReportIsInTheTopBar(t *testing.T) {
	app := newApp(t, withReports(true))
	form := body(app.anon().get("/report"))
	for _, want := range []string{`class="field shot-box"`, "Add a screenshot", `id="shot-capture"`, "Take a screenshot now", `id="shot-file"`, "Attach a picture from your device", `id="shot-status"`} {
		if !strings.Contains(form, want) {
			t.Errorf("report form lacks %q", want)
		}
	}
	// The top bar has the button on signed-out and signed-in pages alike.
	for name, page := range map[string]string{
		"login":     body(app.anon().get("/login")),
		"dashboard": body(app.login(app.owner("1801")).get("/")),
	} {
		head, _, _ := strings.Cut(page, "</header>")
		if !strings.Contains(head, `class="report-button"`) || !strings.Contains(head, "data-report-link") {
			t.Errorf("%s: no Report a problem button in the top bar", name)
		}
	}
	// And not at all when reporting is off.
	off := newApp(t)
	if strings.Contains(body(off.anon().get("/login")), "report-button") {
		t.Error("a report button is shown although reporting is off")
	}
}
