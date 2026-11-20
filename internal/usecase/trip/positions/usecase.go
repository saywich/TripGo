package positions

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/saywich/tripgo/internal/domain/trip"
	"github.com/saywich/tripgo/internal/model"
)

type Usecase struct {
	repo    repository
	tx      txManager
	pending pendingRequests
	saved   metric.Int64Counter
	now     func() time.Time
}

func New(repo repository, tx txManager, pending pendingRequests, saved metric.Int64Counter, now func() time.Time) *Usecase {
	return &Usecase{repo: repo, tx: tx, pending: pending, saved: saved, now: now}
}

func (u *Usecase) Save(ctx context.Context, input trip.Position) (trip.Position, error) {
	if err := input.Validate(u.now()); err != nil {
		return trip.Position{}, err
	}

	var saved trip.Position

	err := u.tx.Do(ctx, func(ctx context.Context) error {
		current, err := u.repo.SearchByID(ctx, input.TripID)
		if err != nil {
			return err
		}

		if current.Status != trip.Active {
			return trip.ErrTripCompleted
		}

		saved, err = u.repo.InsertPosition(ctx, input)
		if err != nil {
			return err
		}

		if err := u.repo.UpdateLastPosition(ctx, input.TripID, saved.RecordedAt); err != nil {
			return err
		}

		model.AfterCommit(ctx, func() {
			u.pending.Cancel(input.TripID)
			u.saved.Add(ctx, 1)
		})

		return nil
	})
	if err != nil {
		return trip.Position{}, err
	}

	return saved, nil
}

func (u *Usecase) List(ctx context.Context, id trip.ID) ([]trip.Position, error) {
	return u.repo.ListPositions(ctx, id)
}
