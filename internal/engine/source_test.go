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

	source, err := BuildSource(QueryOptions{
		InputPath: tmpDir,
		Format:    "parquet",
	})
	if err != nil {
		t.Fatalf("BuildSource failed: %v", err)
	}

	cleanDir := strings.TrimRight(filepath.ToSlash(tmpDir), "/")
	expectedPattern := cleanDir + "/**/*.parquet"
	if !strings.Contains(source, expectedPattern) {
		t.Errorf("expected source to contain %q, got %q", expectedPattern, source)
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

	source, err := BuildSource(QueryOptions{
		InputPath: tmpDir,
		Format:    "csv",
	})
	if err != nil {
		t.Fatalf("BuildSource failed: %v", err)
	}

	cleanDir := strings.TrimRight(filepath.ToSlash(tmpDir), "/")
	expectedPattern := cleanDir + "/**/*.csv"
	if !strings.Contains(source, expectedPattern) {
		t.Errorf("expected source to contain %q, got %q", expectedPattern, source)
	}
	if !strings.Contains(source, "union_by_name=true") {
		t.Errorf("expected union_by_name=true in %q", source)
	}
}
