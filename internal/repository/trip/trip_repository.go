package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	api "github.com/saywich/tripgo/internal/domain/trip"
	"github.com/saywich/tripgo/internal/infra/postgres/transaction"
)

type Repository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *Repository {
	return &Repository{pool: pool, queryTimeout: queryTimeout}
}

func (r *Repository) Create(
	ctx context.Context,
	data api.Data,
) (api.Trip, error) {
	var trip api.Trip

	tripID, err := uuid.NewRandom()
	if err != nil {
		return api.Trip{}, fmt.Errorf("create trip: %w", err)
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
			tripID,
			data.UserID,
			data.DriverID,
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
		return api.Trip{}, fmt.Errorf("create trip: %w", err)
	}

	db := transaction.Executor(ctx, r.pool)

	deadlineCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	err = db.QueryRow(deadlineCtx, query, args...).Scan(
		&trip.ID,
		&trip.UserID,
		&trip.DriverID,
		&trip.StartPoint.Latitude,
		&trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude,
		&trip.EndPoint.Longitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return api.Trip{}, api.ErrTripNotFound
	}

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return api.Trip{}, fmt.Errorf("create trip: cancelled by timeout")
		}

		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == "trips_one_active_trip_per_driver_idx" {
			return api.Trip{}, api.ErrDriverBusy
		}

		return api.Trip{}, fmt.Errorf("create trip: %w", err)
	}

	return trip, nil
}

func (r *Repository) SearchByID(
	ctx context.Context,
	tripID api.ID,
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
		Where(sq.Eq{"id": tripID}).
		Suffix("FOR UPDATE").
		ToSql()
	if err != nil {
		return api.Trip{}, fmt.Errorf("search trip by id: %w", err)
	}

	db := transaction.Executor(ctx, r.pool)

	deadlineCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	err = db.QueryRow(deadlineCtx, query, args...).Scan(
		&trip.ID,
		&trip.UserID,
		&trip.DriverID,
		&trip.StartPoint.Latitude,
		&trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude,
		&trip.EndPoint.Longitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return api.Trip{}, api.ErrTripNotFound
	}

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return api.Trip{}, fmt.Errorf("search trip by id: cancelled by timeout")
		}

		return api.Trip{}, fmt.Errorf("search trip by id: %w", err)
	}

	return trip, nil
}

func (r *Repository) Finish(
	ctx context.Context,
	tripID api.ID,
) (api.Trip, error) {
	var trip api.Trip

	query, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update("trips").
		Set("status", api.Completed).
		Set("finished_at", time.Now()).
		Where(sq.Eq{"id": tripID}).
		Where(sq.Eq{"status": api.Active}).
		Suffix(`RETURNING
			id, user_id, driver_id,
			start_latitude, start_longitude,
			end_latitude, end_longitude,
			price, status, started_at, finished_at`).
		ToSql()
	if err != nil {
		return api.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	db := transaction.Executor(ctx, r.pool)

	deadlineCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	err = db.QueryRow(deadlineCtx, query, args...).Scan(
		&trip.ID,
		&trip.UserID,
		&trip.DriverID,
		&trip.StartPoint.Latitude,
		&trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude,
		&trip.EndPoint.Longitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return api.Trip{}, api.ErrTripNotFound
	}

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return api.Trip{}, fmt.Errorf("finish trip: cancelled by timeout")
		}

		return api.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	return trip, nil
}
