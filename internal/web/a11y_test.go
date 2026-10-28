package web

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Delete/remove buttons repeat down a page, so each needs a name that says
// which row it belongs to; "Delete" alone is meaningless to a screen reader.
func TestRepeatedActionButtonsHaveDistinguishingLabels(t *testing.T) {
	app := newApp(t)
	email := app.owner("1203")
	sq := app.squadronID(email)
	c := app.login(email)
	p, _ := app.store.CreateProgramme(sq, 2030, 4, 1)
	cat, err := app.store.AddRequirementCategory(p.ID, "Fieldcraft", "by May")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.AddRequirementEntry(p.ID, cat, "Cdt Example", "note"); err != nil {
		t.Fatal(err)
	}
	if err := app.store.AddClosure(p.ID, "2030-04-08", "2030-04-15", "Easter"); err != nil {
		t.Fatal(err)
	}
	if err := app.store.AddWeekendNote(p.ID, "2030-04-06", "Camp"); err != nil {
		t.Fatal(err)
	}
	page := body(c.get("/programmes/" + strconv.FormatInt(p.ID, 10)))

	for _, want := range []string{
		`aria-label="Delete closure from 2030-04-08 to 2030-04-15"`,
		`aria-label="Delete weekend note for 2030-04-06"`,
		`aria-label="Delete category Fieldcraft"`,
		`aria-label="Remove this entry from Fieldcraft"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("edit page lacks %s", want)
		}
	}
	if strings.Contains(regexp.MustCompile(`aria-label="[^"]*"`).ReplaceAllString(page, ""), ">x</button>") {
		t.Error("an icon-only 'x' button has no accessible name")
	}

	settings := body(c.get("/squadron"))
	for _, want := range []string{`aria-label="Remove ` + email + ` from the squadron"`, `aria-label="Set role for ` + email + `"`} {
		if !strings.Contains(settings, want) {
			t.Errorf("squadron settings lack %s", want)
		}
	}
}
