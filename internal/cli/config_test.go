package cli

import (
	"os"
	"testing"
)

func TestParseConfigFile(t *testing.T) {
	content := `
# Sample g3a configuration file
funneling /data/funneling.csv
funnelingx = /data/funneling.parquet
with_quotes "C:\Users\test\my data.csv"
single_quotes 'C:\Users\test\data2.csv'
spaced_key = /var/log/app.csv

; Another comment format
// C-style comment
`
	tmpFile, err := os.CreateTemp("", ".g3a.config.*")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	tmpFile.Close()

	aliases, err := ParseConfigFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseConfigFile failed: %v", err)
	}

	expected := map[string]string{
		"funneling":     "/data/funneling.csv",
		"funnelingx":    "/data/funneling.parquet",
		"with_quotes":   `C:\Users\test\my data.csv`,
		"single_quotes": `C:\Users\test\data2.csv`,
		"spaced_key":    "/var/log/app.csv",
	}

	for k, expVal := range expected {
		if val, ok := aliases[k]; !ok || val != expVal {
			t.Errorf("expected alias[%q] = %q, got %q (ok=%v)", k, expVal, val, ok)
		}
	}
}

func TestLoadConfigWithEnv(t *testing.T) {
	content := "sales /path/to/sales.csv\n"
	tmpFile, err := os.CreateTemp("", ".g3a.config.*")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	tmpFile.Close()

	t.Setenv("G3A_CONFIG", tmpFile.Name())

	aliases, cfgPath, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfgPath != tmpFile.Name() {
		t.Errorf("expected cfgPath %q, got %q", tmpFile.Name(), cfgPath)
	}

	if aliases["sales"] != "/path/to/sales.csv" {
		t.Errorf("expected sales to map to /path/to/sales.csv, got %q", aliases["sales"])
	}
}

func TestParseArgsWithAlias(t *testing.T) {
	// Create a dummy CSV file to validate against
	tmpCSV, err := os.CreateTemp("", "dummy_*.csv")
	if err != nil {
		t.Fatalf("create temp csv: %v", err)
	}
	defer os.Remove(tmpCSV.Name())
	tmpCSV.WriteString("id,name,amount\n1,Alice,100\n")
	tmpCSV.Close()

	// Create config mapping alias "funneling" to tmpCSV.Name()
	tmpCfg, err := os.CreateTemp("", ".g3a.config.*")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	defer os.Remove(tmpCfg.Name())
	tmpCfg.WriteString("funneling " + tmpCSV.Name() + "\n")
	tmpCfg.Close()

	t.Setenv("G3A_CONFIG", tmpCfg.Name())

	// Test 1: alias first: g3a funneling --select="count(1)"
	args, err := ParseArgs([]string{"funneling", "--select=count(1)", "--where", "amount > 50"})
	if err != nil {
		t.Fatalf("ParseArgs failed: %v", err)
	}
	if args.Input != tmpCSV.Name() {
		t.Errorf("expected input %q, got %q", tmpCSV.Name(), args.Input)
	}
	if args.Select != "count(1)" {
		t.Errorf("expected select count(1), got %q", args.Select)
	}
	if args.Where != "amount > 50" {
		t.Errorf("expected where amount > 50, got %q", args.Where)
	}
	if args.Format != FormatCSV {
		t.Errorf("expected FormatCSV, got %v", args.Format)
	}

	// Test 2: flags first: g3a --select="sum(amount)" funneling
	args2, err := ParseArgs([]string{"--select", "sum(amount)", "funneling"})
	if err != nil {
		t.Fatalf("ParseArgs flags-first failed: %v", err)
	}
	if args2.Input != tmpCSV.Name() {
		t.Errorf("expected input %q, got %q", tmpCSV.Name(), args2.Input)
	}
	if args2.Select != "sum(amount)" {
		t.Errorf("expected select sum(amount), got %q", args2.Select)
	}

	// Test 3: positional direct filepath (no alias needed)
	args3, err := ParseArgs([]string{tmpCSV.Name(), "--limit=10"})
	if err != nil {
		t.Fatalf("ParseArgs direct file failed: %v", err)
	}
	if args3.Input != tmpCSV.Name() {
		t.Errorf("expected input %q, got %q", tmpCSV.Name(), args3.Input)
	}
	if args3.Limit != 10 {
		t.Errorf("expected limit 10, got %d", args3.Limit)
	}

	// Test 4: unknown alias should return an error
	_, err4 := ParseArgs([]string{"unknown_alias", "--select=*"})
	if err4 == nil {
		t.Fatal("expected error for unknown alias, got nil")
	}
}

func TestDetectFormatDirectory(t *testing.T) {
	// Test parquet directory
	parqDir, err := os.MkdirTemp("", "parq_dir_*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(parqDir)

	f1, err := os.CreateTemp(parqDir, "part_*.parquet")
	if err != nil {
		t.Fatalf("create temp parquet: %v", err)
	}
	f1.Close()

	fmtType, err := DetectFormat(parqDir)
	if err != nil {
		t.Fatalf("DetectFormat on parquet dir failed: %v", err)
	}
	if fmtType != FormatParquet {
		t.Errorf("expected FormatParquet, got %v", fmtType)
	}

	// Test CSV directory
	csvDir, err := os.MkdirTemp("", "csv_dir_*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(csvDir)

	f2, err := os.CreateTemp(csvDir, "part_*.csv")
	if err != nil {
		t.Fatalf("create temp csv: %v", err)
	}
	f2.Close()

	fmtTypeCSV, err := DetectFormat(csvDir)
	if err != nil {
		t.Fatalf("DetectFormat on csv dir failed: %v", err)
	}
	if fmtTypeCSV != FormatCSV {
		t.Errorf("expected FormatCSV, got %v", fmtTypeCSV)
	}
}

func TestParseConfigFile_TrailingSlash(t *testing.T) {
	content := "parq_folder /path/to/my_folder/\n"
	tmpFile, err := os.CreateTemp("", ".g3a.config.*")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	tmpFile.Close()

	aliases, err := ParseConfigFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseConfigFile failed: %v", err)
	}

	if aliases["parq_folder"] != "/path/to/my_folder" {
		t.Errorf("expected trailing slash trimmed to /path/to/my_folder, got %q", aliases["parq_folder"])
	}
}
