-- +goose Up
CREATE TABLE links (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code       TEXT        UNIQUE NOT NULL,
    target_url TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_links_user_created ON links (user_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS links;

