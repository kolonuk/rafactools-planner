package auth

import "testing"

func TestParseClaimEmail(t *testing.T) {
	const domain = "rafac.mod.gov.uk"

	cases := []struct {
		email      string
		wantNumber string
		wantOK     bool
	}{
		{"snco.2491@rafac.mod.gov.uk", "2491", true},
		{"oc.2491@rafac.mod.gov.uk", "2491", true},
		{"SNCO.2491@RAFAC.MOD.GOV.UK", "2491", true}, // case-insensitive
		{"oc.2491@gmail.com", "", false},             // wrong domain
		{"cadet.2491@rafac.mod.gov.uk", "", false},   // wrong prefix
		{"snco@rafac.mod.gov.uk", "", false},         // no number
		{"snco.2491", "", false},                     // no domain at all
		{"not-an-email", "", false},
	}

	for _, c := range cases {
		number, ok := ParseClaimEmail(c.email, domain)
		if ok != c.wantOK || number != c.wantNumber {
			t.Errorf("ParseClaimEmail(%q, %q) = (%q, %v), want (%q, %v)",
				c.email, domain, number, ok, c.wantNumber, c.wantOK)
		}
	}
}
