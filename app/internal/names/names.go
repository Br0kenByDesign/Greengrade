// Package names normalises strain and breeder names so that spelling variants match.
package names

import (
	"strings"
	"unicode"
)

// Strain makes "Gorilla Glue #4", "gorilla-glue 4" and "GORILLA GLUE 4" the same strain.
func Strain(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		} else {
			space = true
		}
	}
	return b.String()
}

// Legal forms at the end of a company name. "Bathera GmbH" and "Bathera" are the same breeder.
var legalForms = map[string]bool{
	"gmbh": true, "mbh": true, "ag": true, "ug": true, "kg": true, "ohg": true, "gbr": true, "ev": true,
	"co": true, "ltd": true, "limited": true, "inc": true, "llc": true, "corp": true, "plc": true,
	"bv": true, "nv": true, "sa": true, "se": true, "sl": true, "srl": true, "spa": true, "aps": true, "ab": true, "as": true,
}

// Breeder normalises manufacturer and breeder names: case, punctuation and apostrophes are ignored
// ("Barney's Farm" = "Barneys Farm"), and trailing legal forms are dropped ("Aurora GmbH & Co. KG" = "Aurora").
func Breeder(s string) string {
	s = strings.NewReplacer("'", "", "’", "", "`", "", "´", "", ".", "").Replace(s)
	words := strings.Fields(Strain(s))
	for len(words) > 1 && legalForms[words[len(words)-1]] {
		words = words[:len(words)-1]
	}
	return strings.Join(words, " ")
}
