package gmaps

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrBlocked reports that Google refused the request rather than returning
// results — rate limiting, a CAPTCHA interstitial, or a consent wall.
//
// This is worth separating from an ordinary failure because the two look
// identical from the outside: both end with zero places. Reported as a plain
// empty result, a block reads as "there are no businesses here", which sends
// the user looking for a better search area instead of at the real cause.
var ErrBlocked = errors.New("blocked by google")

// blockSignals are substrings of the page body that only appear on Google's
// anti-automation interstitials. They are matched case-insensitively.
//
// Deliberately narrow: a false positive marks a perfectly good scrape as
// blocked, which is worse than missing a block, since a missed block still
// shows up as a suspicious zero-result job.
var blockSignals = []string{
	"our systems have detected unusual traffic",
	"unusual traffic from your computer network",
	"/sorry/index",
	"recaptcha/api.js",
	"id=\"recaptcha\"",
}

// detectBlock reports why Google refused a response, or nil if it did not.
//
// It looks at three independent tells because no single one is reliable: the
// status code is absent on the browser path when the interstitial is reached
// by redirect, the URL is unchanged when the block arrives as a 429 body, and
// the body is empty when the request never rendered.
func detectBlock(statusCode int, finalURL string, body []byte) error {
	if reason := blockReason(statusCode, finalURL, body); reason != "" {
		return fmt.Errorf("%w: %s", ErrBlocked, reason)
	}

	return nil
}

func blockReason(statusCode int, finalURL string, body []byte) string {
	switch statusCode {
	case http.StatusTooManyRequests:
		return "HTTP 429, rate limited"
	case http.StatusForbidden:
		return "HTTP 403, request refused"
	}

	lowerURL := strings.ToLower(finalURL)

	switch {
	case strings.Contains(lowerURL, "/sorry/"):
		return "redirected to Google's \"unusual traffic\" page"
	case strings.Contains(lowerURL, "consent.google.com"):
		return "redirected to a consent wall"
	}

	if len(body) > 0 {
		lowerBody := bytes.ToLower(body)

		for _, signal := range blockSignals {
			if bytes.Contains(lowerBody, []byte(signal)) {
				return "anti-automation interstitial served instead of results"
			}
		}
	}

	return ""
}

// IsBlocked reports whether err came from Google refusing the request.
func IsBlocked(err error) bool {
	return errors.Is(err, ErrBlocked)
}
