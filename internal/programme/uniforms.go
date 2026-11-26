package programme

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// A squadron's uniform colours: the names and colours offered for a night's uniform
// and printed in the key at the foot of the PDF.

const (
	maxUniformColours = 12
	maxUniformLabel   = 30
)

var hexColourRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// uniformKey makes a key from a label: lower case letters and digits, joined by dashes.
func uniformKey(label string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(label) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		} else {
			dash = true
		}
	}
	if b.Len() == 0 {
		return "colour"
	}
	return b.String()
}

// UniformColoursInUse counts the nights, across the squadron's months, that wear each colour key.
func (s *Store) UniformColoursInUse(squadronID int64) (map[string]int, error) {
	rows, err := s.db.Query(`SELECT d.uniform_key, COUNT(*) FROM programme_days d JOIN programmes p ON p.id = d.programme_id
		WHERE p.squadron_id = ? AND d.uniform_key <> '' GROUP BY d.uniform_key`, squadronID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// SetUniformColours replaces the squadron's colours. Entries with an empty Key are new
// and get one made from their label. A colour that is left out is removed, unless a night
// still wears it or it is the "Closure" colour the PDF's key needs.
func (s *Store) SetUniformColours(squadronID int64, colours []UniformColour) error {
	if len(colours) == 0 || len(colours) > maxUniformColours {
		return fmt.Errorf("%w: a squadron needs between 1 and %d uniform colours", ErrInvalidInput, maxUniformColours)
	}
	sq, err := s.GetSquadron(squadronID)
	if err != nil {
		return err
	}
	inUse, err := s.UniformColoursInUse(squadronID)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	labels := map[string]bool{}
	keys := map[string]bool{}
	out := make([]UniformColour, 0, len(colours))
	for _, c := range colours {
		label, err := cleanField("a colour's name", c.Label, maxUniformLabel)
		if err != nil {
			return err
		}
		if label == "" {
			return fmt.Errorf("%w: every colour needs a name", ErrInvalidInput)
		}
		if !hexColourRe.MatchString(c.Hex) {
			return fmt.Errorf("%w: %q is not a colour like #1a2b3c", ErrInvalidInput, c.Hex)
		}
		if labels[strings.ToLower(label)] {
			return fmt.Errorf("%w: two colours are called %q", ErrInvalidInput, label)
		}
		labels[strings.ToLower(label)] = true
		key := c.Key
		if key == "" {
			key = uniformKey(label)
			for base, n := key, 2; keys[key] || existsKey(sq.UniformColours, key); n++ {
				key = fmt.Sprintf("%s-%d", base, n)
			}
		}
		keys[key] = true
		keep[key] = true
		out = append(out, UniformColour{Key: key, Label: label, Hex: strings.ToLower(c.Hex)})
	}
	for _, old := range sq.UniformColours {
		if keep[old.Key] {
			continue
		}
		if old.Key == "grey" || strings.EqualFold(strings.TrimSpace(old.Label), "closure") {
			return fmt.Errorf("%w: the Closure colour is needed for the key on the PDF", ErrInvalidInput)
		}
		if n := inUse[old.Key]; n > 0 {
			return fmt.Errorf("%w: %q is worn on %d night(s), so it can't be removed yet", ErrInvalidInput, old.Label, n)
		}
	}
	j, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE squadrons SET uniform_colours = ? WHERE id = ?`, string(j), squadronID)
	return err
}

func existsKey(cs []UniformColour, key string) bool {
	for _, c := range cs {
		if c.Key == key {
			return true
		}
	}
	return false
}
