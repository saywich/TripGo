package domainerror

import (
	"encoding/json"
	"errors"
	"net/http"

	trip "github.com/saywich/tripgo/internal/domain/trip"
	api "github.com/saywich/tripgo/internal/generated"
)

func Problem(err error) (int, api.Problem) {
	status, title, code, detail := 500, "Internal Server Error", "internal_error", "Internal server error"
	var v *trip.ValidationError

	switch {
	case errors.As(err, &v):
		status, title, code, detail = 400, "Bad Request", "invalid_request", v.Error()
	case errors.Is(err, trip.ErrTripNotFound):
		status, title, code, detail = 404, "Trip not found", "trip_not_found", "Trip was not found"
	case errors.Is(err, trip.ErrTripCompleted):
		status, title, code, detail = 409, "Trip completed", "trip_completed", "Operation is not allowed for a completed trip"
	case errors.Is(err, trip.ErrDriverBusy):
		status, title, code, detail = 409, "Driver busy", "driver_busy", "Driver already has an active trip"
	}

	return status, api.Problem{Type: "about:blank", Title: title, Code: code, Status: int32(status), Detail: &detail} //nolint:gosec // status is one of the HTTP constants above.
}

func Write(w http.ResponseWriter, err error) {
	status, body := Problem(err)
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
