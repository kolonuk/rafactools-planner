package programme

import (
	"errors"
	"testing"
)

func TestEditingUniformColours(t *testing.T) {
	st, _ := newStore(t)
	sq, _ := st.GetSquadron(1)
	if len(sq.UniformColours) != 5 {
		t.Fatalf("%d default colours", len(sq.UniformColours))
	}
	var plain, closure UniformColour
	for _, c := range sq.UniformColours {
		if c.Key == "grey" {
			closure = c
		} else if plain.Key == "" {
			plain = c
		}
	}
	edit := func(fn func([]UniformColour) []UniformColour) error {
		cur, _ := st.GetSquadron(1)
		return st.SetUniformColours(1, fn(append([]UniformColour(nil), cur.UniformColours...)))
	}

	t.Run("rename and recolour keep the key, so nights follow", func(t *testing.T) {
		p := mustProg(t, st, 1, 2030, 4)
		fp, _ := st.GetFullProgramme(1, p.ID)
		var d Day
		for _, x := range fp.Days {
			if x.IsParade {
				d = x
				break
			}
		}
		if err := st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: d.ID, IsParade: true, UniformKey: plain.Key}}); err != nil {
			t.Fatal(err)
		}
		if err := edit(func(cs []UniformColour) []UniformColour {
			for i := range cs {
				if cs[i].Key == plain.Key {
					cs[i].Label, cs[i].Hex = "Mess dress", "#112233"
				}
			}
			return cs
		}); err != nil {
			t.Fatal(err)
		}
		got, _ := st.GetSquadron(1)
		found := false
		for _, c := range got.UniformColours {
			if c.Key == plain.Key && c.Label == "Mess dress" && c.Hex == "#112233" {
				found = true
			}
		}
		if !found {
			t.Errorf("colours = %+v", got.UniformColours)
		}
		if n, _ := st.UniformColoursInUse(1); n[plain.Key] != 1 {
			t.Errorf("in use = %v", n)
		}
		// A colour a night wears cannot be removed.
		err := edit(func(cs []UniformColour) []UniformColour {
			var out []UniformColour
			for _, c := range cs {
				if c.Key != plain.Key {
					out = append(out, c)
				}
			}
			return out
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("removing a colour in use: %v", err)
		}
	})

	t.Run("a new colour gets its own key, an unused one can go, Closure stays", func(t *testing.T) {
		if err := edit(func(cs []UniformColour) []UniformColour {
			return append(cs, UniformColour{Label: "Mess dress", Hex: "#AA00BB"})
		}); err == nil {
			t.Error("two colours with one name were accepted")
		}
		if err := edit(func(cs []UniformColour) []UniformColour {
			return append(cs, UniformColour{Label: "Flying suit", Hex: "#AA00BB"}, UniformColour{Label: "Flying-suit", Hex: "#000000"})
		}); err != nil {
			t.Fatal(err)
		}
		got, _ := st.GetSquadron(1)
		n := len(got.UniformColours)
		a, b := got.UniformColours[n-2], got.UniformColours[n-1]
		if a.Key != "flying-suit" || a.Hex != "#aa00bb" || b.Key != "flying-suit-2" {
			t.Errorf("new colours = %+v %+v", a, b)
		}
		if err := edit(func(cs []UniformColour) []UniformColour { return cs[:len(cs)-2] }); err != nil {
			t.Errorf("removing unused colours: %v", err)
		}
		if err := edit(func(cs []UniformColour) []UniformColour {
			var out []UniformColour
			for _, c := range cs {
				if c.Key != closure.Key {
					out = append(out, c)
				}
			}
			return out
		}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("removing the Closure colour: %v", err)
		}
	})

	t.Run("bad values are refused", func(t *testing.T) {
		for name, fn := range map[string]func([]UniformColour) []UniformColour{
			"bad hex":    func(cs []UniformColour) []UniformColour { cs[0].Hex = "red"; return cs },
			"blank name": func(cs []UniformColour) []UniformColour { cs[0].Label = " "; return cs },
			"none":       func(cs []UniformColour) []UniformColour { return nil },
			"too long":   func(cs []UniformColour) []UniformColour { cs[0].Label = "01234567890123456789012345678901"; return cs },
		} {
			if err := edit(fn); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
}
