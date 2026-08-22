package web

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Service struct {
	repo       JobRepository
	dataFolder string
}

func NewService(repo JobRepository, dataFolder string) *Service {
	return &Service{
		repo:       repo,
		dataFolder: dataFolder,
	}
}

func (s *Service) Create(ctx context.Context, job *Job) error {
	return s.repo.Create(ctx, job)
}

func (s *Service) All(ctx context.Context) ([]Job, error) {
	return s.repo.Select(ctx, SelectParams{})
}

// AllWithResults returns every job together with how many results it produced.
// Counting is per-job file I/O, so it belongs here rather than in the template.
func (s *Service) AllWithResults(ctx context.Context) ([]JobView, error) {
	jobs, err := s.All(ctx)
	if err != nil {
		return nil, err
	}

	views := make([]JobView, len(jobs))

	for i, job := range jobs {
		views[i] = JobView{Job: job, Results: s.ResultCount(job.ID)}
	}

	return views, nil
}

// ResultCount reports how many places a job wrote to its CSV. The file is the
// source of truth: it is what Download and View are built from, and unlike the
// stored progress counters it is also correct for jobs scraped by older builds.
// A job with no CSV yet (pending, failed, deleted output) counts as zero.
func (s *Service) ResultCount(id string) int {
	path, err := s.csvPath(id)
	if err != nil {
		return 0
	}

	f, err := os.Open(path)
	if err != nil {
		return 0
	}

	defer func() {
		_ = f.Close()
	}()

	// Read as CSV rather than counting lines: fields such as open_hours and
	// reviews contain embedded newlines, so one record is not one line.
	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true

	count := -1 // the header row is not a result

	for {
		if _, err := reader.Read(); err != nil {
			break
		}

		count++
	}

	return max(count, 0)
}

func (s *Service) Get(ctx context.Context, id string) (Job, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	csvPath, err := s.csvPath(id)
	if err != nil {
		return err
	}

	if err := removeIfExists(csvPath); err != nil {
		return err
	}

	xlsxPath, err := s.xlsxPath(id)
	if err != nil {
		return err
	}

	if err := removeIfExists(xlsxPath); err != nil {
		return err
	}

	return s.repo.Delete(ctx, id)
}

func removeIfExists(path string) error {
	if _, err := os.Stat(path); err == nil {
		return os.Remove(path)
	} else if !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (s *Service) Update(ctx context.Context, job *Job) error {
	return s.repo.Update(ctx, job)
}

func (s *Service) SelectPending(ctx context.Context) ([]Job, error) {
	return s.repo.Select(ctx, SelectParams{Status: StatusPending, Limit: 1})
}

// csvPath returns the on-disk path of a job's CSV output, rejecting ids that
// could escape the data folder.
func (s *Service) csvPath(id string) (string, error) {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid file name")
	}

	return filepath.Join(s.dataFolder, id+".csv"), nil
}

func (s *Service) GetCSV(_ context.Context, id string) (string, error) {
	datapath, err := s.csvPath(id)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(datapath); os.IsNotExist(err) {
		return "", fmt.Errorf("csv file not found for job %s", id)
	}

	return datapath, nil
}

// xlsxPath returns the on-disk path of a job's XLSX export, rejecting ids
// that could escape the data folder.
func (s *Service) xlsxPath(id string) (string, error) {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid file name")
	}

	return filepath.Join(s.dataFolder, id+".xlsx"), nil
}

// GetDownload returns the on-disk path users should download for a job,
// preferring the XLSX export when one exists (it renders non-Latin text like
// Persian reliably, unlike CSV opened in Excel) and falling back to the raw
// CSV for jobs that predate the XLSX export or whose conversion failed.
func (s *Service) GetDownload(_ context.Context, id string) (string, error) {
	xlsxPath, err := s.xlsxPath(id)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(xlsxPath); err == nil {
		return xlsxPath, nil
	}

	csvPath, err := s.csvPath(id)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		return "", fmt.Errorf("output file not found for job %s", id)
	}

	return csvPath, nil
}
