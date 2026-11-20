package positions

import (
	"context"
	"time"

	"github.com/saywich/tripgo/internal/domain/trip"
)

type repository interface {
	SearchByID(context.Context, trip.ID) (trip.Trip, error)
	InsertPosition(context.Context, trip.Position) (trip.Position, error)
	UpdateLastPosition(context.Context, trip.ID, time.Time) error
	ListPositions(context.Context, trip.ID) ([]trip.Position, error)
}

type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

type pendingRequests interface {
	Cancel(trip.ID)
}
