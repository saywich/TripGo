package get

import (
	"context"

	"github.com/google/uuid"

	model "github.com/saywich/tripgo/internal/domain/trip"
)

type Usecase struct{ repo tripRepository }

func New(repo tripRepository) *Usecase { return &Usecase{repo} }

func (u *Usecase) Execute(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	return u.repo.SearchByID(ctx, id)
}
