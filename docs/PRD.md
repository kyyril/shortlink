# Product Requirements Document: Shortlink

## 1. Problem

Long URLs are hard to share in chats, posts, and print. People want a short, stable link that redirects to the original URL, and they want to know if anyone used it.

This project is also a learning vehicle. The product is simple so that the engineering practices stay in focus. See section 9.

## 2. Target users

| User | Need |
| --- | --- |
| Link owner | Create short links, manage them, see click counts |
| Link visitor | Open a short link and land on the target page quickly |
| Developer (learner) | Read and reuse a clean Go REST API as a reference |

## 3. Use cases

1. An owner registers, logs in, and creates a short link for a long URL.
2. An owner picks a custom code (for example `go-docs`) instead of a random one.
3. An owner sets an expiry date for a campaign link.
4. A visitor opens the short link and is redirected.
5. An owner checks how many clicks a link got in the last 30 days.
6. An owner deletes a link they no longer want.

## 4. Scope

### Functional requirements

| ID | Requirement |
| --- | --- |
| FR-1 | Register with email and password. Email is unique, case-insensitive. Password is at least 8 characters. |
| FR-2 | Log in with email and password. Return a short-lived JWT access token. |
| FR-3 | Create a link with `url`, optional `custom_code`, optional `expires_at`. Only `http` and `https` URLs, max 2048 characters. Random codes are 7 characters, base62. Custom codes are 4 to 32 characters from `[A-Za-z0-9_-]`. |
| FR-4 | List own links, newest first, with cursor pagination (`limit`, `cursor`). Default limit 20, max 100. |
| FR-5 | Get one own link by code. |
| FR-6 | Delete one own link by code. |
| FR-7 | `GET /{code}` redirects with `302` to the target URL. Unknown code returns `404`. Expired link returns `410`. |
| FR-8 | Every successful redirect records a click event without slowing down the redirect. |
| FR-9 | Owner can read total clicks and clicks per day for the last 30 days. |
| FR-10 | `/healthz` returns 200 if the process is up. `/readyz` returns 200 only if the database is reachable. |
| FR-11 | All errors use `application/problem+json` (RFC 9457) with a stable `code` field. |
| FR-12 | Rate limit requests per client IP. Over the limit returns `429` with `Retry-After`. |

### Non-functional requirements

| ID | Requirement |
| --- | --- |
| NFR-1 | All config from environment variables. Startup fails fast on invalid config. |
| NFR-2 | Graceful shutdown: in-flight requests finish and buffered clicks are flushed within `SHUTDOWN_TIMEOUT`. |
| NFR-3 | Every request has a timeout and a request ID. The request ID appears in logs and in error responses. |
| NFR-4 | No secrets, passwords, or tokens in logs. |
| NFR-5 | Request bodies are limited in size (1 MB) and reject unknown JSON fields. |
| NFR-6 | Redirect p95 latency under 50 ms on a developer laptop with local PostgreSQL (informational target, measured manually). |

## 5. Non-goals

- Web UI
- OAuth or social login
- Password reset, email verification, refresh tokens
- Custom domains, QR codes, link editing
- Teams or multi-tenant features
- Abuse or malware detection on target URLs
- Distributed rate limiting or multi-region deployment
- Exact click counts under crash (see ARCHITECTURE, click recording trade-off)

## 6. Acceptance criteria

Each criterion must have at least one automated test. `docs/TESTING.md` maps them.

| ID | Given / When / Then |
| --- | --- |
| AC-1 | Given no account, when I register with a valid email and password, then I get `201` and no password hash in the response. |
| AC-2 | Given an existing email in different letter case, when I register, then I get `409` with code `email_taken`. |
| AC-3 | Given wrong password or unknown email, when I log in, then I get `401` with the same message for both cases. |
| AC-4 | Given a valid token, when I create a link with a valid URL, then I get `201` with a 7-character `code` and a `short_url`. |
| AC-5 | Given a URL with scheme `javascript:` or `ftp:`, when I create a link, then I get `422` with code `invalid_url`. |
| AC-6 | Given a custom code already in use, when I create a link, then I get `409` with code `code_taken`. |
| AC-7 | Given no token or an invalid token, when I call a protected endpoint, then I get `401`. |
| AC-8 | Given a link owned by another user, when I get or delete it, then I get `404`. |
| AC-9 | Given 45 links and `limit=20`, when I page through with the returned cursors, then I see all 45 links once each, newest first. |
| AC-10 | Given a tampered cursor, when I list links, then I get `400` with code `invalid_cursor`. |
| AC-11 | Given a valid code, when I open `/{code}`, then I get `302` with the correct `Location`. |
| AC-12 | Given an expired link, when I open `/{code}`, then I get `410`. |
| AC-13 | Given 3 redirects, when I read stats, then total clicks is 3 (after the recorder flushes). |
| AC-14 | Given a full click buffer, when a redirect happens, then the redirect still succeeds and a drop counter increases. |
| AC-15 | Given the process receives SIGTERM, when clicks are buffered, then they are written before exit (within the timeout). |
| AC-16 | Given the database is down, when I call `/readyz`, then I get `503`, and `/healthz` still returns `200`. |
| AC-17 | Given more requests than the limit from one IP, when I send them, then extra requests get `429` with `Retry-After`. |
| AC-18 | Given an unhandled panic in a handler, when it happens, then the client gets `500` problem JSON and the server keeps running. |

## 7. Success indicators

| Indicator | Target |
| --- | --- |
| Acceptance criteria with automated tests | 18 of 18 |
| CI (`make check`) | Green, including `-race` and lint |
| Service package coverage | At least 80% (a signal, not the goal) |
| Time for a new developer to run the project from the README | Under 10 minutes |
| Every technical decision in ARCHITECTURE has alternatives and a trade-off written down | Yes |

## 8. Open questions

- Soft delete versus hard delete. Current decision: hard delete.
- Should deleted codes be reusable? Current decision: yes, immediately.

## 9. Learning goals

After finishing, the developer can explain and defend:

- Why `net/http` plus a small middleware chain instead of a framework
- Why sqlc instead of an ORM
- How context, timeouts, and cancellation flow from request to query
- How domain errors become HTTP responses
- Why integration tests use a real database
- How graceful shutdown orders its steps
