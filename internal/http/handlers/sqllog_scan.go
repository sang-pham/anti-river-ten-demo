package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

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
// @Description Apply rule: exec_time_ms > 500 AND exec_count > 100. Returns abnormal queries with status indicators.
// @Tags sql-logs
// @Produce json
// @Param limit query int false "Maximum number of items to return" minimum(1) maximum(1000) default(100)
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

		ctx := r.Context()

		total, err := h.repo.CountAbnormal(ctx)
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

		items, err := h.repo.ListAbnormal(ctx, limit)
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