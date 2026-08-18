package output_test

import (
	"bytes"
	"image/png"
	"testing"
	"time"

	"github.com/a7g3/g3a/internal/engine"
	"github.com/a7g3/g3a/internal/output"
)

func TestRenderPNG_SingleLevel(t *testing.T) {
	res := &engine.Result{
		Columns:  []string{"category", "count", "total"},
		Rows:     [][]string{{"Electronics", "42", "12345.67"}, {"Clothing", "10", "500.00"}},
		Duration: 25 * time.Millisecond,
		RowCount: 2,
	}

	var buf bytes.Buffer
	if err := output.RenderPNG(&buf, res); err != nil {
		t.Fatalf("RenderPNG failed: %v", err)
	}

	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("Failed to decode rendered PNG: %v", err)
	}

	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatalf("Invalid image dimensions: %v", img.Bounds())
	}
}

func TestRenderPNG_MultiLevelPivot(t *testing.T) {
	res := &engine.Result{
		Columns: []string{
			"funneling_group",
			"2024/Jan",
			"2024/Feb",
			"2024/Mar",
			"2025/Jan",
		},
		Rows: [][]string{
			{"COMPLETED", "1,200", "4,500", "9,800", "12,450"},
			{"PENDING", "300", "450", "210", "500"},
			{"CANCELLED", "50", "80", "40", "100"},
		},
		Duration: 18 * time.Millisecond,
		RowCount: 3,
	}

	var buf bytes.Buffer
	if err := output.RenderPNG(&buf, res); err != nil {
		t.Fatalf("RenderPNG multi-level failed: %v", err)
	}

	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("Failed to decode rendered PNG: %v", err)
	}

	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatalf("Invalid image dimensions: %v", img.Bounds())
	}
}
