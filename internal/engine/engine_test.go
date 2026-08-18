package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/a7g3/g3a/internal/engine"
)

func TestCSVCountAll(t *testing.T) {
	// Create a temp CSV file.
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "test.csv")
	content := "category,amount\nA,10\nA,20\nB,5\nB,15\nB,30\n"
	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	eng, err := engine.New()
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer eng.Close()

	result, err := eng.Run(context.Background(), engine.QueryOptions{
		InputPath: csvPath,
		Format:    "csv",
		Select:    "category, count(1) as n, sum(amount) as total",
		GroupBy:   "category",
		OrderBy:   "category",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.RowCount != 2 {
		t.Errorf("expected 2 rows, got %d", result.RowCount)
	}

	// Row 0: A, 2, 30
	if result.Rows[0][0] != "A" {
		t.Errorf("row[0][0] = %q, want A", result.Rows[0][0])
	}
	if result.Rows[0][1] != "2" {
		t.Errorf("row[0][1] = %q, want 2", result.Rows[0][1])
	}
	if result.Rows[0][2] != "30" {
		t.Errorf("row[0][2] = %q, want 30", result.Rows[0][2])
	}

	// Row 1: B, 3, 50
	if result.Rows[1][0] != "B" {
		t.Errorf("row[1][0] = %q, want B", result.Rows[1][0])
	}
	if result.Rows[1][2] != "50" {
		t.Errorf("row[1][2] = %q, want 50", result.Rows[1][2])
	}
}

func TestCSVWhere(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "test.csv")
	content := "name,score\nalice,90\nbob,40\ncarol,70\n"
	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	eng, err := engine.New()
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer eng.Close()

	result, err := eng.Run(context.Background(), engine.QueryOptions{
		InputPath: csvPath,
		Format:    "csv",
		Select:    "count(1) as n",
		Where:     "score >= 70",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.RowCount != 1 {
		t.Fatalf("expected 1 row, got %d", result.RowCount)
	}
	if result.Rows[0][0] != "2" {
		t.Errorf("count = %q, want 2", result.Rows[0][0])
	}
}
