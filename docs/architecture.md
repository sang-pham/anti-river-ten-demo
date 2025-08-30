# Go REST API Service — Architecture

Goals

- Build a minimal, idiomatic Go REST API with auth (JWT) and PostgreSQL using ORM (GORM).
- Moderate privacy: no secrets in code; env-based config; minimal dependencies.
- Follow the repository rules in [.cursor/rules/go-project-rules.md](.cursor/rules/go-project-rules.md).

High-level design

- HTTP server using net/http.
- Auth service for registration/login/JWT verify.
- Structured logging with log/slog; basic metrics via expvar.
- Graceful shutdown and context-driven cancellation.

Project layout

- [cmd/api/main.go](cmd/api/main.go:1)
- [internal/config/config.go](internal/config/config.go:1)
- [internal/http/server.go](internal/http/server.go:1)
- [internal/http/router.go](internal/http/router.go:1)
- [internal/http/middleware.go](internal/http/middleware.go:1)
- [internal/http/handlers/health.go](internal/http/handlers/health.go:1)
- [internal/http/handlers/auth.go](internal/http/handlers/auth.go:1)
- [internal/http/handlers/middleware.go](internal/http/handlers/middleware.go:1)
- [internal/http/handlers/json.go](internal/http/handlers/json.go:1)
- [internal/db/db.go](internal/db/db.go:1)
- [internal/auth/service.go](internal/auth/service.go:1)
- [internal/observability/logging.go](internal/observability/logging.go:1)
- [internal/observability/metrics.go](internal/observability/metrics.go:1)
- [internal/version/version.go](internal/version/version.go:1)
- [README.md](README.md:1)

API surface

- Auth
  - POST /v1/auth/register
  - POST /v1/auth/login
  - GET /v1/auth/me
- Platform
  - GET /healthz
  - GET /readyz
  - GET /debug/vars

Configuration

- PORT (default 8080)
- LOG_LEVEL (info, debug, warn, error; default info)
- REQUEST_TIMEOUT (e.g., 30s)
- MAX_BODY_BYTES (e.g., 1MiB)
- ALLOWED_ORIGINS (for CORS; optional)
- DATABASE_URL (PostgreSQL DSN)
- JWT_SECRET (HMAC secret)
- JWT_TTL (e.g., 24h)

Dependency policy

- Standard library first: net/http, log/slog, expvar, encoding/json, context.
- Add external deps only if clear benefit and they meet the rules.
- ORM: GORM with postgres driver for DB access and migrations.

Error handling

- No panics for expected failures.
- Return errors with context using fmt.Errorf with %w.
- Single logging boundary: handlers log with request context; do not double-log.
- JSON error envelope: {"error":{"code":"...", "message":"..."}}
  - 400: validation errors, bad JSON
  - 401: missing or invalid credentials
  - 409: resource conflict (e.g., user exists)
  - 500: unexpected server error

Observability

- Logging: slog with source, request_id, route, status, latency.
- Metrics: expvar counters for requests_total, requests_errors_total, request_latency_ms.
- Trace hooks: design extension points; integrate later.

Security

- Never log secrets.
- Limit request body size and JSON depth.
- Set timeouts on server and outbound HTTP client.
- Basic CORS allowlist when ALLOWED_ORIGINS is set.
- JWT auth: HMAC-SHA256 tokens with TTL and subject = user ID.

Performance and reliability

- HTTP server with sensible timeouts and keep-alives.
- DB pooling tuned via database/sql settings exposed by GORM.

Graceful shutdown

- Use context cancellation and http.Server.Shutdown with a deadline.
- Ensure background workers exit on shutdown.

Authentication/Authorization

- Model: [db.User](internal/db/db.go:64) with UUID primary key and fields username, email, password (hashed), created_by, created_time, updated_time. Table name: USER.
- Service: [auth.Service](internal/auth/service.go:21) provides Register, Login, GenerateToken, ParseToken, GetUserByID.
- Middleware: [handlers.RequireAuth](internal/http/handlers/middleware.go:24) validates Bearer token and injects user into context using [authctx.WithUser](internal/authctx/context.go:12).
- Handlers: [handlers.Auth](internal/http/handlers/auth.go:16) exposes register, login, me.

Acceptance criteria

- go build ./... succeeds.
- go test ./... passes (add tests progressively).
- /healthz and /readyz return 200; /debug/vars is exposed.
- Auth flows:
  - Register creates a user with hashed password and unique username/email.
  - Login returns a signed JWT and user info when credentials are valid.
  - Me returns current user when provided a valid Bearer token.

Out of scope for this version

- External LLM/chat features (removed).
- Multi-tenant quota management.
- Prompt templating UI.

Future extensions

- Role-based authorization.
- Password reset flows and email verification.
- OpenTelemetry tracing and Prometheus metrics.

References

- [.cursor/rules/go-project-rules.md](.cursor/rules/go-project-rules.md)
- Effective Go, Code Review Comments, Uber Go Style Guide
