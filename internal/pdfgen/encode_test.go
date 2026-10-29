package pdfgen

import (
	"bytes"
	"testing"

	"github.com/go-pdf/fpdf"

	"rafactools-planner/internal/programme"
)

func TestMapStringsCopiesDeeplyAndLeavesInputAlone(t *testing.T) {
	sq := testSquadron()
	sq.Name = "O’Brien — Sqn"
	fp := testProgramme()
	fp.Days[1].Rows = append(fp.Days[1].Rows, fp.Days[0].Rows...)
	fp.Days[0].Notes = "café"

	tr := fpdf.New("P", "mm", "A4", "").UnicodeTranslatorFromDescriptor("")
	gotSq, gotFp := mapStrings(sq, tr), mapStrings(fp, tr)

	if want := "O\x92Brien \x97 Sqn"; gotSq.Name != want {
		t.Errorf("name = %q, want %q (Windows-1252 bytes)", gotSq.Name, want)
	}
	if gotFp.Days[0].Notes != "caf\xe9" {
		t.Errorf("notes = %q, want caf\\xe9", gotFp.Days[0].Notes)
	}
	if sq.Name != "O’Brien — Sqn" || fp.Days[0].Notes != "café" {
		t.Error("the caller's data was modified")
	}
	if len(gotFp.Days) != len(fp.Days) || len(gotSq.Classifications) != 4 || gotSq.Classifications[1] != "1st Class" {
		t.Error("structure not preserved")
	}
	gotSq.Classifications[0] = "changed"
	if sq.Classifications[0] == "changed" {
		t.Error("slices are shared with the caller's data")
	}
}

func TestRenderAcceptsTypographicAndForeignText(t *testing.T) {
	sq := testSquadron()
	sq.Name = "“Quoted” – Squadron"
	fp := testProgramme()
	fp.Days[0].Notes = "Zoë’s café – 日本語"
	if len(render(t, sq, fp)) == 0 {
		t.Fatal("empty PDF")
	}
}

func TestPDFTextKeepsAccentsAndCollapsesEmojiAndOtherScripts(t *testing.T) {
	text := pdfText(fpdf.New("P", "mm", "A4", "").UnicodeTranslatorFromDescriptor(""))
	for in, want := range map[string]string{
		"Zoë’s café – ok":      "Zo\xebs caf\xe9 \x96 ok"[:0] + "Zo\xeb\x92s caf\xe9 \x96 ok",
		"Fun 😀🎉 night":         "Fun ? night",
		"Family 👨‍👩‍👧 here":    "Family ? here",
		"Thumbs 👍🏽 up":         "Thumbs ? up",
		"Red heart ❤️ and 1️⃣": "Red heart ? and 1",
		"日本語 and Ελληνικά":     "? and ?",
		"what?? really?":       "what?? really?",
		"line one\nline two 😀": "line one\nline two ?",
		"":                     "",
		"🎉🎉🎉":                  "?",
		"a\u200db":             "ab",
	} {
		if got := text(in); got != want {
			t.Errorf("pdfText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmojiInEveryKindOfTextRendersWithoutBreakingThePDF(t *testing.T) {
	fun := "Fun 😀 night 🎉 Zoë 👨‍👩‍👧 👍🏽"
	sq := testSquadron()
	sq.Name, sq.Subtitle = fun, fun
	sq.Classifications = []string{fun, "1st 😀", "Leading", "Senior"}
	fp := testProgramme()
	for i := range fp.Days {
		fp.Days[i].Notes = fun
		for j := range fp.Days[i].Rows {
			for k := range fp.Days[i].Rows[j].Texts {
				fp.Days[i].Rows[j].Texts[k] = fun
			}
		}
	}
	fp.Closures = []programme.Closure{{ID: 1, StartDate: "2030-02-04", EndDate: "2030-02-04", Message: fun}}
	fp.WeekendNotes = []programme.WeekendNote{{Date: "2030-02-02", Text: fun}}
	fp.Requirements = []programme.ReqCategory{{Name: fun, DueLabel: fun, Entries: []programme.ReqEntry{{CadetName: fun, Note: fun}}}}
	out := render(t, sq, fp)
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("no PDF produced")
	}
	// Not one stray byte of UTF-8 emoji (0xF0 ...) reaches the file as text.
	compressOutput = false
	defer func() { compressOutput = true }()
	raw := render(t, sq, fp)
	if bytes.Contains(raw, []byte("\xf0\x9f")) {
		t.Error("raw emoji bytes were written into the PDF")
	}
}
