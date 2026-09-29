package tripservice

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IdempotencyRecord struct {
	RequestHash []byte
	RequestBody []byte
	Status int
	ContentType string
	Location string
	ResponseBody []byte
}

type IdempotencyRepository struct {
	pool *pgxpool.Pool
}

func NewIdempotencyRepository(pool *pgxpool.Pool) *IdempotencyRepository {
	return &IdempotencyRepository{pool: pool}
}

func (r *IdempotencyRepository) Claim(
	ctx context.Context,
	key uuid.UUID,
	requestHash, requestBody []byte,
) (bool, IdempotencyRecord, error) {
	db := getExecutor(ctx, r.pool)
	builder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	query, args, err := builder.Insert("idempotency_requests").
		Columns("key", "request_hash", "request_body").
		Values(key, requestHash, requestBody).
		Suffix("ON CONFLICT (key) DO NOTHING RETURNING key").
		ToSql()
	if err != nil {
		return false, IdempotencyRecord{}, fmt.Errorf("build idempotency claim query: %w", err)
	}

	var insertedKey uuid.UUID

	err = db.QueryRow(ctx, query, args...).Scan(&insertedKey)
	if err == nil {
		return true, IdempotencyRecord{}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, IdempotencyRecord{}, fmt.Errorf("claim idempotency key: %w", err)
	}

	query, args, err = builder.Select(
		"request_hash", "request_body", "response_status",
		"response_content_type", "response_location", "response_body",
	).From("idempotency_requests").Where(sq.Eq{"key": key}).ToSql()

	if err != nil {
		return false, IdempotencyRecord{}, fmt.Errorf("build idempotency lookup query: %w", err)
	}

	var record IdempotencyRecord

	err = db.QueryRow(ctx, query, args...).Scan(
		&record.RequestHash,
		&record.RequestBody,
		&record.Status,
		&record.ContentType,
		&record.Location,
		&record.ResponseBody,
	)
	if err != nil {
		return false, IdempotencyRecord{}, fmt.Errorf("load idempotency key: %w", err)
	}

	return false, record, nil
}

func (r *IdempotencyRepository) Complete(
	ctx context.Context,
	key uuid.UUID,
	status int,
	contentType, location string,
	responseBody []byte,
) error {
	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Update("idempotency_requests").
		Set("response_status", status).
		Set("response_content_type", contentType).
		Set("response_location", location).
		Set("response_body", responseBody).
		Where(sq.Eq{"key": key}).
		ToSql()

	if err != nil {
		return fmt.Errorf("build idempotency completion query: %w", err)
	}

	tag, err := getExecutor(ctx, r.pool).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("complete idempotency key: key not found")
	}

	return nil
}
