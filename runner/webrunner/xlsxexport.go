package webrunner

import (
	"encoding/csv"
	"io"
	"os"

	"github.com/xuri/excelize/v2"
)

const xlsxSheetName = "Sheet1"

// exportXLSX converts a job's scraped CSV output into an .xlsx workbook.
//
// This isn't just a convenience export: opening a UTF-8 CSV in Excel is
// notoriously unreliable at guessing the encoding, which is what turns
// Persian (or any non-Latin) text into mojibake. XLSX stores text as
// explicitly-encoded UTF-8 XML, so that ambiguity doesn't exist — the fix is
// the file format, not a BOM or an encoding flag.
//
// The source CSV always holds every column (the map/"View" feature depends
// on latitude/longitude being present regardless of what the user chose to
// export), so column selection happens here instead of at scrape time: fields
// lists the column names to keep, in that order. An empty/nil fields keeps
// every column, and a fields list that matches nothing known falls back to
// the full column set rather than producing an empty workbook.
func exportXLSX(csvPath, xlsxPath string, fields []string) error {
	src, err := os.Open(csvPath)
	if err != nil {
		return err
	}
	defer src.Close()

	reader := csv.NewReader(src)
	reader.FieldsPerRecord = -1

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

	var (
		colCount int
		indices  []int
	)

	for row := 1; ; row++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		if row == 1 {
			indices = resolveFieldIndices(record, fields)
			if len(indices) == 0 {
				indices = make([]int, len(record))
				for i := range record {
					indices[i] = i
				}
			}
		}

		record = selectColumns(record, indices)

		cell, err := excelize.CoordinatesToCellName(1, row)
		if err != nil {
			return err
		}

		if err := wb.SetSheetRow(xlsxSheetName, cell, &record); err != nil {
			return err
		}

		if row == 1 {
			colCount = len(record)

			endCell, err := excelize.CoordinatesToCellName(colCount, 1)
			if err != nil {
				return err
			}

			if err := wb.SetCellStyle(xlsxSheetName, "A1", endCell, headerStyle); err != nil {
				return err
			}
		}
	}

	if colCount > 0 {
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
