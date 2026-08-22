//nolint:testpackage // exercises the unexported isClosed helper alongside the API
package web

import "testing"

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
