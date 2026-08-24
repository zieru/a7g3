// Package cli handles command-line argument parsing and validation for g3a.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
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
	OutputPNG   OutputFormat = "png"
	OutputImage OutputFormat = "image"
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
	OutFile    string // Output file path (e.g. report.png or data.csv)
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
	return ParseArgs(os.Args[1:])
}

// ParseArgs parses a slice of raw command-line arguments and returns validated Args.
func ParseArgs(rawArgs []string) (*Args, error) {
	a := &Args{}

	fs := flag.NewFlagSet("g3a", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // prevent automatic printing on error

	fs.StringVar(&a.Input, "input", "", "Input file path (.csv, .parquet, .sqlite/.db)")
	fs.StringVar(&a.Select, "select", "*", "SELECT expressions (e.g. \"count(1) as n, sum(amount) as total\")")
	fs.StringVar(&a.GroupBy, "group-by", "", "GROUP BY columns (comma-separated)")
	fs.StringVar(&a.Where, "where", "", "WHERE clause (e.g. \"amount > 100\")")
	fs.StringVar(&a.OrderBy, "order-by", "", "ORDER BY clause (e.g. \"total DESC\")")
	fs.IntVar(&a.Limit, "limit", 0, "Limit number of result rows (0 = no limit)")
	fs.StringVar((*string)(&a.Output), "output", "table", "Output format: table, csv, json, jsonl, toon, png/image")
	fs.StringVar(&a.OutFile, "out-file", "", "Output file path (e.g. table.png or out.csv)")
	fs.StringVar(&a.OutFile, "out", "", "Alias for --out-file")
	fs.IntVar(&a.ChunkSize, "chunk-size", 100_000, "Rows per chunk for CSV/Parquet streaming")
	fs.BoolVar(&a.NoHeader, "no-header", false, "Input CSV has no header row")
	fs.StringVar(&a.Delimiter, "delimiter", "", "CSV delimiter character (auto-detect if empty)")
	fs.StringVar(&a.Table, "table", "", "SQLite table name (auto-detect if empty)")
	fs.BoolVar(&a.Verbose, "verbose", false, "Print verbose/debug output")
	fs.StringVar(&a.Pivot, "pivot", "", "Comma-separated column name(s) to pivot into headers (e.g. month OR year,month)")
	fs.StringVar(&a.PivotFill, "pivot-fill", "0", "Fill value for missing pivot combinations")
	fs.StringVar(&a.PivotSep, "pivot-sep", "/", "Separator between values for multi-column pivot headers")

	fs.Usage = printUsage

	flagArgs, posArgs := splitFlagsAndPositional(rawArgs)

	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage()
			os.Exit(0)
		}
		return nil, err
	}

	// Auto-detect output format from --out-file extension if default output is table
	if a.OutFile != "" && a.Output == OutputTable {
		outExt := strings.ToLower(filepath.Ext(a.OutFile))
		switch outExt {
		case ".png":
			a.Output = OutputPNG
		case ".csv":
			a.Output = OutputCSV
		case ".json":
			a.Output = OutputJSON
		case ".jsonl":
			a.Output = OutputJSONL
		case ".toon":
			a.Output = OutputTOON
		}
	}

	// Resolve input file from positional alias or direct filepath if not specified via --input
	if a.Input == "" {
		if len(posArgs) > 0 {
			target := posArgs[0]
			resolved, err := resolveTargetOrAlias(target)
			if err != nil {
				return nil, err
			}
			a.Input = resolved
		} else {
			return nil, errors.New("--input is required (or specify an alias from ~/.g3a.config)")
		}
	}

	return a, a.validate()
}

// splitFlagsAndPositional separates flag arguments from positional arguments.
func splitFlagsAndPositional(args []string) ([]string, []string) {
	var flagArgs []string
	var posArgs []string

	valFlags := map[string]bool{
		"input":      true,
		"select":     true,
		"group-by":   true,
		"where":      true,
		"order-by":   true,
		"limit":      true,
		"output":     true,
		"out-file":   true,
		"out":        true,
		"chunk-size": true,
		"delimiter":  true,
		"table":      true,
		"pivot":      true,
		"pivot-fill": true,
		"pivot-sep":  true,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)

			cleanFlag := strings.TrimLeft(arg, "-")
			if !strings.Contains(cleanFlag, "=") && valFlags[cleanFlag] {
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
					flagArgs = append(flagArgs, args[i])
				}
			}
		} else {
			posArgs = append(posArgs, arg)
		}
	}

	return flagArgs, posArgs
}

// resolveTargetOrAlias checks whether target is a defined alias in .g3a.config or a direct file path.
func resolveTargetOrAlias(target string) (string, error) {
	aliases, cfgPath, cfgErr := LoadConfig()
	if cfgErr != nil {
		return "", fmt.Errorf("read config file %s: %w", cfgPath, cfgErr)
	}

	if resolved, ok := aliases[target]; ok {
		return resolved, nil
	}

	// Check if target is a file on disk
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}

	if cfgPath != "" {
		return "", fmt.Errorf("alias %q not found in %s and file does not exist", target, cfgPath)
	}
	return "", fmt.Errorf("file %q does not exist (no ~/.g3a.config found)", target)
}

func (a *Args) validate() error {
	if a.Input == "" {
		return errors.New("--input is required")
	}

	// If input contains wildcards (* or ?), handle as glob pattern
	if strings.ContainsAny(a.Input, "*?") {
		ext := strings.ToLower(filepath.Ext(a.Input))
		if ext == ".parquet" || strings.Contains(a.Input, ".parquet") {
			a.Format = FormatParquet
		} else if ext == ".csv" || strings.Contains(a.Input, ".csv") {
			a.Format = FormatCSV
		} else {
			a.Format = FormatParquet
		}
	} else {
		stat, err := os.Stat(a.Input)
		if err != nil {
			return fmt.Errorf("cannot access input file or directory %q: %w", a.Input, err)
		}

		if stat.IsDir() {
			// If input is a directory, default to Parquet directory scanning
			a.Format = FormatParquet
		} else {
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
		}
	}

	switch a.Output {
	case OutputTable, OutputCSV, OutputJSON, OutputJSONL, OutputTOON, OutputPNG, OutputImage:
		// valid
	default:
		return fmt.Errorf("invalid --output value %q (valid: table, csv, json, jsonl, toon, png, image)", a.Output)
	}

	if a.ChunkSize <= 0 {
		return fmt.Errorf("--chunk-size must be > 0, got %d", a.ChunkSize)
	}

	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `g3a — BigQuery-style file query CLI

Usage:
  g3a [alias | FILE] [options]
  g3a --input=FILE [options]

Examples:
  g3a funneling --select="count(1) as n, sum(amount) as total"
  g3a data.csv --select="count(1) as n" --group-by=category
  g3a --input=data.parquet --where="region='WEST'" --output=csv

Alias Configuration:
  Define file aliases in ~/.g3a.config (or C:\Users\<user>\.g3a.config):
    funneling /path/to/funneling.csv
    funnelingx /path/to/funneling.parquet

Options:
`)
	fs := flag.NewFlagSet("g3a", flag.ContinueOnError)
	dummy := &Args{}
	fs.StringVar(&dummy.Input, "input", "", "Input file path (.csv, .parquet, .sqlite/.db)")
	fs.StringVar(&dummy.Select, "select", "*", "SELECT expressions (e.g. \"count(1) as n, sum(amount) as total\")")
	fs.StringVar(&dummy.GroupBy, "group-by", "", "GROUP BY columns (comma-separated)")
	fs.StringVar(&dummy.Where, "where", "", "WHERE clause (e.g. \"amount > 100\")")
	fs.StringVar(&dummy.OrderBy, "order-by", "", "ORDER BY clause (e.g. \"total DESC\")")
	fs.IntVar(&dummy.Limit, "limit", 0, "Limit number of result rows (0 = no limit)")
	fs.StringVar((*string)(&dummy.Output), "output", "table", "Output format: table, csv, json, jsonl, toon (toon=token-efficient for LLM, jsonl=streaming JSON Lines)")
	fs.IntVar(&dummy.ChunkSize, "chunk-size", 100_000, "Rows per chunk for CSV/Parquet streaming")
	fs.BoolVar(&dummy.NoHeader, "no-header", false, "Input CSV has no header row")
	fs.StringVar(&dummy.Delimiter, "delimiter", "", "CSV delimiter character (auto-detect if empty)")
	fs.StringVar(&dummy.Table, "table", "", "SQLite table name (auto-detect if empty)")
	fs.BoolVar(&dummy.Verbose, "verbose", false, "Print verbose/debug output")
	fs.StringVar(&dummy.Pivot, "pivot", "", "Comma-separated column name(s) to pivot into headers (e.g. month OR year,month)")
	fs.StringVar(&dummy.PivotFill, "pivot-fill", "0", "Fill value for missing pivot combinations")
	fs.StringVar(&dummy.PivotSep, "pivot-sep", "/", "Separator between values for multi-column pivot headers")
	fs.SetOutput(os.Stderr)
	fs.PrintDefaults()
}
