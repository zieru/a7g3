package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFormatExcel(t *testing.T) {
	tmpDir := t.TempDir()
	xlsxPath := filepath.Join(tmpDir, "report.xlsx")
	if err := os.WriteFile(xlsxPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("write dummy xlsx: %v", err)
	}

	got, err := DetectFormat(xlsxPath)
	if err != nil {
		t.Fatalf("DetectFormat failed: %v", err)
	}
	if got != FormatExcel {
		t.Errorf("expected %q, got %q", FormatExcel, got)
	}

	args, err := ParseArgs([]string{xlsxPath, "--sheet", "Summary", "--no-header"})
	if err != nil {
		t.Fatalf("ParseArgs failed: %v", err)
	}
	if args.Format != FormatExcel {
		t.Errorf("expected FormatExcel, got %q", args.Format)
	}
	if args.Sheet != "Summary" {
		t.Errorf("expected Sheet='Summary', got %q", args.Sheet)
	}
	if !args.NoHeader {
		t.Errorf("expected NoHeader=true")
	}
}
