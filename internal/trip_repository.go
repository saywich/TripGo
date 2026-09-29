package tripservice

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sq "github.com/Masterminds/squirrel"
	api "github.com/saywich/tripgo/internal/generated"
)

type TripRepository struct {
	pool *pgxpool.Pool
}

func NewTripRepository(pool *pgxpool.Pool) *TripRepository {
	return &TripRepository{pool: pool}
}

func getExecutor(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}

	return pool
}

func (r *TripRepository) Create(
	ctx context.Context,
	data api.TripData,
) (api.Trip, error) {
	var trip api.Trip

	tripId, err := uuid.NewRandom()
	if err != nil {
		return api.Trip{}, fmt.Errorf("Query execution error: %w", err)
	}

	query, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Insert("trips").
		Columns(
			"id",
			"user_id",
			"driver_id",
			"start_latitude",
			"start_longitude",
			"end_latitude",
			"end_longitude",
			"price",
			"status",
			"started_at",
		).
		Values(
			tripId,
			data.UserId,
			data.DriverId,
			data.StartPoint.Latitude,
			data.StartPoint.Longitude,
			data.EndPoint.Latitude,
			data.EndPoint.Longitude,
			data.Price,
			"active",
			time.Now(),
		).
		Suffix(`RETURNING
			id, user_id, driver_id,
			start_latitude, start_longitude,
			end_latitude, end_longitude,
			price, status, started_at, finished_at`).
		ToSql()

	if err != nil {
		return api.Trip{}, fmt.Errorf("Query execution error: %w", err)
	}

	db := getExecutor(ctx, r.pool)

	err = db.QueryRow(ctx, query, args...).Scan(
		&trip.Id,
		&trip.UserId,
		&trip.DriverId,
		&trip.StartPoint.Latitude,
		&trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude,
		&trip.EndPoint.Longitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
	)

	if err != nil {
		return api.Trip{}, fmt.Errorf("Query execution error: %w", err)
	}

	return trip, nil
}

func (r *TripRepository) SearchById(
	ctx context.Context,
	tripId api.TripId,
) (api.Trip, error) {
	var trip api.Trip

	query, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Select(
			"id",
			"user_id",
			"driver_id",
			"start_latitude",
			"start_longitude",
			"end_latitude",
			"end_longitude",
			"price",
			"status",
			"started_at",
			"finished_at",
		).
		From("trips").
		Where(sq.Eq{"id": tripId}).
		ToSql()

	if err != nil {
		return api.Trip{}, fmt.Errorf("build trip search query: %w", err)
	}

	db := getExecutor(ctx, r.pool)
	err = db.QueryRow(ctx, query, args...).Scan(
		&trip.Id,
		&trip.UserId,
		&trip.DriverId,
		&trip.StartPoint.Latitude,
		&trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude,
		&trip.EndPoint.Longitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
	)

	if err != nil {
		return api.Trip{}, fmt.Errorf("search trip by id: %w", err)
	}

	return trip, nil
}

func (r *TripRepository) Finish(
	ctx context.Context,
	tripId api.TripId,
) (api.Trip, error) {
	var trip api.Trip

	query, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update("trips").
		Set("status", api.Completed).
		Set("finished_at", time.Now()).
		Where(sq.Eq{"id": tripId}).
		Where(sq.Eq{"status": api.Active}).
		Suffix(`RETURNING
			id, user_id, driver_id,
			start_latitude, start_longitude,
			end_latitude, end_longitude,
			price, status, started_at, finished_at`).
		ToSql()

	if err != nil {
		return api.Trip{}, fmt.Errorf("build finish trip query: %w", err)
	}

	db := getExecutor(ctx, r.pool)
	err = db.QueryRow(ctx, query, args...).Scan(
		&trip.Id,
		&trip.UserId,
		&trip.DriverId,
		&trip.StartPoint.Latitude,
		&trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude,
		&trip.EndPoint.Longitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
	)

	if err != nil {
		return api.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	return trip, nil
}
