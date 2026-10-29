package pdfgen

import (
	"reflect"
	"strings"
)

// mapStrings returns a deep copy of v (a pointer to a struct, as passed to
// Render) with f applied to every string it holds. The built-in PDF fonts are
// single-byte (Windows-1252), so user text has to be re-encoded before it
// reaches a cell; doing it once over the whole data keeps every drawing call
// unchanged and cannot be forgotten for a new field.
func mapStrings[T any](v *T, f func(string) string) *T {
	out := new(T)
	copyMapped(reflect.ValueOf(out).Elem(), reflect.ValueOf(v).Elem(), f)
	return out
}

func copyMapped(dst, src reflect.Value, f func(string) string) {
	switch src.Kind() {
	case reflect.String:
		dst.SetString(f(src.String()))
	case reflect.Struct:
		for i := 0; i < src.NumField(); i++ {
			if dst.Field(i).CanSet() {
				copyMapped(dst.Field(i), src.Field(i), f)
			}
		}
	case reflect.Slice:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeSlice(src.Type(), src.Len(), src.Len()))
		for i := 0; i < src.Len(); i++ {
			copyMapped(dst.Index(i), src.Index(i), f)
		}
	case reflect.Map:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeMapWithSize(src.Type(), src.Len()))
		for _, k := range src.MapKeys() {
			nv := reflect.New(src.Type().Elem()).Elem()
			copyMapped(nv, src.MapIndex(k), f)
			dst.SetMapIndex(k, nv)
		}
	case reflect.Pointer:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.New(src.Type().Elem()))
		copyMapped(dst.Elem(), src.Elem(), f)
	default:
		dst.Set(src)
	}
}

// byteRunes turns each byte of a Windows-1252 string (what Render's text has
// been re-encoded to) into the rune with the same value. SplitText measures
// by rune and indexes the font's 256-entry width table with it, so it needs
// that form; the bytes themselves are what the cells draw.
func byteRunes(s string) string {
	r := make([]rune, len(s))
	for i := 0; i < len(s); i++ {
		r[i] = rune(s[i])
	}
	return string(r)
}

// pdfText makes user text safe for the PDF's built-in font: it keeps whatever
// that font can show (accents, curly quotes, dashes), drops the invisible parts
// of emoji (joiners, variation selectors, skin-tone modifiers), and turns each
// run of anything else it cannot show (an emoji, a Greek or Chinese word) into
// a single "?" rather than one per character.
func pdfText(tr func(string) string) func(string) string {
	return func(s string) string {
		var b strings.Builder
		lastUnknown := false
		for _, r := range s {
			switch {
			case r == 0x200D || (r >= 0xFE00 && r <= 0xFE0F) || (r >= 0x1F3FB && r <= 0x1F3FF) || r == 0x20E3:
				continue // invisible parts of emoji sequences
			case r == '\n' || r == '\r' || r == '\t' || r == ' ':
				lastUnknown = false
				b.WriteString(tr(string(r)))
			default:
				out := tr(string(r))
				if r != '.' && out == "." { // the translator shows what it cannot map as a full stop
					if !lastUnknown {
						b.WriteByte('?')
					}
					lastUnknown = true
					continue
				}
				lastUnknown = false
				b.WriteString(out)
			}
		}
		return b.String()
	}
}
