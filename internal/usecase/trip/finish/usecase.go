package finish

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"

	model "github.com/saywich/tripgo/internal/domain/trip"
	commithooks "github.com/saywich/tripgo/internal/model"
)

type Usecase struct {
	repo    tripRepository
	tx      txManager
	history historyRepository
	logger  *slog.Logger
	counter metric.Int64Counter
}

func New(repo tripRepository, tx txManager, history historyRepository, logger *slog.Logger, counter metric.Int64Counter) *Usecase {
	return &Usecase{repo, tx, history, logger, counter}
}

func (u *Usecase) Execute(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	var result model.Trip
	err := u.tx.Do(ctx, func(ctx context.Context) error {
		current, err := u.repo.SearchByID(ctx, id)
		if err != nil {
			return err
		}

		if err = current.Finish(time.Now()); err != nil {
			return err
		}

		result, err = u.repo.Finish(ctx, id)
		if err != nil {
			return err
		}

		if err := u.history.Create(ctx, id, model.Active, model.Completed, "Trip finished"); err != nil {
			return err
		}
		commithooks.AfterCommit(ctx, func() { u.counter.Add(ctx, 1); u.logger.InfoContext(ctx, "trip finish", "trip_id", result.ID) })
		return nil
	})
	if err != nil {
		return model.Trip{}, err
	}

	return result, nil
}
