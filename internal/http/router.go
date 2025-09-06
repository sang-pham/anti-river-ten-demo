package http

import (
	"expvar"
	nhttp "net/http"

	"log/slog"

	httpSwagger "github.com/swaggo/http-swagger"

	"go-demo/internal/auth"
	"go-demo/internal/config"
	"go-demo/internal/http/handlers"
	"go-demo/internal/sqllog"
)

func NewRouter(cfg config.Config, log *slog.Logger, authSvc *auth.Service, sqlLogRepo *sqllog.Repository) nhttp.Handler {
	mux := nhttp.NewServeMux()

	// Liveness and readiness
	mux.HandleFunc("GET /healthz", handlers.Healthz)
	mux.HandleFunc("GET /readyz", handlers.Readyz)

	// expvar
	mux.Handle("GET /debug/vars", expvar.Handler())

	// Swagger UI (non-production only)
	if cfg.Env != "production" {
		// Redirect /swagger to /swagger/index.html for convenience
		mux.HandleFunc("GET /swagger", func(w nhttp.ResponseWriter, r *nhttp.Request) {
			nhttp.Redirect(w, r, "/swagger/index.html", nhttp.StatusFound)
		})
		// Mount at /swagger/ (serves UI and doc.json)
		mux.Handle("/swagger/", httpSwagger.WrapHandler)
	}

	// Auth endpoints
	if authSvc != nil {
		ah := handlers.NewAuth(authSvc, log, cfg.MaxBodyBytes)
		mux.Handle("POST /v1/auth/register", ah.Register())
		mux.Handle("POST /v1/auth/login", ah.Login())
		mux.Handle("POST /v1/auth/refresh", ah.Refresh())
		mux.Handle("GET /v1/auth/me", handlers.RequireAuth(authSvc)(ah.Me()))

		// Admin endpoints - require ADMIN role
		adminMiddleware := func(h nhttp.Handler) nhttp.Handler {
			return handlers.RequireAuth(authSvc)(handlers.RequireAdminRole()(h))
		}
		mux.Handle("POST /v1/admin/users", adminMiddleware(ah.CreateUser()))
		mux.Handle("GET /v1/admin/users", adminMiddleware(ah.ListUsers()))
		mux.Handle("PUT /v1/admin/users/{id}/status", adminMiddleware(ah.UpdateUserStatus()))
		mux.Handle("PUT /v1/admin/users/{id}/role", adminMiddleware(ah.UpdateUserRole()))
		mux.Handle("DELETE /v1/admin/users/{id}", adminMiddleware(ah.DeleteUser()))
	}

	// SQL log upload endpoint
	if sqlLogRepo != nil {
		up := handlers.NewSQLLogUpload(sqlLogRepo, log, cfg.MaxBodyBytes)
		mux.Handle("POST /v1/sql-logs/upload", up.Upload())

		// SQL log query endpoints
		q := handlers.NewSQLLogQuery(sqlLogRepo, log)
		mux.Handle("GET /v1/sql-logs/databases", q.ListDatabases())
		mux.Handle("GET /v1/sql-logs", q.ListByDB())
	}
		// SQL log scan endpoint (authenticated)
		if authSvc != nil && sqlLogRepo != nil {
			scan := handlers.NewSQLLogScan(sqlLogRepo, log)
			// Support both GET (manual/curl) and POST (UI actions) to avoid 404 when UI uses POST
			mux.Handle("GET /v1/sql-logs/scan", handlers.RequireAuth(authSvc)(scan.Scan()))
			mux.Handle("POST /v1/sql-logs/scan", handlers.RequireAuth(authSvc)(scan.Scan()))
			// Handle CORS preflight even when ALLOWED_ORIGINS is empty (returns 204)
			mux.Handle("OPTIONS /v1/sql-logs/scan", nhttp.HandlerFunc(func(w nhttp.ResponseWriter, r *nhttp.Request) {
				w.WriteHeader(nhttp.StatusNoContent)
			}))
		}

	// Compose middleware (order matters; first is outermost)
	return chain(mux,
		withRequestID,
		func(h nhttp.Handler) nhttp.Handler { return withRecover(log, h) },
		func(h nhttp.Handler) nhttp.Handler { return withCORS(cfg.AllowedOrigins, h) },
		func(h nhttp.Handler) nhttp.Handler { return withRequestLogging(log, cfg.MaxBodyBytes)(h) },
	)
}
