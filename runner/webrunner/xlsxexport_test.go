//nolint:testpackage // Tests the unexported export function directly.
package webrunner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExportXLSXRoundTripsPersianText(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "in.csv")
	xlsxPath := filepath.Join(dir, "out.xlsx")

	csvContent := "title,phone\n" +
		"داروخانه دکتر مدرس یزدی,+98 21 2278 3596\n" +
		"Coffee Place,555-1234\n"

	if err := os.WriteFile(csvPath, []byte(csvContent), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	if err := exportXLSX(csvPath, xlsxPath); err != nil {
		t.Fatalf("exportXLSX: %v", err)
	}

	wb, err := excelize.OpenFile(xlsxPath)
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer wb.Close()

	rows, err := wb.GetRows(xlsxSheetName)
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}

	want := [][]string{
		{"title", "phone"},
		{"داروخانه دکتر مدرس یزدی", "+98 21 2278 3596"},
		{"Coffee Place", "555-1234"},
	}

	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}

	for i, row := range rows {
		for j, cell := range row {
			if cell != want[i][j] {
				t.Fatalf("row %d col %d = %q, want %q", i, j, cell, want[i][j])
			}
		}
	}

	view, err := wb.GetSheetView(xlsxSheetName, 0)
	if err != nil {
		t.Fatalf("get sheet view: %v", err)
	}

	if view.RightToLeft == nil || !*view.RightToLeft {
		t.Fatal("expected sheet to be right-to-left for Persian content")
	}
}

func TestExportXLSXMissingSourceFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	err := exportXLSX(filepath.Join(dir, "does-not-exist.csv"), filepath.Join(dir, "out.xlsx"))
	if err == nil {
		t.Fatal("expected error for missing source CSV")
	}
}
