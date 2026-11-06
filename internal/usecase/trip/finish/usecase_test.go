package finish_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/mock/gomock"

	trip "github.com/saywich/tripgo/internal/domain/trip"
	uc "github.com/saywich/tripgo/internal/usecase/trip/finish"
)

func TestExecute(t *testing.T) {
	for _, scenario := range []string{"success", "repository", "history", "tx", "business"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			history := NewMockhistoryRepository(ctrl)
			counter, _ := noop.NewMeterProvider().Meter("test").Int64Counter("test")
			u := uc.New(repo, tx, history, slog.New(slog.NewJSONHandler(io.Discard, nil)), counter)
			ctx := context.Background()
			failure := errors.New("database failure")
			expected := error(nil)
			result := trip.Trip{ID: uuid.New(), Status: trip.Active}

			if scenario == "tx" {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(failure)
				expected = failure
			} else {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })

				switch scenario {
				case "repository":
					expected = failure
					repo.EXPECT().SearchByID(gomock.Any(), result.ID).Return(trip.Trip{}, failure)
				case "business":
					expected = trip.ErrTripCompleted
					current := result
					current.Status = trip.Completed
					repo.EXPECT().SearchByID(gomock.Any(), result.ID).Return(current, nil)
				default:
					repo.EXPECT().SearchByID(gomock.Any(), result.ID).Return(result, nil)
					finished := result
					finished.Status = trip.Completed
					repo.EXPECT().Finish(gomock.Any(), result.ID).Return(finished, nil)
					e := error(nil)
					if scenario == "history" {
						e = failure
						expected = failure
					}
					history.EXPECT().Create(gomock.Any(), result.ID, trip.Active, trip.Completed, "Trip finished").Return(e)
				}
			}

			actual, err := u.Execute(ctx, result.ID)

			if !errors.Is(err, expected) {
				t.Fatalf("got %v want %v", err, expected)
			}

			if expected == nil && (actual.ID != result.ID || actual.Status != trip.Completed) {
				t.Fatalf("wrong result: %+v", actual)
			}

			if expected != nil && actual.ID != uuid.Nil {
				t.Fatalf("returned uncommitted trip: %+v", actual)
			}
		})
	}
}
