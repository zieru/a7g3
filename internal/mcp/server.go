// Package mcp implements Model Context Protocol (MCP) server functionality for g3a.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/a7g3/g3a/internal/cli"
	"github.com/a7g3/g3a/internal/engine"
	"github.com/a7g3/g3a/internal/output"
	"github.com/a7g3/g3a/internal/pivot"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the mark3labs MCP Server and DuckDB engine.
type Server struct {
	mcpServer *server.MCPServer
	eng       *engine.Engine
	verbose   bool
}

// NewServer initializes a new g3a MCP Server instance with registered analytical tools.
func NewServer(eng *engine.Engine, verbose bool) *Server {
	s := &Server{
		eng:     eng,
		verbose: verbose,
	}

	mcpServer := server.NewMCPServer(
		"g3a-analytics",
		"1.0.0",
		server.WithDescription("High-performance BigQuery-style analytics and DuckDB query engine for Parquet, CSV, and SQLite"),
	)
	s.mcpServer = mcpServer
	s.registerTools()

	return s
}

// MCPServer returns the underlying mark3labs MCPServer.
func (s *Server) MCPServer() *server.MCPServer {
	return s.mcpServer
}

// registerTools registers all analytics tools to the MCP server.
func (s *Server) registerTools() {
	// 1. list_datasets
	s.mcpServer.AddTool(
		mcp.NewTool(
			"list_datasets",
			mcp.WithDescription("List all configured dataset aliases from ~/.g3a.config or working directory"),
		),
		s.handleListDatasets,
	)

	// 2. describe_dataset
	s.mcpServer.AddTool(
		mcp.NewTool(
			"describe_dataset",
			mcp.WithDescription("Inspect the schema (column names and data types) of a dataset or file"),
			mcp.WithString("dataset", mcp.Required(), mcp.Description("Dataset alias (e.g. 'funneling') or file path (.parquet, .csv, .sqlite)")),
			mcp.WithString("delimiter", mcp.Description("Optional CSV delimiter (e.g. ',' or ';')")),
			mcp.WithString("table", mcp.Description("Optional SQLite table name")),
		),
		s.handleDescribeDataset,
	)

	// 3. query_analytics
	s.mcpServer.AddTool(
		mcp.NewTool(
			"query_analytics",
			mcp.WithDescription("Execute analytical query on Parquet/CSV/SQLite with DuckDB (filtering, aggregations, grouping, sorting, limit, and dynamic pivoting)"),
			mcp.WithString("dataset", mcp.Required(), mcp.Description("Dataset alias (e.g. 'funneling', 'visit') or path to file/directory")),
			mcp.WithString("select", mcp.Description("SQL SELECT expressions (default '*')")),
			mcp.WithString("where", mcp.Description("SQL WHERE clause filter expressions")),
			mcp.WithString("group_by", mcp.Description("SQL GROUP BY column(s)")),
			mcp.WithString("order_by", mcp.Description("SQL ORDER BY expressions")),
			mcp.WithNumber("limit", mcp.Description("Maximum rows to return (default: 50, 0 for unlimited)")),
			mcp.WithString("pivot", mcp.Description("Optional column(s) to pivot into headers (e.g. 'flag_dilayani' or 'regional,service')")),
			mcp.WithString("pivot_fill", mcp.Description("Default fill value for empty cells in pivot matrix (default '0')")),
			mcp.WithString("pivot_sep", mcp.Description("Header separator for multi-column pivot keys (default '/')")),
			mcp.WithString("output_format", mcp.Description("Output format: 'json' (default), 'table', 'csv', 'toon'")),
		),
		s.handleQueryAnalytics,
	)

	// 4. run_sql
	s.mcpServer.AddTool(
		mcp.NewTool(
			"run_sql",
			mcp.WithDescription("Execute raw DuckDB SQL directly (for complex JOINs, CTEs, Window functions, etc.)"),
			mcp.WithString("sql", mcp.Required(), mcp.Description("SQL statement to execute in DuckDB")),
			mcp.WithString("output_format", mcp.Description("Output format: 'json' (default), 'table', 'csv', 'toon'")),
		),
		s.handleRunSQL,
	)

	// 5. export_chart_image
	s.mcpServer.AddTool(
		mcp.NewTool(
			"export_chart_image",
			mcp.WithDescription("Execute query, format pivot if needed, and render high-resolution PNG image file directly"),
			mcp.WithString("dataset", mcp.Required(), mcp.Description("Dataset alias or file path")),
			mcp.WithString("out_file", mcp.Required(), mcp.Description("Destination PNG file path (e.g. 'chart.png')")),
			mcp.WithString("select", mcp.Description("SQL SELECT expressions (default '*')")),
			mcp.WithString("where", mcp.Description("SQL WHERE clause filter expressions")),
			mcp.WithString("group_by", mcp.Description("SQL GROUP BY column(s)")),
			mcp.WithString("order_by", mcp.Description("SQL ORDER BY expressions")),
			mcp.WithNumber("limit", mcp.Description("Maximum rows to return (default: 50, 0 for unlimited)")),
			mcp.WithString("pivot", mcp.Description("Optional column(s) to pivot into headers")),
			mcp.WithString("pivot_fill", mcp.Description("Default fill value for empty cells (default '0')")),
			mcp.WithString("pivot_sep", mcp.Description("Header separator for multi-column pivot keys (default '/')")),
		),
		s.handleExportChartImage,
	)
}

func (s *Server) handleListDatasets(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	aliases, cfgPath, err := cli.LoadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to load config: %v", err)), nil
	}

	type DatasetInfo struct {
		Alias  string `json:"alias"`
		Path   string `json:"path"`
		Format string `json:"format"`
		Exists bool   `json:"exists"`
	}

	var list []DatasetInfo
	for alias, p := range aliases {
		exists := false
		fmtType := "unknown"
		cleanP := filepath.Clean(p)
		if stat, err := os.Stat(cleanP); err == nil {
			exists = true
			if detected, err := cli.DetectFormat(cleanP); err == nil {
				fmtType = string(detected)
			}
			if stat.IsDir() {
				fmtType = fmt.Sprintf("%s (directory)", fmtType)
			}
		} else {
			if detected, err := cli.DetectFormat(p); err == nil {
				fmtType = string(detected)
			}
		}
		list = append(list, DatasetInfo{
			Alias:  alias,
			Path:   p,
			Format: fmtType,
			Exists: exists,
		})
	}

	resp := map[string]any{
		"config_file": cfgPath,
		"datasets":    list,
		"count":       len(list),
	}
	b, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func (s *Server) handleDescribeDataset(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dataset, err := request.RequireString("dataset")
	if err != nil {
		return mcp.NewToolResultError("dataset parameter is required"), nil
	}
	delimiter := request.GetString("delimiter", "")
	table := request.GetString("table", "")

	resolvedPath, err := cli.ResolveTargetOrAlias(dataset)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot resolve dataset: %v", err)), nil
	}

	fmtType, err := cli.DetectFormat(resolvedPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot detect format: %v", err)), nil
	}

	if fmtType == cli.FormatSQLite {
		if err := s.eng.InstallSQLite(ctx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("install sqlite: %v", err)), nil
		}
		if table == "" {
			t, err := s.eng.DetectSQLiteTable(ctx, resolvedPath)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("detect sqlite table: %v", err)), nil
			}
			table = t
		}
	}

	opts := engine.QueryOptions{
		InputPath: resolvedPath,
		Format:    string(fmtType),
		Delimiter: delimiter,
		Table:     table,
		Verbose:   s.verbose,
	}

	res, err := s.eng.Describe(ctx, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("describe error: %v", err)), nil
	}

	var buf bytes.Buffer
	if err := output.Print(&buf, res, "json", s.verbose); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("format output: %v", err)), nil
	}

	return mcp.NewToolResultText(buf.String()), nil
}

func (s *Server) handleQueryAnalytics(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dataset, err := request.RequireString("dataset")
	if err != nil {
		return mcp.NewToolResultError("dataset parameter is required"), nil
	}
	sel := request.GetString("select", "*")
	if sel == "" {
		sel = "*"
	}
	where := request.GetString("where", "")
	groupBy := request.GetString("group_by", "")
	orderBy := request.GetString("order_by", "")
	limit := request.GetInt("limit", 50)
	pivotCol := request.GetString("pivot", "")
	pivotFill := request.GetString("pivot_fill", "0")
	pivotSep := request.GetString("pivot_sep", "/")
	outFmt := strings.ToLower(request.GetString("output_format", "json"))

	resolvedPath, err := cli.ResolveTargetOrAlias(dataset)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot resolve dataset: %v", err)), nil
	}

	fmtType, err := cli.DetectFormat(resolvedPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot detect format: %v", err)), nil
	}

	table := ""
	if fmtType == cli.FormatSQLite {
		if err := s.eng.InstallSQLite(ctx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("install sqlite: %v", err)), nil
		}
		t, err := s.eng.DetectSQLiteTable(ctx, resolvedPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("detect sqlite table: %v", err)), nil
		}
		table = t
	}

	opts := engine.QueryOptions{
		InputPath: resolvedPath,
		Format:    string(fmtType),
		Select:    sel,
		Where:     where,
		GroupBy:   groupBy,
		OrderBy:   orderBy,
		Limit:     limit,
		Table:     table,
		Verbose:   s.verbose,
	}

	res, err := s.eng.Run(ctx, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("query error: %v", err)), nil
	}

	if pivotCol != "" {
		var pivotCols []string
		for _, p := range strings.Split(pivotCol, ",") {
			if t := strings.TrimSpace(p); t != "" {
				pivotCols = append(pivotCols, t)
			}
		}
		res, err = pivot.Apply(res, pivot.Options{
			PivotCols: pivotCols,
			FillValue: pivotFill,
			Separator: pivotSep,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("pivot error: %v", err)), nil
		}
	}

	var buf bytes.Buffer
	if err := output.Print(&buf, res, outFmt, s.verbose); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("print error: %v", err)), nil
	}

	return mcp.NewToolResultText(buf.String()), nil
}

func (s *Server) handleRunSQL(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sqlQuery, err := request.RequireString("sql")
	if err != nil {
		return mcp.NewToolResultError("sql parameter is required"), nil
	}
	outFmt := strings.ToLower(request.GetString("output_format", "json"))

	res, err := s.eng.QueryRaw(ctx, sqlQuery)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sql error: %v", err)), nil
	}

	var buf bytes.Buffer
	if err := output.Print(&buf, res, outFmt, s.verbose); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("print error: %v", err)), nil
	}

	return mcp.NewToolResultText(buf.String()), nil
}

func (s *Server) handleExportChartImage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dataset, err := request.RequireString("dataset")
	if err != nil {
		return mcp.NewToolResultError("dataset parameter is required"), nil
	}
	outFile, err := request.RequireString("out_file")
	if err != nil {
		return mcp.NewToolResultError("out_file parameter is required"), nil
	}
	sel := request.GetString("select", "*")
	if sel == "" {
		sel = "*"
	}
	where := request.GetString("where", "")
	groupBy := request.GetString("group_by", "")
	orderBy := request.GetString("order_by", "")
	limit := request.GetInt("limit", 50)
	pivotCol := request.GetString("pivot", "")
	pivotFill := request.GetString("pivot_fill", "0")
	pivotSep := request.GetString("pivot_sep", "/")

	resolvedPath, err := cli.ResolveTargetOrAlias(dataset)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot resolve dataset: %v", err)), nil
	}

	fmtType, err := cli.DetectFormat(resolvedPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot detect format: %v", err)), nil
	}

	table := ""
	if fmtType == cli.FormatSQLite {
		if err := s.eng.InstallSQLite(ctx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("install sqlite: %v", err)), nil
		}
		t, err := s.eng.DetectSQLiteTable(ctx, resolvedPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("detect sqlite table: %v", err)), nil
		}
		table = t
	}

	opts := engine.QueryOptions{
		InputPath: resolvedPath,
		Format:    string(fmtType),
		Select:    sel,
		Where:     where,
		GroupBy:   groupBy,
		OrderBy:   orderBy,
		Limit:     limit,
		Table:     table,
		Verbose:   s.verbose,
	}

	res, err := s.eng.Run(ctx, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("query error: %v", err)), nil
	}

	if pivotCol != "" {
		var pivotCols []string
		for _, p := range strings.Split(pivotCol, ",") {
			if t := strings.TrimSpace(p); t != "" {
				pivotCols = append(pivotCols, t)
			}
		}
		res, err = pivot.Apply(res, pivot.Options{
			PivotCols: pivotCols,
			FillValue: pivotFill,
			Separator: pivotSep,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("pivot error: %v", err)), nil
		}
	}

	f, err := os.Create(outFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create image file %q: %v", outFile, err)), nil
	}
	defer f.Close()

	if err := output.RenderPNG(f, res); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("render image: %v", err)), nil
	}

	resp := map[string]any{
		"ok":          true,
		"file_path":   outFile,
		"row_count":   res.RowCount,
		"duration_ms": float64(res.Duration.Microseconds()) / 1000.0,
		"message":     fmt.Sprintf("Successfully generated chart image at %s (%d rows)", outFile, res.RowCount),
	}
	b, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}
