package create

import (
	"encoding/json"
	"net/http"

	model "github.com/saywich/tripgo/internal/domain/trip"
	"github.com/saywich/tripgo/internal/http/domainerror"
)

type Handler struct{ usecase usecase }

func New(u usecase) *Handler { return &Handler{u} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, err := DecodeTripData(r.Body)
	if err != nil {
		domainerror.Write(w, &model.ValidationError{Field: "body", Message: err.Error()})

		return
	}

	input := model.Data{DriverID: data.DriverId, UserID: data.UserId, StartPoint: model.Coordinates(data.StartPoint), EndPoint: model.Coordinates(data.EndPoint), Price: data.Price}
	result, err := h.usecase.Execute(r.Context(), input)
	if err != nil {
		domainerror.Write(w, err)

		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Location", "/api/v1/trips/"+result.ID.String())
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(result)
}
