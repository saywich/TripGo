package create

import (
	"context"
	"log/slog"

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

func (u *Usecase) Execute(ctx context.Context, data model.Data) (model.Trip, error) {
	if err := data.Validate(); err != nil {
		return model.Trip{}, err
	}

	var result model.Trip
	err := u.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		result, err = u.repo.Create(ctx, data)
		if err != nil {
			return err
		}

		if err := u.history.CreateInitial(ctx, result.ID, model.Active, "Trip created"); err != nil {
			return err
		}
		commithooks.AfterCommit(ctx, func() { u.counter.Add(ctx, 1); u.logger.InfoContext(ctx, "trip create", "trip_id", result.ID) })
		return nil
	})
	if err != nil {
		return model.Trip{}, err
	}

	return result, nil
}
