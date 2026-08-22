//nolint:testpackage // exercises the unexported isClosed helper alongside the API
package web

import (
	"testing"

	"github.com/gosom/google-maps-scraper/grid"
)

func row(cols map[string]string) func(string) string {
	return func(name string) string { return cols[name] }
}

func TestLeadFilterKeep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter LeadFilter
		cols   map[string]string
		keep   bool
	}{
		{
			name: "no filter keeps everything",
			cols: map[string]string{"title": "A place"},
			keep: true,
		},
		{
			name:   "phone required and present",
			filter: LeadFilter{RequirePhone: true},
			cols:   map[string]string{"phone": "+98 21 8884 9410"},
			keep:   true,
		},
		{
			name:   "phone required and missing",
			filter: LeadFilter{RequirePhone: true},
			cols:   map[string]string{"phone": ""},
		},
		{
			name:   "phone required and only whitespace",
			filter: LeadFilter{RequirePhone: true},
			cols:   map[string]string{"phone": "   "},
		},
		{
			// Open businesses carry an empty status, so the common case must
			// survive an exclude-closed filter.
			name:   "open business survives exclude closed",
			filter: LeadFilter{ExcludeClosed: true},
			cols:   map[string]string{"status": ""},
			keep:   true,
		},
		{
			name:   "closed business is dropped",
			filter: LeadFilter{ExcludeClosed: true},
			cols:   map[string]string{"status": "CLOSED"},
		},
		{
			name:   "closed detection is case insensitive",
			filter: LeadFilter{ExcludeClosed: true},
			cols:   map[string]string{"status": "Permanently closed"},
		},
		{
			name:   "review floor met",
			filter: LeadFilter{MinReviews: 10},
			cols:   map[string]string{"review_count": "10"},
			keep:   true,
		},
		{
			name:   "review floor not met",
			filter: LeadFilter{MinReviews: 10},
			cols:   map[string]string{"review_count": "9"},
		},
		{
			// A missing count must not slip through a floor the user set.
			name:   "missing review count counts as zero",
			filter: LeadFilter{MinReviews: 1},
			cols:   map[string]string{},
		},
		{
			name:   "all filters together",
			filter: LeadFilter{RequirePhone: true, ExcludeClosed: true, MinReviews: 5},
			cols:   map[string]string{"phone": "021 1234 5678", "status": "", "review_count": "12"},
			keep:   true,
		},
		{
			name:   "one failing condition is enough to drop",
			filter: LeadFilter{RequirePhone: true, ExcludeClosed: true, MinReviews: 5},
			cols:   map[string]string{"phone": "021 1234 5678", "status": "CLOSED", "review_count": "12"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.filter.Keep(row(tt.cols)); got != tt.keep {
				t.Errorf("Keep() = %v, want %v", got, tt.keep)
			}
		})
	}
}

func TestLeadFilterActive(t *testing.T) {
	t.Parallel()

	if (LeadFilter{}).Active() {
		t.Error("an empty filter is not active")
	}

	for _, f := range []LeadFilter{
		{RequirePhone: true},
		{ExcludeClosed: true},
		{MinReviews: 1},
	} {
		if !f.Active() {
			t.Errorf("%+v should be active", f)
		}
	}
}

func TestNormalizeIranPhone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"+98 21 8884 9410", "+982188849410"},
		{"021 8884 9410", "+982188849410"},
		{"02188849410", "+982188849410"},
		{"0912 345 6789", "+989123456789"},
		{"+989123456789", "+989123456789"},
		{"00989123456789", "+989123456789"},
		{"(021) 8884-9410", "+982188849410"},
		{"+98 021 8884 9410", "+982188849410"},
		{"", ""},

		// Left alone: not identifiable as Iranian, and guessing would produce
		// a number that looks valid but dials somewhere else.
		{"+1 555 123 4567", "+1 555 123 4567"},
		{"+44 20 7946 0958", "+44 20 7946 0958"},
		{"12345", "12345"},
		{"not a phone", "not a phone"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			if got := NormalizeIranPhone(tt.in); got != tt.want {
				t.Errorf("NormalizeIranPhone(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLeadFilterBoundingBox(t *testing.T) {
	t.Parallel()

	box := grid.BoundingBox{MinLat: 35.79, MinLon: 51.43, MaxLat: 35.81, MaxLon: 51.46}
	filter := LeadFilter{BBox: &box}

	tests := []struct {
		name     string
		lat, lon string
		keep     bool
	}{
		{name: "well inside", lat: "35.80", lon: "51.44", keep: true},
		{name: "on the south-west corner", lat: "35.79", lon: "51.43", keep: true},
		{name: "on the north-east corner", lat: "35.81", lon: "51.46", keep: true},
		{name: "just north", lat: "35.8101", lon: "51.44"},
		{name: "just south", lat: "35.7899", lon: "51.44"},
		{name: "just east", lat: "35.80", lon: "51.4601"},
		{name: "just west", lat: "35.80", lon: "51.4299"},
		{name: "far away", lat: "35.70", lon: "51.33"},

		// Unreadable coordinates cannot be shown to satisfy the area the user
		// asked for, so they are excluded rather than let through.
		{name: "missing coordinates", lat: "", lon: ""},
		{name: "unparseable coordinates", lat: "n/a", lon: "n/a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := filter.Keep(row(map[string]string{
				"latitude":  tt.lat,
				"longitude": tt.lon,
			}))

			if got != tt.keep {
				t.Errorf("Keep(%q,%q) = %v, want %v", tt.lat, tt.lon, got, tt.keep)
			}
		})
	}
}

func TestJobDataLeadFilterParsesBBox(t *testing.T) {
	t.Parallel()

	// Area restriction is opt-in, so this asserts the parsing rather than the
	// default; TestJobDataAreaRestrictionIsOptIn covers the default.
	d := JobData{BBox: "35.79,51.43,35.81,51.46", RestrictToArea: true}

	f := d.LeadFilter()
	if f.BBox == nil {
		t.Fatal("expected the bounding box to be parsed")
	}

	if !f.Active() {
		t.Error("a filter with a bounding box is active")
	}

	// A job with no area must not gain one, or every point-mode job would
	// suddenly start dropping rows.
	if (&JobData{}).LeadFilter().BBox != nil {
		t.Error("a job without an area must not get a box")
	}
}

func TestLeadFilterRadius(t *testing.T) {
	t.Parallel()

	// Tehran centre, 5 km.
	filter := LeadFilter{CentreLat: 35.6892, CentreLon: 51.3890, RadiusMeters: 5000}

	tests := []struct {
		name     string
		lat, lon string
		keep     bool
	}{
		{name: "the centre itself", lat: "35.6892", lon: "51.3890", keep: true},
		{name: "about 2 km north", lat: "35.7072", lon: "51.3890", keep: true},
		{name: "about 8 km north", lat: "35.7612", lon: "51.3890"},
		{name: "another district entirely", lat: "35.8020", lon: "51.4485"},
		{name: "unreadable position", lat: "", lon: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := filter.Keep(row(map[string]string{"latitude": tt.lat, "longitude": tt.lon}))
			if got != tt.keep {
				t.Errorf("Keep(%q,%q) = %v, want %v", tt.lat, tt.lon, got, tt.keep)
			}
		})
	}
}

func TestJobDataLeadFilterRadius(t *testing.T) {
	t.Parallel()

	t.Run("point mode picks up the radius", func(t *testing.T) {
		t.Parallel()

		f := (&JobData{Lat: "35.6892", Lon: "51.3890", Radius: 5000, RestrictToArea: true}).LeadFilter()
		if f.RadiusMeters != 5000 {
			t.Errorf("RadiusMeters = %v, want 5000", f.RadiusMeters)
		}
	})

	t.Run("area mode ignores the radius", func(t *testing.T) {
		t.Parallel()

		// A grid has no single centre; constraining to the box and to a radius
		// around one arbitrary point would double-filter.
		f := (&JobData{
			BBox:           "35.79,51.43,35.81,51.46",
			Lat:            "35.6892",
			Lon:            "51.3890",
			Radius:         5000,
			RestrictToArea: true,
		}).LeadFilter()

		if f.RadiusMeters != 0 {
			t.Errorf("RadiusMeters = %v, want 0 for an area job", f.RadiusMeters)
		}

		if f.BBox == nil {
			t.Error("expected the box to be kept")
		}
	})

	t.Run("no coordinates means no radius filter", func(t *testing.T) {
		t.Parallel()

		if f := (&JobData{Radius: 5000, RestrictToArea: true}).LeadFilter(); f.RadiusMeters != 0 {
			t.Errorf("RadiusMeters = %v, want 0 without a centre", f.RadiusMeters)
		}
	})
}

func TestHaversineMeters(t *testing.T) {
	t.Parallel()

	// Tehran to Isfahan is about 340 km; a few percent of slack is plenty to
	// catch a formula that is wrong rather than merely imprecise.
	got := haversineMeters(35.6892, 51.3890, 32.6546, 51.6680)

	if got < 330_000 || got > 350_000 {
		t.Errorf("Tehran-Isfahan = %.0f m, want roughly 340000", got)
	}

	if d := haversineMeters(35.6892, 51.3890, 35.6892, 51.3890); d != 0 {
		t.Errorf("distance to itself = %v, want 0", d)
	}
}

func TestJobDataAreaRestrictionIsOptIn(t *testing.T) {
	t.Parallel()

	area := JobData{BBox: "35.79,51.43,35.81,51.46"}
	point := JobData{Lat: "35.6892", Lon: "51.3890", Radius: 5000}

	t.Run("area is kept whole by default", func(t *testing.T) {
		t.Parallel()

		// Google's extra results are real businesses matching the keyword;
		// discarding them without being asked would lose leads silently.
		if f := area.LeadFilter(); f.BBox != nil {
			t.Error("a box must not be enforced unless the user asked")
		}

		if f := point.LeadFilter(); f.RadiusMeters != 0 {
			t.Error("a radius must not be enforced unless the user asked")
		}
	})

	t.Run("opting in enforces the box", func(t *testing.T) {
		t.Parallel()

		d := area
		d.RestrictToArea = true

		if f := d.LeadFilter(); f.BBox == nil {
			t.Error("expected the box to be enforced")
		}
	})

	t.Run("opting in enforces the radius", func(t *testing.T) {
		t.Parallel()

		d := point
		d.RestrictToArea = true

		if f := d.LeadFilter(); f.RadiusMeters != 5000 {
			t.Errorf("RadiusMeters = %v, want 5000", f.RadiusMeters)
		}
	})

	t.Run("other filters are unaffected", func(t *testing.T) {
		t.Parallel()

		d := area
		d.RequirePhone = true

		f := d.LeadFilter()
		if !f.RequirePhone || !f.Active() {
			t.Error("lead filters must work independently of the area opt-in")
		}
	})
}
