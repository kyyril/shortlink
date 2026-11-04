-- name: InsertLink :exec
INSERT INTO links (id, user_id, code, target_url, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetLinkByCode :one
SELECT id, user_id, code, target_url, expires_at, created_at
FROM links
WHERE code = $1;

-- name: GetOwnedLink :one
SELECT id, user_id, code, target_url, expires_at, created_at
FROM links
WHERE code = $1
  AND user_id = $2;

-- name: ListLinksFirstPage :many
SELECT id, user_id, code, target_url, expires_at, created_at
FROM links
WHERE user_id = @user_id
ORDER BY created_at DESC, id DESC
LIMIT @page_size;

-- name: ListLinksAfter :many
SELECT id, user_id, code, target_url, expires_at, created_at
FROM links
WHERE user_id = @user_id
  AND (created_at, id) < (@after_created_at::timestamptz, @after_id::uuid)
ORDER BY created_at DESC, id DESC
LIMIT @page_size;

-- name: DeleteOwnedLink :exec
DELETE FROM links
WHERE code = $1
  AND user_id = $2;

