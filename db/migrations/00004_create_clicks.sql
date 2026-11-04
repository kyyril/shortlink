-- +goose Up
CREATE TABLE clicks (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    link_id    UUID NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    clicked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_clicks_link_time ON clicks (link_id, clicked_at);

-- +goose Down
DROP TABLE IF EXISTS clicks;
