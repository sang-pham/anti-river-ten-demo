package sqllog

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"
	"gorm.io/gorm"
)

// Defaults and thresholds (confirmed with stakeholder)
// - Time range default: last 7 days
// - Anomalies when: exec_time_ms >= 1000 OR (exec_time_ms >= 500 AND exec_count >= 100)
// - Suggestions:
//   - avoid_select_star when query contains SELECT * (case-insensitive)
//   - add_index_on_where_columns when slow or frequent+slow
//   - consider_caching when exec_count >= 100
const (
	defaultSlowMs       = int64(1000)
	defaultFreqSlowMs   = int64(500)
	defaultFreqCount    = int64(100)
	defaultMaxAnomalies = 500
	maxAnomaliesCap     = 5000
	defaultTZ           = "Asia/Ho_Chi_Minh"
)

// ReportFilter defines the query window and optional DB filter.
// Threshold fields are optional; when zero or negative, defaults are applied.
type ReportFilter struct {
	From       time.Time
	To         time.Time
	DB         string
	Limit      int
	SlowMs     int64
	FreqSlowMs int64
	FreqCount  int64
	MaxCap     int
}

// ReportSummary contains the high-level metrics.
type ReportSummary struct {
	TotalQueries    int64            `json:"total_queries"`
	AnomalyCount    int64            `json:"anomaly_count"`
	SuggestionCount int64            `json:"suggestion_count"`
	ByDB            map[string]int64 `json:"by_db"`
	From            time.Time        `json:"from"`
	To              time.Time        `json:"to"`
}

// AnomalyDetail captures each anomalous query with reasons and suggestions.
type AnomalyDetail struct {
	DBName      string   `json:"db_name"`
	SQLQuery    string   `json:"sql_query"`
	ExecTimeMs  int64    `json:"exec_time_ms"`
	ExecCount   int64    `json:"exec_count"`
	Reasons     []string `json:"reasons"`
	Suggestions []string `json:"suggestions"`
}

// ReportData is the complete report payload for JSON/CSV/PDF.
type ReportData struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Timezone    string          `json:"timezone"`
	Summary     ReportSummary   `json:"summary"`
	Anomalies   []AnomalyDetail `json:"anomalies"`
}

// DefaultFilter returns a 7-day window ending at now, capped limit and default thresholds.
func DefaultFilter(now time.Time) ReportFilter {
	return ReportFilter{
		From:       now.Add(-7 * 24 * time.Hour),
		To:         now,
		DB:         "",
		Limit:      defaultMaxAnomalies,
		SlowMs:     defaultSlowMs,
		FreqSlowMs: defaultFreqSlowMs,
		FreqCount:  defaultFreqCount,
		MaxCap:     maxAnomaliesCap,
	}
}

func clampLimit(n, cap int) int {
	if cap <= 0 {
		cap = maxAnomaliesCap
	}
	if n <= 0 {
		return defaultMaxAnomalies
	}
	if n > cap {
		return cap
	}
	return n
}

func (r *Repository) Analyze(ctx context.Context, f ReportFilter) (ReportData, error) {
	now := time.Now()
	// Defaults
	if f.From.IsZero() || f.To.IsZero() || f.From.After(f.To) {
		df := DefaultFilter(now)
		if f.From.IsZero() {
			f.From = df.From
		}
		if f.To.IsZero() || f.From.After(f.To) {
			f.To = df.To
		}
	}
	f.Limit = clampLimit(f.Limit, f.MaxCap)
	// Apply default thresholds if omitted or invalid
	if f.SlowMs <= 0 {
		f.SlowMs = defaultSlowMs
	}
	if f.FreqSlowMs <= 0 {
		f.FreqSlowMs = defaultFreqSlowMs
	}
	if f.FreqCount <= 0 {
		f.FreqCount = defaultFreqCount
	}

	// Summary total count
	var total int64
	if err := r.applyFilters(r.db.WithContext(ctx).Model(&SQLLog{}), f).
		Count(&total).Error; err != nil {
		return ReportData{}, fmt.Errorf("count total: %w", err)
	}

	// By DB breakdown
	type dbRow struct {
		DBName string
		Cnt    int64
	}
	var rows []dbRow
	if err := r.applyFilters(r.db.WithContext(ctx).Model(&SQLLog{}), f).
		Select("db_name as db_name, COUNT(*) as cnt").
		Group("db_name").
		Scan(&rows).Error; err != nil {
		return ReportData{}, fmt.Errorf("by db: %w", err)
	}
	byDB := make(map[string]int64, len(rows))
	for _, rw := range rows {
		byDB[rw.DBName] = rw.Cnt
	}

	// Anomalies list (limited) ordered by severity
	var anomsSource []SQLLog
	if err := r.applyAnomalyFilters(r.applyFilters(r.db.WithContext(ctx).Model(&SQLLog{}), f), f).
		Order("exec_time_ms DESC, exec_count DESC").
		Limit(f.Limit).
		Find(&anomsSource).Error; err != nil {
		return ReportData{}, fmt.Errorf("list anomalies: %w", err)
	}

	// Anomaly total count (full, without limit)
	var anomalyCount int64
	if err := r.applyAnomalyFilters(r.applyFilters(r.db.WithContext(ctx).Model(&SQLLog{}), f), f).
		Count(&anomalyCount).Error; err != nil {
		return ReportData{}, fmt.Errorf("count anomalies: %w", err)
	}

	// Build details and suggestions
	anoms := make([]AnomalyDetail, 0, len(anomsSource))
	var suggestionCarriers int64
	for _, it := range anomsSource {
		reasons, suggs := deriveReasonsAndSuggestions(it, f.SlowMs, f.FreqSlowMs, f.FreqCount)
		if len(suggs) > 0 {
			suggestionCarriers++
		}
		anoms = append(anoms, AnomalyDetail{
			DBName:      it.DBName,
			SQLQuery:    it.SQLQuery,
			ExecTimeMs:  it.ExecTimeMs,
			ExecCount:   it.ExecCount,
			Reasons:     reasons,
			Suggestions: suggs,
		})
	}

	loc := mustLoadTZ(defaultTZ)
	data := ReportData{
		GeneratedAt: now.In(loc),
		Timezone:    defaultTZ,
		Summary: ReportSummary{
			TotalQueries:    total,
			AnomalyCount:    anomalyCount,
			SuggestionCount: suggestionCarriers,
			ByDB:            byDB,
			From:            f.From.In(loc),
			To:              f.To.In(loc),
		},
		Anomalies: anoms,
	}
	return data, nil
}

func (r *Repository) applyFilters(db *gorm.DB, f ReportFilter) *gorm.DB {
	db = db.Where("created_at >= ? AND created_at <= ?", f.From, f.To)
	if strings.TrimSpace(f.DB) != "" {
		db = db.Where("db_name = ?", strings.TrimSpace(f.DB))
	}
	return db
}

func (r *Repository) applyAnomalyFilters(db *gorm.DB, f ReportFilter) *gorm.DB {
	// (exec_time_ms >= slowMs) OR (exec_time_ms >= freqSlowMs AND exec_count >= freqCount)
	return db.Where("(exec_time_ms >= ?) OR (exec_time_ms >= ? AND exec_count >= ?)",
		f.SlowMs, f.FreqSlowMs, f.FreqCount)
}

func deriveReasonsAndSuggestions(it SQLLog, slowMs, freqSlowMs, freqCount int64) (reasons []string, suggestions []string) {
	lsql := strings.ToLower(it.SQLQuery)

	addReason := func(s string) {
		if !contains(reasons, s) {
			reasons = append(reasons, s)
		}
	}
	addSuggestion := func(s string) {
		if !contains(suggestions, s) {
			suggestions = append(suggestions, s)
		}
	}

	// Reason: slow_query
	if it.ExecTimeMs >= slowMs {
		addReason("slow_query")
	}
	// Reason: frequent_and_slow
	if it.ExecTimeMs >= freqSlowMs && it.ExecCount >= freqCount {
		addReason("frequent_and_slow")
	}
	// Reason: select_star
	if strings.Contains(lsql, "select *") {
		addReason("select_star")
		addSuggestion("avoid_select_star")
	}

	// Suggestions mapping
	if contains(reasons, "slow_query") || contains(reasons, "frequent_and_slow") {
		addSuggestion("add_index_on_where_columns")
	}
	if it.ExecCount >= freqCount {
		addSuggestion("consider_caching")
	}

	return reasons, suggestions
}

func contains(sl []string, s string) bool {
	for _, v := range sl {
		if v == s {
			return true
		}
	}
	return false
}

// ExportCSV writes a UTF-8 CSV with summary then anomaly table.
func (r *Repository) ExportCSV(data ReportData) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Summary as key,value pairs
	_ = w.Write([]string{"key", "value"})
	_ = w.Write([]string{"generated_at", data.GeneratedAt.Format(time.RFC3339)})
	_ = w.Write([]string{"timezone", data.Timezone})
	_ = w.Write([]string{"from", data.Summary.From.Format(time.RFC3339)})
	_ = w.Write([]string{"to", data.Summary.To.Format(time.RFC3339)})
	_ = w.Write([]string{"total_queries", fmt.Sprintf("%d", data.Summary.TotalQueries)})
	_ = w.Write([]string{"anomaly_count", fmt.Sprintf("%d", data.Summary.AnomalyCount)})
	_ = w.Write([]string{"suggestion_count", fmt.Sprintf("%d", data.Summary.SuggestionCount)})
	// by_db as "DB=Count" joined
	if len(data.Summary.ByDB) > 0 {
		var parts []string
		for k, v := range data.Summary.ByDB {
			parts = append(parts, fmt.Sprintf("%s=%d", k, v))
		}
		_ = w.Write([]string{"by_db", strings.Join(parts, "; ")})
	}

	_ = w.Write([]string{}) // blank line

	// Table header
	_ = w.Write([]string{"db_name", "exec_time_ms", "exec_count", "reasons", "suggestions", "sql_query"})

	for _, a := range data.Anomalies {
		reasons := strings.Join(a.Reasons, "|")
		suggestions := strings.Join(a.Suggestions, "|")
		// Keep SQL single-line for CSV safety
		sqlOneLine := strings.ReplaceAll(a.SQLQuery, "\n", " ")
		_ = w.Write([]string{
			a.DBName,
			fmt.Sprintf("%d", a.ExecTimeMs),
			fmt.Sprintf("%d", a.ExecCount),
			reasons,
			suggestions,
			sqlOneLine,
		})
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("csv write: %w", err)
	}
	return buf.Bytes(), nil
}

// ExportPDF renders a simple A4 portrait report with title, summary, and a table.
func (r *Repository) ExportPDF(data ReportData) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("SQL Log Report", false)
	pdf.AddPage()

	// Title
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(0, 10, "SQL Log Report")
	pdf.Ln(12)

	// Timestamp and range
	pdf.SetFont("Arial", "", 11)
	pdf.Cell(0, 6, fmt.Sprintf("Generated at: %s (%s)", data.GeneratedAt.Format(time.RFC3339), data.Timezone))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Range: %s  to  %s",
		data.Summary.From.Format(time.RFC3339),
		data.Summary.To.Format(time.RFC3339)))
	pdf.Ln(8)

	// Summary stats
	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(0, 6, "Summary")
	pdf.Ln(7)
	pdf.SetFont("Arial", "", 11)
	pdf.CellFormat(60, 6, "Total queries:", "0", 0, "", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("%d", data.Summary.TotalQueries), "0", 1, "", false, 0, "")
	pdf.CellFormat(60, 6, "Anomaly count:", "0", 0, "", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("%d", data.Summary.AnomalyCount), "0", 1, "", false, 0, "")
	pdf.CellFormat(60, 6, "Suggestion count:", "0", 0, "", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("%d", data.Summary.SuggestionCount), "0", 1, "", false, 0, "")
	if len(data.Summary.ByDB) > 0 {
		pdf.Ln(2)
		pdf.Cell(0, 6, "By DB:")
		pdf.Ln(6)
		for k, v := range data.Summary.ByDB {
			pdf.CellFormat(60, 6, fmt.Sprintf(" - %s:", k), "0", 0, "", false, 0, "")
			pdf.CellFormat(0, 6, fmt.Sprintf("%d", v), "0", 1, "", false, 0, "")
		}
	}
	pdf.Ln(6)

	// Table header and wrapped rows (avoid column overlap by using MultiCell and dynamic row height)
	// Adjusted widths to reduce header overflow; still totals ~190mm across A4 portrait page width.
	pdf.SetFont("Arial", "B", 11)
	colWidths := []float64{20, 28, 22, 33, 32, 55} // DB, Exec Time, Exec Count, Reasons, Suggestions, SQL
	headers := []string{"DB", "Exec Time (ms)", "Exec Count", "Reasons", "Suggestions", "SQL"}
	printHeader := func() {
		// Compute wrapped header height using smaller font to reduce overflow
		pdf.SetFont("Arial", "B", 10)
		headerLineH := 5.0
		maxLines := 1
		for i, h := range headers {
			lines := pdf.SplitText(h, colWidths[i])
			if l := len(lines); l > maxLines {
				maxLines = l
			}
		}
		hRow := float64(maxLines) * headerLineH

		startX := pdf.GetX()
		y := pdf.GetY()
		x := startX

		for i, h := range headers {
			// draw header cell border
			pdf.Rect(x, y, colWidths[i], hRow, "")
			// write wrapped header text
			pdf.SetXY(x, y)
			pdf.MultiCell(colWidths[i], headerLineH, h, "", "L", false)
			x += colWidths[i]
			pdf.SetXY(x, y)
		}
		// move cursor to next row
		pdf.SetXY(startX, y+hRow)
		// body font
		pdf.SetFont("Arial", "", 9)
	}
	printHeader()

	lineHeight := 5.0
	pageBottom := 287.0 // A4 height 297mm with ~10mm bottom margin
	for _, a := range data.Anomalies {
		reasons := strings.Join(a.Reasons, "|")
		suggestions := strings.Join(a.Suggestions, "|")
		sqlOne := strings.ReplaceAll(a.SQLQuery, "\n", " ")

		cells := []string{
			a.DBName,
			fmt.Sprintf("%d", a.ExecTimeMs),
			fmt.Sprintf("%d", a.ExecCount),
			reasons,
			suggestions,
			sqlOne,
		}

		// Determine required row height from wrapped lines
		maxLines := 1
		for i, txt := range cells {
			lines := pdf.SplitText(txt, colWidths[i])
			if l := len(lines); l > maxLines {
				maxLines = l
			}
		}
		rowH := float64(maxLines) * lineHeight

		// Page break if needed and reprint header
		if pdf.GetY()+rowH > pageBottom {
			pdf.AddPage()
			printHeader()
		}

		startX := pdf.GetX()
		y := pdf.GetY()
		x := startX

		for i, txt := range cells {
			// draw cell box
			pdf.Rect(x, y, colWidths[i], rowH, "")
			// write wrapped text within the cell box
			pdf.SetXY(x, y)
			pdf.MultiCell(colWidths[i], lineHeight, txt, "", "L", false)
			// move to the top of the next column
			x += colWidths[i]
			pdf.SetXY(x, y)
		}
		// move to next row
		pdf.SetXY(startX, y+rowH)
	}

	out := &bytes.Buffer{}
	if err := pdf.Output(out); err != nil {
		return nil, fmt.Errorf("pdf output: %w", err)
	}
	return out.Bytes(), nil
}

func mustLoadTZ(name string) *time.Location {
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	return time.Local
}

func truncateOneLine(s string, n int) string {
	if n <= 0 {
		return ""
	}
	one := strings.ReplaceAll(s, "\n", " ")
	if len(one) <= n {
		return one
	}
	if n <= 3 {
		return one[:n]
	}
	return one[:n-3] + "..."
}
