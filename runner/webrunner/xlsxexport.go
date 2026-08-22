package webrunner

import (
	"encoding/csv"
	"errors"
	"io"
	"os"

	"github.com/gosom/google-maps-scraper/web"
	"github.com/xuri/excelize/v2"
)

const xlsxSheetName = "Sheet1"

// exportOptions is what the user chose about the workbook they will download:
// which columns, which rows, and whether phone numbers get rewritten.
type exportOptions struct {
	// Fields lists column names to keep, in that order. Empty keeps every
	// column; a list matching nothing known falls back to the full set rather
	// than producing an empty workbook.
	Fields []string

	// Filter drops rows the user does not want to see.
	Filter web.LeadFilter

	// NormalizePhones rewrites Iranian numbers into E.164.
	NormalizePhones bool
}

// exportXLSX converts a job's scraped CSV output into an .xlsx workbook.
//
// This isn't just a convenience export: opening a UTF-8 CSV in Excel is
// notoriously unreliable at guessing the encoding, which is what turns
// Persian (or any non-Latin) text into mojibake. XLSX stores text as
// explicitly-encoded UTF-8 XML, so that ambiguity doesn't exist — the fix is
// the file format, not a BOM or an encoding flag.
//
// The source CSV always holds every column and every row (the map depends on
// latitude/longitude, and a filter the user regrets must not cost a re-scrape),
// so column selection, row filtering and phone rewriting all happen here.
func exportXLSX(csvPath, xlsxPath string, opts exportOptions) error {
	src, err := os.Open(csvPath)
	if err != nil {
		return err
	}
	defer src.Close()

	reader := csv.NewReader(src)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			// Nothing was scraped. An empty workbook is a better answer than
			// a missing file, which reads as a broken download.
			return writeWorkbook(xlsxPath, nil)
		}

		return err
	}

	indices := resolveFieldIndices(header, opts.Fields)
	if len(indices) == 0 {
		indices = make([]int, len(header))
		for i := range header {
			indices[i] = i
		}
	}

	col := make(map[string]int, len(header))
	for i, name := range header {
		col[name] = i
	}

	rows := [][]string{selectColumns(header, indices)}

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(record) {
				return record[i]
			}

			return ""
		}

		if opts.Filter.Active() && !opts.Filter.Keep(get) {
			continue
		}

		// Rewritten before column selection so the phone column is found by
		// its position in the source row, whatever the user chose to export.
		if opts.NormalizePhones {
			if i, ok := col["phone"]; ok && i < len(record) {
				record[i] = web.NormalizeIranPhone(record[i])
			}
		}

		rows = append(rows, selectColumns(record, indices))
	}

	return writeWorkbook(xlsxPath, rows)
}

// writeWorkbook renders rows to an .xlsx file, treating the first as a header.
func writeWorkbook(xlsxPath string, rows [][]string) error {
	wb := excelize.NewFile()
	defer wb.Close()

	rtl := true
	if err := wb.SetSheetView(xlsxSheetName, 0, &excelize.ViewOptions{RightToLeft: &rtl}); err != nil {
		return err
	}

	headerStyle, err := wb.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return err
	}

	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			return err
		}

		if err := wb.SetSheetRow(xlsxSheetName, cell, &row); err != nil {
			return err
		}
	}

	if len(rows) > 0 {
		colCount := len(rows[0])

		endCell, err := excelize.CoordinatesToCellName(colCount, 1)
		if err != nil {
			return err
		}

		if err := wb.SetCellStyle(xlsxSheetName, "A1", endCell, headerStyle); err != nil {
			return err
		}

		endCol, err := excelize.ColumnNumberToName(colCount)
		if err != nil {
			return err
		}

		if err := wb.SetColWidth(xlsxSheetName, "A", endCol, 22); err != nil {
			return err
		}
	}

	return wb.SaveAs(xlsxPath)
}

// resolveFieldIndices maps the requested field names onto their position in
// headers, preserving the order of wanted (not of headers) and dropping any
// name that doesn't exist.
func resolveFieldIndices(headers, wanted []string) []int {
	if len(wanted) == 0 {
		return nil
	}

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
