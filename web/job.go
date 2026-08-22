package web

import (
	"context"
	"errors"
	"time"

	"github.com/gosom/google-maps-scraper/grid"
)

var jobs []Job

const (
	StatusPending = "pending"
	StatusWorking = "working"
	StatusOK      = "ok"
	StatusFailed  = "failed"
)

type SelectParams struct {
	Status string
	Limit  int
}

type JobRepository interface {
	Get(context.Context, string) (Job, error)
	Create(context.Context, *Job) error
	Delete(context.Context, string) error
	Select(context.Context, SelectParams) ([]Job, error)
	Update(context.Context, *Job) error
}

type Job struct {
	ID     string
	Name   string
	Date   time.Time
	Status string
	Data   JobData
}

// JobView decorates a Job with information that lives outside the database.
// Results is the number of places actually written to the job's CSV, which the
// stored progress counters can't answer for jobs that finished before those
// counters existed. Job is embedded, so templates keep reaching .ID, .Name and
// .Data unchanged.
type JobView struct {
	Job

	Results int
}

func (j *Job) Validate() error {
	if j.ID == "" {
		return errors.New("missing id")
	}

	if j.Name == "" {
		return errors.New("missing name")
	}

	if j.Status == "" {
		return errors.New("missing status")
	}

	if j.Date.IsZero() {
		return errors.New("missing date")
	}

	if err := j.Data.Validate(); err != nil {
		return err
	}

	return nil
}

type JobData struct {
	Keywords     []string      `json:"keywords"`
	Lang         string        `json:"lang"`
	Zoom         int           `json:"zoom"`
	Lat          string        `json:"lat"`
	Lon          string        `json:"lon"`
	FastMode     bool          `json:"fast_mode"`
	Radius       int           `json:"radius"`
	Depth        int           `json:"depth"`
	Email        bool          `json:"email"`
	ExtraReviews bool          `json:"extra_reviews"`
	MaxTime      time.Duration `json:"max_time"`
	Proxies      []string      `json:"proxies"`

	// PlacesFound and PlacesCompleted are runtime progress counters,
	// updated periodically while the job is working so the UI can show progress.
	PlacesFound     int `json:"places_found"`
	PlacesCompleted int `json:"places_completed"`

	// BBox, when non-empty, switches the job from a single search at Lat/Lon
	// to a grid of searches covering a rectangle, formatted as
	// "minLat,minLon,maxLat,maxLon". Google caps results per search, not per
	// area, so covering a neighbourhood properly means many small searches
	// rather than one big one.
	BBox string `json:"bbox,omitempty"`

	// GridCellKm is the side length of each grid cell in kilometres. Ignored
	// unless BBox is set; zero means the grid package's own default.
	GridCellKm float64 `json:"grid_cell_km,omitempty"`

	// Lead filters, applied when results are read rather than written: the
	// CSV keeps every row, so changing these is a re-download rather than a
	// re-scrape. See LeadFilter.
	RequirePhone  bool `json:"require_phone,omitempty"`
	ExcludeClosed bool `json:"exclude_closed,omitempty"`
	MinReviews    int  `json:"min_reviews,omitempty"`

	// NormalizePhones rewrites Iranian numbers into E.164 in the downloaded
	// workbook, so a sheet mixing +98/0/00 prefixes can be dialled from and
	// imported without hand-editing.
	NormalizePhones bool `json:"normalize_phones,omitempty"`

	// Error, when non-empty, explains why a job did not produce what the user
	// expected. It is set for outcomes that a bare "failed" status cannot
	// distinguish — being rate limited by Google above all, which otherwise
	// looks exactly like searching an area with no businesses in it.
	Error string `json:"error,omitempty"`

	// Fields, when non-empty, restricts the output CSV to these column names
	// (matching gmaps.Entry.CsvHeaders()). Empty means "all columns", which
	// keeps existing jobs and API/CLI callers unaffected.
	Fields []string `json:"fields"`
}

func (d *JobData) Validate() error {
	if len(d.Keywords) == 0 {
		return errors.New("missing keywords")
	}

	if d.Lang == "" {
		return errors.New("missing lang")
	}

	if len(d.Lang) != 2 {
		return errors.New("invalid lang")
	}

	if d.Depth == 0 {
		return errors.New("missing depth")
	}

	if d.MaxTime == 0 {
		return errors.New("missing max time")
	}

	if d.BBox != "" {
		if _, err := grid.ParseBoundingBox(d.BBox); err != nil {
			return err
		}

		if d.GridCellKm < 0 {
			return errors.New("grid cell size cannot be negative")
		}

		// A grid supplies its own coordinates per cell, so Lat/Lon are unused
		// and the FastMode check below would reject a perfectly valid job.
		return nil
	}

	if d.FastMode && (d.Lat == "" || d.Lon == "") {
		return errors.New("missing geo coordinates")
	}

	return nil
}
