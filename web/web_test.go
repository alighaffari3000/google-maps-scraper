//nolint:testpackage // tests unexported handlers (viewJob, requestWithID, securityHeaders) directly
package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, dir string) *Server {
	t.Helper()

	srv, err := New(NewService(nil, dir), ":0")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return srv
}

func TestViewJobRendersPlaces(t *testing.T) {
	dir := t.TempDir()
	id := "11111111-1111-1111-1111-111111111111"

	csv := "title,latitude,longitude\nPlace,1.5,2.5\n"
	if err := os.WriteFile(filepath.Join(dir, id+".csv"), []byte(csv), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	srv := newTestServer(t, dir)

	req := requestWithID(httptest.NewRequest(http.MethodGet, "/view?id="+id, http.NoBody))
	rec := httptest.NewRecorder()
	srv.viewJob(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{`id="map-modal"`, `initJobMap()`, `"title":"Place"`, `"latitude":1.5`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestViewJobEmptyState(t *testing.T) {
	srv := newTestServer(t, t.TempDir())

	id := "22222222-2222-2222-2222-222222222222"
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/view?id="+id, http.NoBody))
	rec := httptest.NewRecorder()
	srv.viewJob(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "var places = [];") {
		t.Fatalf("expected empty places array, got:\n%s", body)
	}
}

func TestViewJobInvalidID(t *testing.T) {
	srv := newTestServer(t, t.TempDir())

	req := requestWithID(httptest.NewRequest(http.MethodGet, "/view?id=not-a-uuid", http.NoBody))
	rec := httptest.NewRecorder()
	srv.viewJob(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
}

func TestSecurityHeadersAllowMapResources(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"tile.openstreetmap.org", "cdnjs.cloudflare.com"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP missing %q: %s", want, csp)
		}
	}
}

func TestJobDataValidateBBox(t *testing.T) {
	t.Parallel()

	base := func() JobData {
		return JobData{
			Keywords: []string{"pharmacy"},
			Lang:     "fa",
			Depth:    10,
			MaxTime:  time.Minute,
		}
	}

	t.Run("valid area", func(t *testing.T) {
		t.Parallel()

		d := base()
		d.BBox = "35.60,51.20,35.80,51.50"
		d.GridCellKm = 1

		if err := d.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("malformed area is rejected", func(t *testing.T) {
		t.Parallel()

		d := base()
		d.BBox = "35.60,51.20,35.80"

		if err := d.Validate(); err == nil {
			t.Fatal("expected an error for a three-corner box")
		}
	})

	t.Run("negative cell size is rejected", func(t *testing.T) {
		t.Parallel()

		d := base()
		d.BBox = "35.60,51.20,35.80,51.50"
		d.GridCellKm = -1

		if err := d.Validate(); err == nil {
			t.Fatal("expected an error for a negative cell size")
		}
	})

	t.Run("area mode does not need coordinates in fast mode", func(t *testing.T) {
		t.Parallel()

		// Fast mode normally demands Lat/Lon, but a grid supplies its own per
		// cell, so requiring them here would reject a valid job.
		d := base()
		d.FastMode = true
		d.BBox = "35.60,51.20,35.80,51.50"

		if err := d.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("point mode in fast mode still needs coordinates", func(t *testing.T) {
		t.Parallel()

		d := base()
		d.FastMode = true

		if err := d.Validate(); err == nil {
			t.Fatal("expected an error for fast mode without coordinates")
		}
	})
}
