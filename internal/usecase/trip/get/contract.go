package get

import (
	"context"

	"github.com/google/uuid"

	model "github.com/saywich/tripgo/internal/domain/trip"
)

//go:generate go run go.uber.org/mock/mockgen -source contract.go -destination mocks_test.go -package get_test
type tripRepository interface {
	SearchByID(context.Context, uuid.UUID) (model.Trip, error)
}
