package calendar

import (
	"regexp"

	"github.com/imega/mygarage/normalize"
)

var platePatterns = []*regexp.Regexp{
	// Ukraine: AA 1700 YE.
	regexp.MustCompile(`(?i)\b[A-Z]{2}[ -]?[0-9]{4}[ -]?[A-Z]{2}\b`),
	// France SIV: WW-412-DX.
	regexp.MustCompile(`(?i)\b[A-Z]{2}[ -]?[0-9]{3}[ -]?[A-Z]{2}\b`),
	// France FNI: 7149 VV 33.
	regexp.MustCompile(`(?i)\b[0-9]{1,4}[ -]+[A-Z]{1,3}[ -]+[0-9]{2,3}\b`),
	// United Kingdom: LK53 WUO.
	regexp.MustCompile(`(?i)\b[A-Z]{2}[0-9]{2}[ -]?[A-Z]{3}\b`),
	// Germany: KLE QH 426, F LF 24.
	regexp.MustCompile(`(?i)\b[A-Z]{1,3}[ -]+[A-Z]{1,2}[ -]+[0-9]{1,4}\b`),
	// Cyprus and Greece: ABC123, AXI-8995.
	regexp.MustCompile(`(?i)\b[A-Z]{2,3}[ -]?[0-9]{3,4}\b`),
}

func plateFromTitle(title string) string {
	for _, pattern := range platePatterns {
		if plate := pattern.FindString(title); plate != "" {
			return normalize.Plate(plate)
		}
	}
	return ""
}
