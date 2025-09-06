package handlers

import (
	"net/http"

	"log/slog"

	"go-demo/internal/sqllog"
)

type SQLLogQuery struct {
	repo *sqllog.Repository
	log  *slog.Logger
}

func NewSQLLogQuery(repo *sqllog.Repository, log *slog.Logger) *SQLLogQuery {
	if log == nil {
		log = slog.Default()
	}
	return &SQLLogQuery{repo: repo, log: log}
}

// Swagger DTOs
type ListDatabasesResponse struct {
	Databases []string `json:"databases"`
}

type SQLLogItem struct {
	SQLQuery   string `json:"sql_query"`
	ExecTimeMs int64  `json:"exec_time_ms"`
	ExecCount  int64  `json:"exec_count"`
}

type ListByDBResponse struct {
	Items   []SQLLogItem `json:"items"`
	Message string       `json:"message,omitempty"`
}

// Internal response item type used at runtime
type sqlLogItem struct {
	SQLQuery   string `json:"sql_query"`
	ExecTimeMs int64  `json:"exec_time_ms"`
	ExecCount  int64  `json:"exec_count"`
}

// ListDatabases godoc
// @Summary List databases with SQL logs
// @Description Returns distinct database names that have SQL log entries.
// @Tags sql-logs
// @Produce json
// @Success 200 {object} ListDatabasesResponse
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/sql-logs/databases [get]
func (h *SQLLogQuery) ListDatabases() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.repo == nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "repository not configured")
			return
		}
		names, err := h.repo.ListDatabases(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to list databases")
			h.log.Error("list databases failed", "err", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"databases": names,
		})
	})
}

// ListByDB godoc
// @Summary List SQL queries by database
// @Description Provide database name via query parameter "db" to list its SQL queries.
// @Tags sql-logs
// @Produce json
// @Param db query string true "Database name"
// @Success 200 {object} ListByDBResponse
// @Failure 400 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/sql-logs [get]
func (h *SQLLogQuery) ListByDB() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.repo == nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "repository not configured")
			return
		}
		dbName := r.URL.Query().Get("db")
		if dbName == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "missing db parameter")
			return
		}
		rows, err := h.repo.FindByDB(r.Context(), dbName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to query logs")
			h.log.Error("find by db failed", "db", dbName, "err", err)
			return
		}
		if len(rows) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"message": "Không tìm thấy truy vấn nào cho DB này",
				"items":   []sqlLogItem{},
			})
			return
		}
		items := make([]sqlLogItem, 0, len(rows))
		for _, r := range rows {
			items = append(items, sqlLogItem{
				SQLQuery:   r.SQLQuery,
				ExecTimeMs: r.ExecTimeMs,
				ExecCount:  r.ExecCount,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": items,
		})
	})
}
