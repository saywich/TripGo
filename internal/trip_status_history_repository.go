package tripservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	sq "github.com/Masterminds/squirrel"
	openapi_types "github.com/oapi-codegen/runtime/types"
	api "github.com/saywich/tripgo/internal/generated"
)

type TripStatusHistoryRepository struct {
	pool *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTripStatusHistoryRepository(
	pool *pgxpool.Pool,
	queryTimeout time.Duration,
) *TripStatusHistoryRepository {
	return &TripStatusHistoryRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *TripStatusHistoryRepository) CreateInitial(
	ctx context.Context,
	tripId openapi_types.UUID,
	toStatus api.TripStatus,
	reason string,
) error {
	return r.create(ctx, tripId, nil, toStatus, reason)
}

func (r *TripStatusHistoryRepository) Create(
	ctx context.Context,
	tripId openapi_types.UUID,
	fromStatus api.TripStatus,
	toStatus api.TripStatus,
	reason string,
) error {
	return r.create(ctx, tripId, &fromStatus, toStatus, reason)
}

func (r *TripStatusHistoryRepository) create(
	ctx context.Context,
	tripId openapi_types.UUID,
	fromStatus *api.TripStatus,
	toStatus api.TripStatus,
	reason string,
) error {
	builder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Insert("trip_status_history").
		Columns(
			"trip_id",
			"from_status",
			"to_status",
			"reason",
		)

	if fromStatus == nil {
		builder = builder.Values(
			tripId,
			nil,
			toStatus,
			reason,
		)
	} else {
		builder = builder.Values(
			tripId,
			*fromStatus,
			toStatus,
			reason,
		)
	}

	query, args, err := builder.ToSql()

	if err != nil {
		return fmt.Errorf("trip status history create: %w", err)
	}

	db := getExecutor(ctx, r.pool)

	deadlineCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	_, err = db.Exec(deadlineCtx, query, args...)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("trip status history create: cancelled by timeout")
		} else {
			return fmt.Errorf("trip status history create: %w", err)
		}
	}

	return nil
}
