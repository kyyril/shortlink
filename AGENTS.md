# AGENTS.md

Rules for AI coding agents working in this repository. Follow them every time.

## 1. Read first

Before changing anything, read in this order:

1. `docs/IMPLEMENTATION_PLAN.md`: find the next unfinished task. Work on one task at a time.
2. `docs/ARCHITECTURE.md`: layers, packages, decisions.
3. `docs/PRD.md`: requirements and acceptance criteria for the task.
4. `docs/TESTING.md`: how to test the change.
5. `docs/SYSTEM_DESIGN.md` section 7: low-level design (types, SQL, concurrency, error mapping). The rest of that file is background. Do not build Redis, Kafka, sharding, or other scale features unless the plan lists them.

If these documents conflict with the code, stop and report the conflict. Do not guess.

## 2. Repository map

```text
cmd/api/            main.go only. No logic.
internal/app/       wiring, routes, server run and shutdown
internal/config/    env config
internal/auth/      users, login, tokens, auth middleware
internal/link/      link model, service, handler
internal/click/     click recorder, stats
internal/store/     PostgreSQL repositories (internal/store/sqlc is generated)
internal/httpx/     middleware, problem+json, JSON helpers
db/migrations/      goose SQL files
db/queries/         sqlc SQL files
api/openapi.yaml    API contract
docs/               project documents
```

Rules for new code:

- Put code in the domain package it belongs to. Do not create `utils`, `common`, `helpers`, `models`, or `controllers` packages.
- Everything is under `internal/` unless there is a written reason to export it.
- Do not edit files in `internal/store/sqlc`. Change `db/queries/*.sql` and run `make sqlc`.

## 3. Commands

Use these exact commands. Do not invent alternatives.

| Command | Purpose |
| --- | --- |
| `make run` | Start the API |
| `make fmt` | Format code (`gofmt`, `goimports`) |
| `make lint` | Run `golangci-lint` |
| `make test` | Unit and handler tests with `-race` |
| `make test-integration` | Integration tests (needs Docker) |
| `make check` | Lint, tests, and integration tests. Same as CI |
| `make sqlc` | Regenerate sqlc code |
| `make migrate-up` / `make migrate-down` | Apply or roll back migrations |
| `make db-up` / `make db-down` | Start or stop local PostgreSQL |

## 4. Coding rules

Go style:

- Code must pass `gofmt` and `golangci-lint` without warnings.
- Errors: wrap with `fmt.Errorf("verb noun: %w", err)`. Check with `errors.Is` and `errors.As`. Never ignore an error. Never both log and return the same error.
- Return errors, do not panic. Panics are only allowed in `main` for startup bugs.
- `context.Context` is the first parameter of any function that does I/O or can block. Never store a context in a struct.
- Accept interfaces, return structs. Define interfaces in the package that uses them, with only the methods needed.
- Keep functions short and focused. Prefer early returns over deep nesting.
- No global mutable state. No `init()` with side effects. Dependencies come through constructors.
- Time comes from an injected `Clock` or `func() time.Time`, never from `time.Now()` inside business logic.
- Every goroutine has an owner and a way to stop. Say who stops it in a comment or in the type docs.
- Use `log/slog` with key-value pairs. Pick the right level: `debug` for dev detail, `info` for normal events, `warn` for recoverable problems, `error` for failures that need attention. Never log passwords, tokens, or full request bodies.
- Names: short and clear. Package names are lowercase single words. Do not repeat the package name in identifiers (`link.Service`, not `link.LinkService`).
- Comments explain why, not what. Exported identifiers get a doc comment that starts with the name.
- New dependencies need a reason in the PR description. Prefer the standard library.

Layering:

- Handlers decode, call a service, and encode. No business rules.
- Services hold business rules and never import `net/http` or `pgx`.
- Only `internal/httpx` maps errors to HTTP status codes.
- SQL lives in `db/queries`. No SQL strings inside Go code.

## 5. Testing policy

- Every behavior change comes with a test in the same change.
- Bug fix: write a failing test first, then fix.
- Use table-driven tests with `t.Run`. Use `t.Parallel()` where safe, `t.Helper()` in helpers, `t.Cleanup()` for teardown.
- Service tests use small hand-written fakes. Repository tests use a real PostgreSQL container. Do not mock the database.
- Test names describe behavior, for example `TestCreate_CustomCodeTaken_ReturnsErrCodeTaken`.
- No `time.Sleep` to wait for goroutines. Use channels, `sync.WaitGroup`, or polling with a deadline.
- Do not weaken or delete a test to make it pass. If a test is wrong, explain why in the PR.
- Details: `docs/TESTING.md`.

## 6. Verification before saying "done"

Run these and read the output:

1. `make fmt`
2. `make lint`
3. `make test`
4. `make test-integration` if you touched `internal/store`, `db/`, or shutdown logic

Then:

- Check the task's "Done when" line in `docs/IMPLEMENTATION_PLAN.md`.
- Mark the task `[x]` only if every condition is met. Add one evidence line (command and result, or test names).
- If you could not run something, say so. Do not claim it passed.

## 7. Prohibited

General:

- Do not mark a task done without evidence.
- Do not change code outside the current task. Report extra problems separately.
- Do not commit secrets, `.env`, or real credentials.
- Do not edit generated files by hand.
- Do not disable lint rules or add `//nolint` without a written reason on the same line.
- Do not run destructive commands (`rm -rf`, `DROP`, `git reset --hard`, force push) unless the user asked for it.
- Do not add features, endpoints, or config that are not in the PRD.
- Do not invent APIs, flags, or library functions. If unsure, read the source or the docs.

Go specific:

- No `interface{}` or `any` when a concrete type works.
- No `panic` for normal errors. No `os.Exit` outside `main`.
- No `http.DefaultClient` or `http.ListenAndServe` without timeouts.
- No string-built SQL.
- No `time.Sleep` in tests or in production logic.

## 8. Writing style (code comments, docs, commit messages, PR text, replies)

Be direct and short. Say the point first.

- Short, plain sentences. Simple English. One idea per sentence.
- No emoji or emoticons.
- No em-dashes. Use a period, comma, colon, or parentheses.
- No filler openings such as "Certainly", "Great question", "Sure thing".
- No hype or vague words: "robust", "seamless", "powerful", "leverage", "comprehensive", "cutting-edge".
- No long indirect sentences or stacked clauses. Split them.
- No summary paragraph that repeats what was just said.
- No decorative formatting. Use lists and tables only when they make the content easier to scan.
- Commit messages: imperative mood, subject under 72 characters, for example `Add keyset pagination to link list`.

## 9. Per-directory rules

If a subdirectory needs different rules, add an `AGENTS.md` there. The closest file wins. Keep these files short and do not repeat this one.
