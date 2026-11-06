package tripservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IdempotencyRecord struct {
	RequestHash  []byte
	RequestBody  []byte
	Status       int
	ContentType  string
	Location     string
	ResponseBody []byte
}

type IdempotencyRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
	retention    time.Duration
}

func NewIdempotencyRepository(
	pool *pgxpool.Pool,
	queryTimeout time.Duration,
	retention time.Duration,
) *IdempotencyRepository {
	return &IdempotencyRepository{pool: pool, queryTimeout: queryTimeout, retention: retention}
}

func (r *IdempotencyRepository) Claim(
	ctx context.Context,
	key uuid.UUID,
	requestHash, requestBody []byte,
) (bool, IdempotencyRecord, error) {
	db := getExecutor(ctx, r.pool)

	cleanupCtx, cleanupCancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cleanupCancel()
	cleanupQuery, cleanupArgs, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Delete("idempotency_requests").
		Where(sq.Eq{"key": key}).
		Where(sq.Lt{"created_at": time.Now().UTC().Add(-r.retention)}).
		ToSql()
	if err != nil {
		return false, IdempotencyRecord{}, fmt.Errorf("build expired idempotency key cleanup query: %w", err)
	}
	_, err = db.Exec(cleanupCtx, cleanupQuery, cleanupArgs...)
	if err != nil {
		return false, IdempotencyRecord{}, fmt.Errorf("remove expired idempotency key: %w", err)
	}

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

	insertDeadlineCtx, insertCancel := context.WithTimeout(ctx, r.queryTimeout)
	defer insertCancel()

	err = db.QueryRow(insertDeadlineCtx, query, args...).Scan(&insertedKey)
	if err == nil {
		return true, IdempotencyRecord{}, nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false, IdempotencyRecord{}, fmt.Errorf("claim idempotency key: cancelled by timeout")
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

	lookupDeadlineCtx, lookupCancel := context.WithTimeout(ctx, r.queryTimeout)
	defer lookupCancel()

	err = db.QueryRow(lookupDeadlineCtx, query, args...).Scan(
		&record.RequestHash,
		&record.RequestBody,
		&record.Status,
		&record.ContentType,
		&record.Location,
		&record.ResponseBody,
	)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, IdempotencyRecord{}, fmt.Errorf("load idempotency key: cancelled by timeout")
		} else {
			return false, IdempotencyRecord{}, fmt.Errorf("load idempotency key: %w", err)
		}
	}

	return false, record, nil
}

func (r *IdempotencyRepository) CleanupExpired(ctx context.Context) error {
	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Delete("idempotency_requests").
		Where(sq.Lt{"created_at": time.Now().UTC().Add(-r.retention)}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build idempotency cleanup query: %w", err)
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	_, err = r.pool.Exec(deadlineCtx, query, args...)
	if err != nil {
		return fmt.Errorf("cleanup expired idempotency keys: %w", err)
	}
	return nil
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

	deadlineCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	tag, err := getExecutor(ctx, r.pool).Exec(deadlineCtx, query, args...)
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("complete idempotency key: cancelled by timeout")
	}
	if tag.RowsAffected() != 1 {
		return errors.New("complete idempotency key: key not found")
	}

	return nil
}
