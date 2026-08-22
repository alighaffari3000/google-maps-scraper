package web

import "strings"

// NormalizeIranPhone rewrites an Iranian phone number into E.164
// ("+982188849410"), the form bulk SMS and CRM imports expect. Google returns
// the same number as "+98 21 8884 9410", "021 8884 9410" or "0912 345 6789"
// depending on the listing, and a spreadsheet mixing all three cannot be
// dialled from or imported without hand-editing.
//
// Anything that is not recognisably an Iranian number is returned untouched.
// A wrong number is worse than an unformatted one, so the rules below only
// fire on shapes that can be identified with certainty.
func NormalizeIranPhone(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}

	var digits strings.Builder

	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}

	d := digits.String()
	if d == "" {
		return raw
	}

	const (
		iranCC = "98"
		// Iranian subscriber numbers are 10 digits: a 2-4 digit area or
		// mobile prefix plus the line number.
		nationalLen = 10
	)

	switch {
	case strings.HasPrefix(d, "00"+iranCC):
		d = d[len("00"+iranCC):]
	case strings.HasPrefix(trimmed, "+") && strings.HasPrefix(d, iranCC):
		d = d[len(iranCC):]
	case strings.HasPrefix(d, "0"):
		d = d[1:]
	}

	// A leading zero can survive the +98 strip in "+98 021 ..." style inputs.
	d = strings.TrimPrefix(d, "0")

	if len(d) != nationalLen {
		return raw
	}

	return "+" + iranCC + d
}
