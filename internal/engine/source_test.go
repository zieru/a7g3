package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSourceParquetDir(t *testing.T) {
	// 1. Directory with .parquet file
	tmpDir, err := os.MkdirTemp("", "parq_test_*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	f1, err := os.CreateTemp(tmpDir, "data_*.parquet")
	if err != nil {
		t.Fatalf("create temp parquet: %v", err)
	}
	f1.Close()

	// Create a safeBackup file which must be excluded
	backupFile := filepath.Join(tmpDir, "data_092026-4116131e1245-safeBackup-0001.parquet")
	if err := os.WriteFile(backupFile, []byte("dummy"), 0644); err != nil {
		t.Fatalf("create backup parquet: %v", err)
	}

	source, err := BuildSource(QueryOptions{
		InputPath: tmpDir,
		Format:    "parquet",
	})
	if err != nil {
		t.Fatalf("BuildSource failed: %v", err)
	}

	expectedFile := filepath.ToSlash(f1.Name())
	if !strings.Contains(source, expectedFile) {
		t.Errorf("expected source to contain %q, got %q", expectedFile, source)
	}
	backupFileName := filepath.ToSlash(backupFile)
	if strings.Contains(source, backupFileName) {
		t.Errorf("expected source to EXCLUDE backup file %q, but got %q", backupFileName, source)
	}
	if !strings.Contains(source, "union_by_name=true") {
		t.Errorf("expected union_by_name=true in %q", source)
	}
	if !strings.Contains(source, "hive_partitioning=true") {
		t.Errorf("expected hive_partitioning=true in %q", source)
	}

	// 2. Directory with trailing slash
	sourceTrailing, err := BuildSource(QueryOptions{
		InputPath: tmpDir + "/",
		Format:    "parquet",
	})
	if err != nil {
		t.Fatalf("BuildSource with trailing slash failed: %v", err)
	}
	if strings.Contains(sourceTrailing, "//") {
		t.Errorf("expected no double slashes in %q", sourceTrailing)
	}

	// 3. Single parquet file
	singleParquet := f1.Name()
	sourceSingle, err := BuildSource(QueryOptions{
		InputPath: singleParquet,
		Format:    "parquet",
	})
	if err != nil {
		t.Fatalf("BuildSource single file failed: %v", err)
	}
	expectedSingle := filepath.ToSlash(singleParquet)
	if !strings.Contains(sourceSingle, expectedSingle) {
		t.Errorf("expected source to contain single file %q, got %q", expectedSingle, sourceSingle)
	}
}

func TestBuildSourceCSVDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "csv_test_*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	f1, err := os.CreateTemp(tmpDir, "data_*.csv")
	if err != nil {
		t.Fatalf("create temp csv: %v", err)
	}
	f1.Close()

	backupCSV := filepath.Join(tmpDir, "data_backup.tmp.csv")
	if err := os.WriteFile(backupCSV, []byte("dummy"), 0644); err != nil {
		t.Fatalf("create backup csv: %v", err)
	}

	source, err := BuildSource(QueryOptions{
		InputPath: tmpDir,
		Format:    "csv",
	})
	if err != nil {
		t.Fatalf("BuildSource failed: %v", err)
	}

	expectedFile := filepath.ToSlash(f1.Name())
	if !strings.Contains(source, expectedFile) {
		t.Errorf("expected source to contain %q, got %q", expectedFile, source)
	}
	if strings.Contains(source, "backup.tmp") {
		t.Errorf("expected source to EXCLUDE backup csv, got %q", source)
	}
	if !strings.Contains(source, "union_by_name=true") {
		t.Errorf("expected union_by_name=true in %q", source)
	}
}
