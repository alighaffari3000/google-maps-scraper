package webrunner

import (
	"context"
	"encoding/csv"
	"fmt"
	"reflect"
	"sync"

	"github.com/gosom/scrapemate"
)

// filteredCsvWriter is a scrapemate.ResultWriter that writes only a subset of
// the columns a scrapemate.CsvCapable result exposes via CsvHeaders/CsvRow,
// keeping those two slices index-aligned as the source of truth for column
// names instead of duplicating field definitions here.
type filteredCsvWriter struct {
	w      *csv.Writer
	fields []string // column names to keep, in the order they should be written

	once    sync.Once
	indices []int // resolved once from the first result's CsvHeaders()
}

// newFilteredCsvWriter returns a ResultWriter that writes only the given
// column names (matching gmaps.Entry.CsvHeaders()) to w, in that order.
// Unknown column names are silently skipped.
func newFilteredCsvWriter(w *csv.Writer, fields []string) scrapemate.ResultWriter {
	return &filteredCsvWriter{w: w, fields: fields}
}

// Run drains the results channel to completion regardless of write errors:
// scrapemate's producers keep sending on this channel until the job is done,
// so returning early here would leave them blocked forever on a full channel.
// Any error is remembered and returned only after the channel closes.
func (c *filteredCsvWriter) Run(_ context.Context, in <-chan scrapemate.Result) error {
	var firstErr error

	for result := range in {
		if firstErr != nil {
			continue
		}

		elements, err := c.getCsvCapable(result.Data)
		if err != nil {
			firstErr = err

			continue
		}

		if len(elements) == 0 {
			continue
		}

		c.once.Do(func() {
			headers := elements[0].CsvHeaders()
			c.indices = resolveFieldIndices(headers, c.fields)

			if len(c.indices) == 0 {
				// None of the requested fields matched a known column: fall
				// back to the full column set rather than writing an empty
				// CSV for the whole job.
				c.indices = make([]int, len(headers))
				for i := range headers {
					c.indices[i] = i
				}
			}

			firstErr = c.w.Write(selectColumns(headers, c.indices))
		})

		if firstErr != nil {
			continue
		}

		for _, element := range elements {
			if err := c.w.Write(selectColumns(element.CsvRow(), c.indices)); err != nil {
				firstErr = err

				break
			}
		}

		c.w.Flush()
	}

	if firstErr != nil {
		return firstErr
	}

	return c.w.Error()
}

func (c *filteredCsvWriter) getCsvCapable(data any) ([]scrapemate.CsvCapable, error) {
	var elements []scrapemate.CsvCapable

	if reflect.TypeOf(data).Kind() == reflect.Slice {
		s := reflect.ValueOf(data)

		for i := 0; i < s.Len(); i++ {
			val := s.Index(i).Interface()
			if element, ok := val.(scrapemate.CsvCapable); ok {
				elements = append(elements, element)
			} else {
				return nil, fmt.Errorf("%w: unexpected data type: %T", scrapemate.ErrorNotCsvCapable, val)
			}
		}
	} else if element, ok := data.(scrapemate.CsvCapable); ok {
		elements = append(elements, element)
	} else {
		return nil, fmt.Errorf("%w: unexpected data type: %T", scrapemate.ErrorNotCsvCapable, data)
	}

	return elements, nil
}

// resolveFieldIndices maps the requested field names onto their position in
// headers, preserving the order of wanted (not of headers) and dropping any
// name that doesn't exist.
func resolveFieldIndices(headers, wanted []string) []int {
	pos := make(map[string]int, len(headers))
	for i, h := range headers {
		pos[h] = i
	}

	indices := make([]int, 0, len(wanted))

	for _, name := range wanted {
		if i, ok := pos[name]; ok {
			indices = append(indices, i)
		}
	}

	return indices
}

func selectColumns(row []string, indices []int) []string {
	out := make([]string, len(indices))
	for i, idx := range indices {
		if idx < len(row) {
			out[i] = row[idx]
		}
	}

	return out
}
