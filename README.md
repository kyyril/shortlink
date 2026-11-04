# Shortlink

[![CI](https://github.com/kyyril/shortlink/actions/workflows/ci.yml/badge.svg)](https://github.com/kyyril/shortlink/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kyyril/shortlink)](https://goreportcard.com/report/github.com/kyyril/shortlink)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/kyyril/shortlink)](go.mod)

A production-grade, high-throughput URL shortener REST API built with Go. Clean layering, zero-allocation serialization, non-blocking telemetry buffering, connection pooling, idempotency, per-IP token bucket rate limiting, and graceful draining.

---

## Live Production Demo

The service is deployed live on serverless cloud infrastructure:

- **Base URL:** `https://shortlink-production-1c53.up.railway.app`
- **Interactive Swagger UI:** [`/docs`](https://shortlink-production-1c53.up.railway.app/docs) (Try API requests directly from the browser)
- **OpenAPI 3.0 Specification:** [`/openapi.yaml`](https://shortlink-production-1c53.up.railway.app/openapi.yaml)
- **Liveness Probe:** [`/healthz`](https://shortlink-production-1c53.up.railway.app/healthz)
- **Readiness Probe:** [`/readyz`](https://shortlink-production-1c53.up.railway.app/readyz)
- **Prometheus Telemetry:** [`/metrics`](https://shortlink-production-1c53.up.railway.app/metrics)
- **Live 302 Redirect Demo:** [`/repo`](https://shortlink-production-1c53.up.railway.app/repo) (Redirects to this repository)

---

## System Architecture

```mermaid
flowchart TD
    Client(["HTTP Clients / Browsers"])

    %% Edge & Ingress
    Ingress["TLS Termination & HTTP Routing"]

    %% Middleware Pipeline
    MW_CORS["CORS"]
    MW_ReqID["Request ID (UUIDv4)"]
    MW_Recov["Panic Recovery"]
    MW_Metrics["Prometheus Metrics"]
    MW_Log["Structured Access Log (slog)"]
    MW_Timeout["Context Timeout (5s)"]
    MW_Rate["Per-IP Rate Limiter (Token Bucket)"]

    %% Domain Handlers
    H_Auth["Auth Handler (/v1/auth/*)"]
    H_Link["Link Handler (/v1/links/*)"]
    H_Redir["Redirect Handler (GET /{code})"]
    H_Stats["Stats Handler (/v1/links/{code}/stats)"]
    H_Docs["Swagger UI & OpenAPI (/docs)"]

    %% Business Logic Layer
    S_Auth["Auth Service (bcrypt + JWT HS256)"]
    S_Link["Link Service (Base62 Gen + Expiry Logic)"]
    S_Stats["Stats Aggregation Service"]
    S_Idem["Idempotency Engine (SHA-256 Hash Matching)"]

    %% Asynchronous Telemetry Worker
    Channel["Ring/Channel Buffer (Capacity: 1024)"]
    BatchWorker["Batch Collector Worker"]
    CopyFrom["PostgreSQL Binary COPY FROM Streamer"]

    %% Persistence
    DB_Users[("users")]
    DB_Links[("links (B-Tree indexed code)")]
    DB_Clicks[("clicks (Append-only partitioned)")]
    DB_Idem[("idempotency_keys")]

    %% Flow Connections
    Client --> Ingress
    Ingress --> MW_CORS --> MW_ReqID --> MW_Recov --> MW_Metrics --> MW_Log --> MW_Timeout --> MW_Rate
    MW_Rate --> H_Auth & H_Link & H_Redir & H_Stats & H_Docs

    H_Auth --> S_Auth --> DB_Users
    H_Link --> S_Idem --> DB_Idem
    H_Link --> S_Link --> DB_Links
    H_Stats --> S_Stats --> DB_Clicks

    H_Redir -. "Non-blocking write (select/default)" .-> Channel
    H_Redir -- "Immediate 302 Found" --> Client

    Channel --> BatchWorker
    BatchWorker -- "Every 500ms or 100 events" --> CopyFrom --> DB_Clicks
```

---

## Technical Decisions

### 1. Asynchronous, Zero-Loss Redirect Telemetry
Redirect latency must never be bound to database write latency.
- When an HTTP client accesses `GET /{code}`, the target URL is fetched from an indexed query.
- The click event is emitted to a fixed-size buffered channel (`1024` slots) using a non-blocking `select/default` pattern.
- A dedicated background worker goroutine flushes queued clicks every `500ms` or when `100` events accumulate.
- Bulk persistence leverages PostgreSQL's binary **`COPY FROM` protocol**, bypassing standard `INSERT` parsing overhead for maximum write throughput.
- Graceful shutdown handles buffer draining with timeout context, ensuring in-flight click data is flushed to PostgreSQL before process exit.

### 2. Idempotent Link Creation (`Idempotency-Key`)
To prevent duplicate links caused by network retries or double submits:
- `POST /v1/links` accepts an optional `Idempotency-Key` header.
- The request body is digested with `SHA-256`.
- Identical replays return the cached response immediately (`HTTP 201 Created`).
- Modified payloads with the same key are rejected with `HTTP 422 Unprocessable Entity`.

### 3. Keyset (Cursor-Based) Pagination
- Pagination uses deterministic composite cursor ordering `(created_at, id)`.
- Eliminates the `O(N)` query cost degradation of classical `OFFSET/LIMIT` pagination when scanning large datasets.

### 4. Per-IP Token Bucket Rate Limiting
- Built on `golang.org/x/time/rate` with sustained rate and burst capacity.
- Thread-safe `sync.RWMutex` map with an automated background scavenger goroutine that prunes idle client entries to eliminate memory leaks.

### 5. Standardized Error Handling (RFC 9457)
- All client and server errors serialize into RFC 9457 `application/problem+json` format.
- Every error includes a machine-readable `code`, human-readable `detail`, and a traceable `request_id`.

---

## Tech Stack

| Area | Technology | Motivation |
| --- | --- | --- |
| **Language** | Go 1.26 | Predictable performance, fast compile times, zero runtime dependencies |
| **HTTP Routing** | `net/http` stdlib (`ServeMux`) | Go 1.22+ method + path pattern routing without external framework overhead |
| **Database** | PostgreSQL 17 (`pgx/v5`) | Binary protocol support, connection pooling, high concurrency |
| **SQL Compiler** | `sqlc` | Type-safe Go models generated directly from pure SQL; zero reflection |
| **Schema Migrations** | `goose` | Versioned schema migration files tracked in Git (`db/migrations`) |
| **Observability** | Prometheus (`client_golang`) | Real-time RPS throughput, request duration histogram, and buffer drops |
| **Structured Logging** | `log/slog` | High-efficiency JSON log emission with context-aware request attributes |
| **Containerization** | Docker Multi-stage (`scratch`) | Final image footprint ~15 MB running under unprivileged user (`65534`) |
| **Cloud Hosting** | Railway & Neon.tech | Serverless deployment with managed autoscaling database |

---

## API Specification

Complete OpenAPI 3.0 specification is available at [`api/openapi.yaml`](api/openapi.yaml) or via interactive UI at [`/docs`](https://shortlink-production-1c53.up.railway.app/docs).

| Method | Endpoint | Auth Required | Description |
| --- | --- | :---: | --- |
| `POST` | `/v1/auth/register` | No | Register a new user account |
| `POST` | `/v1/auth/login` | No | Authenticate user and issue JWT access token |
| `POST` | `/v1/links` | **Yes** | Create shortlink (supports custom code and `Idempotency-Key`) |
| `GET` | `/v1/links` | **Yes** | List user's links using keyset cursor pagination |
| `GET` | `/v1/links/{code}` | **Yes** | Get detailed metadata of a specific shortlink |
| `DELETE` | `/v1/links/{code}` | **Yes** | Delete owned shortlink (`204 No Content`) |
| `GET` | `/v1/links/{code}/stats` | **Yes** | Retrieve total and 30-day click telemetry |
| `GET` | `/{code}` | No | Public redirect (`302 Found` or `410 Gone` if expired) |
| `GET` | `/docs` | No | Embedded interactive Swagger UI |
| `GET` | `/openapi.yaml` | No | Raw OpenAPI 3.0 YAML specification |
| `GET` | `/healthz` | No | Liveness probe (process up) |
| `GET` | `/readyz` | No | Readiness probe (database connectivity ping) |
| `GET` | `/metrics` | No | Prometheus telemetry metrics |

---

## Quick Start (Local Development)

### Prerequisites
- Go 1.26+
- Docker & Docker Compose
- `make` (optional)

### 1. Run Everything via Docker Compose (Recommended)
Spins up the shortlink API and PostgreSQL database locally:

```bash
docker compose up -d
```

Verify service readiness:
```bash
curl -i http://localhost:8080/readyz
```

Access Swagger UI locally:
Open [http://localhost:8080/docs](http://localhost:8080/docs) in your browser.

---

### 2. Run Manually (Local Go Toolchain)

```bash
# 1. Clone repository
git clone https://github.com/kyyril/shortlink.git
cd shortlink

# 2. Copy environment configuration
cp .env.example .env

# 3. Start local PostgreSQL container
docker compose up -d postgres

# 4. Apply database migrations
$env:DATABASE_URL="postgres://shortlink:shortlink@localhost:5433/shortlink?sslmode=disable"
goose -dir db/migrations postgres $env:DATABASE_URL up

# 5. Run API server
go run ./cmd/api
```

---

## Quality Assurance & Testing

The test suite covers unit logic, concurrency safety, edge-case validations, and end-to-end integration with a real PostgreSQL instance:

```bash
# Run code formatting
make fmt

# Run static analysis (golangci-lint v2)
make lint

# Run unit tests with race condition detector
make test

# Run integration tests against real PostgreSQL instance
make test-integration

# Full CI verification suite
make check
```

---

## Project Structure

```text
cmd/
  └── api/                  # Application entrypoint (main.go - wiring only)
internal/
  ├── app/                  # Router setup, middleware pipeline, /docs & server lifecycle
  ├── auth/                 # User domain, bcrypt password hashing & JWT token verification
  ├── click/                # Click recorder, asynchronous batch worker & stats service
  ├── config/               # Environment-based configuration loader & validator
  ├── httpx/                # RFC 9457 error mapping, rate limiting, JSON codecs, cursor logic
  ├── link/                 # Link domain, Base62 generator, idempotency engine & CRUD service
  └── store/                # PostgreSQL repository implementations (sqlc generated queries)
db/
  ├── migrations/           # Versioned Goose SQL schema migrations
  └── queries/              # Pure SQL queries compiled by sqlc
api/
  └── openapi.yaml          # OpenAPI 3.0 API contract
```

---

## License

This project is licensed under the [MIT License](LICENSE).
