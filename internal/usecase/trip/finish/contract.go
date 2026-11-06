package finish

import (
	"context"

	"github.com/google/uuid"

	model "github.com/saywich/tripgo/internal/domain/trip"
)

//go:generate go run go.uber.org/mock/mockgen -source contract.go -destination mocks_test.go -package finish_test
type tripRepository interface {
	SearchByID(context.Context, uuid.UUID) (model.Trip, error)
	Finish(context.Context, uuid.UUID) (model.Trip, error)
}

type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

type historyRepository interface {
	Create(context.Context, uuid.UUID, model.Status, model.Status, string) error
}
