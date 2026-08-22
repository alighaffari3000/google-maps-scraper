//nolint:testpackage // Tests the unexported export function directly.
package webrunner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gosom/google-maps-scraper/web"
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

	if err := exportXLSX(csvPath, xlsxPath, exportOptions{}); err != nil {
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

	err := exportXLSX(filepath.Join(dir, "does-not-exist.csv"), filepath.Join(dir, "out.xlsx"), exportOptions{})
	if err == nil {
		t.Fatal("expected error for missing source CSV")
	}
}

func TestExportXLSXFiltersColumns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "in.csv")
	xlsxPath := filepath.Join(dir, "out.xlsx")

	csvContent := "title,phone,address,latitude,longitude\n" +
		"Coffee Place,555-1234,1 Main St,37.7749,-122.4194\n"

	if err := os.WriteFile(csvPath, []byte(csvContent), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	if err := exportXLSX(csvPath, xlsxPath, exportOptions{Fields: []string{"title", "phone", "address"}}); err != nil {
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
		{"title", "phone", "address"},
		{"Coffee Place", "555-1234", "1 Main St"},
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
}

func TestExportXLSXUnknownFieldsFallBackToAllColumns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "in.csv")
	xlsxPath := filepath.Join(dir, "out.xlsx")

	csvContent := "title,phone\nCoffee Place,555-1234\n"

	if err := os.WriteFile(csvPath, []byte(csvContent), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	if err := exportXLSX(csvPath, xlsxPath, exportOptions{Fields: []string{"not-a-real-column"}}); err != nil {
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

	want := [][]string{{"title", "phone"}, {"Coffee Place", "555-1234"}}

	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}
}

func TestExportXLSXAppliesLeadFilter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "in.csv")
	xlsxPath := filepath.Join(dir, "out.xlsx")

	csvContent := "title,phone,status,review_count\n" +
		"Has phone,+98 21 1111 1111,,40\n" +
		"No phone,,,40\n" +
		"Closed down,+98 21 2222 2222,CLOSED,40\n" +
		"Too few reviews,+98 21 3333 3333,,2\n"

	if err := os.WriteFile(csvPath, []byte(csvContent), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	opts := exportOptions{
		Filter: web.LeadFilter{RequirePhone: true, ExcludeClosed: true, MinReviews: 10},
	}

	if err := exportXLSX(csvPath, xlsxPath, opts); err != nil {
		t.Fatalf("exportXLSX: %v", err)
	}

	rows := readSheet(t, xlsxPath)

	// Header plus the one row that satisfies every condition.
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}

	if rows[1][0] != "Has phone" {
		t.Errorf("kept %q, want %q", rows[1][0], "Has phone")
	}
}

func TestExportXLSXNormalizesPhones(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "in.csv")
	xlsxPath := filepath.Join(dir, "out.xlsx")

	// The same number in the three shapes Google actually returns, plus a
	// foreign one that must survive untouched.
	csvContent := "title,phone\n" +
		"A,+98 21 8884 9410\n" +
		"B,021 8884 9410\n" +
		"C,0912 345 6789\n" +
		"D,+1 555 123 4567\n"

	if err := os.WriteFile(csvPath, []byte(csvContent), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	if err := exportXLSX(csvPath, xlsxPath, exportOptions{NormalizePhones: true}); err != nil {
		t.Fatalf("exportXLSX: %v", err)
	}

	rows := readSheet(t, xlsxPath)

	want := []string{"+982188849410", "+982188849410", "+989123456789", "+1 555 123 4567"}
	for i, w := range want {
		if got := rows[i+1][1]; got != w {
			t.Errorf("row %d phone = %q, want %q", i+1, got, w)
		}
	}
}

func TestExportXLSXFilterAndFieldsTogether(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "in.csv")
	xlsxPath := filepath.Join(dir, "out.xlsx")

	// status drives the filter but is not exported, so filtering has to read
	// the source row rather than the selected columns.
	csvContent := "title,phone,status\n" +
		"Open,+98 21 1111 1111,\n" +
		"Closed,+98 21 2222 2222,CLOSED\n"

	if err := os.WriteFile(csvPath, []byte(csvContent), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	opts := exportOptions{
		Fields: []string{"title", "phone"},
		Filter: web.LeadFilter{ExcludeClosed: true},
	}

	if err := exportXLSX(csvPath, xlsxPath, opts); err != nil {
		t.Fatalf("exportXLSX: %v", err)
	}

	rows := readSheet(t, xlsxPath)

	if len(rows) != 2 || len(rows[0]) != 2 {
		t.Fatalf("got %d rows of %d columns, want 2x2: %+v", len(rows), len(rows[0]), rows)
	}

	if rows[1][0] != "Open" {
		t.Errorf("kept %q, want %q", rows[1][0], "Open")
	}
}

func readSheet(t *testing.T, xlsxPath string) [][]string {
	t.Helper()

	wb, err := excelize.OpenFile(xlsxPath)
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}

	defer wb.Close()

	rows, err := wb.GetRows(xlsxSheetName)
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}

	return rows
}
