package tripservice

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	sq "github.com/Masterminds/squirrel"
	openapi_types "github.com/oapi-codegen/runtime/types"
	api "github.com/saywich/tripgo/internal/generated"
)

type TripStatusHistoryRepository struct {
	pool *pgxpool.Pool
}

func NewTripStatusHistoryRepository(
	pool *pgxpool.Pool,
) *TripStatusHistoryRepository {
	return &TripStatusHistoryRepository{pool: pool}
}

func (r *TripStatusHistoryRepository) Create(
	ctx context.Context,
	tripId openapi_types.UUID,
	fromStatus api.TripStatus,
	toStatus api.TripStatus,
	reason string,
) error {
	query, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Insert("trip_status_history").
		Columns(
			"trip_id",
			"from_status",
			"to_status",
			"reason",
		).
		Values(
			tripId,
			fromStatus,
			toStatus,
			reason,
		).
		ToSql()

	if err != nil {
		return fmt.Errorf("Query execution error: %w", err)
	}

	db := getExecutor(ctx, r.pool)

	_, err = db.Exec(ctx, query, args...)

	if err != nil {
		return fmt.Errorf("Query execution error: %w", err)
	}

	return nil
}
