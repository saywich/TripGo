package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	api "github.com/saywich/tripgo/internal/domain/trip"
	"github.com/saywich/tripgo/internal/infra/postgres/transaction"
)

type StatusHistoryRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewStatusHistoryRepository(
	pool *pgxpool.Pool,
	queryTimeout time.Duration,
) *StatusHistoryRepository {
	return &StatusHistoryRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *StatusHistoryRepository) CreateInitial(
	ctx context.Context,
	tripID uuid.UUID,
	toStatus api.Status,
	reason string,
) error {
	return r.create(ctx, tripID, nil, toStatus, reason)
}

func (r *StatusHistoryRepository) Create(
	ctx context.Context,
	tripID uuid.UUID,
	fromStatus api.Status,
	toStatus api.Status,
	reason string,
) error {
	return r.create(ctx, tripID, &fromStatus, toStatus, reason)
}

func (r *StatusHistoryRepository) create(
	ctx context.Context,
	tripID uuid.UUID,
	fromStatus *api.Status,
	toStatus api.Status,
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
			tripID,
			nil,
			toStatus,
			reason,
		)
	} else {
		builder = builder.Values(
			tripID,
			*fromStatus,
			toStatus,
			reason,
		)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("trip status history create: %w", err)
	}

	db := transaction.Executor(ctx, r.pool)

	deadlineCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	_, err = db.Exec(deadlineCtx, query, args...)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("trip status history create: cancelled by timeout")
		}

		return fmt.Errorf("trip status history create: %w", err)
	}

	return nil
}
