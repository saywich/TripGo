package create

import (
	"context"

	"github.com/google/uuid"

	model "github.com/saywich/tripgo/internal/domain/trip"
)

//go:generate go run go.uber.org/mock/mockgen -source contract.go -destination mocks_test.go -package create_test
type tripRepository interface {
	Create(context.Context, model.Data) (model.Trip, error)
}

type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

type historyRepository interface {
	CreateInitial(context.Context, uuid.UUID, model.Status, string) error
}
