-- +goose Up
-- Enable citext extension for case-insensitive email queries.
CREATE EXTENSION IF NOT EXISTS "citext";

-- +goose Down
DROP EXTENSION IF EXISTS "citext";

