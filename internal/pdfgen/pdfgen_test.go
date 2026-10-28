package pdfgen

import (
	"bytes"
	"testing"

	"rafactools-planner/internal/programme"
)

func testSquadron() *programme.Squadron {
	return &programme.Squadron{
		Name:            "Test Squadron",
		Subtitle:        "Test",
		ParadeWeekdays:  []int{1},
		Classifications: []string{"A", "B", "C", "D"},
		UniformColours: []programme.UniformColour{
			{Key: "green", Label: "Green", Hex: "#8faa6e"},
		},
	}
}

func testProgramme(notes string, cadetName string) *programme.FullProgramme {
	return &programme.FullProgramme{
		Programme: programme.Programme{ID: 1, Year: 2026, Month: 4},
		Days: []programme.Day{
			{ID: 1, Date: "2026-04-06", IsParade: true, DayLabel: "M", Halves: 1, Notes: notes,
				Rows: []programme.Row{
					{DayID: 1, Half: 1, RowType: "activity", Texts: []string{"Drill", "Drill", "Drill", "Drill"}},
					{DayID: 1, Half: 1, RowType: "instructor", Texts: []string{"Staff", "Staff", "Staff", "Staff"}},
				}},
		},
		Requirements: []programme.ReqCategory{
			{Name: "Category", DueLabel: "(TBC)", Entries: []programme.ReqEntry{{CadetName: cadetName}}},
		},
	}
}

// TestRenderReflectsCurrentContent guards against a regression where the
// PDF export silently stops reading the programme's actual saved state (see
// TODO.md "Verify PDF export always reflects the current month's saved
// state"). The rendered bytes are flate-compressed, so this can't just grep
// for the text — instead it renders two programmes differing only in
// Notes/requirement-tracker content and checks the output actually differs,
// and that rendering the *same* input twice is deterministic (same bytes).
func TestRenderReflectsCurrentContent(t *testing.T) {
	sq := testSquadron()

	a, err := Render(sq, testProgramme("Original notes", "Cadet A"))
	if err != nil {
		t.Fatalf("Render (a): %v", err)
	}
	b, err := Render(sq, testProgramme("Updated notes", "Cadet B"))
	if err != nil {
		t.Fatalf("Render (b): %v", err)
	}
	if bytes.Equal(a, b) {
		t.Error("Render produced identical output for different Notes/requirement content — PDF export isn't reflecting current data")
	}
	// Not asserting determinism for identical input: fpdf embeds a
	// /CreationDate timestamp, so two renders of the same programme a
	// moment apart legitimately differ in a few bytes — that's not the bug
	// this test guards against.
}

func TestRenderIsSinglePage(t *testing.T) {
	data, err := Render(testSquadron(), testProgramme("notes", "cadet"))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// "/Count N" in the Pages object is the page count in an uncompressed
	// PDF cross-reference; fpdf doesn't compress the document structure,
	// only content streams, so this is a reliable enough check without a
	// full PDF parser dependency.
	if !bytes.Contains(data, []byte("/Count 1")) {
		t.Error("expected a single-page PDF (/Count 1 not found) — see TODO.md \"PDF layout fidelity\"")
	}
}
