# Testing

How we prove the code is correct, not only that it compiles.

## 1. Strategy

Four levels. Most tests are at the bottom.

| Level | What it tests | Real dependencies | Speed | Command |
| --- | --- | --- | --- | --- |
| Unit | Service logic, validation, helpers | None. Hand-written fakes. | ms | `make test` |
| Handler | Routing, status codes, JSON, middleware | None. `httptest` with fake services. | ms | `make test` |
| Integration | SQL, migrations, repositories, shutdown | Real PostgreSQL in a container | seconds | `make test-integration` |
| End-to-end | Full app over real HTTP | Real PostgreSQL, real server on a random port | seconds | `make test-integration` |

Integration and end-to-end tests use the build tag `integration`:

```go
//go:build integration
```

## 2. Principles

- Test behavior, not implementation. A refactor that keeps behavior should not break tests.
- Use a real database for anything that involves SQL. Mocked SQL proves nothing about the query.
- Mock only at boundaries you do not own or cannot run locally. Prefer small hand-written fakes over generated mocks.
- Inject time and randomness (`Clock`, `CodeGenerator`). Tests never depend on the wall clock.
- No `time.Sleep` to wait for goroutines. Use channels, `sync.WaitGroup`, or a polling helper with a deadline.
- Tests are independent. Each test creates its own data and can run in any order and in parallel.
- Every bug fix starts with a failing test.

## 3. Conventions

- Table-driven tests with `t.Run` and descriptive case names.
- Test names describe behavior: `TestCreate_CustomCodeTaken_ReturnsErrCodeTaken`.
- `t.Parallel()` where safe. `t.Helper()` in helpers. `t.Cleanup()` for teardown. `t.Setenv()` for env vars.
- Compare with `errors.Is`, not string matching. Compare structs with `cmp.Diff` or field by field.
- Integration tests: one PostgreSQL container per package (`TestMain`), then isolate tests by truncating tables or using unique data per test.
- Test helpers live in `internal/testutil`. Keep them small and boring.
- Always run `go test -race`. Data race findings are bugs, not flaky tests.

## 4. Requirement coverage

Every acceptance criterion in `PRD.md` maps to at least one test.

| AC | Level | Test location (planned) |
| --- | --- | --- |
| AC-1, AC-2, AC-3 | Handler, unit | `internal/auth/handler_test.go`, `service_test.go` |
| AC-4, AC-5, AC-6 | Unit, handler | `internal/link/service_test.go`, `handler_test.go` |
| AC-7 | Handler | `internal/auth/middleware_test.go` |
| AC-8 | Unit, e2e | `internal/link/service_test.go`, `internal/app/e2e_test.go` |
| AC-9, AC-10 | Integration, unit | `internal/store/link_repo_test.go`, `internal/httpx/cursor_test.go` |
| AC-11, AC-12 | Handler, e2e | `internal/link/redirect_test.go` |
| AC-13 | e2e | `internal/app/e2e_test.go` |
| AC-14 | Unit (`-race`) | `internal/click/recorder_test.go` |
| AC-15 | Integration | `internal/app/shutdown_test.go` |
| AC-16 | Integration | `internal/app/health_test.go` |
| AC-17 | Handler | `internal/httpx/ratelimit_test.go` |
| AC-18 | Handler | `internal/httpx/recover_test.go` |

Update the file paths if tests move.

## 5. Key scenarios by area

### Auth

- Register with valid data, short password, invalid email, existing email in different case.
- Login with wrong password and unknown email return identical responses.
- Token: expired, wrong signature, wrong algorithm (`none`, RS256), missing `exp`, malformed header.
- Password hash never appears in any response or log.

### Links

- Create with random code, custom code, taken code, reserved code, expiry in the past.
- Random code conflict: first generated code taken, second succeeds. All attempts taken returns an error.
- URL validation: `javascript:`, `data:`, `ftp:`, missing host, 2049 characters, userinfo in URL, uppercase scheme.
- Ownership: get, delete, and stats on another user's link return 404.
- Delete then redirect returns 404. Deleted code can be created again.

### Pagination

- Empty list, exactly one page, page boundary with equal `created_at` values (tie broken by `id`).
- Limit 0, negative, above max. Tampered, truncated, and empty cursor.
- Links created while paging do not cause duplicates or gaps in already-returned pages.

### Redirect

- Valid code returns 302 with `Location`. Unknown returns 404. Expired returns 410.
- Link expiring at exactly `now`. Decide and test whether that counts as expired.
- Redirect still works when the recorder buffer is full.

### Click recorder

- Flush when batch size is reached. Flush on interval. Drain on `Close`.
- `Close` respects the context deadline when the store is slow.
- `Record` after `Close` does not panic or block.
- Concurrent `Record` calls from many goroutines under `-race`.

### Shutdown and health

- In-flight request finishes during shutdown.
- `/readyz` returns 503 once shutdown starts.
- Buffered clicks are present in the database after exit.
- `/healthz` stays 200 when the database is down. `/readyz` returns 503.

### HTTP layer

- Panic in handler returns 500 problem JSON and the next request works.
- Request ID is generated, echoed in the response header, and present in error bodies and logs.
- Body over 1 MB returns 413. Unknown JSON field returns 400. Wrong content type returns 415.
- Rate limit: burst allowed, then 429 with `Retry-After`, then allowed again after refill.

## 6. Edge cases checklist

Use this when reviewing any new endpoint.

- Empty body, `null`, wrong types, extra fields
- Very long strings, unicode, leading and trailing spaces
- Concurrent requests that hit the same unique key
- Context cancelled mid-request
- Database error that is not a domain error (must become a generic 500)
- Time boundaries: exactly at expiry, timezone offsets in input
- Authorization: no token, other user's resource, deleted user

## 7. Quality gates (CI)

A pull request cannot merge unless all of these pass:

1. `gofmt` and `goimports` produce no diff.
2. `golangci-lint run` has zero findings.
3. `go vet ./...` passes.
4. `go test -race ./...` passes.
5. `go test -race -tags=integration ./...` passes.
6. Coverage of `internal/auth`, `internal/link`, and `internal/click` services is at least 80%. Treat this as a signal. Do not write meaningless tests to reach it.
7. Generated code is up to date: `make sqlc` produces no diff.
8. Migrations apply on an empty database and roll back cleanly.

Flaky tests: fix or delete within one week. Never retry-until-green in CI.

## 8. Not covered (known gaps)

Be honest about what the suite does not prove.

- Load and performance. NFR-6 is checked by hand with a tool such as `hey` or `k6`, not in CI.
- Behavior with more than one app instance (rate limit state is per instance).
- Security testing beyond unit-level checks. No penetration test, no dependency vulnerability gate yet (consider `govulncheck`).
- Crash loss of buffered clicks. This is an accepted trade-off, not tested.
- Real clock and timezone behavior across daylight saving changes on the host.
- Behavior behind real proxies and load balancers.
