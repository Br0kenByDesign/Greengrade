package names

import "testing"

func TestBreeder(t *testing.T) {
	for in, want := range map[string]string{
		"Bathera":                "bathera",
		"bathera GmbH":           "bathera",
		"Bathera G.m.b.H.":       "bathera",
		"Aurora GmbH & Co. KG":   "aurora",
		"Barney's Farm":          "barneys farm",
		"Barneys Farm":           "barneys farm",
		"Barney’s  Farm":         "barneys farm",
		"Four 20 Pharma":         "four 20 pharma",
		"AG":                     "ag",
		"Royal Queen Seeds B.V.": "royal queen seeds",
		"  ":                     "",
	} {
		if got := Breeder(in); got != want {
			t.Errorf("Breeder(%q) = %q, want %q", in, got, want)
		}
	}
}
