# go-demo — REST API in Go

A minimal, idiomatic Go REST API featuring authentication with JWT access tokens and refresh tokens, PostgreSQL via GORM, structured request logging, health endpoints, Dockerized deployment, and Compose orchestration. Follows the project rules in [.cursor/rules/go-project-rules.md](.cursor/rules/go-project-rules.md).

Features

- Auth
  - Register/login with bcrypt-hashed passwords
  - JWT access tokens with custom claim role
  - Opaque refresh tokens (hashed in DB) with rotation on use
  - Default role USER assigned on registration
- Database
  - PostgreSQL with GORM
  - All tables created under DEMO schema
  - Tables: DEMO.USER, DEMO.ROLE, DEMO.REFRESH_TOKEN
  - Seeds roles USER and ADMIN on startup
- Observability
  - Health: /healthz and /readyz
  - Metrics: /debug/vars (expvar)
  - Request logging middleware: captures headers, bodies (truncated), status, and latency
- Containerization
  - Dockerfile for multi-stage build
  - docker-compose for app + Postgres with bridge network

Quickstart

- Local (Go)
  - Copy .env.example to .env and fill values
  - Run: PORT=8080 DATABASE_URL=postgres://postgres:postgres@localhost:5432/go_demo?sslmode=disable JWT_SECRET=replace-me go run ./cmd/api
- Docker Compose
  - docker compose up -d --build
  - API: http://localhost:${PORT:-8080}
  - Postgres: localhost:${POSTGRES_PORT:-5432}

Environment variables

- PORT: HTTP port (default 8080)
- LOG_LEVEL: info, debug, warn, error (default info)
- REQUEST_TIMEOUT: request timeout, e.g. 30s (default 30s)
- MAX_BODY_BYTES: max request body size and log capture cap, e.g. 1048576 (default ~1MiB)
- ALLOWED_ORIGINS: comma-separated CORS allowlist; unset means disabled
- DATABASE_URL: PostgreSQL DSN
  - Compose default inside network: postgres://postgres:postgres@postgres:5432/go_demo?sslmode=disable
  - Host example: postgres://postgres:postgres@localhost:5432/go_demo?sslmode=disable
- JWT_SECRET: HMAC secret for signing JWTs (required)
- JWT_TTL: Access token lifetime (Go duration, e.g., 24h)
- REFRESH_TTL: Refresh token lifetime (Go duration, default 720h = 30 days)

Database schema

- All tables live in the DEMO schema
- DEMO.ROLE (code PK, name, description, created_by, updated_by, created_time, updated_time)
- DEMO.USER (id UUID PK, username, email, password, role FK->ROLE.code, created_by, updated_by, created_time, updated_time)
- DEMO.REFRESH_TOKEN (id UUID PK, user_id UUID FK->USER.id, token_hash sha256 hex, expires_at, created_time)
- Schema and migrations
  - Created on startup in [db.New()](internal/db/db.go:23)
  - AutoMigrate: [Role, User, RefreshToken](internal/db/db.go:53)
- Seeding
  - On startup: roles USER and ADMIN are inserted if missing
  - Programmatic seeding: [cmd/seed/main.go](cmd/seed/main.go:1)

Endpoints (v1)

- Platform
  - GET /healthz — Liveness
  - GET /readyz — Readiness
  - GET /debug/vars — expvar metrics
- Auth
  - POST /v1/auth/register — Create a user
    - Request: { "username": "...", "email": "...", "password": "..." }
    - Notes: New users get role USER
  - POST /v1/auth/login — Login with username or email
    - Request: { "identifier": "...", "password": "..." }
    - Response: { "token", "expires_at", "refresh_token", "refresh_expires_at", "user": { ... , "role": "..." } }
  - POST /v1/auth/refresh — Exchange refresh token for a new access token (rotation)
    - Request: { "refresh_token": "..." }
    - Response: { "token", "expires_at", "refresh_token", "refresh_expires_at", "user": { ... } }
  - GET /v1/auth/me — Get current user (requires Authorization: Bearer <token>)

Request logging

- Middleware captures:
  - method, path, status, resp_size, remote, dur_ms, request_id
  - req_headers (sensitive masked), req_body (truncated), resp_body (truncated)
- Sensitive headers masked: Authorization, Cookie, Set-Cookie, X-API-Key
- See [http.withRequestLogging()](internal/http/middleware.go:150)
- Middleware order: see [http.NewRouter()](internal/http/router.go:14)

Auth implementation references

- Service and tokens
  - Register defaults role USER: [auth.Service.Register()](internal/auth/service.go:41)
  - Login returns access + refresh tokens: [auth.Service.Login()](internal/auth/service.go:76)
  - Access token claims with role: [auth.Claims](internal/auth/service.go:26), [auth.Service.GenerateToken()](internal/auth/service.go:103)
  - Refresh token generate/rotate: [auth.Service.GenerateRefreshToken()](internal/auth/service.go:157), [auth.Service.Refresh()](internal/auth/service.go:187)
- HTTP handlers
  - Auth handlers: [handlers.Auth](internal/http/handlers/auth.go:14) with Register, Login, Refresh, Me
- Routing
  - Routes wired in [http.NewRouter()](internal/http/router.go:14)

Quick auth test (after server running and DB configured)

- Register:
  curl -sS -X POST http://localhost:8080/v1/auth/register \
   -H 'Content-Type: application/json' \
   -d '{"username":"alice","email":"alice@example.com","password":"secret"}'
- Login (username or email):
  LOGIN=$(curl -sS -X POST http://localhost:8080/v1/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"identifier":"alice","password":"secret"}')
  TOKEN=$(echo "$LOGIN" | jq -r .token)
  RTOKEN=$(echo "$LOGIN" | jq -r .refresh_token)
- Me:
  curl -sS http://localhost:8080/v1/auth/me -H "Authorization: Bearer ${TOKEN}"
- Refresh:
  REFRESH=$(curl -sS -X POST http://localhost:8080/v1/auth/refresh \
    -H 'Content-Type: application/json' \
    -d "{\"refresh_token\":\"${RTOKEN}\"}")
  NEW_TOKEN=$(echo "$REFRESH" | jq -r .token)

Docker and Compose

- Dockerfile: multi-stage build at [Dockerfile](Dockerfile:1)
- docker-compose: app + Postgres with bridge network at [docker-compose.yml](docker-compose.yml:1)
- Default DATABASE_URL uses hostname postgres inside the compose network
- Persistence:
  - By default: named volume pgdata -> /var/lib/postgresql/data
  - To use a host bind mount instead, change in docker-compose.yml:
    - From: pgdata:/var/lib/postgresql/data
    - To: /home/sang/web/postgresql/data:/var/lib/postgresql/data

Development notes

- Logging uses slog; request IDs via [http.withRequestID](internal/http/middleware.go:19)
- Panic recovery via [http.withRecover](internal/http/middleware.go:187)
- Build and run
  - go build ./... and go run ./cmd/api
  - docker compose up -d --build

References

- docs/architecture.md
- .cursor/rules/go-project-rules.md

## Swagger UI

This project uses swaggo annotations to generate OpenAPI docs and serves Swagger UI at /swagger in non-production environments.

- Generation

  - Install swag CLI (already added in go install step when setting up): go install github.com/swaggo/swag/cmd/swag@v1.16.4
  - Or use Makefile target: make swagger
  - This generates docs/ with docs.go, swagger.json, swagger.yaml

- Serving the UI

  - The router mounts Swagger UI at /swagger only when APP_ENV != production
  - Set APP_ENV=development (default) in your environment or .env

- Quick usage

  - Update .env (or environment):
    - APP_ENV=development
  - Generate docs:
    - make swagger
  - Run the API (ensure your DATABASE_URL etc. are set), then open:
    - http://localhost:${PORT:-8080}/swagger

- Notes
  - Protected endpoints (e.g., /v1/auth/me) use BearerAuth (Authorization header)
  - Health endpoints are documented:
    - GET /healthz
    - GET /readyz
