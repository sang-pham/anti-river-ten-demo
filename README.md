# go-demo — REST API in Go

A minimal, idiomatic Go REST API with authentication using PostgreSQL and JWT, following the project rules in .cursor/rules/go-project-rules.md.

Quickstart

- Copy .env.example to .env and fill values
- Build: make build
- Run: make run
- Test: make test

Environment variables

- PORT: HTTP port (default 8080)
- LOG_LEVEL: info, debug, warn, error (default info)
- REQUEST_TIMEOUT: request timeout, e.g. 30s (default 30s)
- MAX_BODY_BYTES: max request body size, e.g. 1048576 (default 1048576 ~ 1MiB)
- ALLOWED_ORIGINS: comma-separated CORS allowlist; unset means disabled
- DATABASE_URL: PostgreSQL DSN (e.g., postgres://user:pass@localhost:5432/db?sslmode=disable)
- JWT_SECRET: HMAC secret for signing JWTs (required for auth)
- JWT_TTL: Token lifetime (Go duration, e.g., 24h)

Endpoints (v1)

- GET /healthz — Liveness
- GET /readyz — Readiness
- GET /debug/vars — expvar metrics

Auth endpoints

- POST /v1/auth/register — Create a user: { "username": "...", "email": "...", "password": "..." }
- POST /v1/auth/login — Login with username or email: { "identifier": "...", "password": "..." }
- GET /v1/auth/me — Get current user (requires Authorization: Bearer <token>)

Quick auth test (after server running and DB configured)

- Register:
  curl -sS -X POST http://localhost:8080/v1/auth/register \
   -H 'Content-Type: application/json' \
   -d '{"username":"alice","email":"alice@example.com","password":"secret"}'
- Login (username or email):
  TOKEN=$(curl -sS -X POST http://localhost:8080/v1/auth/login \
   -H 'Content-Type: application/json' \
   -d '{"identifier":"alice","password":"secret"}' | jq -r .token)
- Me:
  curl -sS http://localhost:8080/v1/auth/me -H "Authorization: Bearer ${TOKEN}"

Docker Compose for PostgreSQL

- Provided at docker-compose.yml with persistent storage bind mounted at /home/sang/web/postgresql/data and host port 5432 exposed.

References

- docs/architecture.md
- .cursor/rules/go-project-rules.md
# anti-river-ten-demo
