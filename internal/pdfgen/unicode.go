package pdfgen

import (
	"reflect"
	"strings"
	"sync"

	"github.com/go-pdf/fpdf"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// Text outside Western European (Greek, Cyrillic and so on) cannot be shown by the
// PDF's built-in fonts. When a month holds any, the page is drawn with the Go fonts
// (Latin, Greek, Cyrillic and more) registered under the same name, so none of the
// drawing code changes. A character the Go fonts lack (Chinese, say) is still a "?".

// unicodeDocs holds the documents being drawn with the Unicode fonts.
var unicodeDocs sync.Map

var goRegular, goBold *truetype.Font

func init() {
	var err error
	if goRegular, err = truetype.Parse(goregular.TTF); err != nil {
		panic("pdfgen: " + err.Error())
	}
	if goBold, err = truetype.Parse(gobold.TTF); err != nil {
		panic("pdfgen: " + err.Error())
	}
}

// useUnicodeFont registers the Go fonts as "Helvetica", regular and bold, ahead of the
// built-in one of that name.
func useUnicodeFont(pdf *fpdf.Fpdf) {
	pdf.AddUTF8FontFromBytes("Helvetica", "", goregular.TTF)
	pdf.AddUTF8FontFromBytes("Helvetica", "B", gobold.TTF)
}

// unicodeText keeps what the Go fonts can show, drops the invisible parts of emoji
// and turns each run of anything else into one "?".
func unicodeText(s string) string {
	var b strings.Builder
	lastUnknown := false
	for _, r := range s {
		switch {
		case r == 0x200D || (r >= 0xFE00 && r <= 0xFE0F) || (r >= 0x1F3FB && r <= 0x1F3FF) || r == 0x20E3:
			continue
		case r == '\n' || r == '\r' || r == '\t' || r == ' ':
			lastUnknown = false
			b.WriteRune(r)
		case goRegular.Index(r) == 0 || goBold.Index(r) == 0:
			if !lastUnknown {
				b.WriteByte('?')
			}
			lastUnknown = true
		default:
			lastUnknown = false
			b.WriteRune(r)
		}
	}
	return b.String()
}

// needsUnicode reports whether any text of the squadron or month holds a character the
// built-in fonts cannot show but the Go fonts can.
func needsUnicode(sq any, fp any, core func(string) string) bool {
	need := false
	check := func(s string) string {
		if !need && core(s) != s {
			for _, r := range s {
				if r > 0xFF && goRegular.Index(r) != 0 && goBold.Index(r) != 0 && core(string(r)) != string(r) {
					need = true
					break
				}
			}
		}
		return s
	}
	for _, v := range []any{sq, fp} {
		walkStrings(reflect.ValueOf(v), check)
	}
	return need
}

// walkStrings calls f on every string inside v.
func walkStrings(v reflect.Value, f func(string) string) {
	switch v.Kind() {
	case reflect.String:
		f(v.String())
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			walkStrings(v.Elem(), f)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkStrings(v.Index(i), f)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			walkStrings(v.MapIndex(k), f)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				walkStrings(v.Field(i), f)
			}
		}
	}
}
