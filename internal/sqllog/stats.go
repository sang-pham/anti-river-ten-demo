package sqllog

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// PatternStat represents an aggregated normalized SQL pattern with its occurrence count.
type PatternStat struct {
	Pattern     string `json:"pattern"`
	Occurrences int64  `json:"occurrences"`
}

// PercentileSet maps keys like "p50","p75" to numeric values.
type PercentileSet map[string]float64

// Percentiles groups percentile sets for exec_time_ms and exec_count.
type Percentiles struct {
	ExecTime  PercentileSet `json:"exec_time_ms"`
	ExecCount PercentileSet `json:"exec_count"`
}

// computePercentiles returns overall and per-DB percentiles for exec_time_ms and exec_count.
// It uses PostgreSQL percentile_disc with an ARRAY of fractions (0..1).
func (r *Repository) computePercentiles(ctx context.Context, f ReportFilter) (overall Percentiles, byDB map[string]Percentiles, err error) {
	if len(f.Pcts) == 0 {
		return Percentiles{ExecTime: PercentileSet{}, ExecCount: PercentileSet{}}, map[string]Percentiles{}, nil
	}
	arrExpr := buildArrayExpr(f.Pcts) // e.g., ARRAY[0.5,0.75,0.9]

	// Overall
	baseWhere, args := r.whereClauseArgs(f)
	qOverall := fmt.Sprintf(`
SELECT
  percentile_disc(%s) WITHIN GROUP (ORDER BY exec_time_ms) AS p_exec_time,
  percentile_disc(%s) WITHIN GROUP (ORDER BY exec_count)   AS p_exec_count
FROM "DEMO"."SQL_LOG"
WHERE %s
`, arrExpr, arrExpr, baseWhere)

	type rowOverall struct {
		PExecTime  sql.NullString
		PExecCount sql.NullString
	}
	var o rowOverall
	if err = r.db.WithContext(ctx).Raw(qOverall, args...).Scan(&o).Error; err != nil {
		return overall, byDB, fmt.Errorf("percentiles overall: %w", err)
	}
	overall = Percentiles{
		ExecTime:  parseArrayToPctSet(o.PExecTime.String, f.Pcts),
		ExecCount: parseArrayToPctSet(o.PExecCount.String, f.Pcts),
	}

	// Per DB
	qPerDB := fmt.Sprintf(`
SELECT
  db_name,
  percentile_disc(%s) WITHIN GROUP (ORDER BY exec_time_ms) AS p_exec_time,
  percentile_disc(%s) WITHIN GROUP (ORDER BY exec_count)   AS p_exec_count
FROM "DEMO"."SQL_LOG"
WHERE %s
GROUP BY db_name
`, arrExpr, arrExpr, baseWhere)

	type rowPerDB struct {
		DBName     string
		PExecTime  sql.NullString
		PExecCount sql.NullString
	}
	var rows []rowPerDB
	if err = r.db.WithContext(ctx).Raw(qPerDB, args...).Scan(&rows).Error; err != nil {
		return overall, byDB, fmt.Errorf("percentiles per-db: %w", err)
	}
	byDB = make(map[string]Percentiles, len(rows))
	for _, rw := range rows {
		byDB[rw.DBName] = Percentiles{
			ExecTime:  parseArrayToPctSet(rw.PExecTime.String, f.Pcts),
			ExecCount: parseArrayToPctSet(rw.PExecCount.String, f.Pcts),
		}
	}

	return overall, byDB, nil
}

// computeTopPatterns returns the most frequent normalized SQL query patterns overall and per DB.
// Ranking is by number of occurrences (COUNT(*)) descending. Limit is f.TopPatterns per scope.
func (r *Repository) computeTopPatterns(ctx context.Context, f ReportFilter) (overall []PatternStat, byDB map[string][]PatternStat, err error) {
	if f.TopPatterns <= 0 {
		return nil, map[string][]PatternStat{}, nil
	}
	normExpr := normalizationSQL("sql_query")
	baseWhere, args := r.whereClauseArgs(f)

	// Overall
	qOverall := fmt.Sprintf(`
WITH filt AS (
  SELECT %s AS pattern
  FROM "DEMO"."SQL_LOG"
  WHERE %s
)
SELECT pattern, COUNT(*) AS occurrences
FROM filt
GROUP BY pattern
ORDER BY occurrences DESC
LIMIT ?
`, normExpr, baseWhere)

	argsOverall := append(append([]any{}, args...), f.TopPatterns)
	var overRows []struct {
		Pattern     string
		Occurrences int64
	}
	if err = r.db.WithContext(ctx).Raw(qOverall, argsOverall...).Scan(&overRows).Error; err != nil {
		return overall, byDB, fmt.Errorf("top patterns overall: %w", err)
	}
	overall = make([]PatternStat, 0, len(overRows))
	for _, rw := range overRows {
		overall = append(overall, PatternStat{Pattern: rw.Pattern, Occurrences: rw.Occurrences})
	}

	// Per DB - top N per db_name using window
	qPerDB := fmt.Sprintf(`
WITH filt AS (
  SELECT db_name, %s AS pattern
  FROM "DEMO"."SQL_LOG"
  WHERE %s
),
agg AS (
  SELECT db_name, pattern, COUNT(*) AS occurrences
  FROM filt
  GROUP BY db_name, pattern
),
ranked AS (
  SELECT db_name, pattern, occurrences,
         ROW_NUMBER() OVER (PARTITION BY db_name ORDER BY occurrences DESC, pattern ASC) AS rn
  FROM agg
)
SELECT db_name, pattern, occurrences
FROM ranked
WHERE rn <= ?
ORDER BY db_name ASC, occurrences DESC, pattern ASC
`, normExpr, baseWhere)

	argsPerDB := append(append([]any{}, args...), f.TopPatterns)
	var perDBRows []struct {
		DBName      string
		Pattern     string
		Occurrences int64
	}
	if err = r.db.WithContext(ctx).Raw(qPerDB, argsPerDB...).Scan(&perDBRows).Error; err != nil {
		return overall, byDB, fmt.Errorf("top patterns per-db: %w", err)
	}
	byDB = make(map[string][]PatternStat)
	for _, rw := range perDBRows {
		byDB[rw.DBName] = append(byDB[rw.DBName], PatternStat{Pattern: rw.Pattern, Occurrences: rw.Occurrences})
	}

	return overall, byDB, nil
}

// whereClauseArgs builds the SQL WHERE clause and args for created_at and optional db_name.
func (r *Repository) whereClauseArgs(f ReportFilter) (clause string, args []any) {
	parts := []string{`created_at >= ?`, `created_at <= ?`}
	args = []any{f.From, f.To}
	if strings.TrimSpace(f.DB) != "" {
		parts = append(parts, `db_name = ?`)
		args = append(args, strings.TrimSpace(f.DB))
	}
	return strings.Join(parts, " AND "), args
}

// buildArrayExpr converts a list of fractions to an ARRAY[...] expression string.
func buildArrayExpr(pcts []float64) string {
	parts := make([]string, 0, len(pcts))
	for _, v := range pcts {
		// Ensure reasonable precision; percentile_disc accepts numeric
		parts = append(parts, strconv.FormatFloat(v, 'f', -1, 64))
	}
	return "ARRAY[" + strings.Join(parts, ",") + "]"
}

// parseArrayToPctSet parses a PostgreSQL array literal (e.g., "{1,2,3}") into a PercentileSet.
// It maps values to keys based on pcts provided (0..1 fractions -> "pXX").
func parseArrayToPctSet(arr string, pcts []float64) PercentileSet {
	out := make(PercentileSet, len(pcts))
	if strings.TrimSpace(arr) == "" {
		return out
	}
	raw := strings.TrimSpace(arr)
	raw = strings.TrimPrefix(raw, "{")
	raw = strings.TrimSuffix(raw, "}")
	vals := []string{}
	if raw != "" {
		vals = splitCSVRespectingQuotes(raw)
	}
	for i, s := range vals {
		if i >= len(pcts) {
			break
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		// Try integer, then float
		if iv, err := strconv.ParseInt(s, 10, 64); err == nil {
			out[pctKey(pcts[i])] = float64(iv)
			continue
		}
		if fv, err := strconv.ParseFloat(s, 64); err == nil {
			out[pctKey(pcts[i])] = fv
		}
	}
	return out
}

func pctKey(f float64) string {
	// Round to nearest int percentile value
	n := int(f*100 + 0.5)
	return fmt.Sprintf("p%d", n)
}

// splitCSVRespectingQuotes is a simple splitter for postgres array elements without embedded commas in quotes.
// Our percentile arrays are numeric, so this is conservative.
func splitCSVRespectingQuotes(s string) []string {
	// minimal split; elements are numeric without quotes
	return strings.Split(s, ",")
}

// normalizationSQL builds the conservative normalization SQL expression:
// - lower
// - replace single-quoted string literals with ?
// - replace UUIDs, ISO dates/datetimes, and numeric literals with ?
// - collapse whitespace, trim
func normalizationSQL(col string) string {
	// order replacements string -> uuid -> datetime -> number -> whitespace
	// Single-quoted strings: handles escaped '' using dollar-quoted pattern (no fragile E'' escaping)
	expr := fmt.Sprintf("LOWER(%s)", col)
	// IMPORTANT: do not use '?' anywhere inside regex patterns; GORM may treat them as placeholders.
	// Postgres regex classes: \\y for word boundary, [[:digit:]] for digits, [[:space:]] for whitespace
	// 1) Single-quoted string literals: '...'(with '' as escaped quote)
	expr = fmt.Sprintf("regexp_replace(%s, $$'([^']|'{2})*'$$, CHR(63), 'g')", expr)
	// 2) UUIDs
	expr = fmt.Sprintf("regexp_replace(%s, $$\\y[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\\y$$, CHR(63), 'g')", expr)
	// 3) ISO date/datetime (YYYY-MM-DD[ T]HH:MM:SS[.fff] optional time part) - use {0,1} instead of '?'
	expr = fmt.Sprintf("regexp_replace(%s, $$\\y[[:digit:]]{4}-[[:digit:]]{2}-[[:digit:]]{2}(([[:space:]]|T)[[:digit:]]{2}:[[:digit:]]{2}:[[:digit:]]{2}(\\.[[:digit:]]+){0,1}){0,1}\\y$$, CHR(63), 'g')", expr)
	// 4) Numbers (ints or decimals) - use {0,1} instead of '?'
	expr = fmt.Sprintf("regexp_replace(%s, $$\\y[[:digit:]]+(\\.[[:digit:]]+){0,1}\\y$$, CHR(63), 'g')", expr)
	// 5) Collapse whitespace
	expr = fmt.Sprintf("btrim(regexp_replace(%s, $$[[:space:]]+$$, ' ', 'g'))", expr)
	return expr
}

// applyFilters is reused elsewhere; ensure any future callers still compile if moved.
// Here we keep a convenience wrapper for raw patterns/percentile queries when needed.
func (r *Repository) applyFiltersRaw(db *gorm.DB, f ReportFilter) *gorm.DB {
	return r.applyFilters(db, f)
}
