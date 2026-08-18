// Package output formats and prints query results.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/a7g3/g3a/internal/engine"
)

// JSONOutput is the structured JSON envelope for LLM tool use.
// All fields are always present so parsers don't need null checks.
type JSONOutput struct {
	OK       bool              `json:"ok"`
	Columns  []string          `json:"columns"`
	Rows     []map[string]any  `json:"rows"`
	RowCount int               `json:"row_count"`
	DurationMs float64         `json:"duration_ms"`
	SQL      string            `json:"sql,omitempty"`
	Error    string            `json:"error,omitempty"`
}

// JSONError is the structured error envelope emitted when output=json and an error occurs.
type JSONError struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// Print writes the result to w in the specified format.
// format: "table" | "csv" | "json" | "jsonl" | "toon"
func Print(w io.Writer, result *engine.Result, format string, verbose bool) error {
	switch format {
	case "csv":
		return printCSV(w, result)
	case "json":
		return printJSON(w, result, verbose)
	case "jsonl":
		return printJSONL(w, result)
	case "toon":
		return printTOON(w, result, verbose)
	case "png", "image":
		return RenderPNG(w, result)
	default:
		return printTable(w, result)
	}
}

// PrintError writes an error in a format appropriate for the output mode.
// For json/jsonl it emits a machine-readable JSON object; for others plain text.
func PrintError(w io.Writer, err error, format string) {
	switch format {
	case "json", "jsonl":
		e := JSONError{OK: false, Error: err.Error()}
		b, _ := json.Marshal(e)
		fmt.Fprintln(w, string(b))
	default:
		fmt.Fprintf(w, "g3a: error: %v\n", err)
	}
}

// -------------------------------------------------------------------
// JSON (structured envelope — best for LLM tool use)
// -------------------------------------------------------------------

func printJSON(w io.Writer, result *engine.Result, verbose bool) error {
	out := JSONOutput{
		OK:         true,
		Columns:    result.Columns,
		Rows:       rowsToMaps(result),
		RowCount:   result.RowCount,
		DurationMs: float64(result.Duration.Milliseconds()),
	}
	if verbose {
		out.SQL = result.SQL
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// rowsToMaps converts [][]string rows into []map[string]any for idiomatic JSON.
// Numbers are kept as json.Number so LLMs see 42 not "42".
func rowsToMaps(result *engine.Result) []map[string]any {
	out := make([]map[string]any, 0, result.RowCount)
	for _, row := range result.Rows {
		obj := make(map[string]any, len(result.Columns))
		for i, col := range result.Columns {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			// Attempt numeric coercion so JSON consumers get real numbers.
			obj[col] = coerceValue(val)
		}
		out = append(out, obj)
	}
	return out
}

// coerceValue tries to convert a string cell to int64 or float64.
// Falls back to string if not numeric. "NULL" becomes nil (JSON null).
func coerceValue(s string) any {
	if s == "NULL" {
		return nil
	}
	// Try integer first (no decimal point, no exponent).
	if !strings.ContainsAny(s, ".eEn") {
		var i int64
		if _, err := fmt.Sscanf(s, "%d", &i); err == nil {
			return i
		}
	}
	// Try float.
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err == nil {
		return f
	}
	return s
}

// -------------------------------------------------------------------
// TOON — Token-Oriented Object Notation v4.1 (tabular form)
// Spec: https://github.com/toon-format/spec
//
// For uniform arrays of objects (which query results always are),
// TOON's tabular form declares the field list ONCE and then emits
// one comma-row per result row — 30-60% fewer tokens than JSON.
//
// Shape emitted:
//
//	# g3a query result  (comment, decoder-stripped)
//	result[N]{col1,col2,...}:
//	  val1,val2,...
//	  val1,val2,...
//
// When --verbose, a metadata comment block is prepended:
//
//	# sql: SELECT ...
//	# duration_ms: 21
// -------------------------------------------------------------------

func printTOON(w io.Writer, result *engine.Result, verbose bool) error {
	// Optional metadata comment block (comments are stripped by TOON decoders).
	if verbose && result.SQL != "" {
		// Multi-line SQL: prefix each line with "# ".
		for _, line := range strings.Split(result.SQL, "\n") {
			fmt.Fprintf(w, "# sql: %s\n", line)
		}
		fmt.Fprintf(w, "# duration_ms: %d\n", result.Duration.Milliseconds())
	}

	// Build the tabular header: result[N]{col1,col2,...}:
	fmt.Fprintf(w, "result[%d]{%s}:\n", result.RowCount, toonFieldList(result.Columns))

	// Emit one row per result, indented by 2 spaces (TOON content depth).
	for _, row := range result.Rows {
		fmt.Fprintf(w, "  %s\n", toonRow(row))
	}
	return nil
}

// toonFieldList encodes column names as a TOON field list.
// Names that need quoting (contain comma, brace, colon, or whitespace) are
// quoted with double quotes per TOON §7.3.
func toonFieldList(cols []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = toonQuoteKey(c)
	}
	return strings.Join(parts, ",")
}

// toonRow encodes a single result row as a comma-separated TOON row.
// Cells are quoted when they contain a comma, double-quote, or newline
// (per TOON §7.2 "quote when required" rule).
func toonRow(cells []string) string {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = toonQuoteValue(c)
	}
	return strings.Join(parts, ",")
}

// toonQuoteValue quotes a cell value if required by TOON §7.2.
// Values that look like TOON keywords (true/false/null) or numbers are
// left unquoted — TOON decoders handle them correctly.
func toonQuoteValue(s string) string {
	if s == "NULL" {
		return "null" // TOON null literal
	}
	// Unquoted if it looks like a number or boolean.
	if isTOONBare(s) {
		return s
	}
	// Quote if it contains delimiter, brace, colon, quote, or whitespace.
	if strings.ContainsAny(s, ",{}:\"\n\r\t") || s == "" {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	// Strings that look like TOON booleans/null need quoting.
	switch s {
	case "true", "false", "null":
		return `"` + s + `"`
	}
	return s
}

// toonQuoteKey quotes a column name when it contains characters that
// would break the field list syntax (TOON §7.3).
func toonQuoteKey(k string) string {
	if strings.ContainsAny(k, ",{}[]:\" \t\n\r") || k == "" {
		return `"` + strings.ReplaceAll(k, `"`, `\"`) + `"`
	}
	return k
}

// isTOONBare reports whether s is a bare number token per TOON §4
// number grammar: /^-?[0-9]+(\.[0-9]+)?(e[+-]?[0-9]+)?$/i
func isTOONBare(s string) bool {
	if len(s) == 0 {
		return false
	}
	i := 0
	if s[i] == '-' {
		i++
	}
	if i >= len(s) || s[i] < '0' || s[i] > '9' {
		return false
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			return false
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			return false
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	return i == len(s)
}



func printJSONL(w io.Writer, result *engine.Result) error {
	// First line: metadata header
	meta := map[string]any{
		"type":        "meta",
		"columns":     result.Columns,
		"row_count":   result.RowCount,
		"duration_ms": float64(result.Duration.Milliseconds()),
	}
	if err := writeJSONL(w, meta); err != nil {
		return err
	}
	// Subsequent lines: one data object per row
	for _, row := range result.Rows {
		obj := make(map[string]any, len(result.Columns))
		obj["type"] = "row"
		for i, col := range result.Columns {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			obj[col] = coerceValue(val)
		}
		if err := writeJSONL(w, obj); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONL(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

// -------------------------------------------------------------------
// CSV — RFC 4180
// -------------------------------------------------------------------

func printCSV(w io.Writer, result *engine.Result) error {
	writeCSVRow(w, result.Columns)
	for _, row := range result.Rows {
		writeCSVRow(w, row)
	}
	return nil
}

func writeCSVRow(w io.Writer, fields []string) {
	for i, f := range fields {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		if strings.ContainsAny(f, ",\"\r\n") {
			f = `"` + strings.ReplaceAll(f, `"`, `""`) + `"`
		}
		fmt.Fprint(w, f)
	}
	fmt.Fprintln(w)
}

// -------------------------------------------------------------------
// Table — human-readable ASCII box
// -------------------------------------------------------------------

func printTable(w io.Writer, result *engine.Result) error {
	if len(result.Columns) == 0 {
		fmt.Fprintln(w, "(no columns)")
		return nil
	}

	// Compute column widths.
	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = utf8.RuneCountInString(col)
	}
	for _, row := range result.Rows {
		for i, cell := range row {
			if i < len(widths) {
				if n := utf8.RuneCountInString(cell); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}

	sep := buildSeparator(widths)
	fmt.Fprintln(w, sep)
	fmt.Fprintln(w, buildRow(result.Columns, widths))
	fmt.Fprintln(w, sep)
	for _, row := range result.Rows {
		fmt.Fprintln(w, buildRow(row, widths))
	}
	fmt.Fprintln(w, sep)
	fmt.Fprintf(w, "%d row(s) in %v\n", result.RowCount, result.Duration.Round(1_000_000))
	return nil
}

func buildSeparator(widths []int) string {
	var sb strings.Builder
	sb.WriteByte('+')
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2))
		sb.WriteByte('+')
	}
	return sb.String()
}

func buildRow(cells []string, widths []int) string {
	var sb strings.Builder
	sb.WriteByte('|')
	for i, w := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		pad := w - utf8.RuneCountInString(cell)
		sb.WriteByte(' ')
		sb.WriteString(cell)
		sb.WriteString(strings.Repeat(" ", pad))
		sb.WriteString(" |")
	}
	return sb.String()
}
