package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type ctxKey int

const requestIDKey ctxKey = iota

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = genID()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

type statusWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.size += n
	return n, err
}

// bodyCaptureWriter captures response body up to max bytes while writing through.
type bodyCaptureWriter struct {
	http.ResponseWriter
	status int
	size   int
	max    int64
	buf    bytes.Buffer
}

func (w *bodyCaptureWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *bodyCaptureWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	// write-through
	n, err := w.ResponseWriter.Write(b)
	w.size += n
	// capture up to max
	if w.buf.Len() < int(w.max) {
		remain := int(w.max) - w.buf.Len()
		if remain > 0 {
			if len(b) > remain {
				_, _ = w.buf.Write(b[:remain])
			} else {
				_, _ = w.buf.Write(b)
			}
		}
	}
	return n, err
}

// teeReadCloser duplicates reads into an internal buffer up to max bytes.
type teeReadCloser struct {
	rc  io.ReadCloser
	buf *bytes.Buffer
	max int64
	cur int64
}

func (t *teeReadCloser) Read(p []byte) (int, error) {
	n, err := t.rc.Read(p)
	if n > 0 && t.cur < t.max {
		remain := t.max - t.cur
		cp := n
		if int64(cp) > remain {
			cp = int(remain)
		}
		_, _ = t.buf.Write(p[:cp])
		t.cur += int64(cp)
	}
	return n, err
}

func (t *teeReadCloser) Close() error {
	return t.rc.Close()
}

// sanitize headers for logging (mask sensitive)
func sanitizeHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		joined := strings.Join(v, ",")
		lk := strings.ToLower(k)
		switch lk {
		case "authorization", "cookie", "set-cookie", "x-api-key":
			out[k] = "***"
		default:
			out[k] = joined
		}
	}
	return out
}

func withLogging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(sw, r)
		log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"size", sw.size,
			"remote", r.RemoteAddr,
			"dur_ms", time.Since(start).Milliseconds(),
			"request_id", getRequestID(r.Context()),
		)
	})
}

// withRequestLogging logs request/response headers and bodies (truncated) and latency.
func withRequestLogging(log *slog.Logger, maxBody int64) func(http.Handler) http.Handler {
	if maxBody <= 0 {
		maxBody = 4096 // default cap to avoid huge logs
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Capture request body via tee so handlers can still read it.
			var reqBuf bytes.Buffer
			if r.Body != nil {
				r.Body = &teeReadCloser{rc: r.Body, buf: &reqBuf, max: maxBody}
			}

			// Capture response body
			bw := &bodyCaptureWriter{ResponseWriter: w, max: maxBody}

			next.ServeHTTP(bw, r)

			dur := time.Since(start).Milliseconds()
			log.Info("http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", bw.status,
				"resp_size", bw.size,
				"remote", r.RemoteAddr,
				"dur_ms", dur,
				"request_id", getRequestID(r.Context()),
				"req_headers", sanitizeHeaders(r.Header),
				"req_body", reqBuf.String(),
				"resp_body", bw.buf.String(),
			)
		})
	}
}

func withRecover(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered", "err", rec, "request_id", getRequestID(r.Context()))
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withCORS(allowed []string, next http.Handler) http.Handler {
	if len(allowed) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (isAllowedOrigin(origin, allowed) || allowed[0] == "*") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isAllowedOrigin(origin string, allowed []string) bool {
	for _, a := range allowed {
		if strings.EqualFold(strings.TrimSpace(a), origin) {
			return true
		}
	}
	return false
}

func chain(h http.Handler, m ...func(http.Handler) http.Handler) http.Handler {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

func genID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
