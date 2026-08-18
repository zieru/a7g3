# g3a — BigQuery-style CLI Query Tool

Query local files (CSV, Parquet, SQLite) with full SQL syntax, powered by DuckDB's vectorized engine.

## Features

- 🚀 **Full SQL** — `SELECT`, `WHERE`, `GROUP BY`, `ORDER BY`, `HAVING`, aggregations, window functions
- 📁 **Multiple formats** — CSV, TSV, Parquet, SQLite
- 💾 **Memory-efficient** — DuckDB streams files in chunks; spills to disk automatically for large datasets
- 🖥️ **Multiple output formats** — ASCII table, CSV, JSON
- ⚡ **Fast** — Vectorized columnar execution (DuckDB)

## Install

```bash
go install github.com/a7g3/g3a@latest
```

Or build from source (requires GCC/MinGW on Windows for CGO):

```bash
git clone https://github.com/a7g3/g3a
cd g3a
go build -o g3a.exe .
```

## Usage

```bash
g3a [alias | FILE] [options]
g3a --input=FILE [options]
```

### Alias Configuration (`.g3a.config`)

You can define shortcuts/aliases for your data files in `~/.g3a.config` (e.g. `C:\Users\<user>\.g3a.config` on Windows or `/home/<user>/.g3a.config` on Linux) or `.g3a.config` in your current working directory:

```text
# ~/.g3a.config
funneling /path/to/data.csv
funnelingx /path/to/large_dataset.parquet
sales C:\Users\user\Documents\sales.csv
```

Then query directly using the alias without `--input`:

```bash
g3a funneling --select="count(1) as total"
g3a funnelingx --where="amount > 1000" --output=json
```

### Options

```text
  --input        Input file path (.csv, .tsv, .parquet, .sqlite, .db)
  --select       SELECT expressions (default: *)
  --group-by     GROUP BY columns (comma-separated)
  --where        WHERE clause
  --order-by     ORDER BY clause
  --limit        Limit result rows (0 = no limit)
  --output       Output format: table (default), csv, json, jsonl, toon, png, image
  --out-file     Output file path (e.g. report.png, data.csv)
  --chunk-size   Rows per chunk for streaming (default: 100000)
  --no-header    Input CSV has no header row
  --delimiter    CSV delimiter character (auto-detect if empty)
  --table        SQLite table name (auto-detect if empty)
  --pivot        Pivot column(s) into table headers (e.g. month or year,month)
  --pivot-fill   Fill value for missing pivot cells (default "0")
  --pivot-sep    Separator for multi-column pivot headers (default "/")
  --verbose      Print debug info and SQL being executed
```

## Examples

### Count & sum by category (CSV)
```bash
g3a --input=sales.csv \
    --select="count(1) as count, sum(amount) as total" \
    --group-by=category \
    --order-by="total DESC"
```
```
+-----------+-------+----------+
| category  | count | total    |
+-----------+-------+----------+
| Electronics | 1234 | 98765.00 |
| Clothing  |  890  | 43210.50 |
+-----------+-------+----------+
2 row(s) in 142ms
```

### Full SQL on Parquet
```bash
g3a --input=events.parquet \
    --select="date_trunc('month', event_time) as month, count(*) as events" \
    --where="region = 'APAC'" \
    --group-by="1" \
    --order-by="1"
```

### Query SQLite
```bash
g3a --input=app.db \
    --select="user_id, count(*) as orders, sum(total) as revenue" \
    --group-by=user_id \
    --order-by="revenue DESC" \
    --limit=10
```

### Export to CSV
```bash
g3a --input=huge.csv --select="*" --where="status='active'" --output=csv > filtered.csv
```

### See the SQL being run
```bash
g3a --input=data.csv --select="avg(price)" --verbose
# [g3a] SQL:
# SELECT avg(price)
# FROM read_csv('data.csv', auto_detect=true, buffer_size=100000)
```

## How it works

Under the hood, g3a translates your flags into a DuckDB SQL query:

```
--select="count(1) as n, sum(amount) as total"
--group-by=category
--where="amount > 0"
```
↓
```sql
SELECT count(1) as n, sum(amount) as total
FROM read_csv('data.csv', auto_detect=true, buffer_size=100000)
WHERE amount > 0
GROUP BY category
```

DuckDB reads the file in vectorized chunks, executes the aggregation with minimal memory, and can spill to disk if needed.

## Performance tips

- Use `--chunk-size` to tune memory vs speed (default 100k rows/chunk)
- For very large CSV files, convert to Parquet first — columnar format is much faster for analytics
- Add `--where` to filter early and reduce processed data
- Use `--output=csv` and redirect to a file for large result sets
