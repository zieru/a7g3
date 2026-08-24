package output_test

import (
	"strings"
	"testing"
	"time"

	"github.com/a7g3/g3a/internal/engine"
	"github.com/a7g3/g3a/internal/output"
)

func makeResult() *engine.Result {
	return &engine.Result{
		Columns:  []string{"category", "count", "total"},
		Rows:     [][]string{{"Electronics", "42", "12345.67"}, {"Clothing", "10", "500.00"}},
		Duration: 50 * time.Millisecond,
		RowCount: 2,
	}
}

func TestPrintTable(t *testing.T) {
	var sb strings.Builder
	if err := output.Print(&sb, makeResult(), "table", false); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "Electronics") {
		t.Error("table missing 'Electronics'")
	}
	if !strings.Contains(out, "2 row(s)") {
		t.Errorf("table missing row count footer, got:\n%s", out)
	}
}

func TestPrintCSV(t *testing.T) {
	var sb strings.Builder
	if err := output.Print(&sb, makeResult(), "csv", false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(sb.String()), "\n")
	if len(lines) != 3 { // header + 2 data rows
		t.Errorf("expected 3 CSV lines, got %d:\n%s", len(lines), sb.String())
	}
	if lines[0] != "category,count,total" {
		t.Errorf("header = %q", lines[0])
	}
}

func TestPrintJSON(t *testing.T) {
	var sb strings.Builder
	if err := output.Print(&sb, makeResult(), "json", false); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	// New envelope: {"ok":true,"columns":[...],"rows":[{"category":"Electronics",...}],...}
	if !strings.Contains(out, `"ok": true`) {
		t.Errorf("JSON missing ok field:\n%s", out)
	}
	if !strings.Contains(out, `"row_count": 2`) {
		t.Errorf("JSON missing row_count:\n%s", out)
	}
	if !strings.Contains(out, `"Electronics"`) {
		t.Errorf("JSON missing Electronics value:\n%s", out)
	}
}

func TestPrintJSONL(t *testing.T) {
	var sb strings.Builder
	if err := output.Print(&sb, makeResult(), "jsonl", false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(sb.String()), "\n")
	// first line = meta, then 2 data rows
	if len(lines) != 3 {
		t.Errorf("expected 3 JSONL lines, got %d:\n%s", len(lines), sb.String())
	}
	if !strings.Contains(lines[0], `"type":"meta"`) {
		t.Errorf("first JSONL line should be meta:\n%s", lines[0])
	}
	if !strings.Contains(lines[1], `"type":"row"`) {
		t.Errorf("second JSONL line should be row:\n%s", lines[1])
	}
}

func TestPrintJSONVerbose(t *testing.T) {
	r := makeResult()
	r.SQL = "SELECT category FROM test"
	var sb strings.Builder
	if err := output.Print(&sb, r, "json", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), `"sql"`) {
		t.Errorf("verbose JSON should include sql field:\n%s", sb.String())
	}
}

func TestPrintJSON_DateStringsPreserved(t *testing.T) {
	r := &engine.Result{
		Columns:  []string{"periode", "total_visits"},
		Rows:     [][]string{{"2026-08", "12345"}, {"2026-07", "67890"}},
		Duration: 10 * time.Millisecond,
		RowCount: 2,
	}
	var sb strings.Builder
	if err := output.Print(&sb, r, "json", false); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, `"periode": "2026-08"`) {
		t.Errorf("expected periode to remain string '2026-08', got:\n%s", out)
	}
	if !strings.Contains(out, `"total_visits": 12345`) {
		t.Errorf("expected total_visits to be numeric 12345, got:\n%s", out)
	}
}


