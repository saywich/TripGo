-- +goose Up
-- +goose StatementBegin
CREATE TABLE idempotency_requests (
    key                    UUID PRIMARY KEY,
    request_hash           BYTEA NOT NULL,
    request_body           BYTEA NOT NULL,
    response_status        INTEGER NOT NULL DEFAULT 0,
    response_content_type  TEXT NOT NULL DEFAULT '',
    response_location      TEXT NOT NULL DEFAULT '',
    response_body          BYTEA NOT NULL DEFAULT ''::bytea,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (response_status = 0 OR response_status BETWEEN 200 AND 599)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS idempotency_requests;
-- +goose StatementEnd
