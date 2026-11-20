package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgtype"

	model "github.com/saywich/tripgo/internal/domain/trip"
	"github.com/saywich/tripgo/internal/infra/postgres/transaction"
)

func (r *Repository) InsertPosition(ctx context.Context, p model.Position) (model.Position, error) {
	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Insert("trip_positions").
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(p.TripID, p.Coordinates.Latitude, p.Coordinates.Longitude, p.RecordedAt).
		Suffix("RETURNING id, recorded_at").
		ToSql()
	if err != nil {
		return model.Position{}, fmt.Errorf("build position insert: %w", err)
	}

	db := transaction.Executor(ctx, r.pool)
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	err = db.QueryRow(ctx, query, args...).Scan(&p.ID, &p.RecordedAt)
	if err != nil {
		return model.Position{}, fmt.Errorf("insert position: %w", err)
	}

	return p, nil
}

func (r *Repository) UpdateLastPosition(ctx context.Context, id model.ID, at time.Time) error {
	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Update("trips").
		Set("last_position_at", sq.Expr("GREATEST(last_position_at, ?)", at)).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build last position update: %w", err)
	}

	db := transaction.Executor(ctx, r.pool)
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	_, err = db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update last position: %w", err)
	}

	return nil
}

func (r *Repository) ListPositions(ctx context.Context, id model.ID) ([]model.Position, error) {
	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("p.id", "p.latitude", "p.longitude", "p.recorded_at").
		From("trips t").
		LeftJoin("trip_positions p ON p.trip_id = t.id").
		Where(sq.Eq{"t.id": id}).
		OrderBy("p.recorded_at", "p.id").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build positions query: %w", err)
	}

	db := transaction.Executor(ctx, r.pool)
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	defer rows.Close()

	positions := make([]model.Position, 0)
	found := false

	for rows.Next() {
		found = true

		var positionID pgtype.Int8
		var lat, lon pgtype.Float8
		var at pgtype.Timestamptz

		if err := rows.Scan(&positionID, &lat, &lon, &at); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}

		if positionID.Valid {
			positions = append(positions, model.Position{
				ID:          positionID.Int64,
				TripID:      id,
				Coordinates: model.Coordinates{Latitude: lat.Float64, Longitude: lon.Float64},
				RecordedAt:  at.Time,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read positions: %w", err)
	}

	if !found {
		return nil, model.ErrTripNotFound
	}

	return positions, nil
}

func (r *Repository) StaleTrips(ctx context.Context, age time.Duration, after *model.StaleTrip, limit int) ([]model.StaleTrip, error) {
	const lastActivity = "COALESCE(last_position_at, started_at)"

	interval := pgtype.Interval{Microseconds: age.Microseconds(), Valid: true}
	builder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("id", "driver_id", lastActivity).
		From("trips").
		Where("status = 'active'").
		Where(lastActivity + " < now() - ?::interval", interval).
		OrderBy(lastActivity, "id")

	if after != nil {
		builder = builder.Where("("+lastActivity+", id) > (?, ?)", after.Since, after.ID)
	}

	if after != nil || limit > 0 {
		builder = builder.Suffix("LIMIT ?", limit)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build stale trips query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select stale trips: %w", err)
	}
	defer rows.Close()

	var trips []model.StaleTrip

	for rows.Next() {
		var trip model.StaleTrip

		if err := rows.Scan(&trip.ID, &trip.DriverID, &trip.Since); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}

		trips = append(trips, trip)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read stale trips: %w", err)
	}

	return trips, nil
}
