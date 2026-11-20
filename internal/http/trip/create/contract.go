package create

import (
	"context"

	model "github.com/saywich/tripgo/internal/domain/trip"
)

//go:generate go run go.uber.org/mock/mockgen -source contract.go -destination mocks_test.go -package create_test
type usecase interface {
	Execute(context.Context, model.Data) (model.Trip, error)
}
