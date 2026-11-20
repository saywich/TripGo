package domainerror_test

import (
	"errors"
	"fmt"
	"testing"

	trip "github.com/saywich/tripgo/internal/domain/trip"
	"github.com/saywich/tripgo/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{trip.ErrDriverBusy, 409, "driver_busy"}, {trip.ErrTripCompleted, 409, "trip_completed"}, {trip.ErrTripNotFound, 404, "trip_not_found"}, {&trip.ValidationError{Field: "price", Message: "invalid"}, 400, "invalid_request"}, {errors.New("password=secret"), 500, "internal_error"}} {
		t.Run(tc.code, func(t *testing.T) {
			status, p := domainerror.Problem(fmt.Errorf("wrapped: %w", tc.err))

			if status != tc.status || p.Code != tc.code || int(p.Status) != status {
				t.Fatalf("problem: %+v", p)
			}

			if status == 500 && (p.Detail == nil || *p.Detail != "Internal server error") {
				t.Fatalf("details leaked: %+v", p)
			}
		})
	}
}
