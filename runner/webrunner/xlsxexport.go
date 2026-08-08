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
func exportXLSX(csvPath, xlsxPath string) error {
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

	var colCount int

	for row := 1; ; row++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

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
