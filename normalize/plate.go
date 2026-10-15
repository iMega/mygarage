package normalize

import (
	"strings"
	"unicode"
)

func Plate(s string) string {
	s = strings.ToUpper(s)
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}
