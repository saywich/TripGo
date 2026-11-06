-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS trip_status_history (
    id          BIGSERIAL PRIMARY KEY,
    trip_id     UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    from_status TEXT,
    to_status   TEXT NOT NULL CHECK (to_status IN ('active', 'completed')),
    reason      TEXT,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS trip_status_history_trip_changed_idx
    ON trip_status_history (trip_id, changed_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS trip_status_history;
DROP INDEX IF EXISTS trip_status_history_trip_changed_idx;
-- +goose StatementEnd
