// Package pivot performs in-memory pivot transformation on query results.
//
// A pivot rotates distinct values (or value combinations) from one or more
// "pivot columns" into new column headers, filling cells with values from a
// designated "value column" (the last non-pivot, non-key column by default).
//
// Example (single pivot):
//
//	Input:  month | funneling_group | total
//	        5     | COMPLETED       | 688
//	        6     | COMPLETED       | 70916
//
//	--pivot=month  →
//
//	Output: funneling_group | 5   | 6
//	        COMPLETED       | 688 | 70916
//
// Example (multi-column pivot):
//
//	--pivot=year,month  →  header per (year,month) combo: "2024/5", "2024/6", …
package pivot

import (
	"fmt"
	"sort"
	"strings"

	"github.com/a7g3/g3a/internal/engine"
)

// Options controls pivot behaviour.
type Options struct {
	// PivotCols are column names whose distinct value combinations become
	// new column headers. Must be non-empty.
	PivotCols []string

	// FillValue is the cell value used when a (row-key, pivot-key) pair has
	// no data in the source rows. Defaults to "0".
	FillValue string

	// Separator is the string placed between values when joining multi-column
	// pivot keys into a single header label. Defaults to "/".
	Separator string
}

// Apply transforms result into a pivoted result according to opts.
//
// Column resolution:
//   - Pivot columns  = opts.PivotCols (must exist in result.Columns).
//   - Value column   = last column that is NOT a pivot column.
//   - Key columns    = all remaining columns (not pivot, not value).
//
// Output column order: key cols … | pivot_combo_1 | pivot_combo_2 | …
// Output row order follows first-appearance order of the row-key combination.
// Pivot header order follows first-appearance order in the source rows.
func Apply(result *engine.Result, opts Options) (*engine.Result, error) {
	if len(opts.PivotCols) == 0 {
		return nil, fmt.Errorf("pivot: no pivot columns specified")
	}
	sep := opts.Separator
	if sep == "" {
		sep = "/"
	}
	fill := opts.FillValue
	if fill == "" {
		fill = "0"
	}

	// --- 1. Resolve column indices ----------------------------------------

	colIdx := make(map[string]int, len(result.Columns))
	for i, c := range result.Columns {
		colIdx[c] = i
	}

	pivotIdxs := make([]int, len(opts.PivotCols))
	pivotSet := make(map[int]bool, len(opts.PivotCols))
	for i, pc := range opts.PivotCols {
		idx, ok := colIdx[pc]
		if !ok {
			return nil, fmt.Errorf("pivot: column %q not found (available: %s)",
				pc, strings.Join(result.Columns, ", "))
		}
		if pivotSet[idx] {
			return nil, fmt.Errorf("pivot: duplicate pivot column %q", pc)
		}
		pivotIdxs[i] = idx
		pivotSet[idx] = true
	}

	// Value column = last column not in pivot set.
	valueIdx := -1
	for i := len(result.Columns) - 1; i >= 0; i-- {
		if !pivotSet[i] {
			valueIdx = i
			break
		}
	}
	if valueIdx == -1 {
		return nil, fmt.Errorf("pivot: no value column remaining (all columns are pivot columns)")
	}

	// Key columns = everything except pivot cols and value col, in original order.
	var keyIdxs []int
	for i := range result.Columns {
		if !pivotSet[i] && i != valueIdx {
			keyIdxs = append(keyIdxs, i)
		}
	}

	// --- 2. Single pass: build ordered pivot keys, row keys, and data map --

	// pivotKeyOrder preserves the first-seen order of pivot key combinations.
	var pivotKeyOrder []string
	pivotKeySeen := map[string]bool{}

	// rowKeyOrder preserves the first-seen order of row key combinations.
	var rowKeyOrder []string
	rowKeySeen := map[string]bool{}

	// data[rowKey][pivotKey] = cell value
	data := map[string]map[string]string{}

	for _, row := range result.Rows {
		if len(row) != len(result.Columns) {
			continue // skip malformed rows
		}

		// Build pivot key label (e.g. "5", or "2024/5" for multi-column).
		pivotParts := make([]string, len(pivotIdxs))
		for i, pi := range pivotIdxs {
			pivotParts[i] = row[pi]
		}
		pivotKey := strings.Join(pivotParts, sep)

		// Build row key (internal: null-byte separated to avoid collisions).
		rowParts := make([]string, len(keyIdxs))
		for i, ki := range keyIdxs {
			rowParts[i] = row[ki]
		}
		// Use a non-printable separator internally; display uses original values.
		rowKey := strings.Join(rowParts, "\x00")

		// Track ordering.
		if !pivotKeySeen[pivotKey] {
			pivotKeySeen[pivotKey] = true
			pivotKeyOrder = append(pivotKeyOrder, pivotKey)
		}
		if !rowKeySeen[rowKey] {
			rowKeySeen[rowKey] = true
			rowKeyOrder = append(rowKeyOrder, rowKey)
			data[rowKey] = map[string]string{}
		}

		// Store value (last writer wins for duplicate combos).
		val := row[valueIdx]
		data[rowKey][pivotKey] = val
	}

	// Sort pivot keys so hierarchically related headers group contiguously.
	sort.Strings(pivotKeyOrder)

	// --- 3. Assemble output columns ----------------------------------------
	outCols := make([]string, 0, len(keyIdxs)+len(pivotKeyOrder))
	for _, ki := range keyIdxs {
		outCols = append(outCols, result.Columns[ki])
	}
	outCols = append(outCols, pivotKeyOrder...)

	// --- 4. Assemble output rows -------------------------------------------

	outRows := make([][]string, 0, len(rowKeyOrder))
	for _, rowKey := range rowKeyOrder {
		// Reconstruct key cell display values.
		// rowKey is "\x00"-joined; split back to individual cells.
		var keyCells []string
		if len(keyIdxs) == 0 {
			keyCells = nil
		} else if len(keyIdxs) == 1 {
			keyCells = []string{rowKey}
		} else {
			keyCells = strings.SplitN(rowKey, "\x00", len(keyIdxs))
		}

		outRow := make([]string, 0, len(outCols))
		outRow = append(outRow, keyCells...)
		for _, pk := range pivotKeyOrder {
			val, ok := data[rowKey][pk]
			if !ok {
				val = fill
			}
			outRow = append(outRow, val)
		}
		outRows = append(outRows, outRow)
	}

	return &engine.Result{
		Columns:  outCols,
		Rows:     outRows,
		Duration: result.Duration,
		RowCount: len(outRows),
		SQL:      result.SQL,
	}, nil
}
