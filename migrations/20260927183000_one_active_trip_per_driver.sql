-- +goose Up
-- +goose StatementBegin
CREATE UNIQUE INDEX IF NOT EXISTS trips_one_active_trip_per_driver_idx
    ON trips (driver_id)
    WHERE status = 'active';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS trips_one_active_trip_per_driver_idx;
-- +goose StatementEnd
