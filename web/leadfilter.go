package web

import (
	"strconv"
	"strings"
)

// LeadFilter decides which scraped rows are worth handing to the user.
//
// It is applied when results are read — on download, on the map, and to the
// count shown in the jobs table — never when they are written. The CSV stays
// the complete record of what was scraped, so changing a filter is a re-read
// rather than a re-scrape, and a filter set too aggressively costs nothing to
// undo.
type LeadFilter struct {
	// RequirePhone drops businesses with no phone number. A lead with no way
	// to contact it is a row the user deletes by hand anyway.
	RequirePhone bool

	// ExcludeClosed drops permanently closed businesses.
	ExcludeClosed bool

	// MinReviews drops businesses with fewer than this many reviews. Zero
	// keeps everything.
	MinReviews int
}

// LeadFilter reads the filter the job was created with.
func (d *JobData) LeadFilter() LeadFilter {
	return LeadFilter{
		RequirePhone:  d.RequirePhone,
		ExcludeClosed: d.ExcludeClosed,
		MinReviews:    d.MinReviews,
	}
}

// Active reports whether the filter would drop anything, so callers can skip
// the work entirely for the common unfiltered case.
func (f LeadFilter) Active() bool {
	return f.RequirePhone || f.ExcludeClosed || f.MinReviews > 0
}

// Keep reports whether a row survives the filter. get resolves a column by the
// names in gmaps.Entry.CsvHeaders and must return "" for columns that are
// absent, so the same filter works against rows written by older builds.
func (f LeadFilter) Keep(get func(string) string) bool {
	if f.RequirePhone && strings.TrimSpace(get("phone")) == "" {
		return false
	}

	if f.ExcludeClosed && isClosed(get("status")) {
		return false
	}

	if f.MinReviews > 0 {
		// An unparseable or missing count is treated as zero rather than as a
		// pass: the filter exists to raise the floor, and letting unknowns
		// through would quietly defeat it.
		count, _ := strconv.Atoi(strings.TrimSpace(get("review_count")))
		if count < f.MinReviews {
			return false
		}
	}

	return true
}

// isClosed reports whether Google marked the business permanently closed.
// Open businesses carry an empty status, so only an explicit closed marker
// counts — an unknown value is not evidence of anything.
func isClosed(status string) bool {
	return strings.Contains(strings.ToUpper(strings.TrimSpace(status)), "CLOSED")
}
