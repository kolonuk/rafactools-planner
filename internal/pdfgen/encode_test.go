package pdfgen

import (
	"testing"

	"github.com/go-pdf/fpdf"
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
