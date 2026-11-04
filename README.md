# Shortlink

A high-performance URL shortener REST API written in idiomatic Go.

The project is designed as a production-grade reference and boilerplate for clean Go backend architecture: layering, error handling, logging, testing, graceful shutdown, async telemetry, rate limiting, idempotency, and concurrency in one cohesive codebase.

## Live Demo

The API is deployed and running live:

- **Base URL:** `https://shortlink-production-1c53.up.railway.app`
- **Application Hosting:** [Railway](https://railway.com) (Docker container on serverless infrastructure)
- **Database:** [Neon](https://neon.tech) (Serverless PostgreSQL 17)
- **Swagger UI Docs:** `https://shortlink-production-1c53.up.railway.app/docs` (Interactive documentation embedded directly in the server)
- **OpenAPI 3.0 Spec:** `https://shortlink-production-1c53.up.railway.app/openapi.yaml`
- **Liveness Probe:** `https://shortlink-production-1c53.up.railway.app/healthz`
- **Readiness Probe:** `https://shortlink-production-1c53.up.railway.app/readyz`
- **Demo Redirect:** `https://shortlink-production-1c53.up.railway.app/repo` -> redirects (`302`) to this GitHub repository

## What it does

- Users register and log in (`JWT HS256` access token, bcrypt password hashing).
- Logged-in users create, list (keyset pagination), view, and delete short links with optional custom codes and expiry.
- Anyone can open `/{code}` and get redirected (`302`) to the target URL.
- Every redirect is recorded asynchronously using an in-memory buffer and PostgreSQL binary `COPY FROM`.
- Link owners can inspect total and 30-day click stats.
- Token bucket rate limiting per client IP with idle cleanup.
- Optional `Idempotency-Key` header on link creation.
- Prometheus `/metrics` endpoint with request counts, duration histograms, and click drop counters.
- Health checks: `/healthz` (liveness) and `/readyz` (readiness with database ping).
- Standardized RFC 9457 `application/problem+json` error responses.

## Tech stack

| Area | Choice |
| --- | --- |
| Language | Go 1.26 |
| HTTP | `net/http` stdlib `ServeMux` with a clean middleware chain |
| Database | PostgreSQL 17 with `pgx/v5` connection pool |
| Queries | `sqlc` (type-safe SQL compiler) |
| Migrations | `goose` |
| Logging | `log/slog` (structured JSON in production) |
| Auth | `golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt` |
| Rate limit | `golang.org/x/time/rate` (per-IP token bucket) |
| Telemetry | `prometheus/client_golang` |
| Tests | `testing`, `httptest`, real PostgreSQL integration tests (`-race`) |
| Container | Docker multi-stage scratch image (~15 MB) running as non-root |

## Quick start

### Prerequisites
- Go 1.26+
- Docker and Docker Compose
- `make`

### Running locally with Docker Compose

To spin up the entire application along with PostgreSQL in containers:

```sh
docker compose up -d
```

Check readiness:
```sh
curl -i http://localhost:8080/readyz
```

### Running for local development

```sh
# 1. Copy environment template
cp .env.example .env

# 2. Start PostgreSQL container
make db-up

# 3. Apply database migrations
$env:DATABASE_URL="postgres://shortlink:shortlink@localhost:5433/shortlink?sslmode=disable"
goose -dir db/migrations postgres $env:DATABASE_URL up

# 4. Start the API
make run
```

### Quick API Tour

```sh
# 1. Register
curl -s -X POST http://localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"dev@example.com","password":"securepassword123"}'

# 2. Login
TOKEN=$(curl -s -X POST http://localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"dev@example.com","password":"securepassword123"}' | jq -r .access_token)

# 3. Create short link
curl -s -X POST http://localhost:8080/v1/links \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://go.dev/doc/effective_go","custom_code":"godocs"}'

# 4. Follow redirect
curl -i http://localhost:8080/godocs

# 5. Check stats
curl -s http://localhost:8080/v1/links/godocs/stats \
  -H "Authorization: Bearer $TOKEN"
```

## Configuration

All configuration is loaded from environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `APP_ENV` | `development` | `development` or `production` |
| `HTTP_ADDR` | `:8080` | Server listen address |
| `BASE_URL` | `http://localhost:8080` | Base URL used to build `short_url` |
| `DATABASE_URL` | *required* | PostgreSQL connection string |
| `JWT_SECRET` | *required* | Signing secret (minimum 32 bytes) |
| `JWT_TTL` | `15m` | JWT access token lifetime |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | `json` or `text` |
| `SHUTDOWN_TIMEOUT`| `15s` | Max duration to drain requests and flush telemetry |
| `REQUEST_TIMEOUT` | `5s` | Timeout per HTTP request |
| `RATE_LIMIT_RPS` | `10` | Sustained requests per second per client IP |
| `RATE_LIMIT_BURST`| `20` | Token bucket burst capacity per IP |
| `CLICK_BUFFER_SIZE`| `1024`| Size of the async click telemetry channel |

## API overview

Full OpenAPI 3.0 specification available in `api/openapi.yaml`.

You can explore and test the endpoints interactively via:
- **Interactive Swagger UI:** Visit `/docs` directly on the server (e.g. `https://shortlink-production-1c53.up.railway.app/docs` or `http://localhost:8080/docs`).
- **Raw OpenAPI Spec:** Visit `/openapi.yaml` on the server or inspect [api/openapi.yaml](api/openapi.yaml).

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| POST | `/v1/auth/register` | No | Register new user account |
| POST | `/v1/auth/login` | No | Authenticate user and issue JWT |
| POST | `/v1/links` | Yes | Create shortlink (supports Idempotency-Key) |
| GET | `/v1/links` | Yes | List owned links (keyset pagination) |
| GET | `/v1/links/{code}` | Yes | Get owned link details |
| DELETE | `/v1/links/{code}` | Yes | Delete owned link |
| GET | `/v1/links/{code}/stats`| Yes | Click stats (total & 30-day breakdown) |
| GET | `/{code}` | No | Redirect to long target URL |
| GET | `/healthz` | No | Liveness probe |
| GET | `/readyz` | No | Readiness probe (database ping) |
| GET | `/metrics` | No | Prometheus telemetry metrics |

## Testing & Quality Assurance

```sh
make fmt                # Code formatting (gofmt, goimports)
make lint               # Run golangci-lint
make test               # Unit tests with -race detector
make test-integration   # Integration tests with real PostgreSQL
make check              # Complete CI check (lint + test + test-integration)
```

## Documentation

- `docs/PRD.md`: Requirements and acceptance criteria (AC-1 to AC-18)
- `docs/ARCHITECTURE.md`: Architecture design, layering, and trade-off decisions
- `docs/SYSTEM_DESIGN.md`: Deep dive on scalability, capacity estimation, and interview guide
- `docs/IMPLEMENTATION_PLAN.md`: Step-by-step progress tracker and evidence log
- `docs/TESTING.md`: Testing guidelines and policy
- `AGENTS.md`: Rules and constraints for AI coding agents
