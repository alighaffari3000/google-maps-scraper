package webrunner

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/google-maps-scraper/grid"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/tlmt"
	"github.com/gosom/google-maps-scraper/web"
	"github.com/gosom/google-maps-scraper/web/sqlite"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/adapters/writers/csvwriter"
	"github.com/gosom/scrapemate/scrapemateapp"
	"golang.org/x/sync/errgroup"
)

type webrunner struct {
	srv       *web.Server
	svc       *web.Service
	cfg       *runner.Config
	setupMate func(context.Context, io.Writer, *web.Job) (mateRunner, error)

	// newExiter builds the per-job exit monitor. Nil means exiter.New.
	newExiter func() exiter.Exiter

	// progressInterval is how often job progress is persisted while scraping.
	// Zero means defaultProgressInterval.
	progressInterval time.Duration
}

const defaultProgressInterval = 2 * time.Second

type mateRunner interface {
	Start(context.Context, ...scrapemate.IJob) error
	Close() error
}

func New(cfg *runner.Config) (runner.Runner, error) {
	if cfg.DataFolder == "" {
		return nil, fmt.Errorf("data folder is required")
	}

	if err := os.MkdirAll(cfg.DataFolder, os.ModePerm); err != nil {
		return nil, err
	}

	const dbfname = "jobs.db"

	dbpath := filepath.Join(cfg.DataFolder, dbfname)

	repo, err := sqlite.New(dbpath)
	if err != nil {
		return nil, err
	}

	svc := web.NewService(repo, cfg.DataFolder)

	srv, err := web.New(svc, cfg.Addr)
	if err != nil {
		return nil, err
	}

	ans := webrunner{
		srv:       srv,
		svc:       svc,
		cfg:       cfg,
		setupMate: defaultSetupMate(cfg),
	}

	return &ans, nil
}

func (w *webrunner) Run(ctx context.Context) error {
	egroup, ctx := errgroup.WithContext(ctx)

	egroup.Go(func() error {
		return w.work(ctx)
	})

	egroup.Go(func() error {
		return w.srv.Start(ctx)
	})

	return egroup.Wait()
}

func (w *webrunner) Close(context.Context) error {
	return nil
}

func (w *webrunner) work(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			jobs, err := w.svc.SelectPending(ctx)
			if err != nil {
				return err
			}

			for i := range jobs {
				select {
				case <-ctx.Done():
					return nil
				default:
					t0 := time.Now().UTC()
					if err := w.scrapeJob(ctx, &jobs[i]); err != nil {
						params := map[string]any{
							"job_count": len(jobs[i].Data.Keywords),
							"duration":  time.Now().UTC().Sub(t0).String(),
							"error":     err.Error(),
						}

						evt := tlmt.NewEvent("web_runner", params)

						_ = runner.Telemetry().Send(ctx, evt)

						log.Printf("error scraping job %s: %v", jobs[i].ID, err)
					} else {
						params := map[string]any{
							"job_count": len(jobs[i].Data.Keywords),
							"duration":  time.Now().UTC().Sub(t0).String(),
						}

						_ = runner.Telemetry().Send(ctx, tlmt.NewEvent("web_runner", params))

						log.Printf("job %s scraped successfully", jobs[i].ID)
					}
				}
			}
		}
	}
}

// trackProgress periodically copies the exit monitor's counters onto the job
// and persists them, so the web UI (which polls /jobs) can show live progress.
// It returns once ctx is done. It writes job without synchronisation, so the
// caller must not touch job until this has returned.
func (w *webrunner) trackProgress(ctx context.Context, job *web.Job, exitMonitor exiter.Exiter) {
	interval := w.progressInterval
	if interval <= 0 {
		interval = defaultProgressInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			found, completed := exitMonitor.Progress()

			if found == job.Data.PlacesFound && completed == job.Data.PlacesCompleted {
				continue
			}

			job.Data.PlacesFound = found
			job.Data.PlacesCompleted = completed

			if err := w.svc.Update(ctx, job); err != nil {
				log.Printf("failed to update job progress for %s: %v", job.ID, err)
			}
		}
	}
}

func (w *webrunner) scrapeJob(ctx context.Context, job *web.Job) error {
	job.Status = web.StatusWorking

	err := w.svc.Update(ctx, job)
	if err != nil {
		return err
	}

	if len(job.Data.Keywords) == 0 {
		job.Status = web.StatusFailed

		return w.svc.Update(ctx, job)
	}

	outpath := filepath.Join(w.cfg.DataFolder, job.ID+".csv")

	// Runs last (defers execute LIFO, and this is registered before outfile's
	// and mate's): by the time it fires, the CSV is fully closed on disk and
	// job.Status already reflects the outcome the caller committed to the DB.
	defer func() {
		if job.Status != web.StatusOK {
			return
		}

		xlsxPath := strings.TrimSuffix(outpath, ".csv") + ".xlsx"

		opts := exportOptions{
			Fields:          job.Data.Fields,
			Filter:          job.Data.LeadFilter(),
			NormalizePhones: job.Data.NormalizePhones,
		}

		if err := exportXLSX(outpath, xlsxPath, opts); err != nil {
			log.Printf("failed to export xlsx for job %s: %v", job.ID, err)
		}
	}()

	outfile, err := os.Create(outpath)
	if err != nil {
		return err
	}

	defer func() {
		_ = outfile.Close()
	}()

	setupMate := w.setupMate
	if setupMate == nil {
		setupMate = defaultSetupMate(w.cfg)
	}

	mate, err := setupMate(ctx, outfile, job)
	if err != nil {
		job.Status = web.StatusFailed

		err2 := w.svc.Update(ctx, job)
		if err2 != nil {
			log.Printf("failed to update job status: %v", err2)
		}

		return err
	}

	defer mate.Close()

	var coords string
	if job.Data.Lat != "" && job.Data.Lon != "" {
		coords = job.Data.Lat + "," + job.Data.Lon
	}

	dedup := deduper.New()

	newExiter := w.newExiter
	if newExiter == nil {
		newExiter = exiter.New
	}

	exitMonitor := newExiter()

	seedJobs, err := buildSeedJobs(job, coords, dedup, exitMonitor,
		w.cfg.ExtraReviews || job.Data.ExtraReviews)
	if err != nil {
		err2 := w.svc.Update(ctx, job)
		if err2 != nil {
			log.Printf("failed to update job status: %v", err2)
		}

		return err
	}

	if len(seedJobs) > 0 {
		exitMonitor.SetSeedCount(len(seedJobs))

		allowedSeconds := max(60, len(seedJobs)*10*job.Data.Depth/50+120)

		if job.Data.MaxTime > 0 {
			if job.Data.MaxTime.Seconds() < 180 {
				allowedSeconds = 180
			} else {
				allowedSeconds = int(job.Data.MaxTime.Seconds())
			}
		}

		log.Printf("running job %s with %d seed jobs and %d allowed seconds", job.ID, len(seedJobs), allowedSeconds)

		mateCtx, cancel := context.WithTimeout(ctx, time.Duration(allowedSeconds)*time.Second)
		defer cancel()

		exitMonitor.SetCancelFunc(cancel)

		go exitMonitor.Run(mateCtx)

		// The progress tracker owns job for as long as it runs, so cancel()
		// alone is not enough to touch job again: it only signals, and a tick
		// already in flight would persist the stale "working" status over the
		// final one. Wait for the tracker to exit before writing job again.
		progressDone := make(chan struct{})

		go func() {
			defer close(progressDone)

			w.trackProgress(mateCtx, job, exitMonitor)
		}()

		err = mate.Start(mateCtx, seedJobs...)

		cancel()
		<-progressDone

		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			job.Status = web.StatusFailed

			err2 := w.svc.Update(ctx, job)
			if err2 != nil {
				log.Printf("failed to update job status: %v", err2)
			}

			return err
		}

		// mate.Start reports the run, not the individual searches: a seed job
		// that died (Google redirecting mid-scroll, a blocked request) leaves
		// it nil. Reporting such a run as ok hands the user an empty file and
		// a green status, so only failed seeds with nothing to show for them
		// mark the job failed — a search that genuinely matched nothing still
		// succeeded.
		found, _ := exitMonitor.Progress()

		// Being refused by Google is reported even when some results did get
		// through: a partial block still means the output is short, and the
		// user needs to know the number is a floor rather than the answer.
		if blocked := exitMonitor.SeedsBlocked(); blocked > 0 {
			log.Printf("job %s: google blocked %d search(es)", job.ID, blocked)

			job.Data.Error = blockedMessage(blocked, found)
		}

		if failures := exitMonitor.SeedFailures(); failures > 0 && found == 0 {
			log.Printf("job %s: all %d seed job(s) failed, no places found", job.ID, failures)

			if job.Data.Error == "" {
				job.Data.Error = "Every search failed and nothing was collected. Check the scraper logs for the cause."
			}

			job.Status = web.StatusFailed

			return w.svc.Update(ctx, job)
		}

		job.Data.PlacesFound, job.Data.PlacesCompleted = exitMonitor.Progress()
	}

	job.Status = web.StatusOK

	return w.svc.Update(ctx, job)
}

// buildSeedJobs turns a job into the searches that will be run: one per
// keyword, or one per (keyword, grid cell) when the user drew an area instead
// of picking a point. The grid path exists because Google caps results per
// search rather than per area — covering a neighbourhood means many small
// searches, and the shared deduper stitches the overlapping cells back
// together.
func buildSeedJobs(
	job *web.Job,
	coords string,
	dedup deduper.Deduper,
	exitMonitor exiter.Exiter,
	extraReviews bool,
) ([]scrapemate.IJob, error) {
	keywords := strings.NewReader(strings.Join(job.Data.Keywords, "\n"))

	if job.Data.BBox != "" {
		bbox, err := grid.ParseBoundingBox(job.Data.BBox)
		if err != nil {
			return nil, err
		}

		cellKm := job.Data.GridCellKm
		if cellKm <= 0 {
			cellKm = defaultGridCellKm
		}

		return runner.CreateGridSeedJobs(
			job.Data.Lang,
			keywords,
			job.Data.Depth,
			job.Data.Email,
			bbox,
			cellKm,
			job.Data.Zoom,
			dedup,
			exitMonitor,
			extraReviews,
		)
	}

	return runner.CreateSeedJobs(
		job.Data.FastMode,
		job.Data.Lang,
		keywords,
		job.Data.Depth,
		job.Data.Email,
		coords,
		job.Data.Zoom,
		func() float64 {
			if job.Data.Radius <= 0 {
				return 10000 // 10 km
			}

			return float64(job.Data.Radius)
		}(),
		dedup,
		exitMonitor,
		extraReviews,
	)
}

// defaultGridCellKm mirrors the web form's default so a job stored without an
// explicit cell size still grids the same way the user saw it described.
const defaultGridCellKm = 1.0

// blockedMessage phrases a block for someone looking at the jobs table, who
// needs to know whether to retry, wait, or reach for proxies — not which
// internal counter tripped.
func blockedMessage(blocked, found int) string {
	const advice = " Google rate-limits datacenter IPs; wait a while before retrying, " +
		"or add residential proxies in the Proxies field."

	if found == 0 {
		return fmt.Sprintf("Google blocked %d search(es) and no results were collected.%s", blocked, advice)
	}

	return fmt.Sprintf("Google blocked %d search(es), so these %d result(s) are incomplete.%s", blocked, found, advice)
}

func defaultSetupMate(cfg *runner.Config) func(context.Context, io.Writer, *web.Job) (mateRunner, error) {
	return func(_ context.Context, writer io.Writer, job *web.Job) (mateRunner, error) {
		opts := []func(*scrapemateapp.Config) error{
			scrapemateapp.WithConcurrency(cfg.Concurrency),
			scrapemateapp.WithExitOnInactivity(time.Minute * 3),
		}

		if !job.Data.FastMode {
			opts = append(opts,
				scrapemateapp.WithJS(scrapemateapp.DisableImages()),
			)
		} else {
			opts = append(opts,
				scrapemateapp.WithStealth("firefox"),
			)
		}

		opts = runner.AppendBrowserCapacityOptions(opts, cfg)

		hasProxy := false

		if len(cfg.Proxies) > 0 {
			opts = append(opts, scrapemateapp.WithProxies(cfg.Proxies))
			hasProxy = true
		} else if len(job.Data.Proxies) > 0 {
			opts = append(opts,
				scrapemateapp.WithProxies(job.Data.Proxies),
			)
			hasProxy = true
		}

		if !cfg.DisablePageReuse {
			opts = append(opts,
				scrapemateapp.WithPageReuseLimit(2),
				scrapemateapp.WithBrowserReuseLimit(200),
			)
		}

		log.Printf("job %s has proxy: %v", job.ID, hasProxy)

		// The CSV always holds every column: the "View" map depends on
		// latitude/longitude regardless of what the user chose to export,
		// and exportXLSX applies job.Data.Fields when it converts this file
		// to the .xlsx the user actually downloads.
		writers := []scrapemate.ResultWriter{csvwriter.NewCsvWriter(csv.NewWriter(writer))}

		matecfg, err := scrapemateapp.NewConfig(
			writers,
			opts...,
		)
		if err != nil {
			return nil, err
		}

		return scrapemateapp.NewScrapeMateApp(matecfg)
	}
}
