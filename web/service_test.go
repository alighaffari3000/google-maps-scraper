//nolint:testpackage // shares the internal web test package with web_test.go
package web

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeCSV(t *testing.T, dir, id, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, id+".csv"), []byte(content), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}
}

func TestGetPlacesParsesCSV(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(nil, dir)

	csv := "title,address,latitude,longitude,link,category,phone,website,review_rating\n" +
		"Coffee Place,1 Main St,37.7749,-122.4194,http://maps/1,cafe,555,http://web,4.5\n"
	writeCSV(t, dir, "job-1", csv)

	places, err := svc.GetPlaces(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("GetPlaces: %v", err)
	}

	if len(places) != 1 {
		t.Fatalf("expected 1 place, got %d", len(places))
	}

	p := places[0]
	if p.Title != "Coffee Place" || p.Latitude != 37.7749 || p.Longitude != -122.4194 {
		t.Fatalf("unexpected place: %+v", p)
	}

	if p.ReviewRating != 4.5 {
		t.Fatalf("unexpected rating: %v", p.ReviewRating)
	}
}

func TestGetPlacesSkipsRowsWithoutCoords(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(nil, dir)

	csv := "title,latitude,longitude\n" +
		"No Coords,,\n" +
		"Zero,0,0\n" +
		"Bad,abc,def\n" +
		"Good,1.5,2.5\n"
	writeCSV(t, dir, "job-2", csv)

	places, err := svc.GetPlaces(context.Background(), "job-2")
	if err != nil {
		t.Fatalf("GetPlaces: %v", err)
	}

	if len(places) != 1 {
		t.Fatalf("expected 1 place, got %d", len(places))
	}

	if places[0].Title != "Good" {
		t.Fatalf("unexpected place: %+v", places[0])
	}
}

func TestGetPlacesSkipsNonFiniteAndOutOfRangeCoords(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(nil, dir)

	csv := "title,latitude,longitude,review_rating\n" +
		"NaN,NaN,2.5,3\n" +
		"Inf,Inf,2.5,3\n" +
		"OutOfRange,91,200,3\n" +
		"BadRating,1.5,2.5,NaN\n" +
		"Good,1.5,2.5,4.5\n"
	writeCSV(t, dir, "job-nf", csv)

	places, err := svc.GetPlaces(context.Background(), "job-nf")
	if err != nil {
		t.Fatalf("GetPlaces: %v", err)
	}

	if len(places) != 2 {
		t.Fatalf("expected 2 places (BadRating + Good), got %d: %+v", len(places), places)
	}

	for _, p := range places {
		if p.Title == "BadRating" && p.ReviewRating != 0 {
			t.Fatalf("non-finite rating should be sanitized to 0, got %v", p.ReviewRating)
		}
	}
}

func TestGetDownloadPrefersXLSXOverCSV(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(nil, dir)

	writeCSV(t, dir, "job-both", "title\nA\n")
	if err := os.WriteFile(filepath.Join(dir, "job-both.xlsx"), []byte("fake xlsx"), 0o600); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	got, err := svc.GetDownload(context.Background(), "job-both")
	if err != nil {
		t.Fatalf("GetDownload: %v", err)
	}

	if filepath.Ext(got) != ".xlsx" {
		t.Fatalf("expected xlsx to be preferred, got %q", got)
	}
}

func TestGetDownloadFallsBackToCSV(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(nil, dir)

	writeCSV(t, dir, "job-csv-only", "title\nA\n")

	got, err := svc.GetDownload(context.Background(), "job-csv-only")
	if err != nil {
		t.Fatalf("GetDownload: %v", err)
	}

	if filepath.Ext(got) != ".csv" {
		t.Fatalf("expected csv fallback, got %q", got)
	}
}

func TestGetDownloadMissingFile(t *testing.T) {
	svc := NewService(nil, t.TempDir())

	if _, err := svc.GetDownload(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for missing output")
	}
}

func TestDeleteRemovesBothCSVAndXLSX(t *testing.T) {
	dir := t.TempDir()
	repo := &memoryJobRepoForServiceTest{}
	svc := NewService(repo, dir)

	writeCSV(t, dir, "job-del", "title\nA\n")

	xlsxPath := filepath.Join(dir, "job-del.xlsx")
	if err := os.WriteFile(xlsxPath, []byte("fake xlsx"), 0o600); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	if err := svc.Delete(context.Background(), "job-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "job-del.csv")); !os.IsNotExist(err) {
		t.Fatal("csv file should have been removed")
	}

	if _, err := os.Stat(xlsxPath); !os.IsNotExist(err) {
		t.Fatal("xlsx file should have been removed")
	}
}

type memoryJobRepoForServiceTest struct{}

func (r *memoryJobRepoForServiceTest) Get(context.Context, string) (Job, error) { return Job{}, nil }
func (r *memoryJobRepoForServiceTest) Create(context.Context, *Job) error       { return nil }
func (r *memoryJobRepoForServiceTest) Delete(context.Context, string) error     { return nil }
func (r *memoryJobRepoForServiceTest) Select(context.Context, SelectParams) ([]Job, error) {
	return nil, nil
}
func (r *memoryJobRepoForServiceTest) Update(context.Context, *Job) error { return nil }

func TestGetPlacesMissingCSV(t *testing.T) {
	svc := NewService(nil, t.TempDir())

	if _, err := svc.GetPlaces(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for missing csv")
	}
}

func TestGetPlacesRejectsTraversal(t *testing.T) {
	svc := NewService(nil, t.TempDir())

	if _, err := svc.GetPlaces(context.Background(), "../etc/passwd"); err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestResultCount(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(nil, dir)

	writeCSV(t, dir, "empty", "title,phone\n")
	writeCSV(t, dir, "two", "title,phone\nA,1\nB,2\n")
	// open_hours and reviews legitimately contain newlines, so a record is
	// not the same thing as a line.
	writeCSV(t, dir, "multiline", "title,open_hours\nA,\"Mon\nTue\"\nB,\"Wed\nThu\"\n")

	tests := []struct {
		name string
		id   string
		want int
	}{
		{"header only", "empty", 0},
		{"two rows", "two", 2},
		{"embedded newlines", "multiline", 2},
		{"no csv at all", "missing", 0},
		{"path traversal", "../../etc/passwd", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.ResultCount(tt.id); got != tt.want {
				t.Fatalf("ResultCount(%q) = %d, want %d", tt.id, got, tt.want)
			}
		})
	}
}
