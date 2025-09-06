package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"go-demo/internal/sqllog"
)

type SQLLogUpload struct {
	repo         *sqllog.Repository
	log          *slog.Logger
	maxBodyBytes int64
}

func NewSQLLogUpload(repo *sqllog.Repository, log *slog.Logger, maxBodyBytes int64) *SQLLogUpload {
	if log == nil {
		log = slog.Default()
	}
	return &SQLLogUpload{repo: repo, log: log, maxBodyBytes: maxBodyBytes}
}

// UploadResponse is the success response body for upload endpoint
type UploadResponse struct {
	Message     string   `json:"message"`
	TotalLines  int32    `json:"total_lines"`
	Inserted    int32    `json:"inserted"`
	Skipped     int32    `json:"skipped"`
	Errors      []string `json:"errors"`
	ContentType string   `json:"content_type"`
	Filename    string   `json:"filename"`
}

// Upload godoc
// @Summary Upload SQL log file
// @Description Accepts multipart/form-data with field "file" (.log or .txt), parses valid entries and stores them; malformed lines are reported.
// @Tags sql-logs
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "logsql.txt"
// @Success 200 {object} UploadResponse
// @Failure 400 {object} ErrorEnvelope
// @Failure 500 {object} ErrorEnvelope
// @Router /v1/sql-logs/upload [post]
func (h *SQLLogUpload) Upload() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.repo == nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "repository not configured")
			return
		}

		// Limit request size
		if h.maxBodyBytes > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, h.maxBodyBytes)
		}

		// We don't need to keep form data in memory after parsing
		if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MiB memory cap
			writeError(w, http.StatusBadRequest, "bad_request", "invalid multipart form")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "missing file")
			return
		}
		defer safeClose(file)

		// Validate file type by extension and content-type hint
		if err := validateUpload(header); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}

		var total, inserted, skipped int
		var entries []sqllog.SQLLog
		var errs []string

		ctx := r.Context()
		err = sqllog.ParseStream(ctx, file,
			func(rec sqllog.SQLLog) error {
				total++
				entries = append(entries, rec)
				return nil
			},
			func(perr error) {
				total++
				skipped++
				// keep a bounded list of errors in response
				if len(errs) < 20 {
					errs = append(errs, perr.Error())
				}
				// also log at warn level
				h.log.Warn("sqllog parse error", "err", perr.Error())
			},
		)
		if err != nil && !errors.Is(err, context.Canceled) {
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("cannot parse file: %v", err))
			return
		}

		// Insert if we have at least one valid record
		if len(entries) > 0 {
			if err := h.repo.InsertBatch(ctx, entries); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("insert failed: %v", err))
				return
			}
			inserted = len(entries)
		}

		if total == 0 || inserted == 0 && skipped > 0 {
			// No valid records
			writeJSON(w, http.StatusOK, map[string]any{
				"message":      "no valid records found; nothing inserted",
				"total_lines":  total,
				"inserted":     inserted,
				"skipped":      skipped,
				"errors":       errs,
				"content_type": header.Header.Get("Content-Type"),
				"filename":     header.Filename,
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"message":      "upload processed",
			"total_lines":  total,
			"inserted":     inserted,
			"skipped":      skipped,
			"errors":       errs, // may be empty
			"content_type": header.Header.Get("Content-Type"),
			"filename":     header.Filename,
		})
	})
}

func validateUpload(h *multipart.FileHeader) error {
	name := strings.ToLower(h.Filename)
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".log", ".txt":
		// ok
	default:
		return fmt.Errorf("unsupported file extension: %s (allowed: .log, .txt)", ext)
	}
	// Optional: basic content-type hint (clients may send application/octet-stream)
	ct := strings.ToLower(h.Header.Get("Content-Type"))
	if ct != "" && !(strings.HasPrefix(ct, "text/plain") || ct == "application/octet-stream") {
		return fmt.Errorf("unsupported content-type: %s", ct)
	}
	return nil
}

func safeClose(c io.Closer) {
	_ = c.Close()
}
