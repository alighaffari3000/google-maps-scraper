package web

import (
	"math"
	"strconv"
	"strings"

	"github.com/gosom/google-maps-scraper/grid"
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

	// BBox, when set, drops businesses outside the rectangle the user drew.
	//
	// This is not redundant with running a grid over that rectangle. Each
	// cell is a Google Maps search centred on the cell, and Google answers
	// from a viewport far wider than the cell — in one real run only 25 of
	// 146 results fell inside the drawn box, some strays 17 km away. The
	// search cannot be constrained, so the results have to be.
	BBox *grid.BoundingBox

	// Centre and RadiusMeters drop businesses further than RadiusMeters from
	// the picked point. This is the point-mode counterpart of BBox, and has
	// the same cause: the form has always had a Radius field, but outside
	// fast mode nothing ever enforced it, so results arrived from wherever
	// Google felt like answering from.
	CentreLat, CentreLon float64
	RadiusMeters         float64
}

// LeadFilter reads the filter the job was created with.
func (d *JobData) LeadFilter() LeadFilter {
	filter := LeadFilter{
		RequirePhone:  d.RequirePhone,
		ExcludeClosed: d.ExcludeClosed,
		MinReviews:    d.MinReviews,
	}

	// Area constraints are opt-in: see JobData.RestrictToArea.
	if !d.RestrictToArea {
		return filter
	}

	if d.BBox != "" {
		if box, err := grid.ParseBoundingBox(d.BBox); err == nil {
			filter.BBox = &box
		}

		// An area job has no single centre; the box is the constraint.
		return filter
	}

	lat, errLat := strconv.ParseFloat(strings.TrimSpace(d.Lat), 64)
	lon, errLon := strconv.ParseFloat(strings.TrimSpace(d.Lon), 64)

	if d.Radius > 0 && errLat == nil && errLon == nil {
		filter.CentreLat = lat
		filter.CentreLon = lon
		filter.RadiusMeters = float64(d.Radius)
	}

	return filter
}

// Active reports whether the filter would drop anything, so callers can skip
// the work entirely for the common unfiltered case.
func (f LeadFilter) Active() bool {
	return f.RequirePhone || f.ExcludeClosed || f.MinReviews > 0 ||
		f.BBox != nil || f.RadiusMeters > 0
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

	if f.BBox != nil || f.RadiusMeters > 0 {
		lat, lon, ok := coordinates(get("latitude"), get("longitude"))

		// A place whose position cannot be read has not been shown to be in
		// the area the user asked for, so it is excluded rather than assumed.
		if !ok {
			return false
		}

		if f.BBox != nil && !insideBox(*f.BBox, lat, lon) {
			return false
		}

		if f.RadiusMeters > 0 &&
			haversineMeters(f.CentreLat, f.CentreLon, lat, lon) > f.RadiusMeters {
			return false
		}
	}

	return true
}

func coordinates(latStr, lonStr string) (lat, lon float64, ok bool) {
	lat, errLat := strconv.ParseFloat(strings.TrimSpace(latStr), 64)
	lon, errLon := strconv.ParseFloat(strings.TrimSpace(lonStr), 64)

	return lat, lon, errLat == nil && errLon == nil
}

// haversineMeters is the great-circle distance between two points, mirroring
// gmaps.Entry.haversineDistance, which fast mode already filters by.
func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusM = 6371e3

	rad := func(deg float64) float64 { return deg * math.Pi / 180 }

	dLat := rad(lat2 - lat1)
	dLon := rad(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)

	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// insideBox reports whether coordinates fall within the drawn rectangle.
func insideBox(box grid.BoundingBox, lat, lon float64) bool {
	return lat >= box.MinLat && lat <= box.MaxLat &&
		lon >= box.MinLon && lon <= box.MaxLon
}

// isClosed reports whether Google marked the business permanently closed.
// Open businesses carry an empty status, so only an explicit closed marker
// counts — an unknown value is not evidence of anything.
func isClosed(status string) bool {
	return strings.Contains(strings.ToUpper(strings.TrimSpace(status)), "CLOSED")
}
