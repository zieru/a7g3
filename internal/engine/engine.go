// Package engine wraps DuckDB to execute streaming queries over local files.
package engine

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/marcboeker/go-duckdb/v2"
)

// Engine holds a DuckDB in-memory connection.
type Engine struct {
	db *sql.DB
}

// New creates a new in-memory DuckDB engine.
func New() (*Engine, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	// DuckDB in-memory is single-connection; serialise through one conn.
	db.SetMaxOpenConns(1)
	return &Engine{db: db}, nil
}

// Close releases the DuckDB connection.
func (e *Engine) Close() error {
	return e.db.Close()
}

// exec runs a statement and discards results.
func (e *Engine) exec(ctx context.Context, stmt string) error {
	_, err := e.db.ExecContext(ctx, stmt)
	return err
}

// InstallSQLite installs and loads the DuckDB sqlite_scanner extension.
func (e *Engine) InstallSQLite(ctx context.Context) error {
	if err := e.exec(ctx, "INSTALL sqlite"); err != nil {
		return fmt.Errorf("install sqlite extension: %w", err)
	}
	if err := e.exec(ctx, "LOAD sqlite"); err != nil {
		return fmt.Errorf("load sqlite extension: %w", err)
	}
	return nil
}

// DetectSQLiteTable returns the first non-system table name from the SQLite file.
func (e *Engine) DetectSQLiteTable(ctx context.Context, path string) (string, error) {
	escaped := strings.ReplaceAll(path, "'", "''")
	query := fmt.Sprintf(
		"SELECT name FROM sqlite_scan('%s', 'sqlite_master') WHERE type='table' AND name NOT LIKE 'sqlite_%%' LIMIT 1",
		escaped,
	)
	row := e.db.QueryRowContext(ctx, query)
	var name string
	if err := row.Scan(&name); err != nil {
		// Fallback: just try to read the schema directly
		return "", fmt.Errorf("could not detect table name (use --table=NAME): %w", err)
	}
	return name, nil
}

// QueryOptions holds all query parameters.
type QueryOptions struct {
	InputPath string
	Format    string // "csv" | "parquet" | "sqlite"
	Select    string
	GroupBy   string
	Where     string
	OrderBy   string
	Limit     int
	ChunkSize int
	NoHeader  bool
	Delimiter string
	Table     string // SQLite table override
	Verbose   bool
}

// Result holds query output.
type Result struct {
	Columns  []string
	Rows     [][]string
	Duration time.Duration
	RowCount int
	SQL      string // The SQL that was executed
}

// Run builds and executes the DuckDB SQL query, returning all result rows.
func (e *Engine) Run(ctx context.Context, opts QueryOptions) (*Result, error) {
	query, err := buildQuery(opts)
	if err != nil {
		return nil, err
	}

	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "[g3a] SQL:\n%s\n\n", query)
	}

	start := time.Now()
	sqlRows, err := e.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w\nSQL: %s", err, query)
	}
	defer sqlRows.Close()

	cols, err := sqlRows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	result := &Result{Columns: cols, SQL: query}
	scanBuf := make([]any, len(cols))
	scanPtrs := make([]any, len(cols))
	for i := range scanBuf {
		scanPtrs[i] = &scanBuf[i]
	}

	for sqlRows.Next() {
		if err := sqlRows.Scan(scanPtrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		row := make([]string, len(cols))
		for i, v := range scanBuf {
			row[i] = valueToString(v)
		}
		result.Rows = append(result.Rows, row)
	}

	if err := sqlRows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	result.Duration = time.Since(start)
	result.RowCount = len(result.Rows)
	return result, nil
}

// buildQuery constructs the DuckDB SQL from QueryOptions.
func buildQuery(opts QueryOptions) (string, error) {
	var from string

	switch opts.Format {
	case "csv":
		from = buildCSVSource(opts)
	case "parquet":
		inputPath := opts.InputPath
		if stat, err := os.Stat(inputPath); err == nil && stat.IsDir() {
			inputPath = filepath.ToSlash(filepath.Join(inputPath, "*.parquet"))
		} else {
			inputPath = filepath.ToSlash(inputPath)
		}
		escaped := strings.ReplaceAll(inputPath, "'", "''")
		from = fmt.Sprintf("read_parquet('%s', union_by_name=true)", escaped)
	case "sqlite":
		from = buildSQLiteSource(opts)
	default:
		return "", fmt.Errorf("unknown format: %s", opts.Format)
	}

	var sb strings.Builder
	sb.WriteString("SELECT ")
	sb.WriteString(opts.Select)
	sb.WriteString("\nFROM ")
	sb.WriteString(from)

	if opts.Where != "" {
		sb.WriteString("\nWHERE ")
		sb.WriteString(opts.Where)
	}

	if opts.GroupBy != "" {
		sb.WriteString("\nGROUP BY ")
		sb.WriteString(opts.GroupBy)
	}

	if opts.OrderBy != "" {
		sb.WriteString("\nORDER BY ")
		sb.WriteString(opts.OrderBy)
	}

	if opts.Limit > 0 {
		sb.WriteString(fmt.Sprintf("\nLIMIT %d", opts.Limit))
	}

	return sb.String(), nil
}

// buildCSVSource constructs a DuckDB read_csv() call with options.
func buildCSVSource(opts QueryOptions) string {
	escaped := strings.ReplaceAll(opts.InputPath, "'", "''")

	params := []string{
		fmt.Sprintf("'%s'", escaped),
		"auto_detect=true",
		// Tolerate BOM, non-standard line endings, and encoding quirks.
		"strict_mode=false",
	}
	if opts.NoHeader {
		params = append(params, "header=false")
	}
	if opts.Delimiter != "" {
		d := strings.ReplaceAll(opts.Delimiter, "'", "''")
		params = append(params, fmt.Sprintf("delim='%s'", d))
	}
	if opts.ChunkSize > 0 {
		params = append(params, fmt.Sprintf("buffer_size=%d", opts.ChunkSize))
	}
	return fmt.Sprintf("read_csv(%s)", strings.Join(params, ", "))
}

// buildSQLiteSource returns a sqlite_scan() table function call.
func buildSQLiteSource(opts QueryOptions) string {
	escaped := strings.ReplaceAll(opts.InputPath, "'", "''")
	table := opts.Table
	if table == "" {
		table = "main"
	}
	tableEscaped := strings.ReplaceAll(table, "'", "''")
	return fmt.Sprintf("sqlite_scan('%s', '%s')", escaped, tableEscaped)
}

// valueToString converts a sql.Scan value to a display string.
func valueToString(v any) string {
	if v == nil {
		return "NULL"
	}
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", t)
	}
}
