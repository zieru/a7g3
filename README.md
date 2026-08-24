# g3a — High-Performance CLI Analytics & Query Engine

**g3a** is a blazingly fast, BigQuery-style command-line query tool and analytics engine for local and networked files (Parquet, CSV, TSV, SQLite), powered by DuckDB's vectorized columnar execution.

---

## 🚀 Key Features

- **Vectorized Columnar Execution**: Powered by embedded DuckDB for ultra-fast aggregations, joins, and window functions.
- **Multiple File Formats**: Native support for **Parquet** (single files or directories), **CSV/TSV**, and **SQLite**.
- **In-Memory Dynamic Pivoting**: Transform tabular metrics into multi-column pivot matrices on-the-fly (`--pivot`).
- **Rich Output Formats**:
  - `json`: Structured typed JSON envelope with duration and metadata (optimized for REST APIs and dashboards).
  - `table`: ASCII formatted grid with alignment and execution timing.
  - `csv`: Standard RFC-4180 CSV stream.
  - `jsonl`: Streaming JSON Lines for large-scale data ingestion.
  - `toon`: Token-Optimized Object Notation for LLM consumption.
  - `png` / `image`: Direct high-resolution rendered table image.
- **REST API Integration Ready**: Seamlessly called by HTTP gateways like `goassistant` (`goassisthttp`) for backend-driven dashboard endpoints.

---

## 📦 Installation & Build

### From Source
```bash
git clone https://github.com/a7g3/g3a
cd g3a
go build -o g3a .
```

*Note: Building on Linux / macOS or in Docker containers uses standard CGO. Automated multi-platform builds are handled via GitHub Actions workflow.*

---

## 🛠️ CLI Options Reference

```text
Usage:
  g3a [alias | FILE | DIRECTORY] [options]
  g3a --input=FILE [options]

Core Query Options:
  --input        Input file or directory (.parquet, .csv, .tsv, .sqlite, .db)
  --select       SELECT SQL expressions (default: "*")
  --where        WHERE SQL filter clause
  --group-by     GROUP BY columns (comma-separated or SQL expressions)
  --order-by     ORDER BY columns and directions (e.g. "total DESC, month ASC")
  --limit        Limit number of result rows (0 = unlimited)

Output & Formatting:
  --output       Output format: table (default), json, csv, jsonl, toon, png, image
  --out-file     Save output directly to file (e.g. report.json, chart.png)
  --chunk-size   Streaming buffer size (default: 100,000 rows)
  --no-header    Input CSV does not contain a header row
  --delimiter    Custom CSV delimiter (auto-detected if omitted)
  --table        SQLite table name override

Pivot Engine:
  --pivot        Column(s) to rotate into headers (e.g. "flag_dilayani" or "regional,service")
  --pivot-fill   Default fill value for empty cells in pivot matrix (default: "0")
  --pivot-sep    Header separator for multi-column pivot keys (default: "/")

Diagnostics:
  --verbose      Print the underlying generated DuckDB SQL query and timing
```

---

## 💡 Aliases Configuration (`.g3a.config`)

Define aliases for frequently queried datasets in `~/.g3a.config` (Linux/macOS: `/home/<user>/.g3a.config`, Windows: `C:\Users\<user>\.g3a.config`) or `.g3a.config` in your working directory:

```text
visit /data/antreaja/visit_parquet
funneling /data/reports/funneling.parquet
sales /data/csv/sales_2026.csv
```

Query directly using the alias:
```bash
g3a visit --select="count(1) as total" --output=json
```

---

## 📊 Usage Examples

### 1. Daily & Monthly Aggregations (Parquet Directory)
```bash
g3a /path/to/visit_parquet \
  --select="strftime(\"Trx Date\", '%Y-%m') as periode, count(1) as total_visits, sum(case when flag_dilayani = 1 then 1 else 0 end) as served" \
  --group-by="strftime(\"Trx Date\", '%Y-%m')" \
  --order-by="periode asc" \
  --output=json
```

### 2. High-Performance In-Memory Pivot
Rotate dimension values (e.g. service status) into columnar headers:
```bash
g3a /path/to/visit_parquet \
  --select="regional, service, flag_dilayani, count(1) as total" \
  --group-by="regional, service, flag_dilayani" \
  --pivot="flag_dilayani" \
  --pivot-fill="0" \
  --output=table
```

### 3. Date & Time Best Practices in DuckDB SQL
- **Month Formatting**: `strftime("Trx Date", '%Y-%m')` (returns `YYYY-MM`)
- **Date Truncation**: `date_trunc('month', "Trx Date")`
- **Day of Week**: `strftime("Trx Date", '%A')`
- **Filtering Range**: `--where="\"Trx Date\" >= '2026-01-01' and \"Trx Date\" <= '2026-08-31'"`

---

## 🔌 API & JSON Output Specification

When using `--output=json`, `g3a` emits a structured, type-safe JSON object:
- Numbers (`int64`, `float64`) are cleanly preserved as JSON numbers.
- Date strings (`"2026-08"`, `"2026-08-24"`) and alphanumeric codes are strictly preserved as JSON strings (avoiding premature number truncation).
- `NULL` values are safely encoded as `null`.

**Sample JSON Response:**
```json
{
  "ok": true,
  "columns": ["periode", "total_visits", "served_count", "served_ratio"],
  "row_count": 2,
  "duration_ms": 14.2,
  "rows": [
    {
      "periode": "2026-07",
      "total_visits": 187869,
      "served_count": 186951,
      "served_ratio": "99.5%"
    },
    {
      "periode": "2026-08",
      "total_visits": 124544,
      "served_count": 123690,
      "served_ratio": "99.3%"
    }
  ]
}
```

---

## ⚡ Performance Guidelines

1. **Parquet Preferred**: For large datasets (>1M rows), use Parquet over CSV for up to 10x-50x faster scans and reduced I/O.
2. **Early Filtering**: Use `--where` clauses to push down predicates directly into DuckDB's scan engine.
3. **Partitioned Directories**: Directory paths with partitioned Parquet files (`visit_parquet/*.parquet`) are automatically detected and streamed without memory exhaustion.
