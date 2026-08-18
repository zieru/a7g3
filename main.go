// g3a — BigQuery-style CLI query tool for local files (CSV, Parquet, SQLite).
//
// Usage:
//
//	g3a --input=data.csv --select="count(1) as n, sum(amount) as total" --group-by=category
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/a7g3/g3a/internal/cli"
	"github.com/a7g3/g3a/internal/engine"
	"github.com/a7g3/g3a/internal/output"
	"github.com/a7g3/g3a/internal/pivot"
)

func main() {
	args, parseErr := cli.Parse()
	if parseErr != nil {
		fmt.Fprintf(os.Stderr, "g3a: error: %v\n", parseErr)
		os.Exit(1)
	}

	if err := run(args); err != nil {
		output.PrintError(os.Stderr, err, string(args.Output))
		os.Exit(1)
	}
}

func run(args *cli.Args) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	eng, err := engine.New()
	if err != nil {
		return fmt.Errorf("init engine: %w", err)
	}
	defer eng.Close()

	if args.Format == cli.FormatSQLite {
		if err := eng.InstallSQLite(ctx); err != nil {
			return fmt.Errorf("install sqlite extension: %w", err)
		}
		if args.Table == "" {
			table, err := eng.DetectSQLiteTable(ctx, args.Input)
			if err != nil {
				return fmt.Errorf("detect sqlite table: %w", err)
			}
			args.Table = table
			if args.Verbose {
				fmt.Fprintf(os.Stderr, "[g3a] using sqlite table: %s\n", table)
			}
		}
	}

	opts := engine.QueryOptions{
		InputPath: args.Input,
		Format:    strings.ToLower(string(args.Format)),
		Select:    args.Select,
		GroupBy:   args.GroupBy,
		Where:     args.Where,
		OrderBy:   args.OrderBy,
		Limit:     args.Limit,
		ChunkSize: args.ChunkSize,
		NoHeader:  args.NoHeader,
		Delimiter: args.Delimiter,
		Table:     args.Table,
		Verbose:   args.Verbose,
	}

	result, err := eng.Run(ctx, opts)
	if err != nil {
		return err
	}

	// Apply pivot transformation if requested.
	if args.Pivot != "" {
		pivotCols := splitTrimmed(args.Pivot)
		result, err = pivot.Apply(result, pivot.Options{
			PivotCols: pivotCols,
			FillValue: args.PivotFill,
			Separator: args.PivotSep,
		})
		if err != nil {
			return fmt.Errorf("pivot: %w", err)
		}
	}

	var outWriter io.Writer = os.Stdout
	if args.OutFile != "" {
		f, err := os.Create(args.OutFile)
		if err != nil {
			return fmt.Errorf("create output file %q: %w", args.OutFile, err)
		}
		defer f.Close()
		outWriter = f
	}

	if err := output.Print(outWriter, result, string(args.Output), args.Verbose); err != nil {
		return err
	}

	if args.OutFile != "" && args.Verbose {
		fmt.Fprintf(os.Stderr, "[g3a] wrote output to %s\n", args.OutFile)
	}

	return nil
}

// splitTrimmed splits s by comma and trims spaces from each element.
func splitTrimmed(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
