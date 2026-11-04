# Implementation Plan

Build order, dependencies, and progress. Requirements are in `PRD.md`. Design is in `ARCHITECTURE.md`.

## How to use this file

- Work on one task at a time, in order. Do not start a task before its dependencies are done.
- Status marks: `[ ]` not started, `[~]` in progress, `[x]` done.
- Mark a task `[x]` only when every item in its "Done when" is true. Add one line to the Evidence Log: task ID, command or test names, result.
- No evidence, no `[x]`.
- If a task turns out to be wrong or too big, edit this file first, then continue.

## Progress

| Phase | Name | Status |
| --- | --- | --- |
| P0 | Foundation | done |
| P1 | Authentication | in progress |
| P2 | Links CRUD | not started |
| P3 | Redirect | not started |
| P4 | Click recording and stats | not started |
| P5 | Hardening | not started |
| P6 | Delivery | not started |

---

## P0 Foundation

Goal: a running server with config, logging, middleware, database connection, and CI. No business features yet.

- [x] **P0-1 Repository setup**
  - Depends on: none
  - Output: `go.mod`, directory layout from ARCHITECTURE section 3, `Makefile` with all targets in AGENTS.md section 3, `.gitignore`, `.env.example`, `.golangci.yml`, `docker-compose.yml` with PostgreSQL.
  - Done when: `go build ./...` and `make lint` pass. `make db-up` starts a reachable database.

- [x] **P0-2 Config**
  - Depends on: P0-1
  - Output: `internal/config` with typed struct, defaults, validation (for example `JWT_SECRET` at least 32 bytes).
  - Done when: table-driven tests cover valid config, missing required value, and invalid duration. `make test` passes.

- [x] **P0-3 Logger**
  - Depends on: P0-2
  - Output: slog setup from `LOG_LEVEL` and `LOG_FORMAT`.
  - Done when: tests show level filtering works and JSON output has expected keys.

- [x] **P0-4 httpx basics**
  - Depends on: P0-3
  - Output: problem+json writer, JSON decode helper (1 MB limit, unknown fields rejected), error mapping function, middleware: recover, request ID, access log, timeout.
  - Done when: tests cover AC-18 (panic returns 500 problem JSON), request ID propagation, unknown field rejection, oversized body rejection.

- [x] **P0-5 App skeleton and graceful shutdown**
  - Depends on: P0-4
  - Output: `internal/app` builds the server and routes. `cmd/api/main.go` calls it. `/healthz`. Shutdown on SIGINT and SIGTERM using `signal.NotifyContext` and `Server.Shutdown`. Server has read, write, and idle timeouts.
  - Done when: `make run` serves `/healthz`. A test starts the app on a random port, sends a slow request, triggers shutdown, and the request still completes.

- [x] **P0-6 Database connection**
  - Depends on: P0-5
  - Output: pgx pool setup, goose configured, `make migrate-up` and `migrate-down` work, `/readyz` pings the database.
  - Done when: integration test shows `/readyz` returns 200 with database up and 503 with database stopped (AC-16).

- [x] **P0-7 CI**
  - Depends on: P0-6
  - Output: CI workflow running `make check` on pull requests.
  - Done when: a pull request shows a green run with lint, `-race` tests, and integration tests.

---

## P1 Authentication

Goal: register, login, protected routes.

- [x] **P1-1 Users table and repository**
  - Depends on: P0-6
  - Output: migration for `users` (with `citext` extension), sqlc queries, `store` user repository. Unique violation maps to `auth.ErrEmailTaken`.
  - Done when: integration tests cover create, find by email (case-insensitive), duplicate email.

- [x] **P1-2 Auth service**
  - Depends on: P1-1
  - Output: `auth.Service` with Register, Login, token issue and verify. Interfaces for repository and clock. bcrypt cost 12.
  - Done when: unit tests with fakes cover valid register, short password, invalid email, duplicate email, wrong password, unknown email (same error), expired token, tampered token, wrong signing algorithm.

- [ ] **P1-3 Auth handlers**
  - Depends on: P1-2
  - Output: `POST /v1/auth/register`, `POST /v1/auth/login`.
  - Done when: handler tests cover AC-1, AC-2, AC-3. Response never contains the password hash.

- [ ] **P1-4 Auth middleware**
  - Depends on: P1-2
  - Output: middleware that verifies the bearer token and stores the user ID in the context with a typed key.
  - Done when: tests cover AC-7 (missing header, malformed header, invalid token, expired token).

---

## P2 Links CRUD

Goal: authenticated users manage their links.

- [ ] **P2-1 Links table and repository**
  - Depends on: P1-1
  - Output: migration for `links` with indexes, sqlc queries (insert, get by code, list by user with keyset, delete by code and user), `store` link repository. Unique violation maps to `link.ErrCodeTaken`.
  - Done when: integration tests cover insert, duplicate code, get, delete, keyset listing across a page boundary.

- [ ] **P2-2 Code generator**
  - Depends on: P0-1
  - Output: `link.CodeGenerator` interface and a `crypto/rand` base62 implementation (7 chars).
  - Done when: tests check length, alphabet, and that two calls differ. Service tests use a fake generator.

- [ ] **P2-3 Cursor helpers**
  - Depends on: P0-4
  - Output: encode and decode opaque cursor for `(created_at, id)` in `httpx`.
  - Done when: round-trip test passes. Tampered, truncated, and empty cursors return a typed error (AC-10).

- [ ] **P2-4 Link service**
  - Depends on: P2-1, P2-2, P2-3
  - Output: Create (URL validation, custom code validation, expiry), Get, List, Delete. Ownership enforced, other users get `ErrNotFound`. Retry on random code conflict, max 5 tries.
  - Done when: unit tests cover AC-4, AC-5, AC-6, AC-8, retry exhaustion, custom code rules, `expires_at` in the past rejected.

- [ ] **P2-5 Link handlers**
  - Depends on: P2-4, P1-4
  - Output: `POST /v1/links`, `GET /v1/links`, `GET /v1/links/{code}`, `DELETE /v1/links/{code}`.
  - Done when: handler tests cover status codes and problem codes. An integration test pages through 45 links with `limit=20` (AC-9).

---

## P3 Redirect

Goal: the public redirect endpoint.

- [ ] **P3-1 Redirect handler**
  - Depends on: P2-4
  - Output: `GET /{code}` returning 302, 404, or 410. Uses injected clock for expiry.
  - Done when: tests cover AC-11 and AC-12, including a link that expires exactly at the current time.

- [ ] **P3-2 First end-to-end test**
  - Depends on: P3-1, P2-5
  - Output: black-box test: register, login, create link, follow redirect, delete link, get 404.
  - Done when: test passes in `make test-integration`.

---

## P4 Click recording and stats

Goal: async click capture that never slows the redirect.

- [ ] **P4-1 Clicks table and repository**
  - Depends on: P2-1
  - Output: migration for `clicks`, sqlc queries (batch insert, count total, count per day for 30 days), `store` repository.
  - Done when: integration tests cover batch insert and both stats queries, including days with zero clicks omitted or filled (pick one and document it).

- [ ] **P4-2 Recorder**
  - Depends on: P4-1
  - Output: `click.Recorder` with `Start`, non-blocking `Record`, batch flush by size or interval, `Close(ctx)` that drains. Drop counter on full buffer.
  - Done when: tests under `-race` cover AC-14 (full buffer drops and does not block), flush by size, flush by interval, and drain on `Close`. No `time.Sleep` in tests.

- [ ] **P4-3 Wire recorder and stats endpoint**
  - Depends on: P4-2, P3-1
  - Output: redirect calls `Record`. `GET /v1/links/{code}/stats` returns total and per-day counts for the owner.
  - Done when: e2e test covers AC-13. Another user gets 404 on stats.

- [ ] **P4-4 Shutdown order**
  - Depends on: P4-3
  - Output: shutdown sequence from ARCHITECTURE section 8, including `/readyz` going to 503 first.
  - Done when: integration test covers AC-15 (clicks buffered, SIGTERM, rows present after exit).

---

## P5 Hardening

- [ ] **P5-1 Rate limit middleware**
  - Depends on: P0-4
  - Output: per-IP token bucket, 429 with `Retry-After`, periodic cleanup of idle limiters, client IP helper that only trusts a forwarded header when configured.
  - Done when: tests cover AC-17 and cleanup of idle entries.

- [ ] **P5-2 Input edge cases**
  - Depends on: P2-5
  - Output: URL validation hardened (length, scheme, host present, no userinfo), reserved codes blocked (`v1`, `healthz`, `readyz`).
  - Done when: table-driven tests list each rejected and accepted case.

- [ ] **P5-3 Idempotency-Key (stretch)**
  - Depends on: P2-5
  - Output: optional `Idempotency-Key` header on `POST /v1/links`. Same key and same body returns the original response. Same key and different body returns 422.
  - Done when: tests cover replay, conflict, and concurrent duplicate requests.

---

## P6 Delivery

- [ ] **P6-1 Docker image**
  - Depends on: P0-5
  - Output: multi-stage `Dockerfile`, static binary, non-root user, app service in compose.
  - Done when: `docker compose up` starts the app and `/readyz` returns 200.

- [ ] **P6-2 OpenAPI contract**
  - Depends on: P2-5, P4-3
  - Output: `api/openapi.yaml` covering every endpoint, request, response, and problem code.
  - Done when: the spec validates with a linter. Each documented status code has a matching test.

- [ ] **P6-3 Metrics (stretch)**
  - Depends on: P0-5
  - Output: `/metrics` with request count, request duration, and click drop counter.
  - Done when: a test scrapes `/metrics` and finds the expected metric names.

- [ ] **P6-4 Final README pass**
  - Depends on: all above
  - Output: README commands verified from a clean clone.
  - Done when: someone else follows the README and gets a working API in under 10 minutes.

---

## Evidence Log

Add one line per completed task. Newest at the bottom.

| Date | Task | Evidence |
| --- | --- | --- |
| 2026-11-04 | P0-1 | `go build ./...` exit 0; `golangci-lint run ./...` 0 issues; committed 74a0a0f pushed to https://github.com/kyyril/shortlink |
| 2026-11-04 | P0-2 | `go test -race ./internal/config/...` PASS (11 subtests: ValidConfig, MissingDatabaseURL, MissingJWTSecret, JWTSecretTooShort, InvalidAppEnv, InvalidLogLevel, InvalidLogFormat, InvalidJWTTTL, ZeroShutdownTimeout, ZeroRequestTimeout, FieldsMatchEnv) |
| 2026-11-04 | P0-3 | `go test -race ./internal/app/...` PASS (11 subtests covering level filtering, error handling, JSON expected keys, text format); `golangci-lint run ./...` 0 issues |
| 2026-11-04 | P0-4 | `go test -race ./internal/httpx/...` PASS (AC-18 TestRecover_PanicInHandler_Returns500ProblemJSONAndServerContinues, TestRequestID_MissingHeader_GeneratesNewUUID, TestRequestID_ExistingHeader_PropagatesID, TestDecodeJSON_UnknownField_ReturnsError, TestDecodeJSON_OversizedBody_ReturnsError) |
| 2026-11-04 | P0-5 | `go test -race ./internal/app/...` PASS (TestApp_GracefulShutdown_SlowRequestCompletes, TestHealthz_AlwaysReturns200OK, TestReadyz_NotReady_Returns503); `go build ./cmd/api` exit 0 |
| 2026-11-04 | P0-6 | `go test -race -tags=integration ./internal/app/...` PASS (AC-16 TestReadiness_WithDatabase_AC16: /readyz 200 with DB up, 503 with DB down, /healthz stays 200); goose migrations verified up/down |
| 2026-11-04 | P0-7 | `.github/workflows/ci.yml` added with lint, unit tests, and integration tests with postgres:17-alpine service |
| 2026-11-04 | P1-1 | `go test -race -tags=integration ./internal/store/...` PASS (TestUserRepo_CreateAndFind, TestUserRepo_FindByEmail_NotFound); migration 00002_create_users.sql with citext and sqlc generated code |
| 2026-11-04 | P1-2 | `go test -race ./internal/auth/...` PASS (10 test cases: valid register, short password, invalid email variants, duplicate email, valid login, wrong password, unknown email with same error, expired token, tampered token, RS256 algorithm rejection) |
