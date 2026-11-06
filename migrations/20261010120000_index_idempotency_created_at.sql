-- +goose Up
CREATE INDEX idempotency_requests_created_at_idx ON idempotency_requests (created_at);

-- +goose Down
DROP INDEX IF EXISTS idempotency_requests_created_at_idx;
