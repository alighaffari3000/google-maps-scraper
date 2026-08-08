//nolint:testpackage // Tests the unexported writer directly.
package webrunner

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/gosom/scrapemate"
)

type fakeEntry struct {
	name  string
	phone string
}

func (e fakeEntry) CsvHeaders() []string {
	return []string{"title", "category", "phone"}
}

func (e fakeEntry) CsvRow() []string {
	return []string{e.name, "pharmacy", e.phone}
}

func TestFilteredCsvWriterKeepsOnlySelectedColumnsInRequestedOrder(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	w := newFilteredCsvWriter(csv.NewWriter(&buf), []string{"phone", "title"})

	in := make(chan scrapemate.Result, 2)
	in <- scrapemate.Result{Data: fakeEntry{name: "Pharmacy A", phone: "+98 21 1"}}
	in <- scrapemate.Result{Data: []fakeEntry{{name: "Pharmacy B", phone: "+98 21 2"}}}
	close(in)

	if err := w.Run(context.Background(), in); err != nil {
		t.Fatalf("run: %v", err)
	}

	want := "phone,title\n+98 21 1,Pharmacy A\n+98 21 2,Pharmacy B\n"
	if got := buf.String(); got != want {
		t.Fatalf("output =\n%q\nwant\n%q", got, want)
	}
}

// An unrecognised field name must not silently produce an empty export.
func TestFilteredCsvWriterFallsBackToAllColumnsWhenNothingMatches(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	w := newFilteredCsvWriter(csv.NewWriter(&buf), []string{"does_not_exist"})

	in := make(chan scrapemate.Result, 1)
	in <- scrapemate.Result{Data: fakeEntry{name: "Pharmacy A", phone: "+98 21 1"}}
	close(in)

	if err := w.Run(context.Background(), in); err != nil {
		t.Fatalf("run: %v", err)
	}

	header := strings.SplitN(buf.String(), "\n", 2)[0]
	if header != "title,category,phone" {
		t.Fatalf("header = %q, want the full column set", header)
	}
}

// scrapemate's producers block on the results channel, so the writer must keep
// draining it after an error instead of returning early and deadlocking the job.
func TestFilteredCsvWriterDrainsChannelAfterError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	w := newFilteredCsvWriter(csv.NewWriter(&buf), []string{"title"})

	in := make(chan scrapemate.Result)

	go func() {
		defer close(in)

		in <- scrapemate.Result{Data: "not csv capable"} // triggers the error
		in <- scrapemate.Result{Data: fakeEntry{name: "Pharmacy A"}}
		in <- scrapemate.Result{Data: fakeEntry{name: "Pharmacy B"}}
	}()

	done := make(chan error, 1)

	go func() {
		done <- w.Run(context.Background(), in)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want an error for non-csv-capable data")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not drain the channel: producers would block forever")
	}
}
