package tripservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	api "github.com/saywich/tripgo/internal/generated"
)

type HTTPHandler struct {
	tripService *TripService
	database    readinessChecker
}

type readinessChecker interface {
	Ping(context.Context) error
}

func NewHTTPHandler(
	tripService *TripService,
	database readinessChecker,
) *HTTPHandler {
	return &HTTPHandler{tripService: tripService, database: database}
}

func (h *HTTPHandler) Routes() http.Handler {
	router := chi.NewRouter()

	router.Get("/health", h.health)
	router.Get("/ready", h.ready)
	router.Post("/api/v1/trips", h.createTrip)
	router.Get("/api/v1/trips/{tripId}", h.getTrip)
	router.Post("/api/v1/trips/{tripId}/finish", h.finishTrip)

	return router
}

func (h *HTTPHandler) getTrip(w http.ResponseWriter, r *http.Request) {
	tripId, err := uuid.Parse(chi.URLParam(r, "tripId"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid_request", "tripId must be a valid UUID")
		return
	}

	trip, err := h.tripService.SearchById(r.Context(), tripId)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "Trip not found", "trip_not_found", "Trip was not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "internal_error", "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(trip); err != nil {
		return
	}
}

func (h *HTTPHandler) finishTrip(w http.ResponseWriter, r *http.Request) {
	tripId, err := uuid.Parse(chi.URLParam(r, "tripId"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid_request", "tripId must be a valid UUID")
		return
	}

	trip, err := h.tripService.Finish(r.Context(), tripId)
	if errors.Is(err, ErrTripNotFound) {
		writeProblem(w, http.StatusNotFound, "Trip not found", "trip_not_found", "Trip was not found")
		return
	}
	if errors.Is(err, ErrTripAlreadyCompleted) {
		writeProblem(w, http.StatusConflict, "Trip completed", "trip_completed", "Operation is not allowed for a completed trip")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "internal_error", "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(trip)
}

func (h *HTTPHandler) createTrip(w http.ResponseWriter, r *http.Request) {
	var data api.TripData

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid_request", "Request body must be a valid TripData JSON object")
		return
	}

	if data.UserId == (api.TripId{}) || data.DriverId == (api.TripId{}) || data.Price < 0 ||
		data.StartPoint.Latitude < -90 || data.StartPoint.Latitude > 90 ||
		data.StartPoint.Longitude < -180 || data.StartPoint.Longitude > 180 ||
		data.EndPoint.Latitude < -90 || data.EndPoint.Latitude > 90 ||
		data.EndPoint.Longitude < -180 || data.EndPoint.Longitude > 180 {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid_request", "Request contains missing or invalid trip fields")
		return
	}

	trip, err := h.tripService.Create(r.Context(), data)
	if err != nil {
		if errors.Is(err, ErrDriverBusy) {
			writeProblem(w, http.StatusConflict, "Driver busy", "driver_busy", "Driver already has an active trip")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "internal_error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Location", "/api/v1/trips/"+trip.Id.String())
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(trip)
}

func (h *HTTPHandler) ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	status := http.StatusOK
	healthStatus := api.Ok

	if h.database == nil || h.database.Ping(r.Context()) != nil {
		status = http.StatusServiceUnavailable
		healthStatus = api.Unavailable
	}

	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(api.HealthResponse{
		Status: healthStatus,
	})
}

func (h *HTTPHandler) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(api.HealthResponse{Status: api.Ok})
}

func writeProblem(w http.ResponseWriter, status int, title, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(api.Problem{
		Type:   "about:blank",
		Title:  title,
		Status: int32(status),
		Code:   code,
		Detail: &detail,
	})
}
