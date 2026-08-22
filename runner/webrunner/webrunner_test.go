//nolint:testpackage // This test needs unexported hooks to avoid running a browser.
package webrunner

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/web"
	"github.com/gosom/scrapemate"
)

func TestScrapeJobMarksOKBeforeClosingMate(t *testing.T) {
	t.Parallel()

	repo := &memoryJobRepo{}
	svc := web.NewService(repo, t.TempDir())
	job := web.Job{
		ID:     "job-1",
		Name:   "coffee",
		Date:   time.Now().UTC(),
		Status: web.StatusPending,
		Data: web.JobData{
			Keywords: []string{"coffee"},
			Lang:     "en",
			Zoom:     15,
			Lat:      "37.7749",
			Lon:      "-122.4194",
			FastMode: true,
			Radius:   1000,
			Depth:    10,
			MaxTime:  time.Minute,
		},
	}

	if err := svc.Create(context.Background(), &job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	w := &webrunner{
		svc: svc,
		cfg: &runner.Config{DataFolder: t.TempDir(), Concurrency: 1},
		setupMate: func(_ context.Context, _ io.Writer, _ *web.Job) (mateRunner, error) {
			return fakeMate{
				onClose: func() {
					got, err := svc.Get(context.Background(), job.ID)
					if err != nil {
						t.Fatalf("get job during close: %v", err)
					}
					if got.Status != web.StatusOK {
						t.Fatalf("status during close = %q, want %q", got.Status, web.StatusOK)
					}
				},
			}, nil
		},
	}

	if err := w.scrapeJob(context.Background(), &job); err != nil {
		t.Fatalf("scrape job: %v", err)
	}
}

// The progress tracker runs concurrently with the scrape and persists the job
// on a ticker, so it must be fully stopped before scrapeJob writes the final
// status. Otherwise a tick still in flight commits the stale "working" status
// afterwards and the job looks stuck at "working" forever in the UI.
func TestScrapeJobWaitsForProgressTrackerBeforeFinishing(t *testing.T) {
	t.Parallel()

	repo := &memoryJobRepo{workingUpdateDelay: 300 * time.Millisecond}
	svc := web.NewService(repo, t.TempDir())
	job := web.Job{
		ID:     "job-progress",
		Name:   "coffee",
		Date:   time.Now().UTC(),
		Status: web.StatusPending,
		Data: web.JobData{
			Keywords: []string{"coffee"},
			Lang:     "en",
			Zoom:     15,
			Lat:      "37.7749",
			Lon:      "-122.4194",
			FastMode: true,
			Radius:   1000,
			Depth:    10,
			MaxTime:  time.Minute,
		},
	}

	if err := svc.Create(context.Background(), &job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	w := &webrunner{
		svc:              svc,
		cfg:              &runner.Config{DataFolder: t.TempDir(), Concurrency: 1},
		progressInterval: time.Millisecond,
		// Report ever-changing progress so every tick actually persists.
		newExiter: func() exiter.Exiter { return &countingExiter{} },
		setupMate: func(_ context.Context, _ io.Writer, _ *web.Job) (mateRunner, error) {
			// Outlive many progress ticks so one is mid-flight at the end.
			return fakeMate{startDelay: 50 * time.Millisecond}, nil
		},
	}

	if err := w.scrapeJob(context.Background(), &job); err != nil {
		t.Fatalf("scrape job: %v", err)
	}

	repo.sealAfterScrape()

	// Give any tracker that outlived scrapeJob time to commit a stale write.
	time.Sleep(600 * time.Millisecond)

	if repo.sawLateUpdate() {
		t.Error("progress tracker wrote the job after scrapeJob returned: it can clobber the final status")
	}

	got, err := svc.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}

	if got.Status != web.StatusOK {
		t.Errorf("final status = %q, want %q", got.Status, web.StatusOK)
	}
}

// scrapemate reports the run as a whole, so a seed job that died mid-scrape
// still leaves mate.Start returning nil. Marking such a job "ok" hands the user
// an empty export with a green status and no hint that anything went wrong.
func TestScrapeJobFailsWhenAllSeedsFailedAndNothingFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exiter   *stubExiter
		expected string
	}{
		{
			name:     "seeds failed with no results",
			exiter:   &stubExiter{failures: 1},
			expected: web.StatusFailed,
		},
		{
			name:     "search genuinely matched nothing",
			exiter:   &stubExiter{},
			expected: web.StatusOK,
		},
		{
			name:     "some seeds failed but places were still found",
			exiter:   &stubExiter{failures: 1, found: 12, completed: 12},
			expected: web.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := web.NewService(&memoryJobRepo{}, t.TempDir())
			job := web.Job{
				ID:     "job-" + tt.name,
				Name:   "companies",
				Date:   time.Now().UTC(),
				Status: web.StatusPending,
				Data: web.JobData{
					Keywords: []string{"companies"},
					Lang:     "fa",
					Zoom:     15,
					Lat:      "35.73",
					Lon:      "51.43",
					Depth:    10,
					MaxTime:  time.Minute,
				},
			}

			if err := svc.Create(context.Background(), &job); err != nil {
				t.Fatalf("create job: %v", err)
			}

			w := &webrunner{
				svc:       svc,
				cfg:       &runner.Config{DataFolder: t.TempDir(), Concurrency: 1},
				newExiter: func() exiter.Exiter { return tt.exiter },
				setupMate: func(_ context.Context, _ io.Writer, _ *web.Job) (mateRunner, error) {
					return fakeMate{}, nil
				},
			}

			if err := w.scrapeJob(context.Background(), &job); err != nil {
				t.Fatalf("scrape job: %v", err)
			}

			got, err := svc.Get(context.Background(), job.ID)
			if err != nil {
				t.Fatalf("get job: %v", err)
			}

			if got.Status != tt.expected {
				t.Errorf("final status = %q, want %q", got.Status, tt.expected)
			}
		})
	}
}

// stubExiter reports fixed progress and failure counts.
type stubExiter struct {
	found     int
	completed int
	failures  int
	blocked   int
}

func (e *stubExiter) SetSeedCount(int)                 {}
func (e *stubExiter) SetCancelFunc(context.CancelFunc) {}
func (e *stubExiter) IncrSeedCompleted(int)            {}
func (e *stubExiter) IncrSeedFailed(int)               {}
func (e *stubExiter) IncrSeedBlocked(int)              {}
func (e *stubExiter) IncrPlacesFound(int)              {}
func (e *stubExiter) IncrPlacesCompleted(int)          {}
func (e *stubExiter) SeedFailures() int                { return e.failures }
func (e *stubExiter) SeedsBlocked() int                { return e.blocked }
func (e *stubExiter) Progress() (int, int)             { return e.found, e.completed }
func (e *stubExiter) Run(ctx context.Context)          { <-ctx.Done() }

type fakeMate struct {
	onClose    func()
	startDelay time.Duration
}

func (m fakeMate) Start(context.Context, ...scrapemate.IJob) error {
	if m.startDelay > 0 {
		time.Sleep(m.startDelay)
	}

	return nil
}

func (m fakeMate) Close() error {
	if m.onClose != nil {
		m.onClose()
	}

	return nil
}

// countingExiter reports a progress value that changes on every read, so the
// tracker always has something new to persist.
type countingExiter struct {
	mu sync.Mutex
	n  int
}

func (e *countingExiter) SetSeedCount(int)                 {}
func (e *countingExiter) SetCancelFunc(context.CancelFunc) {}
func (e *countingExiter) IncrSeedCompleted(int)            {}
func (e *countingExiter) IncrSeedFailed(int)               {}
func (e *countingExiter) IncrSeedBlocked(int)              {}
func (e *countingExiter) IncrPlacesFound(int)              {}
func (e *countingExiter) IncrPlacesCompleted(int)          {}
func (e *countingExiter) SeedFailures() int                { return 0 }
func (e *countingExiter) SeedsBlocked() int                { return 0 }
func (e *countingExiter) Run(ctx context.Context)          { <-ctx.Done() }

func (e *countingExiter) Progress() (int, int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.n++

	return 100, e.n
}

type memoryJobRepo struct {
	mu   sync.Mutex
	jobs map[string]web.Job

	// workingUpdateDelay simulates a slow in-flight progress write: the job is
	// snapshotted when Update is called but committed only after the delay, so
	// a tracker write started before the scrape ended can still land after the
	// final status write.
	workingUpdateDelay time.Duration

	sealed     bool
	lateUpdate bool
}

func (r *memoryJobRepo) Get(_ context.Context, id string) (web.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.jobs[id], nil
}

func (r *memoryJobRepo) Create(_ context.Context, job *web.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.jobs == nil {
		r.jobs = make(map[string]web.Job)
	}

	r.jobs[job.ID] = *job

	return nil
}

func (r *memoryJobRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.jobs, id)

	return nil
}

func (r *memoryJobRepo) Select(_ context.Context, params web.SelectParams) ([]web.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	jobs := make([]web.Job, 0, len(r.jobs))

	for id := range r.jobs {
		job := r.jobs[id]
		if params.Status == "" || job.Status == params.Status {
			jobs = append(jobs, job)
		}
	}

	return jobs, nil
}

func (r *memoryJobRepo) Update(_ context.Context, job *web.Job) error {
	snapshot := *job

	// Only progress writes are slow, so the final status write cannot simply
	// outwait them.
	if r.workingUpdateDelay > 0 && snapshot.Status == web.StatusWorking {
		time.Sleep(r.workingUpdateDelay)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.sealed {
		r.lateUpdate = true
	}

	r.jobs[snapshot.ID] = snapshot

	return nil
}

// sealAfterScrape marks the point where scrapeJob has returned, so any later
// write is recorded as a leak.
func (r *memoryJobRepo) sealAfterScrape() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sealed = true
}

func (r *memoryJobRepo) sawLateUpdate() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lateUpdate
}

func TestScrapeJobReportsBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		blocked     int
		failures    int
		found       int
		wantStatus  string
		wantMessage bool
	}{
		{
			name:        "blocked with nothing collected",
			blocked:     2,
			failures:    2,
			found:       0,
			wantStatus:  web.StatusFailed,
			wantMessage: true,
		},
		{
			// A partial block still succeeds, but the user has to be told the
			// count is a floor rather than the answer.
			name:        "blocked after some results got through",
			blocked:     1,
			failures:    1,
			found:       14,
			wantStatus:  web.StatusOK,
			wantMessage: true,
		},
		{
			name:       "clean run says nothing",
			found:      14,
			wantStatus: web.StatusOK,
		},
		{
			// Failing for some other reason must not be reported as a block,
			// which would send the user after proxies for an unrelated bug.
			name:        "failed for another reason",
			failures:    1,
			found:       0,
			wantStatus:  web.StatusFailed,
			wantMessage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := web.NewService(&memoryJobRepo{}, t.TempDir())
			job := web.Job{
				ID:     "job-" + tt.name,
				Name:   "companies",
				Date:   time.Now().UTC(),
				Status: web.StatusPending,
				Data: web.JobData{
					Keywords: []string{"companies"},
					Lang:     "fa",
					Zoom:     15,
					Lat:      "35.73",
					Lon:      "51.43",
					Depth:    10,
					MaxTime:  time.Minute,
				},
			}

			if err := svc.Create(context.Background(), &job); err != nil {
				t.Fatalf("create job: %v", err)
			}

			w := &webrunner{
				svc: svc,
				cfg: &runner.Config{DataFolder: t.TempDir(), Concurrency: 1},
				setupMate: func(context.Context, io.Writer, *web.Job) (mateRunner, error) {
					return fakeMate{}, nil
				},
				newExiter: func() exiter.Exiter {
					return &stubExiter{found: tt.found, failures: tt.failures, blocked: tt.blocked}
				},
			}

			if err := w.scrapeJob(context.Background(), &job); err != nil {
				t.Fatalf("scrape job: %v", err)
			}

			got, err := svc.Get(context.Background(), job.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}

			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tt.wantStatus)
			}

			if tt.wantMessage && got.Data.Error == "" {
				t.Error("expected an explanation to be recorded")
			}

			if !tt.wantMessage && got.Data.Error != "" {
				t.Errorf("unexpected explanation: %q", got.Data.Error)
			}

			if tt.blocked > 0 && !strings.Contains(got.Data.Error, "blocked") {
				t.Errorf("message %q does not mention the block", got.Data.Error)
			}

			if tt.blocked == 0 && strings.Contains(got.Data.Error, "blocked") {
				t.Errorf("message %q wrongly blames a block", got.Data.Error)
			}
		})
	}
}

func TestBuildSeedJobsGridVsPoint(t *testing.T) {
	t.Parallel()

	newJob := func() *web.Job {
		return &web.Job{
			ID:   "j1",
			Name: "pharmacies",
			Data: web.JobData{
				Keywords: []string{"pharmacy"},
				Lang:     "fa",
				Zoom:     15,
				Depth:    10,
			},
		}
	}

	t.Run("point mode produces one job per keyword", func(t *testing.T) {
		t.Parallel()

		job := newJob()
		job.Data.Lat = "35.73"
		job.Data.Lon = "51.43"

		jobs, err := buildSeedJobs(job, "35.73,51.43", deduper.New(), nil, false)
		if err != nil {
			t.Fatalf("buildSeedJobs: %v", err)
		}

		if len(jobs) != 1 {
			t.Fatalf("got %d seed jobs, want 1", len(jobs))
		}
	})

	t.Run("area mode produces one job per cell", func(t *testing.T) {
		t.Parallel()

		// Roughly 11 km of latitude at 2 km cells: several rows, so the count
		// must be well above the single job point mode would create.
		job := newJob()
		job.Data.BBox = "35.60,51.20,35.70,51.30"
		job.Data.GridCellKm = 2

		jobs, err := buildSeedJobs(job, "", deduper.New(), nil, false)
		if err != nil {
			t.Fatalf("buildSeedJobs: %v", err)
		}

		if len(jobs) <= 1 {
			t.Fatalf("got %d seed jobs, want a grid of several", len(jobs))
		}
	})

	t.Run("smaller cells produce more jobs", func(t *testing.T) {
		t.Parallel()

		coarse := newJob()
		coarse.Data.BBox = "35.60,51.20,35.70,51.30"
		coarse.Data.GridCellKm = 4

		fine := newJob()
		fine.Data.BBox = "35.60,51.20,35.70,51.30"
		fine.Data.GridCellKm = 1

		coarseJobs, err := buildSeedJobs(coarse, "", deduper.New(), nil, false)
		if err != nil {
			t.Fatalf("coarse: %v", err)
		}

		fineJobs, err := buildSeedJobs(fine, "", deduper.New(), nil, false)
		if err != nil {
			t.Fatalf("fine: %v", err)
		}

		if len(fineJobs) <= len(coarseJobs) {
			t.Errorf("1km grid produced %d jobs, 4km produced %d; expected more",
				len(fineJobs), len(coarseJobs))
		}
	})

	t.Run("malformed area is an error, not a silent point search", func(t *testing.T) {
		t.Parallel()

		job := newJob()
		job.Data.BBox = "not-a-box"

		if _, err := buildSeedJobs(job, "", deduper.New(), nil, false); err == nil {
			t.Fatal("expected an error for a malformed bounding box")
		}
	})
}
