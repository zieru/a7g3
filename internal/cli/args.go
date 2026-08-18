// Package cli handles command-line argument parsing and validation for g3a.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InputFormat represents the type of input file.
type InputFormat string

const (
	FormatCSV     InputFormat = "csv"
	FormatParquet InputFormat = "parquet"
	FormatSQLite  InputFormat = "sqlite"
)

// OutputFormat represents how results are printed.
type OutputFormat string

const (
	OutputTable OutputFormat = "table"
	OutputCSV   OutputFormat = "csv"
	OutputJSON  OutputFormat = "json"
	OutputJSONL OutputFormat = "jsonl"
	OutputTOON  OutputFormat = "toon"
)

// Args holds all parsed CLI arguments.
type Args struct {
	Input      string
	Format     InputFormat
	Select     string
	GroupBy    string
	Where      string
	OrderBy    string
	Limit      int
	Output     OutputFormat
	ChunkSize  int
	NoHeader   bool
	Delimiter  string
	Table      string // SQLite table name override
	Verbose    bool
	// Pivot options
	Pivot      string // comma-separated column names to pivot on
	PivotFill  string // fill value for missing combos (default "0")
	PivotSep   string // separator for multi-column pivot headers (default "/")
}

// Parse parses os.Args and returns validated Args.
func Parse() (*Args, error) {
	a := &Args{}

	flag.StringVar(&a.Input, "input", "", "Input file path (.csv, .parquet, .sqlite/.db)")
	flag.StringVar(&a.Select, "select", "*", "SELECT expressions (e.g. \"count(1) as n, sum(amount) as total\")")
	flag.StringVar(&a.GroupBy, "group-by", "", "GROUP BY columns (comma-separated)")
	flag.StringVar(&a.Where, "where", "", "WHERE clause (e.g. \"amount > 100\")")
	flag.StringVar(&a.OrderBy, "order-by", "", "ORDER BY clause (e.g. \"total DESC\")")
	flag.IntVar(&a.Limit, "limit", 0, "Limit number of result rows (0 = no limit)")
	flag.StringVar((*string)(&a.Output), "output", "table", "Output format: table, csv, json, jsonl, toon (toon=token-efficient for LLM, jsonl=streaming JSON Lines)")
	flag.IntVar(&a.ChunkSize, "chunk-size", 100_000, "Rows per chunk for CSV/Parquet streaming")
	flag.BoolVar(&a.NoHeader, "no-header", false, "Input CSV has no header row")
	flag.StringVar(&a.Delimiter, "delimiter", "", "CSV delimiter character (auto-detect if empty)")
	flag.StringVar(&a.Table, "table", "", "SQLite table name (auto-detect if empty)")
	flag.BoolVar(&a.Verbose, "verbose", false, "Print verbose/debug output")
	flag.StringVar(&a.Pivot, "pivot", "", "Comma-separated column name(s) to pivot into headers (e.g. month OR year,month)")
	flag.StringVar(&a.PivotFill, "pivot-fill", "0", "Fill value for missing pivot combinations")
	flag.StringVar(&a.PivotSep, "pivot-sep", "/", "Separator between values for multi-column pivot headers")

	flag.Usage = printUsage
	flag.Parse()

	return a, a.validate()
}

func (a *Args) validate() error {
	if a.Input == "" {
		return errors.New("--input is required")
	}

	if _, err := os.Stat(a.Input); err != nil {
		return fmt.Errorf("cannot access input file %q: %w", a.Input, err)
	}

	ext := strings.ToLower(filepath.Ext(a.Input))
	switch ext {
	case ".csv", ".tsv", ".txt":
		a.Format = FormatCSV
		if a.Delimiter == "" && ext == ".tsv" {
			a.Delimiter = "\t"
		}
	case ".parquet":
		a.Format = FormatParquet
	case ".sqlite", ".db", ".sqlite3":
		a.Format = FormatSQLite
	default:
		return fmt.Errorf("unsupported file extension %q (supported: .csv, .tsv, .parquet, .sqlite, .db)", ext)
	}

	switch a.Output {
	case OutputTable, OutputCSV, OutputJSON, OutputJSONL, OutputTOON:
		// valid
	default:
		return fmt.Errorf("invalid --output value %q (valid: table, csv, json, jsonl, toon)", a.Output)
	}

	if a.ChunkSize <= 0 {
		return fmt.Errorf("--chunk-size must be > 0, got %d", a.ChunkSize)
	}

	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `g3a — BigQuery-style file query CLI

Usage:
  g3a --input=FILE [options]

Examples:
  g3a --input=data.csv --select="count(1) as n, sum(amount) as total" --group-by=category
  g3a --input=data.parquet --select="max(price), min(price)" --where="region='WEST'" --output=csv
  g3a --input=data.sqlite --select="*" --limit=100

Options:
`)
	flag.PrintDefaults()
}
