package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"go-demo/internal/sqllog"
)

type SQLLogScan struct {
	repo *sqllog.Repository
	log  *slog.Logger
}

func NewSQLLogScan(repo *sqllog.Repository, log *slog.Logger) *SQLLogScan {
	if log == nil {
		log = slog.Default()
	}
	return &SQLLogScan{repo: repo, log: log}
}

// Scan godoc
// @Summary Scan for abnormal SQL queries
// @Description Apply rule: exec_time_ms > threshold AND exec_count > threshold (defaults: 500ms, 100). Thresholds can be overridden via query params.
// @Tags sql-logs
// @Produce json
// @Param limit query int false "Maximum number of items to return" minimum(1) maximum(1000) default(100)
// @Param dbName query string false "Database name to filter results"
// @Param exec_time_ms query int false "Minimum exec_time_ms to be considered abnormal" default(500)
// @Param exec_count query int false "Minimum exec_count to be considered abnormal" default(100)
// @Success 200 {object} map[string]any
// @Failure 400 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/sql-logs/scan [get]
func (h *SQLLogScan) Scan() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.repo == nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "repository not configured")
			return
		}

		// Parse optional limit
		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "invalid limit")
				return
			}
			if n < 1 {
				n = 1
			}
			if n > 1000 {
				n = 1000
			}
			limit = n
		}

		// Optional database filter
		dbName := strings.TrimSpace(r.URL.Query().Get("dbName"))

		// Thresholds (with defaults)
		execTimeMs := sqllog.AbnormalExecTimeThreshold
		if v := strings.TrimSpace(r.URL.Query().Get("exec_time_ms")); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				writeError(w, http.StatusBadRequest, "bad_request", "invalid exec_time_ms")
				return
			}
			execTimeMs = n
		}
		execCount := sqllog.AbnormalExecCountThreshold
		if v := strings.TrimSpace(r.URL.Query().Get("exec_count")); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				writeError(w, http.StatusBadRequest, "bad_request", "invalid exec_count")
				return
			}
			execCount = n
		}

		ctx := r.Context()

		var (
			total int64
			err   error
		)

		if dbName != "" {
			total, err = h.repo.CountAbnormalByDBWithThresholds(ctx, dbName, execTimeMs, execCount)
		} else {
			total, err = h.repo.CountAbnormalWithThresholds(ctx, execTimeMs, execCount)
		}
		if err != nil {
			h.log.Error("count abnormal failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "count failed")
			return
		}

		if total == 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"message": "No abnormal queries detected",
				"total":   0,
				"items":   []any{},
			})
			return
		}

		var items []sqllog.SQLLog
		if dbName != "" {
			items, err = h.repo.ListAbnormalByDBWithThresholds(ctx, dbName, limit, execTimeMs, execCount)
		} else {
			items, err = h.repo.ListAbnormalWithThresholds(ctx, limit, execTimeMs, execCount)
		}
		if err != nil {
			h.log.Error("list abnormal failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "list failed")
			return
		}

		// Build response with visual indicator via status
		respItems := make([]map[string]any, 0, len(items))
		for _, it := range items {
			respItems = append(respItems, map[string]any{
				"db_name":      it.DBName,
				"sql_query":    it.SQLQuery,
				"exec_time_ms": it.ExecTimeMs,
				"exec_count":   it.ExecCount,
				"status":       "abnormal",
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"message": "scan complete",
			"total":   total,
			"items":   respItems,
		})
	})
}
