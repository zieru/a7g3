// Package mcp implements Model Context Protocol (MCP) server functionality for g3a.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/a7g3/g3a/internal/cli"
	"github.com/a7g3/g3a/internal/engine"
	"github.com/a7g3/g3a/internal/grapari"
	"github.com/a7g3/g3a/internal/output"
	"github.com/a7g3/g3a/internal/pivot"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the mark3labs MCP Server, DuckDB engine, and GraPARI knowledge system.
type Server struct {
	mcpServer *server.MCPServer
	eng       *engine.Engine
	syncer    *grapari.Syncer
	watcher   *grapari.Watcher
	verbose   bool
}

// NewServer initializes a new g3a MCP Server instance with registered analytical and GraPARI knowledge tools.
func NewServer(eng *engine.Engine, syncer *grapari.Syncer, watcher *grapari.Watcher, verbose bool) *Server {
	s := &Server{
		eng:     eng,
		syncer:  syncer,
		watcher: watcher,
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
			mcp.WithString("dataset", mcp.Required(), mcp.Description("Dataset alias (e.g. 'funneling') or file path (.parquet, .csv, .sqlite, .xlsx)")),
			mcp.WithString("delimiter", mcp.Description("Optional CSV delimiter (e.g. ',' or ';')")),
			mcp.WithString("table", mcp.Description("Optional SQLite table name")),
			mcp.WithString("sheet", mcp.Description("Optional Excel worksheet name")),
			mcp.WithBoolean("all_varchar", mcp.Description("Force reading all columns as text (VARCHAR) to avoid mixed-type parse errors")),
		),
		s.handleDescribeDataset,
	)

	// 3. query_analytics
	s.mcpServer.AddTool(
		mcp.NewTool(
			"query_analytics",
			mcp.WithDescription("Execute analytical query on Parquet/CSV/SQLite/Excel with DuckDB (filtering, aggregations, grouping, sorting, limit, and dynamic pivoting)"),
			mcp.WithString("dataset", mcp.Required(), mcp.Description("Dataset alias (e.g. 'funneling', 'visit') or path to file/directory")),
			mcp.WithString("select", mcp.Description("SQL SELECT expressions (default '*')")),
			mcp.WithString("where", mcp.Description("SQL WHERE clause filter expressions")),
			mcp.WithString("group_by", mcp.Description("SQL GROUP BY column(s)")),
			mcp.WithString("order_by", mcp.Description("SQL ORDER BY expressions")),
			mcp.WithNumber("limit", mcp.Description("Maximum rows to return (default: 50, 0 for unlimited)")),
			mcp.WithString("sheet", mcp.Description("Optional Excel worksheet name")),
			mcp.WithBoolean("all_varchar", mcp.Description("Force reading all columns as text (VARCHAR) to avoid mixed-type parse errors")),
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
			mcp.WithString("out_file", mcp.Description("Optional destination PNG file path (e.g. '/tmp/chart.png'). If omitted, a temporary file is automatically created.")),
			mcp.WithString("select", mcp.Description("SQL SELECT expressions (default '*')")),
			mcp.WithString("where", mcp.Description("SQL WHERE clause filter expressions")),
			mcp.WithString("group_by", mcp.Description("SQL GROUP BY column(s)")),
			mcp.WithString("order_by", mcp.Description("SQL ORDER BY expressions")),
			mcp.WithNumber("limit", mcp.Description("Maximum rows to return (default: 50, 0 for unlimited)")),
			mcp.WithString("sheet", mcp.Description("Optional Excel worksheet name")),
			mcp.WithBoolean("all_varchar", mcp.Description("Force reading all columns as text (VARCHAR) to avoid mixed-type parse errors")),
			mcp.WithString("pivot", mcp.Description("Optional column(s) to pivot into headers")),
			mcp.WithString("pivot_fill", mcp.Description("Default fill value for empty cells (default '0')")),
			mcp.WithString("pivot_sep", mcp.Description("Header separator for multi-column pivot keys (default '/')")),
		),
		s.handleExportChartImage,
	)

	// 6. search_grapari_knowledge
	s.mcpServer.AddTool(
		mcp.NewTool(
			"search_grapari_knowledge",
			mcp.WithDescription("Cari referensi SOP GraPARI, link spreadsheet/form, petunjuk MSISDN, dan email PIC/CC secara ringkas dan hemat token"),
			mcp.WithString("query", mcp.Required(), mcp.Description("Kata kunci atau pertanyaan (misal: 'referral orbit', 'user raisa', 'psb kendala', 'goodie bag')")),
			mcp.WithString("category", mcp.Description("Filter opsional berdasarkan kategori (misal: 'User Management', 'Orbit & NTE', 'Kindi (Halo A1)')")),
			mcp.WithNumber("limit", mcp.Description("Jumlah task maksimal yang dikembalikan (default: 3)")),
		),
		s.handleSearchGrapariKnowledge,
	)

	// 7. trigger_grapari_sync
	s.mcpServer.AddTool(
		mcp.NewTool(
			"trigger_grapari_sync",
			mcp.WithDescription("Jalankan sinkronisasi bertahap (incremental sync) dari spreadsheet/CSV GraPARI ke goaiassistant vector engine"),
			mcp.WithBoolean("force_all", mcp.Description("Set true untuk embed ulang seluruh baris tanpa melihat cache hash")),
		),
		s.handleTriggerGrapariSync,
	)

	// 8. grapari_sync_status
	s.mcpServer.AddTool(
		mcp.NewTool(
			"grapari_sync_status",
			mcp.WithDescription("Lihat ringkasan status knowledge base GraPARI: jumlah task, kategori yang aktif, dan waktu sync terakhir"),
		),
		s.handleGrapariSyncStatus,
	)

	// 9. restart_service
	s.mcpServer.AddTool(
		mcp.NewTool(
			"restart_service",
			mcp.WithDescription("Restart g3a MCP service secara graceful agar memuat binary atau konfigurasi baru"),
		),
		s.handleRestartService,
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
	sheet := request.GetString("sheet", "")
	allVarchar := request.GetBool("all_varchar", false)

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

	if fmtType == cli.FormatExcel {
		if err := s.eng.InstallExcel(ctx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("install excel: %v", err)), nil
		}
	}

	opts := engine.QueryOptions{
		InputPath:  resolvedPath,
		Format:     string(fmtType),
		Delimiter:  delimiter,
		Table:      table,
		Sheet:      sheet,
		AllVarchar: allVarchar,
		Verbose:    s.verbose,
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
	sheet := request.GetString("sheet", "")
	allVarchar := request.GetBool("all_varchar", false)
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

	if fmtType == cli.FormatExcel {
		if err := s.eng.InstallExcel(ctx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("install excel: %v", err)), nil
		}
	}

	opts := engine.QueryOptions{
		InputPath:  resolvedPath,
		Format:     string(fmtType),
		Select:     sel,
		Where:      where,
		GroupBy:    groupBy,
		OrderBy:    orderBy,
		Limit:      limit,
		Table:      table,
		Sheet:      sheet,
		AllVarchar: allVarchar,
		Verbose:    s.verbose,
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
	outFile := request.GetString("out_file", "")
	if strings.TrimSpace(outFile) == "" {
		cleanDataset := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
				return r
			}
			return '_'
		}, dataset)
		outFile = filepath.Join(os.TempDir(), fmt.Sprintf("%s_chart_%d.png", cleanDataset, time.Now().UnixMilli()))
	}
	sel := request.GetString("select", "*")
	if sel == "" {
		sel = "*"
	}
	where := request.GetString("where", "")
	groupBy := request.GetString("group_by", "")
	orderBy := request.GetString("order_by", "")
	limit := request.GetInt("limit", 50)
	sheet := request.GetString("sheet", "")
	allVarchar := request.GetBool("all_varchar", false)
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

	if fmtType == cli.FormatExcel {
		if err := s.eng.InstallExcel(ctx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("install excel: %v", err)), nil
		}
	}

	opts := engine.QueryOptions{
		InputPath:  resolvedPath,
		Format:     string(fmtType),
		Select:     sel,
		Where:      where,
		GroupBy:    groupBy,
		OrderBy:    orderBy,
		Limit:      limit,
		Table:      table,
		Sheet:      sheet,
		AllVarchar: allVarchar,
		Verbose:    s.verbose,
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
	outMsg := fmt.Sprintf("[ATTACH_FILE:%s|CAPTION:Visualisasi Data %s]\n%s", outFile, dataset, string(b))
	return mcp.NewToolResultText(outMsg), nil
}

func (s *Server) handleSearchGrapariKnowledge(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.syncer == nil {
		return mcp.NewToolResultError("GraPARI knowledge syncer is not configured"), nil
	}

	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query parameter is required"), nil
	}
	category := request.GetString("category", "")
	limit := request.GetInt("limit", 3)

	results := s.syncer.Search(query, category, limit)
	if len(results) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("Tidak ditemukan SOP GraPARI yang sesuai dengan query: %q", query)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Ditemukan %d referensi SOP GraPARI:\n\n", len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, r.AnswerText))
	}

	return mcp.NewToolResultText(strings.TrimSpace(sb.String())), nil
}

func (s *Server) handleTriggerGrapariSync(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.syncer == nil {
		return mcp.NewToolResultError("GraPARI knowledge syncer is not configured"), nil
	}

	forceAll := request.GetBool("force_all", false)
	inserted, updated, deleted, err := s.syncer.Sync(ctx, forceAll)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync error: %v", err)), nil
	}

	resp := map[string]any{
		"ok":       true,
		"inserted": inserted,
		"updated":  updated,
		"deleted":  deleted,
		"message":  fmt.Sprintf("Sinkronisasi berhasil: +%d baru, ~%d diperbarui, -%d dihapus", inserted, updated, deleted),
	}
	b, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func (s *Server) handleGrapariSyncStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.syncer == nil {
		return mcp.NewToolResultError("GraPARI knowledge syncer is not configured"), nil
	}

	tasks := s.syncer.GetTasks()
	categories := make(map[string]int)
	for _, t := range tasks {
		categories[t.Category]++
	}

	resp := map[string]any{
		"total_tasks": len(tasks),
		"categories":  categories,
		"status":      "active",
	}
	b, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func (s *Server) handleRestartService(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = cli.RestartSelf([]string{"mcp"})
	}()

	resp := map[string]any{
		"ok":      true,
		"message": "Sinyal restart telah dikirim. Service g3a akan dimuat ulang dalam beberapa saat.",
	}
	b, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}
