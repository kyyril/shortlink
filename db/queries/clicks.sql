-- name: InsertClicks :copyfrom
INSERT INTO clicks (link_id, clicked_at)
VALUES ($1, $2);

-- name: CountTotalClicks :one
SELECT count(*)::bigint AS total
FROM clicks
WHERE link_id = $1;

-- name: ClicksPerDay :many
SELECT (clicked_at AT TIME ZONE 'UTC')::date AS day, count(*)::bigint AS clicks
FROM clicks
WHERE link_id = @link_id
  AND clicked_at >= @since
GROUP BY day
ORDER BY day;
