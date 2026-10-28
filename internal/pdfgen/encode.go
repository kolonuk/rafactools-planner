package pdfgen

import "reflect"

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
