package positions

import (
	"context"
	"time"

	"github.com/saywich/tripgo/internal/domain/trip"
)

//go:generate go run go.uber.org/mock/mockgen -source contract.go -destination mocks_test.go -package positions
type repository interface {
	StaleTrips(context.Context, time.Duration, *trip.StaleTrip, int) ([]trip.StaleTrip, error)
}

type gateway interface {
	RequestPosition(context.Context, trip.StaleTrip) error
}

type ticker interface {
	Ticks() <-chan time.Time
	Stop()
}

type ClockTicker struct {
	*time.Ticker
}

func (t ClockTicker) Ticks() <-chan time.Time {
	return t.C
}
