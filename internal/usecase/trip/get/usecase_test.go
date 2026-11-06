package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	trip "github.com/saywich/tripgo/internal/domain/trip"
	uc "github.com/saywich/tripgo/internal/usecase/trip/get"
)

func TestExecute(t *testing.T) {
	for _, want := range []error{nil, trip.ErrTripNotFound, errors.New("database failure")} {
		ctrl := gomock.NewController(t)
		repo := NewMocktripRepository(ctrl)
		id := uuid.New()
		repo.EXPECT().SearchByID(gomock.Any(), id).Return(trip.Trip{ID: id}, want)
		got, err := uc.New(repo).Execute(context.Background(), id)

		if !errors.Is(err, want) || got.ID != id {
			t.Fatalf("got %+v, %v", got, err)
		}
	}
}
