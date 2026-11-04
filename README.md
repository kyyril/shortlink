# Shortlink

A URL shortener REST API written in Go.

The project is small on purpose. Its real job is to be a reference and boilerplate for idiomatic Go backend work: layering, error handling, logging, testing, graceful shutdown, and concurrency in one codebase you can read in an afternoon.

> Status: planning. Code is built phase by phase. Check `docs/IMPLEMENTATION_PLAN.md` for what exists today. The commands below work once Phase 0 is done.

## What it does

- Users register and log in (JWT access token).
- Logged-in users create, list, view, and delete short links.
- Anyone can open `/{code}` and get redirected to the target URL.
- Every redirect is recorded asynchronously. Link owners can read click stats.

## Why this project

A URL shortener is simple to explain, but it touches the backend topics that matter in real teams:

- Authentication and ownership checks
- Unique code generation and conflict handling
- Cursor pagination
- Expiry and time handling (testable clock)
- Async work with goroutines, channels, and graceful shutdown
- Rate limiting
- Consistent error responses

## Tech stack

| Area | Choice |
| --- | --- |
| Language | Go (1.22 or newer, use the latest stable) |
| HTTP | `net/http` with `ServeMux` and a small middleware chain |
| Database | PostgreSQL with `pgx` |
| Queries | `sqlc` (SQL in, typed Go out) |
| Migrations | `goose` |
| Logging | `log/slog` |
| Auth | `golang-jwt/jwt/v5`, `bcrypt` |
| Rate limit | `golang.org/x/time/rate` |
| Tests | `testing`, `httptest`, `testcontainers-go` |
| Lint | `golangci-lint` |

Why each choice was made, and what the trade-off is: see `docs/ARCHITECTURE.md`.

## Quick start

Requirements: Go, Docker, `make`.

```sh
cp .env.example .env
make tools        # installs sqlc, goose, golangci-lint at pinned versions
make db-up        # starts PostgreSQL with Docker Compose
make migrate-up   # applies migrations
make run          # starts the API on :8080
```

Try it:

```sh
curl -s -X POST localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","password":"correct-horse-1"}'

TOKEN=$(curl -s -X POST localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","password":"correct-horse-1"}' | jq -r .access_token)

curl -s -X POST localhost:8080/v1/links \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://go.dev/doc/"}'

curl -i localhost:8080/<code-from-previous-response>
```

## Configuration

All config comes from environment variables. The app fails to start if a required value is missing or invalid.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_ENV` | `development` | `development` or `production` |
| `HTTP_ADDR` | `:8080` | Listen address |
| `BASE_URL` | `http://localhost:8080` | Used to build the `short_url` field |
| `DATABASE_URL` | required | PostgreSQL connection string |
| `JWT_SECRET` | required | At least 32 bytes |
| `JWT_TTL` | `15m` | Access token lifetime |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | `json` or `text` |
| `SHUTDOWN_TIMEOUT` | `15s` | Max time to drain requests and flush clicks |
| `REQUEST_TIMEOUT` | `5s` | Per-request timeout |
| `RATE_LIMIT_RPS` | `10` | Sustained requests per second per IP |
| `RATE_LIMIT_BURST` | `20` | Burst size per IP |
| `CLICK_BUFFER_SIZE` | `1024` | Buffered click events before dropping |

## API overview

Full contract is in `api/openapi.yaml` (added in Phase 6).

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| POST | `/v1/auth/register` | no | Create account |
| POST | `/v1/auth/login` | no | Get access token |
| POST | `/v1/links` | yes | Create short link |
| GET | `/v1/links` | yes | List own links (cursor pagination) |
| GET | `/v1/links/{code}` | yes | Get one own link |
| DELETE | `/v1/links/{code}` | yes | Delete own link |
| GET | `/v1/links/{code}/stats` | yes | Click stats |
| GET | `/{code}` | no | Redirect to target URL |
| GET | `/healthz` | no | Liveness |
| GET | `/readyz` | no | Readiness (checks database) |

Errors use RFC 9457 `application/problem+json`.

## Testing

```sh
make test               # unit and handler tests, with -race
make test-integration   # needs Docker, uses a real PostgreSQL container
make lint
make check              # lint + test + test-integration, same as CI
```

Strategy and scenarios: `docs/TESTING.md`.

## Project layout

```text
cmd/api/            main.go, only wiring and process startup
internal/
  app/              builds dependencies, routes, runs the server
  config/           typed env config
  auth/             register, login, tokens, auth middleware
  link/             link domain: model, service, handler
  click/            async click recorder and stats
  store/            PostgreSQL repositories (sqlc generated code inside)
  httpx/            middleware, problem+json, JSON helpers, pagination
db/
  migrations/       goose SQL files
  queries/          sqlc SQL files
api/openapi.yaml
docs/
```

## Documentation

- `docs/PRD.md`: what we build and how we know it is correct
- `docs/ARCHITECTURE.md`: how it works and why
- `docs/SYSTEM_DESIGN.md`: high-level to low-level design, scaling, trade-offs, interview Q&A
- `docs/IMPLEMENTATION_PLAN.md`: build order and progress
- `docs/TESTING.md`: how correctness is proven
- `AGENTS.md`: rules for AI coding agents
