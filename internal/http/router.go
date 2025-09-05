package http

import (
	"expvar"
	nhttp "net/http"

	"log/slog"

	httpSwagger "github.com/swaggo/http-swagger"

	"go-demo/internal/auth"
	"go-demo/internal/config"
	"go-demo/internal/http/handlers"
)

func NewRouter(cfg config.Config, log *slog.Logger, authSvc *auth.Service) nhttp.Handler {
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
	}

	// Compose middleware (order matters; first is outermost)
	return chain(mux,
		withRequestID,
		func(h nhttp.Handler) nhttp.Handler { return withRecover(log, h) },
		func(h nhttp.Handler) nhttp.Handler { return withCORS(cfg.AllowedOrigins, h) },
		func(h nhttp.Handler) nhttp.Handler { return withRequestLogging(log, cfg.MaxBodyBytes)(h) },
	)
}
