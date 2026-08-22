package gmaps

import (
	"errors"
	"testing"
)

func TestDetectBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		url        string
		body       string
		blocked    bool
	}{
		{
			name:       "rate limited",
			statusCode: 429,
			url:        "https://www.google.com/maps/search/cafe",
			blocked:    true,
		},
		{
			name:       "forbidden",
			statusCode: 403,
			blocked:    true,
		},
		{
			name:    "redirected to the sorry page",
			url:     "https://www.google.com/sorry/index?continue=https://maps.google.com/",
			blocked: true,
		},
		{
			name:    "consent wall",
			url:     "https://consent.google.com/m?continue=https://maps.google.com/",
			blocked: true,
		},
		{
			name:    "interstitial in the body",
			body:    `<html><body>Our systems have detected unusual traffic from your computer network.</body></html>`,
			blocked: true,
		},
		{
			name:    "captcha in the body",
			body:    `<html><head><script src="https://www.google.com/recaptcha/api.js"></script></head></html>`,
			blocked: true,
		},
		{
			name:       "ordinary results page",
			statusCode: 200,
			url:        "https://www.google.com/maps/search/cafe/@35.7,51.4,15z",
			body:       `<html><body><div role="feed"><a href="/maps/place/x"></a></div></body></html>`,
		},
		{
			// An empty area is not a block, and treating it as one would send
			// the user chasing proxies for a search that simply matched nothing.
			name:       "genuinely empty results",
			statusCode: 200,
			url:        "https://www.google.com/maps/search/nothinghere/@35.7,51.4,15z",
			body:       `<html><body><div role="feed"></div></body></html>`,
		},
		{
			name: "nothing to go on",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := detectBlock(tt.statusCode, tt.url, []byte(tt.body))

			if tt.blocked {
				if err == nil {
					t.Fatal("expected a block to be detected")
				}

				if !errors.Is(err, ErrBlocked) {
					t.Fatalf("error %v does not wrap ErrBlocked", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected block: %v", err)
			}
		})
	}
}

func TestIsBlocked(t *testing.T) {
	t.Parallel()

	if IsBlocked(nil) {
		t.Error("nil is not a block")
	}

	if IsBlocked(errors.New("some other failure")) {
		t.Error("an unrelated error is not a block")
	}

	if !IsBlocked(detectBlock(429, "", nil)) {
		t.Error("a detected block should report as one")
	}
}

func TestBlockReasonIsSpecific(t *testing.T) {
	t.Parallel()

	// The message reaches the user in the jobs table, so it has to say which
	// of the several block shapes happened, not just "blocked".
	got := detectBlock(429, "", nil).Error()
	want := "blocked by google: HTTP 429, rate limited"

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
