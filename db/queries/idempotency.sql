-- name: GetIdempotencyKey :one
SELECT key, user_id, request_hash, status_code, response_body, created_at
FROM idempotency_keys
WHERE key = $1 AND user_id = $2;

-- name: InsertIdempotencyKey :exec
INSERT INTO idempotency_keys (key, user_id, request_hash, status_code, response_body, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

