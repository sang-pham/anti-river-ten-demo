package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-demo/internal/sqllog"
)

type SQLLogReport struct {
	repo         *sqllog.Repository
	log          *slog.Logger
	maxBodyBytes int64
}

func NewSQLLogReport(repo *sqllog.Repository, log *slog.Logger, maxBodyBytes int64) *SQLLogReport {
	if log == nil {
		log = slog.Default()
	}
	return &SQLLogReport{repo: repo, log: log, maxBodyBytes: maxBodyBytes}
}

// ReportJSON godoc
// @Summary SQL log report (JSON)
// @Description Aggregated anomalies and metrics within a time range. Defaults: last 7 days. Thresholds: slow_ms >= 1000 OR (exec_time_ms >= 500 AND exec_count >= 100).
// @Tags sql-logs
// @Produce json
// @Security BearerAuth
// @Param from query string false "Start time (RFC3339 or YYYY-MM-DD)"
// @Param to query string false "End time (RFC3339 or YYYY-MM-DD)"
// @Param db query string false "Filter by database name"
// @Param limit query int false "Max anomalies to return" minimum(1) maximum(5000) default(500)
// @Param slow_ms query int false "Slow threshold in ms"
// @Param freq_slow_ms query int false "Frequent+slow time threshold in ms"
// @Param freq_count query int false "Frequent count threshold"
// @Param cap query int false "Hard cap upper bound for anomalies count"
// @Success 200 {object} sqllog.ReportData
// @Failure 400 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/sql-logs/report [get]
func (h *SQLLogReport) ReportJSON() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.repo == nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "repository not configured")
			return
		}
		filter, err := parseReportFilter(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		data, err := h.repo.Analyze(r.Context(), filter)
		if err != nil {
			h.log.Error("analyze report failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "could not build report")
			return
		}
		writeJSON(w, http.StatusOK, data)
	})
}

// ReportCSV godoc
// @Summary SQL log report (CSV)
// @Description Download the aggregated report as CSV.
// @Tags sql-logs
// @Produce text/csv
// @Security BearerAuth
// @Param from query string false "Start time (RFC3339 or YYYY-MM-DD)"
// @Param to query string false "End time (RFC3339 or YYYY-MM-DD)"
// @Param db query string false "Filter by database name"
// @Param limit query int false "Max anomalies to return" minimum(1) maximum(5000) default(500)
// @Param slow_ms query int false "Slow threshold in ms"
// @Param freq_slow_ms query int false "Frequent+slow time threshold in ms"
// @Param freq_count query int false "Frequent count threshold"
// @Param cap query int false "Hard cap upper bound for anomalies count"
// @Success 200 {string} string "CSV content"
// @Failure 400 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/sql-logs/report.csv [get]
func (h *SQLLogReport) ReportCSV() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.repo == nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "repository not configured")
			return
		}
		filter, err := parseReportFilter(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		data, err := h.repo.Analyze(r.Context(), filter)
		if err != nil {
			h.log.Error("analyze report failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "could not build report")
			return
		}
		b, err := h.repo.ExportCSV(data)
		if err != nil {
			h.log.Error("export csv failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "could not export csv")
			return
		}
		name := buildFilename("csv")
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		_, _ = w.Write(b)
	})
}

// ReportPDF godoc
// @Summary SQL log report (PDF)
// @Description Download the aggregated report as PDF.
// @Tags sql-logs
// @Produce application/pdf
// @Security BearerAuth
// @Param from query string false "Start time (RFC3339 or YYYY-MM-DD)"
// @Param to query string false "End time (RFC3339 or YYYY-MM-DD)"
// @Param db query string false "Filter by database name"
// @Param limit query int false "Max anomalies to return" minimum(1) maximum(5000) default(500)
// @Param slow_ms query int false "Slow threshold in ms"
// @Param freq_slow_ms query int false "Frequent+slow time threshold in ms"
// @Param freq_count query int false "Frequent count threshold"
// @Param cap query int false "Hard cap upper bound for anomalies count"
// @Success 200 {string} string "PDF content"
// @Failure 400 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/sql-logs/report.pdf [get]
func (h *SQLLogReport) ReportPDF() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.repo == nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "repository not configured")
			return
		}
		filter, err := parseReportFilter(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		data, err := h.repo.Analyze(r.Context(), filter)
		if err != nil {
			h.log.Error("analyze report failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "could not build report")
			return
		}
		b, err := h.repo.ExportPDF(data)
		if err != nil {
			h.log.Error("export pdf failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "could not export pdf")
			return
		}
		name := buildFilename("pdf")
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		_, _ = w.Write(b)
	})
}

// parseReportFilter reads from,to,db,limit from query.
// - from/to accept RFC3339 or "2006-01-02" (date only). Defaults to last 7 days.
// - limit defaults to 500 and max 5000.
// - db optional exact match.
func parseReportFilter(r *http.Request) (sqllog.ReportFilter, error) {
	q := r.URL.Query()
	now := time.Now()
	df := sqllog.DefaultFilter(now)

	fromStr := strings.TrimSpace(q.Get("from"))
	toStr := strings.TrimSpace(q.Get("to"))
	db := strings.TrimSpace(q.Get("db"))
	limitStr := strings.TrimSpace(q.Get("limit"))

	// Optional threshold overrides
	slowMsStr := strings.TrimSpace(q.Get("slow_ms"))
	freqSlowMsStr := strings.TrimSpace(q.Get("freq_slow_ms"))
	freqCountStr := strings.TrimSpace(q.Get("freq_count"))
	capStr := strings.TrimSpace(q.Get("cap"))

	from := df.From
	to := df.To

	var err error
	if fromStr != "" {
		if from, err = parseTime(fromStr); err != nil {
			return sqllog.ReportFilter{}, fmt.Errorf("invalid 'from': %w", err)
		}
	}
	if toStr != "" {
		if to, err = parseTime(toStr); err != nil {
			return sqllog.ReportFilter{}, fmt.Errorf("invalid 'to': %w", err)
		}
		// If to is date-only at midnight (heuristic), extend to end of day to be inclusive
		if isMidnight(to) && len(toStr) == len("2006-01-02") {
			to = to.Add(24*time.Hour - time.Nanosecond)
		}
	}

	limit := df.Limit
	if limitStr != "" {
		if v, e := strconv.Atoi(limitStr); e == nil {
			limit = v
		}
	}

	// Start with defaults then apply overrides if valid
	f := sqllog.ReportFilter{
		From:       from,
		To:         to,
		DB:         db,
		Limit:      limit,
		SlowMs:     df.SlowMs,
		FreqSlowMs: df.FreqSlowMs,
		FreqCount:  df.FreqCount,
		MaxCap:     df.MaxCap,
	}

	if slowMsStr != "" {
		if v, e := strconv.ParseInt(slowMsStr, 10, 64); e == nil && v > 0 {
			f.SlowMs = v
		}
	}
	if freqSlowMsStr != "" {
		if v, e := strconv.ParseInt(freqSlowMsStr, 10, 64); e == nil && v > 0 {
			f.FreqSlowMs = v
		}
	}
	if freqCountStr != "" {
		if v, e := strconv.ParseInt(freqCountStr, 10, 64); e == nil && v > 0 {
			f.FreqCount = v
		}
	}
	if capStr != "" {
		if v, e := strconv.Atoi(capStr); e == nil && v > 0 {
			f.MaxCap = v
		}
	}

	return f, nil
}

func parseTime(s string) (time.Time, error) {
	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// Then date only
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("must be RFC3339 or YYYY-MM-DD")
}

func isMidnight(t time.Time) bool {
	return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0
}

func buildFilename(ext string) string {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	now := time.Now().In(loc)
	stamp := now.Format("20060102-1504")
	return fmt.Sprintf("sql-report-%s.%s", stamp, ext)
}
