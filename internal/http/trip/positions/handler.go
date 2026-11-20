package positions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/saywich/tripgo/internal/domain/trip"
	api "github.com/saywich/tripgo/internal/generated"
	"github.com/saywich/tripgo/internal/http/domainerror"
)

type usecase interface {
	Save(context.Context, trip.Position) (trip.Position, error)
	List(context.Context, trip.ID) ([]trip.Position, error)
}

type Handler struct {
	usecase usecase
	logger  *slog.Logger
}

func New(u usecase, logger *slog.Logger) *Handler {
	return &Handler{usecase: u, logger: logger}
}

func tripID(r *http.Request) (trip.ID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "tripID"))
	if err != nil {
		return id, &trip.ValidationError{Field: "tripId", Message: "must be a valid UUID"}
	}

	return id, nil
}

func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	id, err := tripID(r)
	if err != nil {
		domainerror.Write(w, err)
		return
	}

	input, err := decodePosition(http.MaxBytesReader(w, r.Body, 1<<20), id)
	if err != nil {
		domainerror.Write(w, err)
		return
	}

	saved, err := h.usecase.Save(r.Context(), input)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toAPI(saved))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	id, err := tripID(r)
	if err != nil {
		domainerror.Write(w, err)
		return
	}

	positions, err := h.usecase.List(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	out := make([]api.TripPosition, 0, len(positions))
	for _, p := range positions {
		out = append(out, toAPI(p))
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(api.TripPositionsResponse{Positions: out})
}

func decodePosition(body io.Reader, id trip.ID) (trip.Position, error) {
	var input struct {
		Coordinates struct {
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
		} `json:"coordinates"`
		RecordedAt time.Time `json:"recorded_at"`
	}

	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&input); err != nil {
		return trip.Position{}, &trip.ValidationError{Field: "body", Message: "invalid position JSON"}
	}

	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return trip.Position{}, &trip.ValidationError{Field: "body", Message: "expected one JSON object"}
	}

	if input.Coordinates.Latitude == nil || input.Coordinates.Longitude == nil {
		return trip.Position{}, &trip.ValidationError{Field: "coordinates", Message: "latitude and longitude are required"}
	}

	return trip.Position{
		TripID:      id,
		Coordinates: trip.Coordinates{Latitude: *input.Coordinates.Latitude, Longitude: *input.Coordinates.Longitude},
		RecordedAt:  input.RecordedAt,
	}, nil
}

func toAPI(p trip.Position) api.TripPosition {
	return api.TripPosition{
		Id:          p.ID,
		TripId:      p.TripID,
		Coordinates: api.Coordinates{Latitude: p.Coordinates.Latitude, Longitude: p.Coordinates.Longitude},
		RecordedAt:  p.RecordedAt,
	}
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, _ := domainerror.Problem(err)
	if status == http.StatusInternalServerError {
		h.logger.ErrorContext(r.Context(), "positions API", "error", err)
	}

	domainerror.Write(w, err)
}
